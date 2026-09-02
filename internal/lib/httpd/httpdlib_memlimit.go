// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Machine-memory detection for httpd.setMaxBufferBudgetFromRAM - an OPT-IN
// helper that sizes the listenWith memory guard to the host. It is deliberately
// cgroup-aware and Linux-first: a container's cgroup limit is the number that
// matters (a `/proc/meminfo` read alone would report the host's RAM, not the
// container's, and auto-sizing off that is the classic over-commit that gets a
// process OOM-killed). The detected limit is min(host MemTotal, tightest cgroup
// memory limit found walking the process's cgroup to the root), so a limit set
// on a parent slice is honoured. On a non-Linux host (no cgroup, no /proc) the
// caller gets a friendly error and is pointed at the explicit setter.

package httpdlib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupV1UnlimitedFloor is the threshold at or above which a cgroup v1
// memory.limit_in_bytes is treated as "no limit". v1 encodes unlimited as a
// page-rounded near-int64-max sentinel (e.g. 0x7FFFFFFFFFFFF000), so any value
// this large is not a real cap.
const cgroupV1UnlimitedFloor = int64(1) << 62

// cgroupWalkCap bounds the leaf-to-root cgroup ancestor walk (defensive; real
// hierarchies are a handful deep).
const cgroupWalkCap = 64

// detectMemoryLimitBytes returns the effective machine memory limit in bytes:
// the minimum of the host's MemTotal and any finite cgroup memory limit. It
// errors only when neither signal is available (not a Linux host).
func detectMemoryLimitBytes() (int64, error) {
	host, hostOK := readMemTotalBytes()
	cg, cgOK := readCgroupMemoryLimitBytes()
	switch {
	case hostOK && cgOK:
		if cg < host {
			return cg, nil
		}
		return host, nil
	case cgOK:
		return cg, nil
	case hostOK:
		return host, nil
	default:
		return 0, fmt.Errorf("could not determine the machine memory limit (no cgroup or /proc/meminfo - not a Linux host?)")
	}
}

// readMemTotalBytes parses /proc/meminfo's MemTotal (kB) into bytes.
func readMemTotalBytes() (int64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	return parseMemTotalBytes(string(data))
}

// parseMemTotalBytes pulls "MemTotal:   N kB" out of /proc/meminfo content and
// returns N * 1024. Pure, for testing.
func parseMemTotalBytes(meminfo string) (int64, bool) {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		// "MemTotal:" "16384000" "kB"
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		// Reject a value large enough that kb*1024 would overflow int64 (the
		// bound is (2^63-1)/1024 = 2^53-1); no real machine approaches it, and
		// /proc/meminfo is trusted, but the guard keeps the multiply total.
		if err != nil || kb <= 0 || kb >= (1<<53) {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

// readCgroupMemoryLimitBytes returns the tightest finite cgroup memory limit for
// this process (v2 preferred, then v1), or (0, false) if the process is
// unconfined or cgroups are unavailable.
func readCgroupMemoryLimitBytes() (int64, bool) {
	if v, ok := readCgroupV2Limit(); ok {
		return v, true
	}
	return readCgroupV1Limit()
}

// readCgroupV2Limit walks the process's unified-hierarchy cgroup from its leaf
// to the root, returning the smallest finite memory.max.
func readCgroupV2Limit() (int64, bool) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0, false
	}
	rel, ok := cgroupV2Path(string(data))
	if !ok {
		return 0, false
	}
	root := "/sys/fs/cgroup"
	dir := filepath.Join(root, rel)
	best, found := int64(0), false
	for i := 0; i < cgroupWalkCap; i++ {
		if b, err := os.ReadFile(filepath.Join(dir, "memory.max")); err == nil {
			if v, isNum := parseCgroupV2MemoryMax(string(b)); isNum {
				if !found || v < best {
					best, found = v, true
				}
			}
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir || len(parent) < len(root) {
			break
		}
		dir = parent
	}
	return best, found
}

// readCgroupV1Limit walks the process's memory-controller cgroup from its leaf
// to the root, returning the smallest finite memory.limit_in_bytes.
func readCgroupV1Limit() (int64, bool) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0, false
	}
	rel, ok := cgroupV1MemoryPath(string(data))
	if !ok {
		return 0, false
	}
	root := "/sys/fs/cgroup/memory"
	dir := filepath.Join(root, rel)
	best, found := int64(0), false
	for i := 0; i < cgroupWalkCap; i++ {
		if b, err := os.ReadFile(filepath.Join(dir, "memory.limit_in_bytes")); err == nil {
			if v, isNum := parseCgroupV1Limit(string(b)); isNum {
				if !found || v < best {
					best, found = v, true
				}
			}
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir || len(parent) < len(root) {
			break
		}
		dir = parent
	}
	return best, found
}

// cgroupV2Path extracts the unified-hierarchy path from /proc/self/cgroup, whose
// v2 line is "0::/some/path". Pure, for testing.
func cgroupV2Path(procSelfCgroup string) (string, bool) {
	for _, line := range strings.Split(procSelfCgroup, "\n") {
		if strings.HasPrefix(line, "0::") {
			return strings.TrimPrefix(line, "0::"), true
		}
	}
	return "", false
}

// cgroupV1MemoryPath extracts the memory-controller path from /proc/self/cgroup,
// whose v1 lines are "hierarchyID:controllers:path" (controllers comma-listed).
// Pure, for testing.
func cgroupV1MemoryPath(procSelfCgroup string) (string, bool) {
	for _, line := range strings.Split(procSelfCgroup, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		for _, ctrl := range strings.Split(parts[1], ",") {
			if ctrl == "memory" {
				return parts[2], true
			}
		}
	}
	return "", false
}

// parseCgroupV2MemoryMax parses a memory.max value: "max" is no limit
// (false); a positive integer is a real cap. Pure, for testing.
func parseCgroupV2MemoryMax(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "max" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// parseCgroupV1Limit parses a memory.limit_in_bytes value: a near-int64-max
// sentinel is "unlimited" (false); a smaller positive integer is a real cap.
// Pure, for testing.
func parseCgroupV1Limit(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 || v >= cgroupV1UnlimitedFloor {
		return 0, false
	}
	return v, true
}

// budgetFromLimit applies the caller's fraction to the detected limit, floors
// the result at one default body so the guard can never reject every listenWith,
// and caps it at the sanity ceiling (an absurd /proc or cgroup value can't push
// the budget past what setMaxBufferBudget itself would accept).
func budgetFromLimit(limit int64, fraction float64) int64 {
	b := int64(float64(limit) * fraction)
	if b < minBufferBudget {
		b = minBufferBudget
	}
	if b > maxBufferBudget {
		b = maxBufferBudget
	}
	return b
}
