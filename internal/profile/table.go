// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package profile

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Table writes the human-readable flat profile for the collector's mode, as
// the View set with SetView asks (rows per statement or per function, --top,
// --focus, path display).
func (c *Collector) Table(w io.Writer) {
	if c.mode == ModeAllocs {
		c.tableAllocs(w)
		return
	}
	c.tableStatements(w)
}

// viewSnapshot returns the current View under the lock.
func (c *Collector) viewSnapshot() View {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.view
}

// runLine describes the run for a report header: its wall time so far and the
// number of tasks profiled (the main one plus every spawned task).
func (c *Collector) runLine() string {
	c.mu.Lock()
	start, end := c.wallStart, c.wallEnd
	if start.IsZero() && c.started {
		start = c.runStart
	}
	c.mu.Unlock()
	spawned := c.tasks.Load()
	tasks := "1 task (main)"
	if spawned > 0 {
		tasks = fmt.Sprintf("%d tasks (main + %d spawned)", spawned+1, spawned)
	}
	if start.IsZero() {
		return tasks
	}
	if end.IsZero() {
		end = time.Now()
	}
	return fmt.Sprintf("%s wall, %s", dur(end.Sub(start).Round(time.Microsecond)), tasks)
}

// writeGrid writes rows under header as columns two spaces apart, indented by
// two; right marks the columns aligned right (numbers). The last column is not
// padded, so a long source text does not widen the table.
func writeGrid(w io.Writer, header []string, rows [][]string, right []bool) {
	all := append([][]string{header}, rows...)
	width := make([]int, len(header))
	for _, r := range all {
		for k, cell := range r {
			if n := utf8.RuneCountInString(cell); n > width[k] {
				width[k] = n
			}
		}
	}
	for _, r := range all {
		var b strings.Builder
		b.WriteString("  ")
		for k, cell := range r {
			pad := strings.Repeat(" ", width[k]-utf8.RuneCountInString(cell))
			last := k == len(r)-1
			switch {
			case right[k]:
				b.WriteString(pad + cell)
			case last:
				b.WriteString(cell)
			default:
				b.WriteString(cell + pad)
			}
			if !last {
				b.WriteString("  ")
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

// shownNote reports a table cut by --top or --focus.
func shownNote(w io.Writer, shown, total int, what string) {
	if shown < total {
		fmt.Fprintf(w, "  (showing %d of %d %s)\n", shown, total, what)
	}
}

func (c *Collector) tableStatements(w io.Writer) {
	rows := c.stmtSorted()
	v := c.viewSnapshot()
	fi := c.funcIndex()
	var totalSelf, totalWait, totalCum time.Duration
	var totalHits int64
	for _, r := range rows {
		totalSelf += r.self
		totalWait += r.wait
		totalHits += r.hits
	}
	if len(rows) > 0 {
		totalCum = rows[0].cum // the hottest statement's cumulative is ~ the run
	}

	fmt.Fprintf(w, "Jennifer statement profile: %s\n", c.runLine())
	fmt.Fprintln(w, "SELF excludes nested statements and waits; WAIT is time blocked in a builtin that")
	fmt.Fprintln(w, "waits (time.sleep, channel.recv, task.wait, httpd.accept, net reads); SELF, WAIT")
	fmt.Fprintln(w, "and CUM are summed over tasks, so with spawned tasks they can exceed the wall time.")
	fmt.Fprintln(w)
	if v.ByFunction {
		c.tableFunctions(w, rows, fi, v)
	} else {
		var grid [][]string
		for _, r := range rows {
			fn := fi.name(r.posKey)
			if !v.keep(r.posKey, fn) {
				continue
			}
			if v.Top > 0 && len(grid) >= v.Top {
				break
			}
			grid = append(grid, []string{
				strconv.FormatInt(r.hits, 10), dur(r.self), dur(r.wait), dur(r.cum),
				fn, v.posString(r.posKey), c.source(r.file, r.line),
			})
		}
		writeGrid(w, []string{"HITS", "SELF", "WAIT", "CUM", "FUNCTION", "POSITION", "SOURCE"}, grid,
			[]bool{true, true, true, true, false, false, false})
		shownNote(w, len(grid), len(rows), "positions")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d positions, %d statement executions, %s total self time, %s total wait (top-of-tree cum %s)\n",
		len(rows), totalHits, dur(totalSelf), dur(totalWait), dur(totalCum))
	c.tableCallDepth(w, fi, v)
}

// tableFunctions renders the statement rows rolled up per function (--by=function).
func (c *Collector) tableFunctions(w io.Writer, rows []*stmtSample, fi funcIndex, v View) {
	funcs := funcRows(rows, fi)
	var grid [][]string
	for _, f := range funcs {
		name, where := f.name, v.displayPath(f.file)
		if name == "" {
			name = topLevelName
		} else {
			where = v.posString(f.decl)
		}
		if !v.keep(posKey{f.file, f.decl.line, f.decl.col}, name) {
			continue
		}
		if v.Top > 0 && len(grid) >= v.Top {
			break
		}
		grid = append(grid, []string{
			strconv.FormatInt(f.hits, 10), dur(f.self), dur(f.wait), dur(f.cum), name, where,
		})
	}
	writeGrid(w, []string{"CALLS", "SELF", "WAIT", "CUM", "FUNCTION", "POSITION"}, grid,
		[]bool{true, true, true, true, false, false})
	shownNote(w, len(grid), len(funcs), "functions")
}

// depthSitesShown caps the per-call-site depth rows so a program with many call
// sites doesn't drown the summary; the deepest are what matter for spotting
// runaway recursion.
const depthSitesShown = 10

// tableCallDepth renders the max nested method-call depth reached in the run -
// overall and per deepest call site. Silent when no method call was recorded.
func (c *Collector) tableCallDepth(w io.Writer, fi funcIndex, v View) {
	rows, overall := c.depthsSorted()
	if overall == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Max call depth (deepest chain of nested function calls): %d\n", overall)
	var grid [][]string
	for idx, d := range rows {
		if idx >= depthSitesShown {
			break
		}
		grid = append(grid, []string{strconv.Itoa(d.depth), fi.name(d.posKey), v.posString(d.posKey)})
	}
	writeGrid(w, []string{"DEPTH", "FUNCTION", "CALL SITE"}, grid, []bool{true, false, false})
	if len(rows) > depthSitesShown {
		fmt.Fprintf(w, "... and %d more call sites\n", len(rows)-depthSitesShown)
	}
}

func (c *Collector) tableAllocs(w io.Writer) {
	v := c.viewSnapshot()
	fi := c.funcIndex()
	fmt.Fprintf(w, "Jennifer allocation profile (value-semantics copies): %s\n", c.runLine())
	fmt.Fprintln(w)

	eagers := c.eventsSorted(c.eager)
	fmt.Fprintln(w, "Eager copies - a store that deep-copied a compound value (def, assignment, parameter,")
	fmt.Fprintln(w, "literal element, field / index / append write, slice, throw, catch, defer argument):")
	if len(eagers) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		var grid [][]string
		var total, totalBytes int64
		for _, e := range eagers {
			total += e.count
			totalBytes += e.bytes
			fn := fi.name(e.posKey)
			if !v.keep(e.posKey, fn) || (v.Top > 0 && len(grid) >= v.Top) {
				continue
			}
			grid = append(grid, []string{strconv.FormatInt(e.count, 10), sizeStr(e.bytes), e.typ, fn,
				v.posString(e.posKey), c.source(e.file, e.line)})
		}
		writeGrid(w, []string{"COUNT", "BYTES", "TYPE", "FUNCTION", "POSITION", "SOURCE"}, grid,
			[]bool{true, true, false, false, false, false})
		shownNote(w, len(grid), len(eagers), "sites")
		fmt.Fprintf(w, "  %d copies across %d sites, ~%s copied (sorted by bytes; sizes are estimates)\n",
			total, len(eagers), sizeStr(totalBytes))
	}

	fmt.Fprintln(w)
	spawns := c.eventsSorted(c.spawn)
	fmt.Fprintln(w, "Spawn-frame deep copies - a scope snapshot captured at spawn launch:")
	if len(spawns) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		var grid [][]string
		for _, e := range spawns {
			fn := fi.name(e.posKey)
			if !v.keep(e.posKey, fn) || (v.Top > 0 && len(grid) >= v.Top) {
				continue
			}
			grid = append(grid, []string{strconv.FormatInt(e.count, 10), dur(e.total), fn,
				v.posString(e.posKey), c.source(e.file, e.line)})
		}
		writeGrid(w, []string{"COUNT", "TOTAL", "FUNCTION", "POSITION", "SOURCE"}, grid,
			[]bool{true, true, false, false, false})
		shownNote(w, len(grid), len(spawns), "sites")
	}
}
