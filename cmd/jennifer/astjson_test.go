// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bytes"
	"testing"
)

// TestWriteIndentCapped pins indentation is capped at maxIndentDepth so a
// pathologically deep AST (a 50k-operator chain) does not make `jennifer ast`
// quadratic in depth. JSON ignores whitespace, so a deeper node is still valid.
func TestWriteIndentCapped(t *testing.T) {
	var shallow, deep bytes.Buffer
	writeIndent(&shallow, maxIndentDepth)
	writeIndent(&deep, maxIndentDepth*1000)
	if deep.Len() != shallow.Len() {
		t.Fatalf("indent not capped: depth %d wrote %d bytes, cap-depth wrote %d",
			maxIndentDepth*1000, deep.Len(), shallow.Len())
	}
	if shallow.Len() != maxIndentDepth*len(indentUnit) {
		t.Fatalf("cap-depth indent = %d bytes, want %d", shallow.Len(), maxIndentDepth*len(indentUnit))
	}
}
