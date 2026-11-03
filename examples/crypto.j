# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Exercises the `crypto` library: crypto-grade random, constant-time
 * comparison, and the two key-derivation functions. The random draws are
 * unpredictable, so the golden prints only their shape (a length, an in-range
 * check); the comparison and KDF outputs are deterministic.
 * @module crypto
 */

use io;
use convert;
use encoding;
use crypto;

# Crypto-grade random: unguessable and unseedable, so assert only the shape.
def token as bytes init crypto.randBytes(16);
io.printf("randBytes(16) len = %d\n", len($token));
def roll as int init crypto.randInt(1, 6);
io.printf("randInt(1,6) in range = %t\n", $roll >= 1 and $roll <= 6);

# Constant-time equality of two byte strings (MAC comparison).
def a as bytes init convert.bytesFromString("same-value", "utf-8");
def b as bytes init convert.bytesFromString("same-value", "utf-8");
def c as bytes init convert.bytesFromString("diff-value", "utf-8");
io.printf("hmacEqual(equal)   = %t\n", crypto.hmacEqual($a, $b));
io.printf("hmacEqual(unequal) = %t\n", crypto.hmacEqual($a, $c));

# Key derivation (deterministic, RFC test vectors).
def password as bytes init convert.bytesFromString("password", "utf-8");
def salt as bytes init convert.bytesFromString("salt", "utf-8");
def empty as bytes;
io.printf(
    "pbkdf(password,salt,1,32) = %s\n",
    encoding.toText(crypto.pbkdf2($password, $salt, 1, 32, "sha256"), "hex"));
io.printf("hkdf derived length = %d\n", len(crypto.hkdf($password, $salt, $empty, 42, "sha256")));

use hash;

# Authenticated encryption (AES-256-GCM). The key / nonce are random, but the
# round-trip and tamper-detection outcomes are deterministic.
def key as bytes init crypto.randBytes(32);
def secret as bytes init convert.bytesFromString("attack at dawn", "utf-8");
def box as bytes init crypto.encrypt($key, $secret);
io.printf(
    "decrypt round-trip = %s\n",
    convert.stringFromBytes(crypto.decrypt($key, $box), "utf-8"));
def tampered as bytes init $box;
$tampered[len($tampered) - 1] = ($tampered[len($tampered) - 1] + 1) % 256;
try {
    crypto.decrypt($key, $tampered);
    io.printf("tamper detected = false\n");
} catch (e) {
    io.printf("tamper detected = true\n");
}

# Ed25519 signatures: sign with the private key, verify with the public key.
def kp as crypto.Keypair init crypto.signKeypair();
def sig as bytes init crypto.sign($kp.private, $secret);
io.printf("verify genuine = %t\n", crypto.verify($kp.public, $secret, $sig));
io.printf(
    "verify forged  = %t\n",
    crypto.verify($kp.public, convert.bytesFromString("retreat", "utf-8"), $sig));

# X25519 key agreement: both sides reach the same 32-byte secret from their own
# private key and the other's public key.
def alice as crypto.Keypair init crypto.x25519Keypair();
def bob as crypto.Keypair init crypto.x25519Keypair();
def sa as bytes init crypto.x25519($alice.private, $bob.public);
def sb as bytes init crypto.x25519($bob.private, $alice.public);
io.printf("x25519 agree = %t, %d bytes\n", $sa == $sb, len($sa));
# The RFC 7748 test vector: a fixed private key gives the published public key.
def rfcPriv as bytes init encoding.fromText(
    "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a",
    "hex");
io.printf("x25519 rfc 7748 = %s\n", encoding.toText(crypto.x25519Public($rfcPriv), "hex"));

# SHA-384 digest (added in the hash fill-out).
io.printf("sha384 length = %d\n", len(hash.compute($secret, "sha384")));
