// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

import (
	"os"
	"strings"
	"testing"
)

// Each library's data kind (M29.21) is documented on its library page.
func TestDataKindsDocumented(t *testing.T) {
	if len(DataKinds()) == 0 {
		t.Fatal("no data kinds recorded")
	}
	for lib, kind := range DataKinds() {
		if !HasLibrary(lib) {
			t.Errorf("data kind for unknown library %q", lib)
		}
		doc, err := os.ReadFile("../../docs/libraries/" + lib + ".md")
		if err != nil {
			t.Errorf("%s: %v", lib, err)
			continue
		}
		// The page's "Errors" table has a row for the kind.
		_, errs, ok := strings.Cut(string(doc), "\n### Errors\n")
		if !ok || !strings.Contains(errs, "| `\""+kind+"\"` |") {
			t.Errorf("docs/libraries/%s.md has no Errors table row for kind %q", lib, kind)
		}
	}
}
