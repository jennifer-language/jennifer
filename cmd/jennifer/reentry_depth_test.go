// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestReentryRecursionStaysCatchable pins that recursion which bounces through a
// dispatch boundary - meta.call self-recursion, bare recursion interleaved with
// meta.call, and a module method that recurses back via meta.callMain - trips
// the *catchable* call-depth guard rather than overflowing the Go stack fatally.
//
// The call-depth counter is threaded down the logical call chain (across the
// dispatch boundary) rather than reset per crossing, so these chains accumulate
// depth and raise a catchable "call stack too deep" error. If the counter reset
// at each crossing (an earlier design), the Go stack would grow unbounded and
// the process would crash uncatchably - which loadForTest would see as a
// non-testExitPass code, failing this test.
func TestReentryRecursionStaysCatchable(t *testing.T) {
	dir := t.TempDir()

	modSrc := `use meta;
# A module method that recurses back into the host via meta.callMain.
export func bounce() { return meta.callMain("hostBounce"); }
`
	modPath := filepath.Join(dir, "reentrymod.j")
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	prog := fmt.Sprintf(`use testing;
use meta;
import %q as rm;

# 1. Pure meta.call self-recursion.
func pureCall() { return meta.call("pureCall"); }

# 2. Bare recursion interleaved with a meta.call re-entry.
func innerLeg() { return meta.call("outerLeg"); }
func outerLeg() { return innerLeg(); }

# 3. Host method that recurses through a module method that calls back via
#    meta.callMain (host -> module -> host -> ...).
func hostBounce() { return rm.bounce(); }

def caught1 as bool init false;
try { pureCall(); } catch (e) { $caught1 = true; testing.assertEqual($e.kind, "runtime"); }
testing.assertTrue($caught1);

def caught2 as bool init false;
try { outerLeg(); } catch (e) { $caught2 = true; }
testing.assertTrue($caught2);

def caught3 as bool init false;
try { hostBounce(); } catch (e) { $caught3 = true; }
testing.assertTrue($caught3);
`, modPath)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("re-entry recursion program failed with code %d (a fatal Go stack overflow would show here)", code)
	}
}
