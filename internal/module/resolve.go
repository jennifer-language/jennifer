// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package module implements the Jennifer module system's resolution layer:
// turning an `import "..."` path string into a concrete file on disk, and
// resolving the system module directory from its CLI / env / compile-time
// layers. It is the resolution half of the module system; the `import`
// statement, loader, and per-module scope that consume it are separate.
//
// Import paths are OS-independent logical strings, always `/`-separated
// (like a URL or the `.j` string literal). A `\` in an import path is a
// syntax error, never a Windows separator. The shape is classified on the
// logical string; only then is the file located with `path/filepath`, so
// host separators and drive letters are the stdlib's concern at resolve
// time, not the grammar's.
package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind classifies an import path by its leading token.
type Kind int

const (
	// Local: "./f.j" or "../f.j" - relative to the importing file's
	// directory; the search path is not consulted.
	Local Kind = iota
	// Absolute: an absolute filesystem path ("/abs/f.j"); the search path
	// is not consulted.
	Absolute
	// Module: a bare path ("f.j", "sub/f.j") - resolved by walking the
	// module search path; the importing file's directory is never used.
	Module
)

// Classify determines the shape of a logical import path, or returns an
// error for a malformed one (empty, backslash-bearing, or not ending in
// `.j`). It does not touch the filesystem.
func Classify(importPath string) (Kind, error) {
	if importPath == "" {
		return 0, fmt.Errorf("empty import path")
	}
	if strings.ContainsRune(importPath, '\\') {
		return 0, fmt.Errorf("import path %q must use '/' separators, not '\\' (paths are OS-independent)", importPath)
	}
	if !strings.HasSuffix(importPath, ".j") {
		return 0, fmt.Errorf("import path %q must end in '.j'", importPath)
	}
	switch {
	case strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../"):
		return Local, nil
	case filepath.IsAbs(filepath.FromSlash(importPath)):
		return Absolute, nil
	default:
		return Module, nil
	}
}

// Resolve turns a logical import path into a canonical absolute file path.
//
//   - importingDir is the directory of the file that contains the import,
//     used only for Local imports.
//   - searchDirs is the ordered module search path (system module dir
//     first, then each `-I DIR`), used only for Module imports.
//
// A Module import that resolves in more than one search dir is a hard
// error (ambiguous - a `-I` dir may add names but never silently shadow a
// system module or another `-I`); one that resolves nowhere is a
// not-found error naming the search path. The returned path is cleaned and
// absolute (canonical), the key the loader uses for run-once and cycle
// detection.
func Resolve(importPath, importingDir string, searchDirs []string, vendorRoot string) (string, error) {
	// A leading `@` is a vendored-deck reference, expanded under the vendor root.
	// Handled before Classify because the deck-entry form (`@scope/package/`)
	// intentionally does not end in `.j`.
	if strings.HasPrefix(importPath, "@") {
		return resolveVendor(importPath, vendorRoot)
	}
	kind, err := Classify(importPath)
	if err != nil {
		return "", err
	}
	native := filepath.FromSlash(importPath)
	switch kind {
	case Local:
		return canonical(filepath.Join(importingDir, native))
	case Absolute:
		return canonical(native)
	default: // Module
		return resolveModule(importPath, native, searchDirs)
	}
}

