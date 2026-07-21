# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# jwt_demo.j - sign and verify JSON Web Tokens. Shows the HMAC (HS256) and
# Ed25519 (EdDSA) algorithms, which need no external keys, plus decode-without-
# verify and the algorithm-confusion / expiry rejections. RS256 / ES256 work the
# same way with a PEM RSA / EC key in place of the secret.
#
#     jennifer run examples/modules/jwt_demo.j

use io;
use json;
use crypto;
use convert;
use time;
import "../../modules/jwt.j" as jwt;

# --- HS256: a shared-secret token ---
def secret as bytes init convert.bytesFromString("change-me-in-production", "utf-8");

# Claims: subject, role, and an expiry one hour out (NumericDate = Unix seconds).
def exp as int init time.unix(time.now()) + 3600;
def claims as json.Value init json.decode("{\"sub\":\"ada\",\"role\":\"admin\",\"exp\":" +
    convert.toString($exp) + "}");

def token as string init jwt.sign($claims, $secret, "HS256");
io.printf("HS256 token:\n  %s\n\n", $token);

# Read the claims without verifying (e.g. to inspect before choosing a key).
io.printf("decoded (unverified) sub = %s\n", json.asString(jwt.decode($token), "/sub"));

# Verify: checks the signature, the header alg, and the exp claim.
def verified as json.Value init jwt.verify($token, $secret, "HS256");
io.printf("verified sub = %s, role = %s\n\n", json.asString($verified, "/sub"),
    json.asString($verified, "/role"));

# --- security properties ---
def wrong as bytes init convert.bytesFromString("the-wrong-secret", "utf-8");
io.printf("wrong key  -> %s\n", verifyOutcome($token, $wrong, "HS256"));
# Algorithm-confusion: the same bytes, but the caller expects RS256.
io.printf("wrong alg  -> %s\n", verifyOutcome($token, $secret, "RS256"));

# --- EdDSA: a public-key token, no external key material needed ---
def kp as crypto.Keypair init crypto.signKeypair();
def edToken as string init jwt.sign($claims, $kp.private, "EdDSA");
io.printf("\nEdDSA verified sub = %s\n",
    json.asString(jwt.verify($edToken, $kp.public, "EdDSA"), "/sub"));

# verifyOutcome verifies a token and reports "accepted" or the rejection reason -
# the shape a caller wraps around jwt.verify.
func verifyOutcome(token as string, key as bytes, alg as string) {
    try {
        jwt.verify($token, $key, $alg);
        return "accepted";
    } catch (e) {
        return "rejected (" + $e.message + ")";
    }
}

# --- jwt_auth: JWT as a web.before middleware ---
# Not a separate module - this is the whole of it. Register `requireJwt` with
# `web.before`; it pulls the bearer token from the Authorization header,
# verifies it, and rejects the request on failure. (Illustrative: needs the
# `web` framework and a configured secret.)
#
#     func requireJwt(ctx as web.Context) {
#         def auth as string init web.header($ctx, "Authorization");
#         if (not strings.startsWith($auth, "Bearer ")) {
#             web.respond($ctx, 401, "missing bearer token");
#             return false;                 # stop the chain
#         }
#         def token as string init strings.substring($auth, 7, len($auth));
#         try {
#             def claims as json.Value init jwt.verify($token, JWT_SECRET, "HS256");
#             web.set($ctx, "user", json.asString($claims, "/sub"));
#         } catch (e) {
#             web.respond($ctx, 401, "invalid token");
#             return false;
#         }
#         return true;                       # continue to the handler
#     }
