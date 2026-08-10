// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package reqcheck validates a Jennifer source file's requirement header against
// the running interpreter, at read time (before lex / parse). A file declares what
// it needs with typed `# pragma-jennifer-<key>: <value>` directives in its leading
// header block:
//
//	# pragma-jennifer-version: >=0.25.0
//	# pragma-jennifer-capability: net
//
// The single shared entry point, CheckRequirements, is called at every first-read
// seam (the CLI run path, the `include` preprocessor, and module loading), so a
// program, a module, and each `include`d file self-check independently - which is
// why `include` needs no cross-file merge: the effective floor is the max and the
// effective capability set the union, with each file validated on its own.
//
// Multiplicity is per key: `version` is single-valued (a duplicate is a hard error
// - two floors is a contradiction), `capability` is a set (multiple lines, or a
// comma / space list, accumulate). Version is evaluated first, so a module using a
// key only a newer interpreter knows - and correspondingly bumping its version
// floor - fails the version check on an older build before the unknown key is
// reached. Any line that opens `# pragma-jennifer-*` but does not parse, a bad
// version grammar, an unknown key, or an unknown capability is a hard error: a typo
// must never silently disable the guard.
package reqcheck

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"jennifer-lang.dev/jennifer/internal/capabilities"
	"jennifer-lang.dev/jennifer/internal/version"
)

// markerRe matches the distinctive directive prefix (post-`#`); a line carrying it
// must be a well-formed pragma or it is malformed. pragmaRe captures the key and
// value of a well-formed directive.
var (
	markerRe = regexp.MustCompile(`^pragma-jennifer-`)
	// The key is length-bounded so an absurdly long run of letters cannot become a
	// megabyte-long "key" echoed in an error; a real key is a short word.
	pragmaRe = regexp.MustCompile(`^pragma-jennifer-([a-zA-Z]{1,32})\s*:\s*(.+?)\s*$`)
	listSep  = regexp.MustCompile(`[,\s]+`)
)

// maxValueLen bounds a directive's value (a version like ">=0.25.0", a capability
// list). A real value is tiny; anything larger is a malformed directive, which
// keeps a pathologically long value from being parsed or echoed downstream.
const maxValueLen = 256

// CanonicalLine rewrites a single `# pragma-jennifer-<key>: <value>` line comment
// (the whole lexeme, leading `#` included) to its canonical single-spaced form and
// reports whether it was a pragma at all. `jennifer fmt` calls it so a pragma
// renders identically no matter what spacing it was written with. A comment that is
// not a well-formed pragma (ordinary prose, or a malformed directive) is returned
// unchanged with ok=false, so fmt leaves it verbatim and the check / lint stay the
// only things that reject it. The value's internal whitespace runs collapse to
// single spaces (a version has none; a capability list keeps `a, b`).
func CanonicalLine(lexeme string) (string, bool) {
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lexeme), "#"))
	m := pragmaRe.FindStringSubmatch(body)
	if m == nil {
		return lexeme, false
	}
	key, val := m[1], m[2]
	switch key {
	case "version":
		// A version floor never contains internal whitespace, so collapse it all: a
		// value typed as ">=  0. 5 .0" canonicalizes to ">=0.5.0".
		val = strings.Join(strings.Fields(val), "")
	case "capability":
		// A capability set is comma/space-separated; render a clean comma-space list
		// (splitting on the same separator the check uses, so names never merge).
		var names []string
		for _, c := range listSep.Split(strings.TrimSpace(val), -1) {
			if c != "" {
				names = append(names, c)
			}
		}
		val = strings.Join(names, ", ")
	default:
		// An unrecognised key: collapse whitespace runs to a single space without
		// assuming any inner structure.
		val = strings.Join(strings.Fields(val), " ")
	}
	return "# pragma-jennifer-" + key + ": " + val, true
}

// trunc bounds a string echoed in an error message so a pathologically long pragma
// line cannot produce a diagnostic larger than the source. The rune loop stops at
// the cap, so it never materialises a big []rune.
func trunc(s string) string {
	const max = 120
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + "..."
		}
		n++
	}
	return s
}

// Directive is one parsed `# pragma-jennifer-<key>: <value>` line.
type Directive struct {
	Key  string // "version", "capability", or an unrecognised key
	Val  string // the value text (a version like ">=0.25.0", or a capability name)
	Line int    // 1-based source line
}

// MalformedError is a structurally-broken directive (a line that opens
// `# pragma-jennifer-*` but does not parse). It carries the line so lint can position
// it; the read-time check wraps it with the file name.
type MalformedError struct {
	Line   int
	Detail string
}

func (e *MalformedError) Error() string { return e.Detail }

