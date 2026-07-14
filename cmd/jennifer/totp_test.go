// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestTotpModule drives the totp module through the real import path (module
// boundary + cross-module Options struct): an RFC 6238 Appendix B vector, a
// generate/verify round-trip, and the otpauth provisioning URI. Any mismatch
// throws in the .j program and fails loadForTest, so this proves the module
// loads and computes correctly in the CLI, with no clock dependence (the
// deterministic *At entry points take an explicit Unix time).
func TestTotpModule(t *testing.T) {
	totpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "totp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// base32("12345678901234567890"), the RFC 6238 SHA-1 seed.
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	prog := fmt.Sprintf(`use testing;
import %q as totp;
def eight as totp.Options;
$eight.digits = 8;
testing.assertEqual(totp.generateAt(%q, 59, $eight), "94287082");
def o as totp.Options;
def code as string init totp.generateAt(%q, 1234567890, $o);
testing.assertTrue(totp.verifyAt(%q, $code, 1234567890, $o));
testing.assertTrue(totp.verifyAt(%q, $code, 1234567920, $o));
testing.assertFalse(totp.verifyAt(%q, $code, 1234568000, $o));
testing.assertEqual(totp.uri("ACME", "jane@acme.example", %q, $o),
    "otpauth://totp/ACME:jane%%40acme.example?secret=%s&issuer=ACME&algorithm=SHA1&digits=6&period=30");`,
		totpMod, secret, secret, secret, secret, secret, secret, secret)
	progPath := filepath.Join(dir, "totp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("totp program failed with code %d", code)
	}
}
