// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package profile

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// pos renders a source position as file:line:col.
func (p posKey) pos() string {
	return fmt.Sprintf("%s:%d:%d", p.file, p.line, p.col)
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
// sizeStr renders a byte count compactly in binary units: 512 B, 11.0 KiB,
// 1.8 MiB.
func sizeStr(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1024*1024*1024))
	}
}

func dur(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	return d.String()
}
