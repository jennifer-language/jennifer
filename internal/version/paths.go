// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package version

import "strings"

// DefaultPrefix is the install prefix of a build that sets none: the FHS
// layout a distribution package installs to.
const DefaultPrefix = "/usr"

// Prefix is the install prefix the build was made for (`make PREFIX=<dir>`).
// Like Version it is written by the generated init() in version_gen.go
// (scripts/gen-version.sh), not by `-ldflags -X`, which TinyGo ignores; a plain
// `go build` / `go test` leaves it at DefaultPrefix. Every built-in data path
// derives from it (SysmoddirDefault, DocDir, SysconfDir), so no Go code names
// `/usr` or `/etc` directly.
var Prefix = DefaultPrefix

// prefixDir returns Prefix without a trailing slash ("" for the root "/"), so
// the derived paths join with a single "/". Paths are joined with "/" on every
// platform: a Windows build keeps the forward-slash default it always had.
func prefixDir() string {
	return strings.TrimRight(Prefix, "/")
}

// SysmoddirDefault is the built-in system module directory, the lowest layer
// below `--sysmoddir` and JENNIFER_SYSMODDIR: <PREFIX>/share/jennifer/modules.
func SysmoddirDefault() string { return prefixDir() + "/share/jennifer/modules" }

// DocDir is where the packages install the documentation, JENNIFER.md among
// it (which decks sync their copies from): <PREFIX>/share/doc/jennifer.
func DocDir() string { return prefixDir() + "/share/doc/jennifer" }

// SysconfDir is the configuration directory of the prefix, where the resolver
// looks for resolv.conf and hosts before the system's /etc: <PREFIX>/etc,
// except that the FHS prefix /usr (and the root) keep their configuration in
// /etc itself, as autoconf's sysconfdir does.
func SysconfDir() string {
	p := prefixDir()
	if p == "" || p == DefaultPrefix {
		return "/etc"
	}
	return p + "/etc"
}
