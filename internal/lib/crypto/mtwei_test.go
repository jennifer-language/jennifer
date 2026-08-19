// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package cryptolib

import (
	"bytes"
	"math/big"
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
