// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package capabilities reports the host capabilities the interpreter was compiled
// with. It backs the `# pragma-jennifer-capability:` header check and the `meta`
// query, so a module that needs a host facility the build lacks (e.g. `net` on
// jennifer-tiny) is refused at read time with a clear error instead of failing deep
// in a runtime stub.
//
// The set is fixed at build time by a build-tag-specific file
// (capabilities_std.go for the standard Go build, capabilities_tiny.go for the
// TinyGo build), mirroring the `net` library's own `!tinygo` / `tinygo` split. It
// is deliberately open-ended: a TinyGo build rebuilt with a network stack would add
// "net" to its file and every check and query updates with no special-casing.
//
// Capability names:
//   - "net"  - TCP / UDP / TLS / DNS sockets and the network-backed libraries.
//   - "exec" - os/exec: launching and managing external processes.
//   - "sql"  - the `sql` library's database drivers (MySQL / MariaDB / PostgreSQL).
package capabilities

import "sort"

// Has reports whether the build includes the named capability.
func Has(name string) bool {
	for _, c := range buildSet {
		if c == name {
			return true
		}
	}
	return false
}

// All returns the build's capability names, sorted, as a fresh slice.
func All() []string {
	out := append([]string(nil), buildSet...)
	sort.Strings(out)
	return out
}

// Known reports whether name is a capability the interpreter knows about at all
// (available or not) - used to tell "capability this build lacks" from "not a real
// capability name" (a typo) in the header check.
func Known(name string) bool {
	for _, c := range known {
		if c == name {
			return true
		}
	}
	return false
}

// KnownNames returns every capability name the interpreter understands (available
// or not), sorted, as a fresh slice - for a helpful "known: ..." error listing.
func KnownNames() []string {
	out := append([]string(nil), known...)
	sort.Strings(out)
	return out
}

// known is the full set of capability names the interpreter understands, across all
// builds. A `capability:` pragma naming something outside this set is a malformed
// directive (a typo), distinct from naming a real capability this build lacks.
var known = []string{"net", "exec", "sql"}
