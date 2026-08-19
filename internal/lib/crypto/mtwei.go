// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// EC-SRP (Elliptic Curve Secure Remote Password) key agreement as used by
// MikroTik's MAC-Telnet / Winbox login on RouterOS 6.43+ and all of v7. This
// is a dependency-free port (math/big + crypto/sha256 + the library random
// source) of the reference C implementation (mtwei.*, IEEE P1363.2 EC-SRP over
// a Curve25519 group expressed in short-Weierstrass form). No OpenSSL, no
// bignum library: it stays TinyGo-clean.
//
// The three registered functions (crypto.mtweiKeygen / mtweiId / mtweiClientKey)
// are the client side a MAC-Telnet implementation needs. The server side
// (mtweiServerKey) is kept unexported for the round-trip self-test.

package cryptolib

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// mtweiCurve holds the short-Weierstrass form of Curve25519 (y^2 = x^3 + a*x + b
// mod p) plus the constants the protocol needs. All values are lifted verbatim
// from the reference implementation.
type mtweiCurve struct {
	p, a, b  *big.Int // field modulus and curve coefficients
	gx, gy   *big.Int // generator
	order    *big.Int // subgroup order
	w2m, m2w *big.Int // Weierstrass<->Montgomery x-coordinate conversion offsets
	sqrtExp  *big.Int // (p+3)/8, the p==5 (mod 8) square-root exponent
	tonelli  *big.Int // 2^((p-1)/4) mod p, the sqrt disambiguation factor
}

var (
	mtweiOnce sync.Once
	mtwei     *mtweiCurve
)

func hexBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("cryptolib: bad mtwei constant " + s)
	}
	return n
}

func mtweiInit() *mtweiCurve {
	mtweiOnce.Do(func() {
		c := &mtweiCurve{
			p:     hexBig("7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffed"),
			a:     hexBig("2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa984914a144"),
			b:     hexBig("7b425ed097b425ed097b425ed097b425ed097b425ed097b4260b5e9c7710c864"),
			gx:    hexBig("2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaad245a"),
			gy:    hexBig("5f51e65e475f794b1fe122d388b72eb36dc2b28192839e4dd6163a5d81312c14"),
			order: hexBig("1000000000000000000000000000000014def9dea2f79cd65812631a5cf5d3ed"),
			w2m:   hexBig("555555555555555555555555555555555555555555555555555555555552db9c"),
			m2w:   hexBig("2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaad2451"),
		}
		// (p+3)/8 for the p == 5 (mod 8) square-root formula.
		c.sqrtExp = new(big.Int).Add(c.p, big.NewInt(3))
		c.sqrtExp.Rsh(c.sqrtExp, 3)
		// 2^((p-1)/4) mod p, used to fix up a candidate root.
		e := new(big.Int).Sub(c.p, big.NewInt(1))
		e.Rsh(e, 2)
		c.tonelli = new(big.Int).Exp(big.NewInt(2), e, c.p)
		mtwei = c
	})
	return mtwei
}

// ecPoint is an affine curve point; inf marks the point at infinity.
type ecPoint struct {
	x, y *big.Int
	inf  bool
}

func (c *mtweiCurve) infinity() *ecPoint { return &ecPoint{inf: true} }

// add returns p1 + p2 on the curve (standard affine short-Weierstrass rules,
// with the doubling and inverse special cases).
func (c *mtweiCurve) add(p1, p2 *ecPoint) *ecPoint {
	if p1.inf {
		return &ecPoint{x: cloneOr(p2.x), y: cloneOr(p2.y), inf: p2.inf}
	}
	if p2.inf {
		return &ecPoint{x: cloneOr(p1.x), y: cloneOr(p1.y), inf: p1.inf}
	}
	if p1.x.Cmp(p2.x) == 0 {
		// x1 == x2: either doubling (y equal, non-zero) or inverses (y sum 0).
		ySum := new(big.Int).Add(p1.y, p2.y)
		ySum.Mod(ySum, c.p)
		if p1.y.Cmp(p2.y) != 0 || p1.y.Sign() == 0 || ySum.Sign() == 0 {
			return c.infinity()
		}
		return c.double(p1)
	}
	// lambda = (y2 - y1) / (x2 - x1)
	num := new(big.Int).Sub(p2.y, p1.y)
	den := new(big.Int).Sub(p2.x, p1.x)
	return c.addWithSlope(p1, p2, num, den)
}

