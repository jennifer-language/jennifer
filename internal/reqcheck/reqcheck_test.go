// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package reqcheck

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/version"
)

// withVersion runs fn with version.Version pinned to v (restored after), so the
// tests can exercise a real release build rather than the "dev" bypass.
func withVersion(v string, fn func()) {
	saved := version.Version
	version.Version = v
	defer func() { version.Version = saved }()
	fn()
}

func TestNoHeader(t *testing.T) {
	src := "# SPDX-License-Identifier: LGPL-3.0-only\nuse io;\nio.printf(\"hi\\n\");\n"
	if err := CheckRequirements(src, "prog.j"); err != nil {
		t.Errorf("no pragma should pass, got %v", err)
	}
}

func TestVersionFloor(t *testing.T) {
	src := "# pragma-jennifer-version: >=0.25.0\nuse io;\n"
	// A dev build satisfies every floor.
	withVersion("dev", func() {
		if err := CheckRequirements(src, "m.j"); err != nil {
			t.Errorf("dev build should bypass, got %v", err)
		}
	})
	withVersion("0.24.0-dev+7.abc", func() {
		if err := CheckRequirements(src, "m.j"); err != nil {
			t.Errorf("-dev build should bypass, got %v", err)
		}
	})
	// A clean release below the floor fails.
	withVersion("0.24.1", func() {
		err := CheckRequirements(src, "m.j")
		if err == nil || !strings.Contains(err.Error(), "requires jennifer >=0.25.0") {
			t.Errorf("expected floor error, got %v", err)
		}
	})
	// A clean release at or above the floor passes.
	withVersion("0.25.0", func() {
		if err := CheckRequirements(src, "m.j"); err != nil {
			t.Errorf("0.25.0 should satisfy >=0.25.0, got %v", err)
		}
	})
	withVersion("1.0.0", func() {
		if err := CheckRequirements(src, "m.j"); err != nil {
			t.Errorf("1.0.0 should satisfy >=0.25.0, got %v", err)
		}
	})
}

func TestDuplicateVersion(t *testing.T) {
	src := "# pragma-jennifer-version: >=0.24.0\n# pragma-jennifer-version: >=0.25.0\nuse io;\n"
	if err := CheckRequirements(src, "m.j"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate-version error, got %v", err)
	}
}

func TestCapability(t *testing.T) {
	// net / exec are present on the standard (non-tinygo) test build.
	if err := CheckRequirements("# pragma-jennifer-capability: net\n", "m.j"); err != nil {
		t.Errorf("net capability should be available on the standard build, got %v", err)
	}
	// A comma / space list on one line, and multiple lines, both accumulate.
	if err := CheckRequirements("# pragma-jennifer-capability: net, exec\n", "m.j"); err != nil {
		t.Errorf("net+exec list should pass, got %v", err)
	}
	// An unknown capability name is a malformed directive (a typo).
	err := CheckRequirements("# pragma-jennifer-capability: nett\n", "m.j")
	if err == nil || !strings.Contains(err.Error(), "unknown capability") {
		t.Errorf("expected unknown-capability error, got %v", err)
	}
}

func TestMalformed(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"no-colon", "# pragma-jennifer-version >=0.25.0\n", "malformed"},
		{"bad-version", "# pragma-jennifer-version: >=1.x\n", "malformed version"},
		{"no-ge", "# pragma-jennifer-version: 0.25.0\n", "must be `>=major.minor.patch`"},
		// The unknown-key error must name the KEY, not the value (a regression guard:
		// it previously echoed the value).
		{"unknown-key", "# pragma-jennifer-platform: linux\n", `unknown pragma key "platform"`},
		// An absurdly long value / key is a malformed directive, not something echoed
		// whole into a diagnostic.
		{"long-value", "# pragma-jennifer-version: >=" + strings.Repeat("9", 500) + "\n", "too long"},
		{"long-key", "# pragma-jennifer-" + strings.Repeat("x", 100) + ": y\n", "malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckRequirements(c.src, "m.j")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

// A malformed pragma on a pathologically long line yields a bounded diagnostic, not
// one the size of the source.
func TestLongLineDiagnosticBounded(t *testing.T) {
	src := "# pragma-jennifer-" + strings.Repeat("z", 100000) + "\n" // marker, but no valid parse
	err := CheckRequirements(src, "m.j")
	if err == nil {
		t.Fatal("expected a malformed error")
	}
	if len(err.Error()) > 500 {
		t.Errorf("diagnostic is %d bytes; should be bounded", len(err.Error()))
	}
}

func TestCanonicalLine(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"#pragma-jennifer-version:   >=            0. 5 .0", "# pragma-jennifer-version: >=0.5.0", true},
		{"# pragma-jennifer-version: >=0.24.0", "# pragma-jennifer-version: >=0.24.0", true},
		{"#  pragma-jennifer-capability:   net   exec,sql", "# pragma-jennifer-capability: net, exec, sql", true},
		{"# see the pragma-jennifer-version: note", "# see the pragma-jennifer-version: note", false}, // prose
		{"# pragma-jennifer-version >=0.1.0", "# pragma-jennifer-version >=0.1.0", false},             // no colon
		{"# an ordinary comment", "# an ordinary comment", false},
	}
	for _, c := range cases {
		got, ok := CanonicalLine(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("CanonicalLine(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
		// Idempotent: canonicalizing a canonical line is a no-op.
		if ok {
			if again, _ := CanonicalLine(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		}
	}
}

// Version is evaluated before an unknown key, so an old interpreter reading a
// module that uses a newer key (and bumped its floor) reports the floor, not the
// key it does not recognise.
func TestVersionEvaluatedFirst(t *testing.T) {
	src := "# pragma-jennifer-version: >=0.99.0\n# pragma-jennifer-futurekey: x\n"
	withVersion("0.25.0", func() {
		err := CheckRequirements(src, "m.j")
		if err == nil || !strings.Contains(err.Error(), "requires jennifer") {
			t.Errorf("expected version error first, got %v", err)
		}
	})
}

// The directive is honored only in the leading header block (tolerant of shebang,
// SPDX, and the docblock); one after code is not scanned.
func TestHeaderScanTolerance(t *testing.T) {
	src := "#!/usr/bin/env -S jennifer run\n" +
		"# SPDX-License-Identifier: LGPL-3.0-only\n" +
		"/**\n * A module.\n * @module m\n */\n" +
		"# pragma-jennifer-version: >=0.25.0\n" +
		"use io;\n"
	withVersion("0.24.0", func() {
		if err := CheckRequirements(src, "m.j"); err == nil {
			t.Error("pragma after shebang/SPDX/docblock should still be honored")
		}
	})
	// A pragma sitting after code is not in the header and is ignored.
	after := "use io;\nio.printf(\"x\");\n# pragma-jennifer-version: >=0.99.0\n"
	withVersion("0.24.0", func() {
		if err := CheckRequirements(after, "m.j"); err != nil {
			t.Errorf("a pragma after code must be ignored, got %v", err)
		}
	})
}
