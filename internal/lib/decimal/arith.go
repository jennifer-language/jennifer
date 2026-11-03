// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// dec is an exact decimal: the value coef * 10^-scale, scale >= 0. It is
// immutable once built (copies of a decimal.Decimal handle share it), so every
// operation allocates its result.
type dec struct {
	coef  *big.Int
	scale int
}

// maxDigits bounds the digits a result may carry (coefficient digits and
// scale alike), so a pow or a parsed exponent cannot allocate without bound.
const maxDigits = 100000

// maxBits is the coefficient size matching maxDigits (log2(10) ~ 3.3220).
const maxBits = maxDigits * 33220 / 10000

var errLimit = errors.New("limit")

var (
	bigOne = big.NewInt(1)
	bigTwo = big.NewInt(2)
	bigTen = big.NewInt(10)
	// powers caches 10^0 .. 10^63, the scales everyday money arithmetic uses.
	powers = func() []*big.Int {
		out := make([]*big.Int, 64)
		p := big.NewInt(1)
		for i := range out {
			out[i] = new(big.Int).Set(p)
			p.Mul(p, bigTen)
		}
		return out
	}()
)

// pow10 returns 10^n; callers must not modify the result.
func pow10(n int) *big.Int {
	if n < len(powers) {
		return powers[n]
	}
	return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil)
}

// checked refuses a result past maxDigits.
func checked(d dec) (dec, error) {
	if d.scale > maxDigits || d.coef.BitLen() > maxBits {
		return dec{}, errLimit
	}
	return d, nil
}

func fromInt(n int64) dec {
	return dec{coef: big.NewInt(n), scale: 0}
}

// parse reads [+-]digits[.digits][e[+-]digits], or [+-].digits[...]. No spaces,
// separators, NaN or infinities.
func parse(s string) (dec, error) {
	bad := fmt.Errorf("%q is not a decimal number", s)
	in := s
	neg := false
	if in != "" && (in[0] == '+' || in[0] == '-') {
		neg = in[0] == '-'
		in = in[1:]
	}
	mant, exp := in, ""
	if i := strings.IndexAny(in, "eE"); i >= 0 {
		mant, exp = in[:i], in[i+1:]
		if exp == "" {
			return dec{}, bad
		}
	}
	intPart, frac := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, frac = mant[:i], mant[i+1:]
	}
	if intPart == "" && frac == "" {
		return dec{}, bad
	}
	for _, part := range []string{intPart, frac} {
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return dec{}, bad
			}
		}
	}
	e := 0
	if exp != "" {
		n, err := strconv.Atoi(exp)
		if err != nil {
			return dec{}, bad
		}
		if n > maxDigits || n < -maxDigits {
			return dec{}, errLimit
		}
		e = n
	}
	digits := intPart + frac
	if len(digits) > maxDigits {
		return dec{}, errLimit
	}
	coef, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return dec{}, bad
	}
	if neg {
		coef.Neg(coef)
	}
	scale := len(frac) - e
	if scale < 0 {
		if -scale > maxDigits {
			return dec{}, errLimit
		}
		coef.Mul(coef, pow10(-scale))
		scale = 0
	}
	return checked(dec{coef: coef, scale: scale})
}

// fromFloat takes a float through its shortest round-trip decimal form, so
// 0.1 becomes 0.1 rather than the binary value's full expansion.
func fromFloat(f float64) (dec, error) {
	return parse(strconv.FormatFloat(f, 'g', -1, 64))
}

// String renders plain notation: the scale's digits after the point, never an
// exponent.
func (d dec) String() string {
	digits := new(big.Int).Abs(d.coef).String()
	if d.scale > 0 {
		if len(digits) <= d.scale {
			digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
		}
		cut := len(digits) - d.scale
		digits = digits[:cut] + "." + digits[cut:]
	}
	if d.coef.Sign() < 0 {
		return "-" + digits
	}
	return digits
}

// align returns both coefficients at the larger scale.
func align(a, b dec) (*big.Int, *big.Int, int) {
	switch {
	case a.scale == b.scale:
		return a.coef, b.coef, a.scale
	case a.scale < b.scale:
		return new(big.Int).Mul(a.coef, pow10(b.scale-a.scale)), b.coef, b.scale
	default:
		return a.coef, new(big.Int).Mul(b.coef, pow10(a.scale-b.scale)), a.scale
	}
}

func add(a, b dec) (dec, error) {
	x, y, s := align(a, b)
	return checked(dec{coef: new(big.Int).Add(x, y), scale: s})
}

func sub(a, b dec) (dec, error) {
	x, y, s := align(a, b)
	return checked(dec{coef: new(big.Int).Sub(x, y), scale: s})
}

func mul(a, b dec) (dec, error) {
	if a.scale+b.scale > maxDigits || a.coef.BitLen()+b.coef.BitLen() > maxBits+1 {
		return dec{}, errLimit
	}
	return checked(dec{coef: new(big.Int).Mul(a.coef, b.coef), scale: a.scale + b.scale})
}

func cmp(a, b dec) int {
	x, y, _ := align(a, b)
	return x.Cmp(y)
}

// mode is a rounding rule, applied to the discarded remainder of a division.
type mode int

const (
	halfEven mode = iota
	halfUp
	halfDown
	up
	down
	ceiling
	floor
)

var modeNames = map[string]mode{
	"half-even": halfEven,
	"half-up":   halfUp,
	"half-down": halfDown,
	"up":        up,
	"down":      down,
	"ceiling":   ceiling,
	"floor":     floor,
}

