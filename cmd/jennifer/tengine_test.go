// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTengineModule drives the tengine template engine through the real import
// path: a base layout pulls in a page's {{ define }}d body, which ranges a list
// and html-escapes each item. A mismatch throws in the .j program and fails
// loadForTest, so this proves parse, define extraction, template dispatch, range,
// and the html pipe all work across the module boundary.
func TestTengineModule(t *testing.T) {
	tengineMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "tengine.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := `use testing;
use json;
import "` + tengineMod + `" as tengine;
def set as tengine.Set init tengine.newSet();
$set = tengine.add($set, "base", "<h1>{{ .title }}</h1>{{ template \"body\" . }}");
$set = tengine.add($set, "page", "{{ define \"body\" }}<ul>{{- range $i, $it := .items }}<li>{{ $i }}:{{ $it.name | html }}{{ if eq $it.kind \"post\" }} [post]{{ else if gt $i 0 }} [$.n={{ $.count }}]{{ end }}</li>{{ end -}}</ul>{{ end }}");
def out as string init tengine.render($set, "base", json.decode("{\"title\":\"Hi\",\"count\":2,\"items\":[{\"name\":\"a<b\",\"kind\":\"post\"},{\"name\":\"c&d\",\"kind\":\"page\"}]}"));
testing.assertEqual($out, "<h1>Hi</h1><ul><li>0:a&lt;b [post]</li><li>1:c&amp;d [$.n=2]</li></ul>");`
	progPath := filepath.Join(dir, "tengine.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("tengine program failed with code %d", code)
	}
}
