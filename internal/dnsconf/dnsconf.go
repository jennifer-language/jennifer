// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package dnsconf picks the resolver configuration (resolv.conf) and hosts
// file name resolution uses (M29.5). Go's pure resolver, which the cgo-free
// release binaries use, reads only /etc/resolv.conf and /etc/hosts, so a
// system without them (a minimal container, a userland whose /etc lives under
// another prefix, Android) fails every lookup with no hint why. Each file is
// looked for in order:
//
//  1. the environment variable (JENNIFER_RESOLV_CONF / JENNIFER_HOSTS), a file
//     path; a named file that does not exist fails every lookup, it is never
//     skipped;
//  2. <PREFIX>/etc/<file> (version.SysconfDir), when the build's prefix is not
//     /usr;
//  3. /etc/<file>.
//
// When a file other than the system's is chosen, the default binary routes
// Go's resolver through an in-process stub (dnsconf_std.go) that answers from
// the chosen hosts file and forwards everything else to the chosen
// nameservers. When the system files are chosen, nothing changes. When no
// resolv.conf exists, Go's loopback default (127.0.0.1:53, [::1]:53) stays:
// the interpreter never invents a public resolver; Hint names the files and
// the variable for the error a failed lookup then throws.
package dnsconf

import (
	"os"
	"runtime"
	"strings"

	"jennifer-lang.dev/jennifer/internal/version"
)

// ResolvConfEnv names a resolv.conf to use instead of the system's.
const ResolvConfEnv = "JENNIFER_RESOLV_CONF"

// HostsEnv names a hosts file to use instead of the system's.
const HostsEnv = "JENNIFER_HOSTS"

// Source says where a chosen file came from.
type Source string

const (
	SourceEnv    Source = "environment" // the variable
	SourceBuild  Source = "build"       // <PREFIX>/etc
	SourceSystem Source = "system"      // /etc
	SourceNone   Source = "none"        // nothing found
)

// File is one chosen configuration file.
type File struct {
	Path    string   // the file in effect; "" when none
	Source  Source   // where it came from
	Env     string   // the variable that overrides it
	Missing bool     // the variable names a file that does not exist
	Tried   []string // every candidate looked at, in order
}

// Config is the resolver configuration in effect.
type Config struct {
	Resolv File
	Hosts  File
}

// systemDir is the operating system's configuration directory, the last layer.
// It is the one place the resolver names /etc: the system's own files are what
// Go's resolver reads, and what a stub is not needed for.
const systemDir = "/etc"

// usesFiles reports whether the platform's resolver is configured by
// resolv.conf / hosts files at all. Windows takes its DNS servers from the
// network adapters, so only an explicit variable applies there.
func usesFiles() bool { return runtime.GOOS != "windows" }

// choose applies the precedence for one file name ("resolv.conf", "hosts").
func choose(envName, base string, getenv func(string) string, isFile func(string) bool) File {
	if v := getenv(envName); v != "" {
		return File{Path: v, Source: SourceEnv, Env: envName, Missing: !isFile(v), Tried: []string{v}}
	}
	f := File{Source: SourceNone, Env: envName}
	if !usesFiles() {
		f.Source = SourceSystem
		return f
	}
	var cands []File
	if dir := version.SysconfDir(); dir != systemDir {
		cands = append(cands, File{Path: dir + "/" + base, Source: SourceBuild})
	}
	cands = append(cands, File{Path: systemDir + "/" + base, Source: SourceSystem})
	for _, c := range cands {
		f.Tried = append(f.Tried, c.Path)
		if isFile(c.Path) {
			f.Path, f.Source = c.Path, c.Source
			return f
		}
	}
	return f
}

// Resolve picks both files; getenv and isFile are injectable for tests.
func Resolve(getenv func(string) string, isFile func(string) bool) Config {
	return Config{
		Resolv: choose(ResolvConfEnv, "resolv.conf", getenv, isFile),
		Hosts:  choose(HostsEnv, "hosts", getenv, isFile),
	}
}

// isRegularFile reports whether path names an existing file (not a directory).
func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// Load picks both files from the process environment and filesystem.
func Load() Config { return Resolve(os.Getenv, isRegularFile) }

// Custom reports whether the configuration differs from what Go's resolver
// reads by itself, so the default binary installs its stub resolver.
func (c Config) Custom() bool {
	custom := func(f File) bool { return f.Missing || f.Source == SourceEnv || f.Source == SourceBuild }
	return custom(c.Resolv) || custom(c.Hosts)
}

// Hint explains a failed lookup when the configuration is the likely cause: a
// variable naming a missing file, or no resolv.conf at all. "" otherwise.
func (c Config) Hint() string {
	for _, f := range []File{c.Resolv, c.Hosts} {
		if f.Missing {
			return f.Env + " names " + quote(f.Path) + ", which does not exist or is not a file"
		}
	}
	if c.Resolv.Source == SourceNone {
		return "no resolver configuration: " + ResolvConfEnv + " is not set and " +
			missingList(c.Resolv.Tried) + ", so lookups go to the loopback default " +
			"127.0.0.1:53 / [::1]:53; set " + ResolvConfEnv + " to a resolv.conf naming your DNS servers"
	}
	return ""
}

// Explain appends Hint to a lookup failure's message, once.
func (c Config) Explain(msg string) string {
	h := c.Hint()
	if h == "" || strings.Contains(msg, h) {
		return msg
	}
	return msg + " (" + h + ")"
}

func missingList(paths []string) string {
	switch len(paths) {
	case 0:
		return "no file applies"
	case 1:
		return paths[0] + " does not exist"
	}
	return "none of " + strings.Join(paths, ", ") + " exists"
}

// quote double-quotes a path for a message without Go escaping noise.
func quote(s string) string { return "\"" + s + "\"" }

// Describe renders one file for `jennifer version -v`: the path in effect and
// its source, then the candidates looked at.
func (f File) Describe(name string) []string {
	var head string
	switch {
	case f.Missing:
		head = name + ": " + f.Path + " (environment " + f.Env + "; MISSING, lookups fail)"
	case f.Source == SourceNone:
		head = name + ": (none; lookups use the loopback default 127.0.0.1:53)"
		if name != "resolv.conf" {
			head = name + ": (none)"
		}
	case f.Path == "":
		head = name + ": (the operating system's settings)"
	case f.Source == SourceEnv:
		head = name + ": " + f.Path + " (environment " + f.Env + ")"
	default:
		head = name + ": " + f.Path + " (" + string(f.Source) + ")"
	}
	out := []string{head}
	if f.Source != SourceEnv {
		looked := "  looked for: " + f.Env + " (unset)"
		if len(f.Tried) > 0 {
			looked += ", " + strings.Join(f.Tried, ", ")
		}
		out = append(out, looked)
	}
	return out
}