const modeList = `"half-even", "half-up", "half-down", "up", "down", "ceiling", "floor"`

// roundQuo returns num / den rounded to an integer by the mode. den != 0.
func roundQuo(num, den *big.Int, m mode) *big.Int {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	// sign of the exact quotient; q rounded toward zero, r has num's sign
	neg := (num.Sign() < 0) != (den.Sign() < 0)
	away := false
	switch m {
	case up:
		away = true
	case down:
	case ceiling:
		away = !neg
	case floor:
		away = neg
	default:
		// compare the discarded part with one half: 2|r| against |den|
		twice := new(big.Int).Mul(new(big.Int).Abs(r), bigTwo)
		c := twice.Cmp(new(big.Int).Abs(den))
		switch m {
		case halfUp:
			away = c >= 0
		case halfDown:
			away = c > 0
		case halfEven:
			away = c > 0 || (c == 0 && q.Bit(0) == 1)
		}
	}
	if away {
		if neg {
			q.Sub(q, bigOne)
		} else {
			q.Add(q, bigOne)
		}
	}
	return q
}

// roundTo returns d at exactly `scale` decimal places: padded with zeros when
// that adds places, rounded by the mode when it removes them.
func roundTo(d dec, scale int, m mode) (dec, error) {
	if scale > maxDigits {
		return dec{}, errLimit
	}
	if scale >= d.scale {
		return checked(dec{coef: new(big.Int).Mul(d.coef, pow10(scale-d.scale)), scale: scale})
	}
	return dec{coef: roundQuo(d.coef, pow10(d.scale-scale), m), scale: scale}, nil
}

var errDivZero = errors.New("division by zero")

// errInexact means a scale-less division has no finite decimal result.
var errInexact = errors.New("inexact")

// div returns a / b at `scale` places (rounded by the mode), or, with scale <
// 0, exactly: at the scale difference of the operands (a.scale - b.scale, at
// least 0), with more places only when the quotient needs them, as Java's
// BigDecimal and Python's decimal choose.
func div(a, b dec, scale int, m mode) (dec, error) {
	if b.coef.Sign() == 0 {
		return dec{}, errDivZero
	}
	if scale < 0 {
		return divExact(a, b)
	}
	if scale > maxDigits {
		return dec{}, errLimit
	}
	// a / b = (ca / cb) * 10^(sb - sa); at `scale` places the coefficient is
	// ca * 10^(scale + sb - sa) / cb.
	e := scale + b.scale - a.scale
	num, den := new(big.Int).Set(a.coef), new(big.Int).Set(b.coef)
	if e >= 0 {
		num.Mul(num, pow10(e))
	} else {
		den.Mul(den, pow10(-e))
	}
	return checked(dec{coef: roundQuo(num, den, m), scale: scale})
}

// divExact divides when the quotient has a finite decimal expansion: after
// reducing ca / cb, the denominator may hold only the primes 2 and 5.
func divExact(a, b dec) (dec, error) {
	num, den := new(big.Int).Set(a.coef), new(big.Int).Set(b.coef)
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(num), den)
	if g.Sign() != 0 {
		num.Quo(num, g)
		den.Quo(den, g)
	}
	twos, fives := 0, 0
	rest := new(big.Int).Set(den)
	five := big.NewInt(5)
	m := new(big.Int)
	for rest.Bit(0) == 0 && rest.Sign() != 0 {
		rest.Rsh(rest, 1)
		twos++
	}
	for {
		q, r := new(big.Int).QuoRem(rest, five, m)
		if r.Sign() != 0 {
			break
		}
		rest = q
		fives++
	}
	if rest.Cmp(bigOne) != 0 {
		return dec{}, errInexact
	}
	k := max(twos, fives)
	// num / den = num * (10^k / den) / 10^k
	coef := new(big.Int).Mul(num, new(big.Int).Quo(pow10(k), den))
	scale := k + a.scale - b.scale
	if scale < 0 {
		coef.Mul(coef, pow10(-scale))
		scale = 0
	}
	return checked(dec{coef: coef, scale: scale})
}

// normalize removes trailing zeros after the point.
func normalize(d dec) dec {
	c, s := new(big.Int).Set(d.coef), d.scale
	r := new(big.Int)
	for s > 0 {
		q, rem := new(big.Int).QuoRem(c, bigTen, r)
		if rem.Sign() != 0 {
			break
		}
		c, s = q, s-1
	}
	return dec{coef: c, scale: s}
}

// isInteger reports whether the value has no fractional part.
func isInteger(d dec) bool {
	if d.scale == 0 {
		return true
	}
	return new(big.Int).Rem(d.coef, pow10(d.scale)).Sign() == 0
}

// pow raises d to a non-negative integer power exactly.
func pow(d dec, n int) (dec, error) {
	if d.coef.Sign() == 0 || n == 0 {
		if n == 0 {
			return fromInt(1), nil
		}
		return dec{coef: new(big.Int), scale: 0}, nil
	}
	// the result needs about n times the operand's size
	if int64(d.scale)*int64(n) > maxDigits || int64(d.coef.BitLen()-1)*int64(n) > maxBits {
		return dec{}, errLimit
	}
	return checked(dec{
		coef:  new(big.Int).Exp(d.coef, big.NewInt(int64(n)), nil),
		scale: d.scale * n,
	})
}
