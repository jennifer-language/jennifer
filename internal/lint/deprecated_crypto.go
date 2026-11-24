// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// deprecatedRSANames are the `crypto.sign` / `verify` names deprecated in
// 0.28.0 (M29.51) and removed in 0.29.0, with the names that replace them.
var deprecatedRSANames = map[string]string{
	"rsa-sha256": "rsa-pkcs1-sha256",
	"rsa-sha384": "rsa-pkcs1-sha384",
	"rsa-sha512": "rsa-pkcs1-sha512",
}

// checkDeprecatedCrypto (L301, from checkDeprecated) reports the deprecated
// `crypto` call shapes that only their arguments tell apart: a literal
// `"rsa-sha*"` algorithm in `sign` / `verify`, and a password whose static
// type is `string` in `hashPassword` / `verifyPassword` (both take `bytes`).
// An algorithm or password of unknown static type is not reported.
func checkDeprecatedCrypto(c *checkCtx, t *typer, lib string, qc *parser.QualifiedCallExpr, resolve func(string) *binding) {
	if lib != "crypto" {
		return
	}
	const removed = "is deprecated and removed in 0.29.0"
	p := qc.Prefix
	switch qc.Callee {
	case "sign", "verify":
		if len(qc.Args) == 0 {
			return
		}
		if lit, ok := qc.Args[0].(*parser.StringLit); ok {
			if now, old := deprecatedRSANames[lit.Value]; old {
				c.report("L301", lit, fmt.Sprintf("`%s.%s(%q, ...)` %s; use %q (the same PKCS#1 v1.5 signature)", p, qc.Callee, lit.Value, removed, now))
			}
		}
	case "hashPassword", "verifyPassword":
		i := 1
		if qc.Callee == "verifyPassword" {
			i = 0
		}
		if len(qc.Args) <= i {
			return
		}
		if ty, ok := t.typeOf(qc.Args[i], resolve); ok && ty.Kind == parser.TypeString {
			c.report("L301", qc.Args[i], fmt.Sprintf("a `string` password to `%s.%s` %s; pass `bytes`: `encoding.encode(\"utf-8\", password)`", p, qc.Callee, removed))
		}
	}
}
