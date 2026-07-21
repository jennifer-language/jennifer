#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Generate and verify a time-based one-time password (the six-digit two-factor
 * code an authenticator app shows), and build the otpauth provisioning URI.
 * @module totp_demo
 */
use io;
import "../../modules/totp.j" as totp;

def secret as string init "JBSWY3DPEHPK3PXP";     # the base32 secret both sides share
def o as totp.Options;                            # zero-value: 6 digits, 30 s, SHA-1

# A deterministic code at a fixed instant, then verify it across the skew window.
def at as int init 1700000000;
def code as string init totp.generateAt($secret, $at, $o);
io.printf("code at t=%d:          %s\n", $at, $code);
io.printf("verify (same step):    %t\n", totp.verifyAt($secret, $code, $at, $o));
io.printf("verify (30 s later):   %t\n", totp.verifyAt($secret, $code, $at + 30, $o));
io.printf("verify (2 min later):  %t\n", totp.verifyAt($secret, $code, $at + 120, $o));
io.printf("verify (wrong code):   %t\n", totp.verifyAt($secret, "000000", $at, $o));

# The live code for the current clock (what the user would type right now).
io.printf("code now:              %s\n", totp.generate($secret, $o));

# The otpauth:// URI an authenticator app enrols by scanning a QR code.
io.printf("%s\n", totp.uri("ACME Corp", "jane@acme.example", $secret, $o));
