// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package profile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// topLevelName is the function a row outside every function is shown under.
const topLevelName = "<top level>"

// sourceWidth caps the source text a row shows, in runes.
const sourceWidth = 60

// funcIndex maps positions to the function declared around them. It is built
// once per render from a snapshot of the declared regions.
type funcIndex map[string][]funcRegion

// funcIndex snapshots the declared regions, each file's sorted by start.
func (c *Collector) funcIndex() funcIndex {
	c.mu.Lock()
	idx := make(funcIndex, len(c.regions))
	for f, rs := range c.regions {
		cp := append([]funcRegion(nil), rs...)
		sort.SliceStable(cp, func(i, j int) bool {
			if cp[i].line != cp[j].line {
				return cp[i].line < cp[j].line
			}
			return cp[i].col < cp[j].col
		})
		idx[f] = cp
	}
	c.mu.Unlock()
	return idx
}

// region returns the region holding p: the last one starting at or before it.
func (fi funcIndex) region(p posKey) (funcRegion, bool) {
	rs := fi[p.file]
	k := sort.Search(len(rs), func(i int) bool {
		return rs[i].line > p.line || (rs[i].line == p.line && rs[i].col > p.col)
	})
	if k == 0 {
		return funcRegion{}, false
	}
	return rs[k-1], true
}

// name returns the function holding p, or topLevelName.
func (fi funcIndex) name(p posKey) string {
	if r, ok := fi.region(p); ok && r.name != "" {
		return r.name
	}
	return topLevelName
}

// displayPath shows file relative to the working directory, a system module as
// <sys>/name.j, and anything outside both as it is.
func (v View) displayPath(file string) string {
	if file == "" || !filepath.IsAbs(file) {
		return file
	}
	if v.SysDir != "" {
		if rel, ok := under(v.SysDir, file); ok {
			return "<sys>/" + rel
		}
	}
	if v.WorkDir != "" {
		if rel, ok := under(v.WorkDir, file); ok {
			return rel
		}
	}
	return file
}

// under returns file relative to dir when file lies inside dir.
func under(dir, file string) (string, bool) {
	rel, err := filepath.Rel(dir, file)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// posString renders p with its display path.
func (v View) posString(p posKey) string {
	return posKey{v.displayPath(p.file), p.line, p.col}.pos()
}

// keep reports whether a row at p in function fn passes the --focus filter.
func (v View) keep(p posKey, fn string) bool {
	if v.Focus == nil {
		return true
	}
	return v.Focus.MatchString(v.displayPath(p.file)) || v.Focus.MatchString(p.file) || v.Focus.MatchString(fn)
}

// source returns line n of file, trimmed and cut to sourceWidth runes, or "".
// Files are read once per collector; a file that cannot be read shows nothing.
func (c *Collector) source(file string, n int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sources == nil {
		c.sources = map[string][]string{}
	}
	lines, ok := c.sources[file]
	if !ok {
		if data, err := os.ReadFile(file); err == nil {
			lines = strings.Split(string(data), "\n")
		}
		c.sources[file] = lines
	}
	if n < 1 || n > len(lines) {
		return ""
	}
	s := strings.TrimSpace(strings.ReplaceAll(lines[n-1], "\t", " "))
	if r := []rune(s); len(r) > sourceWidth {
		s = string(r[:sourceWidth-3]) + "..."
	}
	return s
}

// funcSample is one function's roll-up of its statement rows.
type funcSample struct {
	file string
	name string // "" for the file's top level
	decl posKey // the function's declaration; zero for a top level
	hits int64  // calls: the hits of the body's first statement
	self time.Duration
	wait time.Duration
	cum  time.Duration // summed over the body's own statements
}

// funcRows rolls the statement rows up per function. Self and wait sum every
// statement inside the function (a spawn body included); cum sums the
// function's own statements only, since a nested statement's cum is inside
// the statement holding it; hits counts calls (the first statement runs once
// per call). A file's top-level statements roll up into one row.
func funcRows(rows []*stmtSample, fi funcIndex) []*funcSample {
	type key struct{ file, name string }
	byPos := make(map[posKey]*stmtSample, len(rows))
	for _, r := range rows {
		byPos[r.posKey] = r
	}
	out := map[key]*funcSample{}
	get := func(file string, r funcRegion, found bool) *funcSample {
		name := ""
		if found {
			name = r.name
		}
		k := key{file, name}
		f := out[k]
		if f == nil {
			f = &funcSample{file: file, name: name}
			if name != "" {
				f.decl = posKey{file, r.line, r.col}
				if len(r.body) > 0 {
					if s := byPos[r.body[0]]; s != nil {
						f.hits = s.hits
					}
					for _, b := range r.body {
						if s := byPos[b]; s != nil {
							f.cum += s.cum
						}
					}
				}
			}
			out[k] = f
		}
		return f
	}
	for _, r := range rows {
		reg, found := fi.region(r.posKey)
		f := get(r.file, reg, found)
		f.self += r.self
		f.wait += r.wait
		if f.name == "" && (!found || (reg.line == r.line && reg.col == r.col)) {
			// A top-level statement: its cum is the file's top level's own.
			f.cum += r.cum
			if f.hits == 0 {
				f.hits = 1
			}
		}
	}
	list := make([]*funcSample, 0, len(out))
	for _, f := range out {
		list = append(list, f)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].cum != list[j].cum {
			return list[i].cum > list[j].cum
		}
		if list[i].self != list[j].self {
			return list[i].self > list[j].self
		}
		if list[i].file != list[j].file {
			return list[i].file < list[j].file
		}
		return list[i].name < list[j].name
	})
	return list
}
