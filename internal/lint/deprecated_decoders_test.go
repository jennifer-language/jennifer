// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedDecoderShapes: L301 reports the decoder and cap call shapes
// 0.28.0 deprecates (M29.44, M29.47) and leaves their current forms alone.
func TestDeprecatedDecoderShapes(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"xml.decode with namespaces", "use xml;\ndef v as xml.Value init xml.decode(\"<a/>\", {\"x\": \"urn:x\"});", 1,
			"`xml.decode(s, ns)` is deprecated and removed in 0.29.0; use `xml.decodeWith(s, xml.DecodeOptions{namespaces: ...})`"},
		{"aliased xml.decode with namespaces", "use xml as x;\ndef v as x.Value init x.decode(\"<a/>\", {\"x\": \"urn:x\"});", 1, "`x.decodeWith(s, x.DecodeOptions{"},
		{"xml.decode", "use xml;\ndef v as xml.Value init xml.decode(\"<a/>\");", 0, ""},
		{"xml.decodeWith", "use xml;\ndef v as xml.Value init xml.decodeWith(\"<a/>\", xml.DecodeOptions{namespaces: {\"x\": \"urn:x\"}});", 0, ""},
		{"compress.unpack with a cap", "use compress;\ndef d as bytes;\ndef b as bytes init compress.unpack(\"gzip\", $d, 100);", 1,
			"`compress.unpack(format, data, maxBytes)` is deprecated and removed in 0.29.0; use `compress.unpackWith(format, data, compress.UnpackOptions{maxBytes: ...})`"},
		{"compress.unpack", "use compress;\ndef d as bytes;\ndef b as bytes init compress.unpack(\"gzip\", $d);", 0, ""},
		{"compress.unpackWith", "use compress;\ndef d as bytes;\ndef b as bytes init compress.unpackWith(\"gzip\", $d, compress.UnpackOptions{maxBytes: 100});", 0, ""},
		{"images.decode with a cap", "use images;\ndef d as bytes;\ndef m as images.Image init images.decode($d, 100);", 1,
			"`images.decode(data, maxPixels)` is deprecated and removed in 0.29.0; use `images.decodeWith(data, images.DecodeOptions{maxPixels: ...})`"},
		{"images.decode", "use images;\ndef d as bytes;\ndef m as images.Image init images.decode($d);", 0, ""},
		{"images.decodeWith", "use images;\ndef d as bytes;\ndef m as images.Image init images.decodeWith($d, images.DecodeOptions{maxPixels: 100});", 0, ""},
		{"archive.pack tgz", "use archive;\ndef d as bytes;\ndef b as bytes init archive.pack(\"tgz\", []);", 1,
			"the `archive.pack` format \"tgz\" is deprecated and removed in 0.29.0; use \"tar.gz\""},
		{"archive.unpack tgz", "use archive;\ndef d as bytes;\ndef e as list of archive.Entry init archive.unpack(\"tgz\", $d);", 1, "`archive.unpack` format \"tgz\""},
		{"archive.unpackWith tgz", "use archive;\ndef d as bytes;\ndef e as list of archive.Entry init archive.unpackWith(\"tgz\", $d, archive.UnpackOptions{});", 1, "`archive.unpackWith` format \"tgz\""},
		{"archive.unpack tar.gz", "use archive;\ndef d as bytes;\ndef e as list of archive.Entry init archive.unpack(\"tar.gz\", $d);", 0, ""},
		{"archive format from a variable", "use archive;\ndef d as bytes;\ndef f as string init \"tgz\";\ndef e as list of archive.Entry init archive.unpack($f, $d);", 0, ""},
		{"json.decode is untouched", "use json;\ndef v as json.Value init json.decode(\"1\");", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Fatalf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			for _, d := range diags {
				if d.ID == "L301" && !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}