// double returns 2*p.
func (c *mtweiCurve) double(p *ecPoint) *ecPoint {
	if p.inf || p.y.Sign() == 0 {
		return c.infinity()
	}
	// lambda = (3*x^2 + a) / (2*y)
	num := new(big.Int).Mul(p.x, p.x)
	num.Mul(num, big.NewInt(3))
	num.Add(num, c.a)
	den := new(big.Int).Lsh(p.y, 1)
	return c.addWithSlope(p, p, num, den)
}

// addWithSlope finishes an add/double once the slope numerator/denominator are
// known: lambda = num/den, x3 = lambda^2 - x1 - x2, y3 = lambda*(x1 - x3) - y1.
func (c *mtweiCurve) addWithSlope(p1, p2 *ecPoint, num, den *big.Int) *ecPoint {
	denInv := new(big.Int).ModInverse(new(big.Int).Mod(den, c.p), c.p)
	if denInv == nil {
		return c.infinity()
	}
	lambda := new(big.Int).Mul(new(big.Int).Mod(num, c.p), denInv)
	lambda.Mod(lambda, c.p)

	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, p1.x)
	x3.Sub(x3, p2.x)
	x3.Mod(x3, c.p)

	y3 := new(big.Int).Sub(p1.x, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, p1.y)
	y3.Mod(y3, c.p)

	return &ecPoint{x: x3, y: y3}
}

// mul returns k*p by left-to-right double-and-add.
func (c *mtweiCurve) mul(p *ecPoint, k *big.Int) *ecPoint {
	res := c.infinity()
	if p.inf || k.Sign() == 0 {
		return res
	}
	for i := k.BitLen() - 1; i >= 0; i-- {
		res = c.double(res)
		if k.Bit(i) == 1 {
			res = c.add(res, p)
		}
	}
	return res
}

// sqrtModP returns a square root of n mod p (p == 5 mod 8), or nil if none.
func (c *mtweiCurve) sqrtModP(n *big.Int) *big.Int {
	n = new(big.Int).Mod(n, c.p)
	if n.Sign() == 0 {
		return big.NewInt(0)
	}
	cand := new(big.Int).Exp(n, c.sqrtExp, c.p)
	sq := new(big.Int).Mul(cand, cand)
	sq.Mod(sq, c.p)
	if sq.Cmp(n) == 0 {
		return cand
	}
	cand.Mul(cand, c.tonelli)
	cand.Mod(cand, c.p)
	sq.Mul(cand, cand)
	sq.Mod(sq, c.p)
	if sq.Cmp(n) == 0 {
		return cand
	}
	return nil
}

// decompress builds the curve point with the given x and the y whose least
// significant bit equals yBit; ok is false when x is not on the curve.
func (c *mtweiCurve) decompress(x *big.Int, yBit int) (*ecPoint, bool) {
	x = new(big.Int).Mod(x, c.p)
	// rhs = x^3 + a*x + b
	rhs := new(big.Int).Mul(x, x)
	rhs.Mul(rhs, x)
	ax := new(big.Int).Mul(c.a, x)
	rhs.Add(rhs, ax)
	rhs.Add(rhs, c.b)
	rhs.Mod(rhs, c.p)

	y := c.sqrtModP(rhs)
	if y == nil {
		return nil, false
	}
	if int(y.Bit(0)) != (yBit & 1) {
		y = new(big.Int).Sub(c.p, y)
	}
	return &ecPoint{x: x, y: y}, true
}

