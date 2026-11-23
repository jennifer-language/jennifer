// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"bytes"
	"math"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// M29.52: `%f|prec` rounds the shortest round-trip decimal half to even, as
// intl.formatNumber and decimal.round do, not the binary value.
func TestPrintfPrecRoundsDecimal(t *testing.T) {
	in := interpreter.New()
	Install(in)
	f := interpreter.FloatVal
	cases := []struct {
		format string
		v      float64
		want   string
	}{
		{"%f|prec=2", 1002.675, "1002.68"}, // binary 1002.67499...
		{"%f|prec=2", 2.675, "2.68"},       // binary 2.67499...
		{"%f|prec=2", 1.005, "1.00"},       // a tie: 0 is even
		{"%f|prec=0", 2.5, "2"},
		{"%f|prec=0", 3.5, "4"},
		{"%f|prec=0", 0.5, "0"},
		{"%f|prec=0", 1.5, "2"},
		{"%f|prec=2", 0.125, "0.12"},
		{"%f|prec=2", 0.375, "0.38"},
		{"%f|prec=2", 0.1251, "0.13"},
		{"%f|prec=2", -1002.675, "-1002.68"},
		{"%f|prec=0", -2.5, "-2"},
		{"%f|prec=2", -0.001, "-0.00"},
		{"%f|prec=2", 3.14159, "3.14"},
		{"%f|prec=2", 3.5, "3.50"},
		{"%f|prec=1", 99.95, "100.0"},
		{"%f|prec=2", 999.995, "1000.00"},
		{"%f|prec=6", 0.1, "0.100000"},
		{"%f|prec=20", 0.1, "0.10000000000000000000"},
		{"%f|prec=3", 42, "42.000"},
		{"%f|prec=2", 1e21, "1000000000000000000000.00"},
		{"%f|prec=0", 123456789012345680000, "123456789012345680000"},
		{"%f|prec=2", 5e-324, "0.00"},
		{`%f|prec=2|group=3|sep=","`, 1234567.125, "1,234,567.12"},
		{"%f|prec=2|trim=true", 2.675, "2.68"},
		{"%f|prec=2|fill=0|pad=9", -2.675, "-00002.68"},
		{"%f|prec=2|sign=always", 2.675, "+2.68"},
		{"%f|sci=true|prec=2", 2675, "2.68e+03"},
		{"%f|sci=true|prec=2", 0.00123, "1.23e-03"},
		{"%f|sci=true|prec=1", 9.95, "1.0e+01"},
		{"%f|sci=true|prec=0", 2.5, "2e+00"},
		{"%f|sci=true|prec=3", 1e300, "1.000e+300"},
		{"%f|sci=true", 1002.675, "1.002675e+03"},
		{"%f|sci=true|prec=2", 0, "0.00e+00"},
		{"%f|prec=2", math.Inf(1), "+Inf"},
		{"%f|prec=2", math.Inf(-1), "-Inf"},
		{"%f|prec=2", math.NaN(), "NaN"},
		{"%f|sci=true|prec=2", math.Inf(1), "+Inf"},
		{"%f", math.Inf(-1), "-Inf"},
		{"%f|sign=always", math.Inf(1), "+Inf"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if _, err := callPrintf(in, &buf, vals(c.format, f(c.v))); err != nil {
			t.Errorf("%s %v: %v", c.format, c.v, err)
			continue
		}
		if buf.String() != c.want {
			t.Errorf("%s %v = %q, want %q", c.format, c.v, buf.String(), c.want)
		}
	}
	// %a's per-element prec shares the rule.
	var buf bytes.Buffer
	list := interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), []interpreter.Value{f(1002.675), f(2.5), f(-0.125)})
	if _, err := callPrintf(in, &buf, vals("%a|prec=2", list)); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "1002.68") || !strings.Contains(got, "2.50") || !strings.Contains(got, "-0.12") {
		t.Errorf("%%a|prec=2 = %q, want 1002.68, 2.50, -0.12", got)
	}
}

// roundFixed agrees with an independent big-integer half-even rounding of the
// shortest decimal (intl.formatNumber's method) on random values.
func TestRoundFixedMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(52))
	for i := 0; i < 20000; i++ {
		v := math.Abs(float64(rng.Int63n(1_000_000_000)) / math.Pow(10, float64(rng.Intn(9))))
		if rng.Intn(4) == 0 {
			v = math.Abs(rng.NormFloat64() * math.Pow(10, float64(rng.Intn(30)-15)))
		}
		prec := rng.Intn(8)
		s := strconv.FormatFloat(v, 'f', -1, 64)
		if got, want := roundFixed(s, prec), referenceRound(s, prec); got != want {
			t.Fatalf("roundFixed(%s, %d) = %s, want %s", s, prec, got, want)
		}
	}
}

func referenceRound(s string, prec int) string {
	whole, frac, _ := strings.Cut(s, ".")
	for len(frac) < prec {
		frac += "0"
	}
	keep, rest := frac[:prec], frac[prec:]
	n, _ := new(big.Int).SetString(whole+keep, 10)
	if rest != "" {
		r, _ := new(big.Int).SetString(rest, 10)
		half := new(big.Int).Mul(big.NewInt(5), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(rest)-1)), nil))
		if c := r.Cmp(half); c > 0 || (c == 0 && n.Bit(0) == 1) {
			n.Add(n, big.NewInt(1))
		}
	}
	d := n.String()
	if len(d) <= prec {
		d = strings.Repeat("0", prec-len(d)+1) + d
	}
	if prec == 0 {
		return d
	}
	return d[:len(d)-prec] + "." + d[len(d)-prec:]
}