// Parse scans the leading header block of rawSource (tolerant of the shebang, the
// SPDX block, and a `/** */` docblock) and returns its `# pragma-jennifer-*`
// directives in order, stopping at the first line of actual code. A `capability`
// value that is a comma / space list is split into one Directive per name. A line
// that opens the marker but does not parse stops the scan with a *MalformedError.
// Parse does no validation against the running build - it is the shared front end
// for both the read-time check and lint.
func Parse(rawSource string) ([]Directive, *MalformedError) {
	var out []Directive
	inBlock := false
	// Iterate line by line rather than strings.Split(rawSource, "\n"): Parse returns
	// at the first line of code, so a full split would eagerly allocate a header for
	// every line of a possibly-huge file (up to O(size)) only to discard the bulk.
	// IndexByte advances one line at a time and stops the moment the header ends.
	rest := rawSource
	for lineNo := 1; len(rest) > 0; lineNo++ {
		raw := rest
		if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
			raw, rest = rest[:idx], rest[idx+1:]
		} else {
			rest = ""
		}
		line := strings.TrimSpace(raw)
		if inBlock {
			if strings.Contains(line, "*/") {
				inBlock = false
			}
			continue
		}
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "/*"):
			if !strings.Contains(line[2:], "*/") {
				inBlock = true
			}
			continue
		case !strings.HasPrefix(line, "#"):
			return out, nil // first code line: header block is over
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if !markerRe.MatchString(body) {
			continue // an ordinary comment (shebang, SPDX, description)
		}
		m := pragmaRe.FindStringSubmatch(body)
		if m == nil {
			return out, &MalformedError{Line: lineNo, Detail: fmt.Sprintf("malformed `# pragma-jennifer-*` directive: %q", trunc(line))}
		}
		key, val := m[1], strings.TrimSpace(m[2])
		if len(val) > maxValueLen {
			return out, &MalformedError{Line: lineNo, Detail: fmt.Sprintf("`%s` pragma value is too long (%d bytes; limit %d)", key, len(val), maxValueLen)}
		}
		if key == "capability" {
			for _, c := range listSep.Split(val, -1) {
				if c != "" {
					out = append(out, Directive{Key: key, Val: c, Line: lineNo})
				}
			}
			continue
		}
		out = append(out, Directive{Key: key, Val: val, Line: lineNo})
	}
	return out, nil
}

// CheckRequirements scans the leading header of rawSource for `# pragma-jennifer-*`
// directives and validates them against this interpreter. It returns a descriptive
// error (naming the file) when a version floor is unmet, a capability is
// unavailable, or a directive is malformed; nil when the header is satisfied or
// absent. path is used only for the file name in messages. Version is evaluated
// first, so an unmet floor reports before an unknown key an older interpreter would
// not recognise.
func CheckRequirements(rawSource, path string) error {
	name := filepath.Base(path)
	if name == "." || name == "" {
		name = path
	}
	dirs, mErr := Parse(rawSource)
	if mErr != nil {
		return fmt.Errorf("%s:%d: %s", name, mErr.Line, mErr.Detail)
	}

	var versions, caps, unknown []Directive
	for _, d := range dirs {
		switch d.Key {
		case "version":
			versions = append(versions, d)
		case "capability":
			caps = append(caps, d)
		default:
			unknown = append(unknown, d)
		}
	}

	if len(versions) > 1 {
		return fmt.Errorf("%s:%d: duplicate `version` pragma (a file states one version floor)", name, versions[1].Line)
	}
	if len(versions) == 1 {
		v := versions[0]
		if !strings.HasPrefix(v.Val, ">=") {
			return fmt.Errorf("%s:%d: version pragma must be `>=major.minor.patch`, got %q", name, v.Line, trunc(v.Val))
		}
		min := strings.TrimSpace(v.Val[2:])
		ok, err := version.AtLeast(min)
		if err != nil {
			return fmt.Errorf("%s:%d: malformed version pragma %q", name, v.Line, trunc(v.Val))
		}
		if !ok {
			return fmt.Errorf("%s: requires jennifer >=%s, but this build is %s", name, trunc(min), version.Version)
		}
	}
	for _, c := range caps {
		if !capabilities.Known(c.Val) {
			return fmt.Errorf("%s:%d: unknown capability %q (known: %s)", name, c.Line, trunc(c.Val), strings.Join(capabilities.KnownNames(), ", "))
		}
		if !capabilities.Has(c.Val) {
			return fmt.Errorf("%s: needs capability %q, unavailable in this build (%s)", name, c.Val, buildName())
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%s:%d: unknown pragma key %q", name, unknown[0].Line, unknown[0].Key)
	}
	return nil
}

// buildName names the running binary for a capability-unavailable message.
func buildName() string {
	if runtime.Compiler == "tinygo" {
		return "jennifer-tiny"
	}
	return "jennifer"
}
