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
def a as bytes init convert.bytesFromString("utf-8", "same-value");
def b as bytes init convert.bytesFromString("utf-8", "same-value");
def c as bytes init convert.bytesFromString("utf-8", "diff-value");
io.printf("equal(equal)   = %t\n", hash.equal($a, $b));
io.printf("equal(unequal) = %t\n", hash.equal($a, $c));

# Key derivation (deterministic, RFC test vectors).
def password as bytes init convert.bytesFromString("utf-8", "password");
def salt as bytes init convert.bytesFromString("utf-8", "salt");
def empty as bytes;
io.printf(
    "pbkdf(password,salt,1,32) = %s\n",
    encoding.toText("hex", crypto.pbkdf2("sha256", $password, $salt, 1, 32)));
io.printf("hkdf derived length = %d\n", len(crypto.hkdf("sha256", $password, $salt, $empty, 42)));

use hash;

# Authenticated encryption (AES-256-GCM). The key / nonce are random, but the
# round-trip and tamper-detection outcomes are deterministic. The caller makes
# a fresh 12-byte nonce per message (never reused under one key) and keeps it
# with the box; `empty` is the (unused) additional authenticated data.
def key as bytes init crypto.randBytes(32);
def nonce as bytes init crypto.randBytes(12);
def secret as bytes init convert.bytesFromString("utf-8", "attack at dawn");
def box as bytes init crypto.encrypt("aes-gcm", $secret, $key, $nonce, $empty);
io.printf(
    "decrypt round-trip = %s\n",
    convert.stringFromBytes("utf-8", crypto.decrypt("aes-gcm", $box, $key, $nonce, $empty)));
def tampered as bytes init $box;
$tampered[len($tampered) - 1] = ($tampered[len($tampered) - 1] + 1) % 256;
try {
    crypto.decrypt("aes-gcm", $tampered, $key, $nonce, $empty);
    io.printf("tamper detected = false\n");
} catch (e) {
    io.printf("tamper detected = true\n");
}

# Ed25519 signatures: sign with the private key, verify with the public key.
def kp as crypto.Keypair init crypto.keypair("ed25519");
def sig as bytes init crypto.sign("ed25519", $secret, $kp.private);
io.printf("verify genuine = %t\n", crypto.verify("ed25519", $secret, $sig, $kp.public));
io.printf(
    "verify forged  = %t\n",
    crypto.verify("ed25519", convert.bytesFromString("utf-8", "retreat"), $sig, $kp.public));

# X25519 key agreement: both sides reach the same 32-byte secret from their own
# private key and the other's public key.
def alice as crypto.Keypair init crypto.keypair("x25519");
def bob as crypto.Keypair init crypto.keypair("x25519");
def sa as bytes init crypto.agree("x25519", $alice.private, $bob.public);
def sb as bytes init crypto.agree("x25519", $bob.private, $alice.public);
io.printf("x25519 agree = %t, %d bytes\n", $sa == $sb, len($sa));
# The RFC 7748 test vector: a fixed private key gives the published public key.
def rfcPriv as bytes init encoding.fromText(
    "hex",
    "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a");
io.printf("x25519 rfc 7748 = %s\n", encoding.toText("hex", crypto.publicKey("x25519", $rfcPriv)));

# SHA-384 digest (added in the hash fill-out).
io.printf("sha384 length = %d\n", len(hash.compute("sha384", $secret)));