// resolveVendor expands an `@scope/package[/rest]` deck reference to a concrete
// file under the vendor root. The `@` is the vendor-root shortcut, and a
// reference that does not end in `.j` gets the package-named entry appended
// (`@a/b` and `@a/b/` -> vendorRoot/a/b/b.j); an explicit `@a/b/util.j` (or a
// subdir file) resolves as written. Rejects a missing vendor root, a stray `@`,
// a `.`/`..` segment, and anything resolving outside the deck directory.
func resolveVendor(importPath, vendorRoot string) (string, error) {
	if vendorRoot == "" {
		return "", fmt.Errorf("import %q needs a vendor directory, but none was found (pass --vendor DIR, set JENNIFER_VENDOR, or add a `vendor/` dir above the program)", importPath)
	}
	if strings.ContainsRune(importPath, '\\') {
		return "", fmt.Errorf("import path %q must use '/' separators, not '\\'", importPath)
	}
	rest := importPath[1:] // drop the leading '@'
	if strings.ContainsRune(rest, '@') {
		return "", fmt.Errorf("import path %q: '@' is only valid as the first character", importPath)
	}
	explicitFile := strings.HasSuffix(rest, ".j")
	rest = strings.TrimSuffix(rest, "/") // a trailing slash is the entry-form spelling
	segs := strings.Split(rest, "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return "", fmt.Errorf("import path %q must be @scope/package[/file.j]", importPath)
	}
	for _, s := range segs {
		if s == "." || s == ".." || s == "" {
			return "", fmt.Errorf("import path %q must not contain '.' or '..' segments", importPath)
		}
	}
	scope, pkg := segs[0], segs[1]
	deckRoot, err := canonical(filepath.Join(vendorRoot, scope, pkg))
	if err != nil {
		return "", err
	}
	var target string
	if explicitFile {
		target = filepath.Join(vendorRoot, filepath.FromSlash(rest))
	} else {
		if len(segs) != 2 {
			return "", fmt.Errorf("import path %q: only @scope/package expands to an entry file; a subdirectory needs an explicit `.j` file", importPath)
		}
		target = filepath.Join(deckRoot, pkg+".j") // package-named entry
	}
	c, err := canonical(target)
	if err != nil {
		return "", err
	}
	if c != deckRoot && !strings.HasPrefix(c, deckRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("import path %q resolves outside its package directory", importPath)
	}
	return c, nil
}

// FindVendorRoot resolves the vendor directory for `@scope/package` imports,
// with the same override layering as the system module dir: an explicit dir
// wins, else the JENNIFER_VENDOR environment variable, else an upward walk from
// startDir to the nearest `vendor/` directory. Returns "" when none is found
// (an `@` import then errors with guidance).
func FindVendorRoot(explicit, startDir string) string {
	if explicit != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(explicit)
	}
	if env := os.Getenv(VendorEnv); env != "" {
		if abs, err := filepath.Abs(env); err == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(env)
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		cand := filepath.Join(dir, "vendor")
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached the filesystem root without a vendor/
		}
		dir = parent
	}
}

// resolveModule walks the search path, requiring exactly one match.
func resolveModule(importPath, native string, searchDirs []string) (string, error) {
	// Collect matches by canonical path, deduping as we go: duplicate,
	// aliased, or symlinked search dirs that resolve to the same file are not
	// an ambiguity, only two genuinely distinct files are.
	seen := map[string]bool{}
	var matches []string
	for _, dir := range searchDirs {
		cand := filepath.Join(dir, native)
		fi, err := os.Stat(cand)
		if err != nil {
			// A genuine not-exist just means "not here" - keep searching. Any
			// other stat failure (EACCES on the candidate) is surfaced rather
			// than swallowed as a misleading "module not found".
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("module %q: cannot access %q: %v", importPath, cand, err)
			}
			continue
		}
		if fi.IsDir() {
			continue
		}
		c, err := canonical(cand)
		if err != nil {
			return "", err
		}
		if !seen[c] {
			seen[c] = true
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		if len(searchDirs) == 0 {
			return "", fmt.Errorf("module %q not found: the module search path is empty", importPath)
		}
		return "", fmt.Errorf("module %q not found on the search path %v", importPath, searchDirs)
	default:
		return "", fmt.Errorf("module %q is ambiguous: found in multiple search dirs: %s", importPath, strings.Join(matches, ", "))
	}
}

// canonical returns the cleaned absolute form of a path. It does not
// require the path to exist (a Local / Absolute import to a missing file
// is the loader's not-found error, positioned at the import).
func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %v", path, err)
	}
	cleaned := filepath.Clean(abs)
	// Resolve symlinks so one file reached via a symlinked directory and via
	// its real path collapse to one identity (the run-once / cycle /
	// struct-identity key). EvalSymlinks needs the path to exist; a Local /
	// Absolute import to a missing file falls back to the cleaned absolute
	// path, and the loader raises the positioned not-found error.
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved, nil
	}
	return cleaned, nil
}
