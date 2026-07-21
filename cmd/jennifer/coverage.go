// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// statementPositions walks a parsed program and returns every executable
// statement position - the coverage denominator - plus the set of files those
// statements live in. It mirrors the interpreter's statement traversal
// (execStmt records posFor(s) for each statement in a block / body / loop /
// for-header), so the set lines up with the profiler's recorded hit positions.
// The file set lets coverage scope to the tested program and exclude imported
// modules that merely happened to run.
func statementPositions(prog *parser.Program) (map[profile.Position]struct{}, map[string]bool) {
	set := map[profile.Position]struct{}{}
	files := map[string]bool{}
	add := func(n parser.Node) {
		if n == nil {
			return
		}
		line, col := n.Pos()
		f := n.Filename()
		set[profile.Position{File: f, Line: line, Col: col}] = struct{}{}
		files[f] = true
	}
	var visitStmt func(s parser.Stmt)
	var visitBlock func(b *parser.Block)
	visitBlock = func(b *parser.Block) {
		if b == nil {
			return
		}
		for _, s := range b.Stmts {
			visitStmt(s)
		}
	}
	visitStmt = func(s parser.Stmt) {
		if s == nil {
			return
		}
		add(s) // execStmt records the position of every statement it runs
		switch st := s.(type) {
		case *parser.IfStmt:
			visitBlock(st.Then)
			for _, b := range st.ElseIfBodies {
				visitBlock(b)
			}
			visitBlock(st.Else)
		case *parser.WhileStmt:
			visitBlock(st.Body)
		case *parser.ForStmt:
			visitStmt(st.Init) // execFor runs init / step through execStmt too
			visitStmt(st.Step)
			visitBlock(st.Body)
		case *parser.ForEachStmt:
			visitBlock(st.Body)
		case *parser.RepeatStmt:
			visitBlock(st.Body)
		case *parser.TryStmt:
			visitBlock(st.Body)
			visitBlock(st.CatchBody)
		case *parser.Block:
			visitBlock(st)
		}
	}
	for _, s := range prog.TopLevel {
		visitStmt(s)
	}
	for _, m := range prog.Methods {
		if m != nil && m.Body != nil {
			visitBlock(m.Body)
		}
	}
	return set, files
}

// fileCoverage is one file's tally.
type fileCoverage struct {
	File      string             `json:"file"`
	Total     int                `json:"total"`
	Covered   int                `json:"covered"`
	Percent   float64            `json:"percent"`
	Uncovered []profile.Position `json:"uncovered"`
}

// renderCoverage computes per-file and total statement coverage from the
// program's executable positions and the profiler's hit positions, and renders
// it as `text` (default) or `json`.
func renderCoverage(prog *parser.Program, hits map[profile.Position]int64, format string) (string, error) {
	total, files := statementPositions(prog)
	// Union in hit positions in the tested files (e.g. spawn-body statements
	// the static walk does not enter): they ran, so they are covered members of
	// the denominator.
	for p := range hits {
		if files[p.File] {
			total[p] = struct{}{}
		}
	}

	byFile := map[string]*fileCoverage{}
	for p := range total {
		fc := byFile[p.File]
		if fc == nil {
			fc = &fileCoverage{File: p.File}
			byFile[p.File] = fc
		}
		fc.Total++
		if hits[p] > 0 {
			fc.Covered++
		} else {
			fc.Uncovered = append(fc.Uncovered, p)
		}
	}

	sortedFiles := make([]*fileCoverage, 0, len(byFile))
	var grandTotal, grandCovered int
	for _, fc := range byFile {
		fc.Percent = pct(fc.Covered, fc.Total)
		sort.Slice(fc.Uncovered, func(i, j int) bool {
			if fc.Uncovered[i].Line != fc.Uncovered[j].Line {
				return fc.Uncovered[i].Line < fc.Uncovered[j].Line
			}
			return fc.Uncovered[i].Col < fc.Uncovered[j].Col
		})
		sortedFiles = append(sortedFiles, fc)
		grandTotal += fc.Total
		grandCovered += fc.Covered
	}
	sort.Slice(sortedFiles, func(i, j int) bool { return sortedFiles[i].File < sortedFiles[j].File })

	if format == "json" {
		return coverageJSON(sortedFiles, grandCovered, grandTotal)
	}
	return coverageText(sortedFiles, grandCovered, grandTotal), nil
}

func pct(covered, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(covered) * 100 / float64(total)
}

func coverageText(files []*fileCoverage, covered, total int) string {
	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Coverage (statements):")
	for _, fc := range files {
		fmt.Fprintf(&b, "  %s  %d/%d  (%.1f%%)\n", fc.File, fc.Covered, fc.Total, fc.Percent)
		if len(fc.Uncovered) > 0 {
			parts := make([]string, len(fc.Uncovered))
			for i, p := range fc.Uncovered {
				parts[i] = fmt.Sprintf("%d:%d", p.Line, p.Col)
			}
			fmt.Fprintf(&b, "    uncovered: %s\n", strings.Join(parts, ", "))
		}
	}
	fmt.Fprintf(&b, "  total: %d/%d (%.1f%%)\n", covered, total, pct(covered, total))
	return b.String()
}

func coverageJSON(files []*fileCoverage, covered, total int) (string, error) {
	if files == nil {
		files = []*fileCoverage{}
	}
	for _, fc := range files {
		if fc.Uncovered == nil {
			fc.Uncovered = []profile.Position{}
		}
	}
	out := struct {
		Files   []*fileCoverage `json:"files"`
		Covered int             `json:"covered"`
		Total   int             `json:"total"`
		Percent float64         `json:"percent"`
	}{Files: files, Covered: covered, Total: total, Percent: pct(covered, total)}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}
