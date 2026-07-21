// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"fmt"
	"strings"
)

// A check selection is expressed with the same `IDS` / `!IDS` grammar in
// three places: the `--checks` CLI flag, the optional `.jennifer-lint`
// project file, and (per statement) `# lint-disable:` comments. A leading
// `!` on an entry means "exclude". One flag, one direction: a list is
// either all-includes (run only these) or all-excludes (run everything
// except these); mixing the two is an error. Unknown IDs are always an
// error - silently ignoring a typo would defeat the auditability the ID
// scheme buys.

// ResolveSelection computes the set of enabled check IDs. Precedence: an
// explicit `--checks` spec wins (per-run tuning); otherwise the
// `.jennifer-lint` dotfile if present (per-project defaults); otherwise
// every check is enabled.
func ResolveSelection(checksFlag string, hasChecksFlag bool, dotfile string, hasDotfile bool) (map[string]bool, error) {
	switch {
	case hasChecksFlag:
		entries, err := splitCommaSpec(checksFlag)
		if err != nil {
			return nil, err
		}
		return applyIDSpec(entries, "--checks")
	case hasDotfile:
		entries, err := splitDotfileSpec(dotfile)
		if err != nil {
			return nil, err
		}
		return applyIDSpec(entries, ".jennifer-lint")
	default:
		return enabledSet(selectableIDs()), nil
	}
}

// enabledSet returns a fresh set with every id enabled.
func enabledSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// idEntry is one parsed selection entry: an ID and whether it was negated.
type idEntry struct {
	id  string
	neg bool
}

// splitCommaSpec parses the comma-separated `--checks` value.
func splitCommaSpec(spec string) ([]idEntry, error) {
	var out []idEntry
	for _, raw := range strings.Split(spec, ",") {
		e, ok, err := parseEntry(raw)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// splitDotfileSpec parses the `.jennifer-lint` file: one or more entries
// per line (comma- or whitespace-separated), `#` starts a comment,
// blank lines ignored.
func splitDotfileSpec(content string) ([]idEntry, error) {
	var out []idEntry
	for _, line := range strings.Split(content, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, raw := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		}) {
			e, ok, err := parseEntry(raw)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// parseEntry parses a single `IDS` or `!IDS` token. Returns ok=false for an
// empty token (allowing trailing commas). Rejects unknown IDs.
func parseEntry(raw string) (idEntry, bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return idEntry{}, false, nil
	}
	neg := false
	if strings.HasPrefix(s, "!") {
		neg = true
		s = strings.TrimSpace(s[1:])
	}
	if !isKnownID(s) {
		return idEntry{}, false, fmt.Errorf("unknown check ID %q", s)
	}
	if !isSelectable(s) {
		return idEntry{}, false, fmt.Errorf("check ID %q is an always-on source-error class and cannot be selected", s)
	}
	return idEntry{id: s, neg: neg}, true, nil
}

// applyIDSpec turns a parsed entry list into an enabled-set. All-includes
// means "run only these"; all-excludes means "run everything except these".
// Mixing the two directions is an error.
func applyIDSpec(entries []idEntry, source string) (map[string]bool, error) {
	var includes, excludes []string
	for _, e := range entries {
		if e.neg {
			excludes = append(excludes, e.id)
		} else {
			includes = append(includes, e.id)
		}
	}
	if len(includes) > 0 && len(excludes) > 0 {
		return nil, fmt.Errorf("%s mixes include and exclude entries; use one direction (all IDs, or all !IDs)", source)
	}
	if len(excludes) > 0 {
		set := enabledSet(selectableIDs())
		for _, id := range excludes {
			delete(set, id)
		}
		return set, nil
	}
	if len(includes) > 0 {
		set := make(map[string]bool, len(includes))
		for _, id := range includes {
			set[id] = true
		}
		return set, nil
	}
	return enabledSet(selectableIDs()), nil
}