func cloneOr(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

// pad32 renders n as a 32-byte big-endian slice.
func pad32(n *big.Int) []byte {
	var buf [32]byte
	n.FillBytes(buf[:])
	return buf[:]
}

// tangle mixes a validator-derived point into target (mutating it in place) and
// returns the validator as a big integer. negate selects the y parity of the
// decompressed mix point. Mirrors the reference `tangle`.
func (c *mtweiCurve) tangle(target *ecPoint, validator []byte, negate int) *big.Int {
	v := new(big.Int).SetBytes(validator)
	vpt := c.mul(&ecPoint{x: c.gx, y: c.gy}, v)

	vpubX := new(big.Int).Add(vpt.x, c.w2m)
	vpubX.Mod(vpubX, c.p)
	sum := sha256.Sum256(pad32(vpubX))

	edpx := new(big.Int).SetBytes(sum[:])
	var mixed *ecPoint
	for {
		h := sha256.Sum256(pad32(edpx))
		edpxm := new(big.Int).SetBytes(h[:])
		edpxm.Add(edpxm, c.m2w)
		edpxm.Mod(edpxm, c.p)
		if pt, ok := c.decompress(edpxm, negate); ok {
			mixed = pt
			break
		}
		edpx.Add(edpx, big.NewInt(1))
	}

	res := c.add(target, mixed)
	target.x, target.y, target.inf = res.x, res.y, res.inf
	return v
}

// mtweiKeygen generates a client (validator == nil) or validator-entangled
// (server) keypair. Returns the 32-byte private scalar (big-endian, as consumed
// by BN_bin2bn) and the 33-byte compressed public key.
func mtweiKeygen(validator []byte) (priv []byte, pub []byte) {
	c := mtweiInit()
	var pk [32]byte
	RandFill(pk[:])
	// The reference clamps these fixed byte positions of the big-endian scalar.
	pk[0] &= 248
	pk[31] &= 127
	pk[31] |= 64

	d := new(big.Int).SetBytes(pk[:])
	point := c.mul(&ecPoint{x: c.gx, y: c.gy}, d)
	if validator != nil {
		c.tangle(point, validator, 0)
	}

	x := new(big.Int).Add(point.x, c.w2m)
	x.Mod(x, c.p)
	out := make([]byte, 33)
	copy(out[:32], pad32(x))
	if point.y.Bit(0) == 1 {
		out[32] = 1
	}
	return pk[:], out
}

// mtweiID derives the 32-byte SRP validator from the credentials and salt:
// SHA256(salt || SHA256(username ":" password)).
func mtweiID(username, password string, salt []byte) []byte {
	inner := sha256.New()
	inner.Write([]byte(username))
	inner.Write([]byte{':'})
	inner.Write([]byte(password))
	v := inner.Sum(nil)

	outer := sha256.New()
	outer.Write(salt)
	outer.Write(v)
	return outer.Sum(nil)
}

// mtweiClientKey runs the client half of EC-SRP and returns the 32-byte shared
// authenticator sent in the MAC-Telnet password control packet. serverKey and
// clientKey are the 33-byte compressed public keys; validator is from mtweiID.
func mtweiClientKey(priv, serverKey, clientKey, validator []byte) ([]byte, error) {
	c := mtweiInit()
	if len(serverKey) != 33 || len(clientKey) != 33 {
		return nil, fmt.Errorf("public keys must be 33 bytes")
	}
	if len(validator) != 32 {
		return nil, fmt.Errorf("validator must be 32 bytes")
	}
	spx := new(big.Int).SetBytes(serverKey[:32])
	spx.Add(spx, c.m2w)
	spx.Mod(spx, c.p)
	serverPub, ok := c.decompress(spx, int(serverKey[32]))
	if !ok {
		return nil, fmt.Errorf("server public key is not on the curve")
	}

	v := c.tangle(serverPub, validator, 1)

	h := sha256.New()
	h.Write(clientKey[:32])
	h.Write(serverKey[:32])
	buf := h.Sum(nil)

	vh := new(big.Int).SetBytes(buf)
	vh.Mul(v, vh)
	vh.Mod(vh, c.order)
	vh.Add(vh, new(big.Int).SetBytes(priv))
	vh.Mod(vh, c.order)

	pt := c.mul(serverPub, vh)
	zin := new(big.Int).Add(pt.x, c.w2m)
	zin.Mod(zin, c.p)

	final := sha256.New()
	final.Write(buf)
	final.Write(pad32(zin))
	return final.Sum(nil), nil
}

// mtweiServerKey runs the server half of EC-SRP; kept for the round-trip test.
func mtweiServerKey(priv, clientKey, serverKey, validator []byte) ([]byte, error) {
	c := mtweiInit()
	cpx := new(big.Int).SetBytes(clientKey[:32])
	cpx.Add(cpx, c.m2w)
	cpx.Mod(cpx, c.p)
	clientPub, ok := c.decompress(cpx, int(clientKey[32]))
	if !ok {
		return nil, fmt.Errorf("client public key is not on the curve")
	}

	v := new(big.Int).SetBytes(validator)

	h := sha256.New()
	h.Write(clientKey[:32])
	h.Write(serverKey[:32])
	buf := h.Sum(nil)

	vpt := c.mul(&ecPoint{x: c.gx, y: c.gy}, v)
	hv := c.mul(vpt, new(big.Int).SetBytes(buf))
	clientPub = c.add(clientPub, hv)

	pt := c.mul(clientPub, new(big.Int).SetBytes(priv))
	zin := new(big.Int).Add(pt.x, c.w2m)
	zin.Mod(zin, c.p)

	final := sha256.New()
	final.Write(buf)
	final.Write(pad32(zin))
	return final.Sum(nil), nil
}

// ----- registered wrappers -------------------------------------------------

// mtweiKeygenFn implements crypto.mtweiKeygen() -> crypto.Keypair. The client
// keypair for a MAC-Telnet EC-SRP login: private is the 32-byte scalar, public
// the 33-byte compressed point sent to the router.
func mtweiKeygenFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiKeygen expects no arguments, got %d", len(args))
	}
	priv, pub := mtweiKeygen(nil)
	return interpreter.NamespacedStructVal(LibraryName, "Keypair", []interpreter.StructField{
		{Name: "public", Value: interpreter.BytesVal(pub)},
		{Name: "private", Value: interpreter.BytesVal(priv)},
	}), nil
}

