// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package profile

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// pos renders a source position as file:line:col.
func (p posKey) pos() string {
	return fmt.Sprintf("%s:%d:%d", p.file, p.line, p.col)
}

// Table writes the human-readable flat profile for the collector's mode.
func (c *Collector) Table(w io.Writer) {
	if c.mode == ModeAllocs {
		c.tableAllocs(w)
		return
	}
	c.tableStatements(w)
}

func (c *Collector) tableStatements(w io.Writer) {
	rows := c.stmtSorted()
	var totalSelf, totalCum time.Duration
	var totalHits int64
	for _, r := range rows {
		totalSelf += r.self
		totalHits += r.hits
	}
	if len(rows) > 0 {
		totalCum = rows[0].cum // the hottest statement's cumulative is ~ the run
	}

	fmt.Fprintln(w, "Jennifer statement profile (wall-clock, self = excluding nested statements)")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "HITS\tSELF\tCUM\t  POSITION")
	for _, r := range rows {
		fmt.Fprintf(tw, "%d\t%s\t%s\t  %s\n", r.hits, dur(r.self), dur(r.cum), r.pos())
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d positions, %d statement executions, %s total self time (top-of-tree cum %s)\n",
		len(rows), totalHits, dur(totalSelf), dur(totalCum))
	c.tableCallDepth(w)
}

// depthSitesShown caps the per-call-site depth rows so a program with many call
// sites doesn't drown the summary; the deepest are what matter for spotting
// runaway recursion.
const depthSitesShown = 10

// tableCallDepth renders the max nested method-call depth reached in the run -
// overall and per deepest call site. Silent when no method call was recorded.
func (c *Collector) tableCallDepth(w io.Writer) {
	rows, overall := c.depthsSorted()
	if overall == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Max call depth (deepest chain of nested method calls): %d\n", overall)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "DEPTH\t  CALL SITE")
	for idx, d := range rows {
		if idx >= depthSitesShown {
			break
		}
		fmt.Fprintf(tw, "%d\t  %s\n", d.depth, d.pos())
	}
	tw.Flush()
	if len(rows) > depthSitesShown {
		fmt.Fprintf(w, "... and %d more call sites\n", len(rows)-depthSitesShown)
	}
}

func (c *Collector) tableAllocs(w io.Writer) {
	fmt.Fprintln(w, "Jennifer allocation profile (value-semantics copies)")
	fmt.Fprintln(w)

	eagers := c.eventsSorted(c.eager)
	fmt.Fprintln(w, "Eager copies - a def / assignment / parameter binding that deep-copied a compound value:")
	if len(eagers) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "COUNT\t  POSITION")
		var total int64
		for _, e := range eagers {
			total += e.count
			fmt.Fprintf(tw, "%d\t  %s\n", e.count, e.pos())
		}
		tw.Flush()
		fmt.Fprintf(w, "  %d copies across %d sites\n", total, len(eagers))
	}

	fmt.Fprintln(w)
	spawns := c.eventsSorted(c.spawn)
	fmt.Fprintln(w, "Spawn-frame deep copies - a scope snapshot captured at spawn launch:")
	if len(spawns) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "COUNT\tTOTAL\t  POSITION")
		for _, e := range spawns {
			fmt.Fprintf(tw, "%d\t%s\t  %s\n", e.count, dur(e.total), e.pos())
		}
		tw.Flush()
	}
}

// Trace writes the Chrome Trace Event Format stream (consumed by
// chrome://tracing and https://www.speedscope.app/). Each recorded method
// call becomes a complete ("X") event; the post-order start/end spans nest
// into a flame chart. Only valid for the statement profile - allocation
// events have no timeline to hang off, so the CLI rejects --allocs --format=trace.
func (c *Collector) Trace(w io.Writer) error {
	type event struct {
		Name string                 `json:"name"`
		Ph   string                 `json:"ph"`
		Ts   float64                `json:"ts"`  // microseconds since start
		Dur  float64                `json:"dur"` // microseconds
		Pid  int                    `json:"pid"`
		Tid  int                    `json:"tid"`
		Args map[string]interface{} `json:"args"`
	}
	calls := c.callsSnapshot()
	events := make([]event, 0, len(calls))
	// Map raw goroutine ids to small, stable track numbers so concurrent spawn
	// spans render on separate tracks instead of overlapping on tid 1. The main
	// goroutine's id maps to track 1 (it records first).
	tidOf := map[int]int{}
	for _, ce := range calls {
		tid, ok := tidOf[ce.gid]
		if !ok {
			tid = len(tidOf) + 1
			tidOf[ce.gid] = tid
		}
		events = append(events, event{
			Name: ce.name,
			Ph:   "X",
			Ts:   float64(ce.start.Nanoseconds()) / 1000.0,
			Dur:  float64((ce.end - ce.start).Nanoseconds()) / 1000.0,
			Pid:  1,
			Tid:  tid,
			Args: map[string]interface{}{"pos": fmt.Sprintf("%s:%d:%d", ce.file, ce.line, ce.col)},
		})
	}
	doc := struct {
		TraceEvents     []event `json:"traceEvents"`
		DisplayTimeUnit string  `json:"displayTimeUnit"`
	}{TraceEvents: events, DisplayTimeUnit: "ms"}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// dur formats a duration compactly; the zero value prints as "0s".
func dur(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	return d.String()
}
