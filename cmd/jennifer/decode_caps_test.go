// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"strings"
	"testing"
)

// TestDecodeCapsAreLimit: a reached nesting or node cap of a document decoder
// (json, toml, yaml, xml, asn1, and the shared write surface) is kind "limit"
// (M28.68), so a caller tells it from malformed input without the message.
func TestDecodeCapsAreLimit(t *testing.T) {
	cases := []struct{ name, body string }{
		{"json depth", `json.decode(strings.repeat("[", 1001) + strings.repeat("]", 1001));`},
		{"json depth in an object", `json.decode(strings.repeat('{"a":', 1001) + "1" + strings.repeat('}', 1001));`},
		{"json decodeWith nodes", `json.decodeWith("[1,2,3,4]", json.DecodeOptions{maxDepth: 0, maxNodes: 4});`},
		{"json decodeWith depth", `json.decodeWith("[[[1]]]", json.DecodeOptions{maxDepth: 2, maxNodes: 0});`},
		{"toml depth", `toml.decode("a = " + strings.repeat("[", 1001) + strings.repeat("]", 1001));`},
		{"toml key segments", `toml.decode(strings.repeat("a.", 1001) + "b = 1");`},
		{"yaml depth", `yaml.decode(strings.repeat("[", 1001) + strings.repeat("]", 1001));`},
		{"xml depth", `xml.decode(strings.repeat("<a>", 1001) + strings.repeat("</a>", 1001));`},
		{"asn1 depth", `asn1.decode(nested(1002));`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `use io; use json; use toml; use yaml; use xml; use asn1; use strings; use binary;
# nested is n DER SEQUENCEs around a NULL.
func nested(n as int) {
	def b as bytes init binary.concat(binary.make(1, 5), binary.make(1, 0));
	for (def i in 0..$n) {
		def k as int init len($b);
		def head as bytes init binary.make(1, 48);
		if ($k < 128) {
			$head = binary.concat($head, binary.make(1, $k));
		} elseif ($k < 256) {
			$head = binary.concat(binary.concat($head, binary.make(1, 129)), binary.make(1, $k));
		} else {
			$head = binary.concat(binary.concat($head, binary.make(1, 130)), binary.concat(binary.make(1, $k // 256), binary.make(1, $k % 256)));
		}
		$b = binary.concat($head, $b);
	}
	return $b;
}
try { ` + c.body + ` io.printf("no error\n"); } catch (e) { io.printf("%s %s\n", $e.kind, $e.message); }`
			out, err := runProgramOutput("caps.j", src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.HasPrefix(out, "limit ") {
				t.Errorf("got %q, want limit", out)
			}
		})
	}
}

// TestDecodeWith: json.decodeWith decodes as json.decode within its caps, and
// refuses options that would raise a cap or are negative.
func TestDecodeWith(t *testing.T) {
	out, err := runProgramOutput("caps.j", `use io; use json;
def v as json.Value init json.decodeWith("[1,[2,3]]", json.DecodeOptions{maxDepth: 2, maxNodes: 5});
io.printf("%s\n", json.encode($v));
for (def o in [json.DecodeOptions{maxDepth: 1001, maxNodes: 0}, json.DecodeOptions{maxDepth: 0, maxNodes: -1}]) {
	try {
		json.decodeWith("1", $o);
	} catch (e) {
		io.printf("%s: %s\n", $e.kind, $e.message);
	}
}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "[1,[2,3]]\n" +
		"runtime: json.decodeWith: maxDepth must be 0 (the default) to 1000, got 1001\n" +
		"runtime: json.decodeWith: maxNodes must be 0 (the default) to 2097152, got -1\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