// mtweiIdFn implements crypto.mtweiId(username, password, salt) -> bytes: the
// 32-byte SRP validator for the login.
func mtweiIdFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiId expects 3 arguments (username, password, salt), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString || args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiId: username and password must be strings")
	}
	if args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiId: salt must be bytes")
	}
	if len(args[2].Bytes) != 16 {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiId: salt must be 16 bytes, got %d", len(args[2].Bytes))
	}
	out := mtweiID(args[0].Str, args[1].Str, args[2].Bytes)
	return interpreter.BytesVal(out), nil
}

// mtweiClientKeyFn implements crypto.mtweiClientKey(private, serverKey,
// clientKey, validator) -> bytes: the 32-byte authenticator sent as the
// password in an EC-SRP MAC-Telnet login.
func mtweiClientKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiClientKey expects 4 arguments (private, serverKey, clientKey, validator), got %d", len(args))
	}
	for i, name := range []string{"private", "serverKey", "clientKey", "validator"} {
		if args[i].Kind != interpreter.KindBytes {
			return interpreter.Null(), fmt.Errorf("crypto.mtweiClientKey: %s must be bytes", name)
		}
	}
	if len(args[0].Bytes) != 32 {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiClientKey: private must be 32 bytes, got %d", len(args[0].Bytes))
	}
	out, err := mtweiClientKey(args[0].Bytes, args[1].Bytes, args[2].Bytes, args[3].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.mtweiClientKey: %v", err)
	}
	return interpreter.BytesVal(out), nil
}
