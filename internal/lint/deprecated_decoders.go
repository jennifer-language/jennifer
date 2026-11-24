// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkDeprecatedDecoders reports (L301) the decoder and cap call shapes 0.28.0
// deprecates (M29.44, M29.47; removed in 0.29.0, M30.25) on a call to the
// built-in library lib: the two-argument `xml.decode`, the positional caps of
// `compress.unpack` and `images.decode`, and `archive`'s "tgz" format written
// as a literal.
func checkDeprecatedDecoders(c *checkCtx, lib string, qc *parser.QualifiedCallExpr) {
	if msg := deprecatedDecoderShape(lib, qc); msg != "" {
		c.report("L301", qc, msg)
	}
}

// deprecatedDecoderShape names the fix for a deprecated call shape, or "".
func deprecatedDecoderShape(lib string, qc *parser.QualifiedCallExpr) string {
	p, n := qc.Prefix, len(qc.Args)
	const removed = "is deprecated and removed in 0.29.0"
	switch {
	case lib == "xml" && qc.Callee == "decode" && n == 2:
		return fmt.Sprintf("`%s.decode(s, ns)` %s; use `%s.decodeWith(s, %s.DecodeOptions{namespaces: ...})`", p, removed, p, p)
	case lib == "compress" && qc.Callee == "unpack" && n == 3:
		return fmt.Sprintf("`%s.unpack(format, data, maxBytes)` %s; use `%s.unpackWith(format, data, %s.UnpackOptions{maxBytes: ...})`", p, removed, p, p)
	case lib == "images" && qc.Callee == "decode" && n == 2:
		return fmt.Sprintf("`%s.decode(data, maxPixels)` %s; use `%s.decodeWith(data, %s.DecodeOptions{maxPixels: ...})`", p, removed, p, p)
	case lib == "archive" && (qc.Callee == "pack" || qc.Callee == "unpack" || qc.Callee == "unpackWith") && n > 0:
		if lit, ok := qc.Args[0].(*parser.StringLit); ok && lit.Value == "tgz" {
			return fmt.Sprintf("the `%s.%s` format \"tgz\" %s; use \"tar.gz\"", p, qc.Callee, removed)
		}
	}
	return ""
}
