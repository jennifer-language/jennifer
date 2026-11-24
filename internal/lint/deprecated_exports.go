// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// ModuleDeprecations reads the `@deprecated` tag of each exported function's
// docblock in the imported modules, keyed by import prefix and then function
// name; the value is the tag's text ("" for a bare tag). read
// returns a source file by the name its functions carry (an `include`d file's
// functions name that file). A file that cannot be read is skipped.
func ModuleDeprecations(mods map[string]*parser.Program, read func(string) ([]byte, error)) map[string]map[string]string {
	if len(mods) == 0 {
		return nil
	}
	lines := map[string][]string{}
	byProg := map[*parser.Program]map[string]string{}
	out := map[string]map[string]string{}
	for prefix, prog := range mods {
		if deps, ok := byProg[prog]; ok {
			if deps != nil {
				out[prefix] = deps
			}
			continue
		}
		var deps map[string]string
		for _, m := range prog.Methods {
			if !m.Exported {
				continue
			}
			file := m.Filename()
			src, ok := lines[file]
			if !ok {
				if b, err := read(file); err == nil {
					src = strings.Split(string(b), "\n")
				}
				lines[file] = src
			}
			line, _ := m.Pos()
			if text, ok := deprecatedTag(src, line); ok {
				if deps == nil {
					deps = map[string]string{}
				}
				deps[m.Name] = text
			}
		}
		byProg[prog] = deps
		if deps != nil {
			out[prefix] = deps
		}
	}
	return out
}

// deprecatedTag finds the docblock that ends on the line above line `line`
// (1-based) and returns its `@deprecated` text, continuation lines joined.
func deprecatedTag(src []string, line int) (string, bool) {
	end := line - 2
	if end < 0 || end >= len(src) || strings.TrimSpace(src[end]) != "*/" {
		return "", false
	}
	start := end - 1
	for start >= 0 && !strings.HasPrefix(strings.TrimSpace(src[start]), "/**") {
		start--
	}
	if start < 0 {
		return "", false
	}
	var parts []string
	found := false
	for _, l := range src[start+1 : end] {
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "*"))
		switch {
		case strings.HasPrefix(body, "@deprecated"):
			found = true
			if rest := strings.TrimSpace(strings.TrimPrefix(body, "@deprecated")); rest != "" {
				parts = append(parts, rest)
			}
		case strings.HasPrefix(body, "@"):
			if found {
				return joinDeprecated(parts), true
			}
		case found && body != "":
			parts = append(parts, body)
		}
	}
	if !found {
		return "", false
	}
	return joinDeprecated(parts), true
}

func joinDeprecated(parts []string) string {
	return strings.Join(parts, " ")
}

// reportDeprecatedExport reports (L301) a call to a module function whose
// docblock carries `@deprecated`, with the tag's text. It returns whether e is
// a call into an imported module, which the library checks then skip.
func reportDeprecatedExport(c *checkCtx, e parser.Expr) bool {
	qc, ok := e.(*parser.QualifiedCallExpr)
	if !ok || c.cfg.Modules[qc.Prefix] == nil {
		return false
	}
	if text, ok := c.cfg.DeprecatedExports[qc.Prefix][qc.Callee]; ok {
		msg := fmt.Sprintf("`%s.%s` is deprecated", qc.Prefix, qc.Callee)
		if text != "" {
			msg += ": " + text
		}
		c.report("L301", qc, msg)
	}
	return true
}
