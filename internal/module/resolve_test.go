// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		path string
		want Kind
	}{
		{"./f.j", Local},
		{"../f.j", Local},
		{"../../a/b.j", Local},
		{"/abs/f.j", Absolute},
		{"f.j", Module},
		{"sub/f.j", Module},
		{"a/b/c.j", Module},
	}
	for _, c := range cases {
		got, err := Classify(c.path)
		if err != nil {
			t.Errorf("Classify(%q): unexpected error %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestClassifyRejects(t *testing.T) {
	cases := []struct {
		path, want string
	}{
		{"", "empty"},
		{`sub\f.j`, "must use '/'"},
		{`.\f.j`, "must use '/'"},
		{"f.txt", "must end in '.j'"},
		{"nodotj", "must end in '.j'"},
	}
	for _, c := range cases {
		_, err := Classify(c.path)
		if err == nil {
			t.Errorf("Classify(%q): expected an error", c.path)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("Classify(%q): error %q missing %q", c.path, err.Error(), c.want)
		}
	}
}

func TestResolveLocalAndAbsolute(t *testing.T) {
	dir := t.TempDir()
	importing := filepath.Join(dir, "pkg")

	// Local: relative to the importing file's directory, canonicalized.
	got, err := Resolve("./util.j", importing, nil, "")
	if err != nil {
		t.Fatalf("local: %v", err)
	}
	if want := filepath.Join(importing, "util.j"); got != want {
		t.Errorf("local = %q, want %q", got, want)
	}
	// Local with ..: navigation within the tree is allowed.
	got, _ = Resolve("../shared/x.j", importing, nil, "")
	if want := filepath.Join(dir, "shared", "x.j"); got != want {
		t.Errorf("local .. = %q, want %q", got, want)
	}
	// Absolute: exactly that file, ignoring importingDir and searchDirs.
	abs := filepath.Join(dir, "opt", "m.j")
	got, err = Resolve(abs, importing, []string{"/should/be/ignored"}, "")
	if err != nil {
		t.Fatalf("absolute: %v", err)
	}
	if got != abs {
		t.Errorf("absolute = %q, want %q", got, abs)
	}
}

func TestResolveModuleSearchPath(t *testing.T) {
	root := t.TempDir()
	sysdir := filepath.Join(root, "sys")
	idir := filepath.Join(root, "extra")
	mustMkdir(t, sysdir)
	mustMkdir(t, idir)
	mustWrite(t, filepath.Join(sysdir, "core.j"), "")
	mustWrite(t, filepath.Join(idir, "helper.j"), "")

	// Found in the system dir.
	got, err := Resolve("core.j", "/irrelevant", []string{sysdir, idir}, "")
	if err != nil {
		t.Fatalf("core.j: %v", err)
	}
	if want := filepath.Join(sysdir, "core.j"); got != want {
		t.Errorf("core.j = %q, want %q", got, want)
	}
	// Found only in the -I dir (a -I dir adds names).
	got, err = Resolve("helper.j", "/irrelevant", []string{sysdir, idir}, "")
	if err != nil {
		t.Fatalf("helper.j: %v", err)
	}
	if want := filepath.Join(idir, "helper.j"); got != want {
		t.Errorf("helper.j = %q, want %q", got, want)
	}
}

func TestResolveModuleDuplicateIsError(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	mustMkdir(t, a)
	mustMkdir(t, b)
	mustWrite(t, filepath.Join(a, "dup.j"), "")
	mustWrite(t, filepath.Join(b, "dup.j"), "")

	_, err := Resolve("dup.j", "/x", []string{a, b}, "")
	if err == nil {
		t.Fatal("expected an ambiguity error for a duplicate module name")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q should mention ambiguity", err.Error())
	}
}

// A search path that lists the same directory more than once (a duplicate, or
// an alias like `-I "$SYSMODDIR"`) must not make every import ambiguous: the
// same file found twice is one file.
func TestResolveDuplicateSearchDirNotAmbiguous(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "d")
	mustMkdir(t, d)
	mustWrite(t, filepath.Join(d, "m.j"), "")

	got, err := Resolve("m.j", "/x", []string{d, d}, "")
	if err != nil {
		t.Fatalf("duplicate search dir should resolve, got %v", err)
	}
	if want := filepath.Join(d, "m.j"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A file reached via a symlinked directory and via its real path is one module
// identity (canonical() resolves symlinks), so it is not ambiguous.
func TestResolveSymlinkedDirSameIdentity(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	mustMkdir(t, real)
	mustWrite(t, filepath.Join(real, "m.j"), "")
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := Resolve("m.j", "/x", []string{real, link}, "")
	if err != nil {
		t.Fatalf("symlinked + real dir should resolve to one identity, got %v", err)
	}
	// Both resolve to the real file (symlinks resolved).
	realResolved, _ := filepath.EvalSymlinks(filepath.Join(real, "m.j"))
	if got != realResolved {
		t.Errorf("got %q, want %q", got, realResolved)
	}
}

func TestResolveModuleNotFound(t *testing.T) {
	if _, err := Resolve("missing.j", "/x", []string{t.TempDir()}, ""); err == nil {
		t.Fatal("expected a not-found error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err.Error())
	}
}

func TestResolveRejectsBadPath(t *testing.T) {
	if _, err := Resolve(`sub\x.j`, "/x", nil, ""); err == nil {
		t.Error("expected backslash rejection to propagate through Resolve")
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A leading `@` expands under the vendor root: the trailing-slash / bare form
// gets the package-named entry appended; an explicit file resolves as written.
func TestResolveVendorExpansion(t *testing.T) {
	vr := filepath.FromSlash("/proj/vendor")
	cases := []struct{ imp, want string }{
		{"@acme/widgets/", "/proj/vendor/acme/widgets/widgets.j"},
		{"@acme/widgets", "/proj/vendor/acme/widgets/widgets.j"},
		{"@acme/widgets/util.j", "/proj/vendor/acme/widgets/util.j"},
		{"@acme/widgets/sub/util.j", "/proj/vendor/acme/widgets/sub/util.j"},
	}
	for _, c := range cases {
		got, err := Resolve(c.imp, "/irrelevant", nil, vr)
		if err != nil {
			t.Errorf("%q: %v", c.imp, err)
			continue
		}
		if want := filepath.Clean(filepath.FromSlash(c.want)); got != want {
			t.Errorf("%q: got %q, want %q", c.imp, got, want)
		}
	}
}

// Vendor references reject a missing root, a stray `@`, `.`/`..` escapes, a
// subdirectory entry form, and a too-short path.
func TestResolveVendorErrors(t *testing.T) {
	if _, err := Resolve("@acme/widgets/", "/x", nil, ""); err == nil {
		t.Error("an @ import with no vendor root must error")
	}
	vr := filepath.FromSlash("/proj/vendor")
	for _, imp := range []string{
		"@acme", "@", "@/widgets/", "@acme//",
		"@acme/../../etc.j", "@acme/widgets/../x.j", "@acme/wid@gets/", "@acme/widgets/sub/",
	} {
		if _, err := Resolve(imp, "/x", nil, vr); err == nil {
			t.Errorf("%q should be a resolution error", imp)
		}
	}
}

// FindVendorRoot: explicit wins, then the upward walk to the nearest vendor/,
// then "" when none exists.
func TestFindVendorRoot(t *testing.T) {
	if got := FindVendorRoot("/explicit/dir", "/start"); got != filepath.Clean("/explicit/dir") {
		t.Errorf("explicit: got %q", got)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := FindVendorRoot("", filepath.Join(root, "a", "b")), filepath.Join(root, "vendor"); got != want {
		t.Errorf("upward walk: got %q, want %q", got, want)
	}
	if got := FindVendorRoot("", t.TempDir()); got != "" {
		t.Errorf("no vendor dir should give \"\", got %q", got)
	}
}
