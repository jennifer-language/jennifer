// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package cryptolib

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
)

// TestMtweiRoundTrip is the correctness anchor for the EC-SRP port: a client and
// a server that share credentials + salt must independently derive the SAME
// 32-byte key. That mutual agreement is exactly what a real RouterOS login
// checks, so a passing round trip means the point arithmetic, the compressed
// decode, the tangle, and the two docrypto halves all interoperate.
func TestMtweiRoundTrip(t *testing.T) {
	salt := []byte("0123456789abcdef") // 16 bytes
	user, pass := "admin", "s3cret"

	validator := mtweiID(user, pass, salt)

	// Client keypair is plain; the server's is entangled with the validator.
	clientPriv, clientPub := mtweiKeygen(nil)
	serverPriv, serverPub := mtweiKeygen(validator)

	clientKey, err := mtweiClientKey(clientPriv, serverPub, clientPub, validator)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	serverKey, err := mtweiServerKey(serverPriv, clientPub, serverPub, validator)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	if !bytes.Equal(clientKey, serverKey) {
		t.Fatalf("shared key mismatch:\n client %x\n server %x", clientKey, serverKey)
	}
	if len(clientKey) != 32 {
		t.Fatalf("shared key length = %d, want 32", len(clientKey))
	}
}

// TestMtweiWrongPasswordDiffers - a wrong password on the client yields a key
// the server does not compute, i.e. auth would fail (as it must).
func TestMtweiWrongPasswordDiffers(t *testing.T) {
	salt := []byte("fedcba9876543210")
	serverValidator := mtweiID("admin", "correct", salt)
	clientValidator := mtweiID("admin", "wrong", salt)

	clientPriv, clientPub := mtweiKeygen(nil)
	serverPriv, serverPub := mtweiKeygen(serverValidator)

	clientKey, err := mtweiClientKey(clientPriv, serverPub, clientPub, clientValidator)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	serverKey, err := mtweiServerKey(serverPriv, clientPub, serverPub, serverValidator)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	if bytes.Equal(clientKey, serverKey) {
		t.Fatal("wrong password produced a matching key")
	}
}

// TestMtweiCurveSanity checks the generator and a couple of decoded points lie
// on the curve, catching a mistranscribed constant.
func TestMtweiCurveSanity(t *testing.T) {
	c := mtweiInit()
	if !c.onCurve(&ecPoint{x: c.gx, y: c.gy}) {
		t.Fatal("generator is not on the curve")
	}
	// 2G, 3G must also be on the curve.
	g := &ecPoint{x: c.gx, y: c.gy}
	g2 := c.double(g)
	g3 := c.add(g2, g)
	if !c.onCurve(g2) || !c.onCurve(g3) {
		t.Fatal("2G/3G not on the curve")
	}
	// The subgroup order times G is the point at infinity.
	if inf := c.mul(g, c.order); !inf.inf {
		t.Fatal("order*G is not the identity")
	}
}

// onCurve is a test helper: y^2 == x^3 + a*x + b (mod p).
func (c *mtweiCurve) onCurve(p *ecPoint) bool {
	if p.inf {
		return true
	}
	lhs := new(big.Int).Mul(p.y, p.y)
	lhs.Mod(lhs, c.p)
	rhs := new(big.Int).Mul(p.x, p.x)
	rhs.Mul(rhs, p.x)
	ax := new(big.Int).Mul(c.a, p.x)
	rhs.Add(rhs, ax)
	rhs.Add(rhs, c.b)
	rhs.Mod(rhs, c.p)
	return lhs.Cmp(rhs) == 0
}

// mixedPoint replicates the validator-derived `mixed` point tangle computes
// internally, so a test can craft serverKey = compress(-mixed) - the input that
// collapses tangle's sum to the point at infinity.
func mixedPoint(c *mtweiCurve, validator []byte, negate int) *ecPoint {
	v := new(big.Int).SetBytes(validator)
	vpt := c.mul(&ecPoint{x: c.gx, y: c.gy}, v)
	vpubX := new(big.Int).Add(vpt.x, c.w2m)
	vpubX.Mod(vpubX, c.p)
	sum := sha256.Sum256(pad32(vpubX))
	edpx := new(big.Int).SetBytes(sum[:])
	for {
		h := sha256.Sum256(pad32(edpx))
		edpxm := new(big.Int).SetBytes(h[:])
		edpxm.Add(edpxm, c.m2w)
		edpxm.Mod(edpxm, c.p)
		if pt, ok := c.decompress(edpxm, negate); ok {
			return pt
		}
		edpx.Add(edpx, big.NewInt(1))
	}
}

// encodePoint renders a point as the 33-byte compressed public key (same format
// as mtweiKeygen's output).
func encodePoint(c *mtweiCurve, p *ecPoint) []byte {
	x := new(big.Int).Add(p.x, c.w2m)
	x.Mod(x, c.p)
	out := make([]byte, 33)
	copy(out[:32], pad32(x))
	if p.y.Bit(0) == 1 {
		out[32] = 1
	}
	return out
}

// TestMtweiClientKeyRejectsDegeneratePoint pins a crafted serverKey equal to
// -mixed makes tangle's point addition collapse to infinity; mtweiClientKey must
// return a catchable error, not dereference the nil coordinate (a fatal crash).
func TestMtweiClientKeyRejectsDegeneratePoint(t *testing.T) {
	c := mtweiInit()
	validator := mtweiID("admin", "s3cret", []byte("0123456789abcdef"))
	mixed := mixedPoint(c, validator, 1)
	neg := &ecPoint{x: new(big.Int).Set(mixed.x), y: new(big.Int).Sub(c.p, mixed.y)}
	neg.y.Mod(neg.y, c.p)
	serverKey := encodePoint(c, neg)

	clientPriv, clientPub := mtweiKeygen(nil)
	_, err := mtweiClientKey(clientPriv, serverKey, clientPub, validator)
	if err == nil {
		t.Fatal("crafted degenerate serverKey must be rejected, not crash")
	}
	if !strings.Contains(err.Error(), "degenerate") {
		t.Fatalf("want a degenerate-point error, got: %v", err)
	}
}
