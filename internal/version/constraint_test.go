// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package version

import (
	"strings"
	"testing"
)

// TestConstraintSuffixRefused: a pragma constraint with a pre-release or build
// suffix was cut at the '-' / '+' and compared as its core, so `>=0.25.0+junk`
// passed as `>=0.25.0`. It is an error now, on every build.
func TestConstraintSuffixRefused(t *testing.T) {
	for _, build := range []string{"0.26.0", "0.25.0-dev+183.aa1f0122", "dev"} {
		withVersion(build, func() {
			for _, v := range []string{"0.25.0+junk", "0.25.0-rc1", "0.25.0-dev+1.abc", "0.25.0-"} {
				if err := CheckConstraint(v); err == nil || !strings.Contains(err.Error(), "suffix") {
					t.Errorf("CheckConstraint(%q) = %v, want a suffix error", v, err)
				}
				if _, err := AtLeast(v); err == nil {
					t.Errorf("build %s: AtLeast(%q) accepted a suffix", build, v)
				}
				if _, err := Below(v); err == nil {
					t.Errorf("build %s: Below(%q) accepted a suffix", build, v)
				}
			}
			for _, v := range []string{"0.25.0", "1.2.3", "10.20.30"} {
				if err := CheckConstraint(v); err != nil {
					t.Errorf("CheckConstraint(%q) = %v", v, err)
				}
			}
		})
	}
}

// TestOwnVersionSuffixAccepted: the interpreter's own version keeps its -dev
// build suffix; the comparison side still parses it (M28.42).
func TestOwnVersionSuffixAccepted(t *testing.T) {
	withVersion("0.25.0-dev+183.aa1f0122", func() {
		if ok, err := AtLeast("0.26.0"); err != nil || !ok {
			t.Errorf("dev build AtLeast(0.26.0) = %v, %v; want true", ok, err)
		}
		if ok, err := Below("0.26.0"); err != nil || ok {
			t.Errorf("dev build Below(0.26.0) = %v, %v; want false", ok, err)
		}
		if c, err := Compare(Version, "0.25.0"); err != nil || c != 0 {
			t.Errorf("Compare(%s, 0.25.0) = %d, %v", Version, c, err)
		}
	})
	// A release build with build metadata still compares by its core.
	withVersion("0.26.0+distro.1", func() {
		if ok, err := AtLeast("0.26.0"); err != nil || !ok {
			t.Errorf("AtLeast(0.26.0) = %v, %v; want true", ok, err)
		}
		if ok, err := AtLeast("0.26.1"); err != nil || ok {
			t.Errorf("AtLeast(0.26.1) = %v, %v; want false", ok, err)
		}
	})
}
