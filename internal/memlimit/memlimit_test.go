// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package memlimit

import "testing"

// The cgroup / meminfo parsers behind Detect.
func TestMemLimitParsers(t *testing.T) {
	// /proc/meminfo -> bytes.
	if got, ok := parseMemTotalBytes("MemFree: 100 kB\nMemTotal:   16384000 kB\nBuffers: 1 kB\n"); !ok || got != 16384000*1024 {
		t.Errorf("parseMemTotalBytes = (%d, %v), want (%d, true)", got, ok, int64(16384000)*1024)
	}
	if _, ok := parseMemTotalBytes("Buffers: 1 kB\n"); ok {
		t.Error("parseMemTotalBytes should fail when MemTotal is absent")
	}

	// cgroup v2 memory.max: "max" = unlimited, a number = a cap.
	if _, ok := parseCgroupV2MemoryMax("max\n"); ok {
		t.Error(`parseCgroupV2MemoryMax("max") should report no limit`)
	}
	if got, ok := parseCgroupV2MemoryMax(" 536870912 \n"); !ok || got != 536870912 {
		t.Errorf("parseCgroupV2MemoryMax = (%d, %v), want (536870912, true)", got, ok)
	}

	// cgroup v1 limit_in_bytes: a near-int64-max sentinel = unlimited.
	if _, ok := parseCgroupV1Limit("9223372036854771712\n"); ok {
		t.Error("parseCgroupV1Limit should treat the sentinel as unlimited")
	}
	if got, ok := parseCgroupV1Limit("268435456"); !ok || got != 268435456 {
		t.Errorf("parseCgroupV1Limit = (%d, %v), want (268435456, true)", got, ok)
	}

	// /proc/self/cgroup path extraction.
	if p, ok := cgroupV2Path("0::/system.slice/app.service\n"); !ok || p != "/system.slice/app.service" {
		t.Errorf("cgroupV2Path = (%q, %v)", p, ok)
	}
	if p, ok := cgroupV1MemoryPath("9:cpu,cpuacct:/x\n8:memory:/system.slice/app\n"); !ok || p != "/system.slice/app" {
		t.Errorf("cgroupV1MemoryPath = (%q, %v)", p, ok)
	}
	if _, ok := cgroupV1MemoryPath("9:cpu,cpuacct:/x\n"); ok {
		t.Error("cgroupV1MemoryPath should fail when no memory controller line is present")
	}
}
