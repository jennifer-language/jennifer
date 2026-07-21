#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The SASL authentication mechanisms that the mail clients (smtp / pop / imap) use to send credentials: the one-step base64 encoders plus the challenge-response CRAM-MD5 and SCRAM.
 * No networking; each call renders the token a server expects.
 * @module sasl_demo
 */
use io;
use convert;
use encoding;
import "../../modules/sasl.j" as sasl;

# PLAIN: authzid \0 authcid \0 password, base64-encoded in one step.
io.printf("PLAIN       -> %s\n", sasl.plain("alice", "s3cret"));

# LOGIN: username and password sent as two separate base64 challenges.
io.printf("LOGIN user  -> %s\n", sasl.loginUser("alice"));
io.printf("LOGIN pass  -> %s\n", sasl.loginPass("s3cret"));

# XOAUTH2 (the "use a token" half of OAuth2): user + OAuth2 access token.
io.printf("XOAUTH2     -> %s\n", sasl.bearer("alice@example.com", "ya29.a0AccessToken"));

# CRAM-MD5: the server sends a base64 challenge; the client answers with an
# HMAC-MD5 of it keyed by the password (the password never crosses the wire).
def challenge as string init encoding.toText(convert.bytesFromString("<12345.1@mail.example>", "utf-8"), "base64");
io.printf("CRAM-MD5    -> %s\n", sasl.cram("alice", "s3cret", $challenge));

# SCRAM (here SCRAM-SHA-256) is a salted multi-step exchange; this prints the
# client-first token (the nonce is fresh each run). The full round -
# scramClientFinal / scramFinalToken / scramVerify - is exercised against the
# RFC 5802 / 7677 vectors in modules/sasl_test.j.
def sc as sasl.Scram init sasl.scramStart("alice", "sha256");
io.printf("SCRAM first -> %s\n", sasl.scramClientFirst($sc));
