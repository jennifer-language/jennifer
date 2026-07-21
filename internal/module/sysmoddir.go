// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package module

import (
	"fmt"
	"os"
)

// SysmoddirEnv is the environment variable that overrides the system
// module directory (below a `--sysmoddir` flag, above the compile-time
// default).
const SysmoddirEnv = "JENNIFER_SYSMODDIR"

// VendorEnv is the environment variable that overrides the vendor root for
// `@scope/package` deck imports (below a `--vendor` flag, above the upward
// `vendor/` walk).
const VendorEnv = "JENNIFER_VENDOR"

// compileDefaultSysmoddir is the built-in system module directory. It is a
// var (not a const) so the build can override it the same way the version
// string is stamped - a generated file's init(), since TinyGo ignores
// `-ldflags -X`. Left at the FHS location a distro package installs to;
// on a fresh checkout with nothing installed there it simply resolves to
// no modules, which is fine (a program that imports nothing still runs).
var compileDefaultSysmoddir = "/usr/share/jennifer/modules"

// CompileDefaultSysmoddir returns the built-in system module directory
// (the lowest-precedence layer), for `jennifer version -v` reporting.
func CompileDefaultSysmoddir() string { return compileDefaultSysmoddir }

// Source names the layer a resolved sysmoddir came from, for `meta`
// reporting and `jennifer version -v`.
type Source string

const (
	SourceCLI     Source = "cli"     // --sysmoddir
	SourceEnv     Source = "env"     // JENNIFER_SYSMODDIR
	SourceCompile Source = "compile" // compile-time default
)

// Sysmoddir is a resolved system module directory plus the layer it came
// from.
type Sysmoddir struct {
	Dir    string
	Source Source
}

// ResolveSysmoddir applies the precedence --sysmoddir > JENNIFER_SYSMODDIR
// > compile-time default. cliFlag is "" when `--sysmoddir` was not given;
// getenv reads the environment (injectable for tests). It does not touch
// the filesystem - call Validate for that.
func ResolveSysmoddir(cliFlag string, getenv func(string) string) Sysmoddir {
	if cliFlag != "" {
		return Sysmoddir{Dir: cliFlag, Source: SourceCLI}
	}
	if env := getenv(SysmoddirEnv); env != "" {
		return Sysmoddir{Dir: env, Source: SourceEnv}
	}
	return Sysmoddir{Dir: compileDefaultSysmoddir, Source: SourceCompile}
}

// Validate refuses to start when a sysmoddir named explicitly (via the CLI
// flag or the env var) is missing or is not a directory. The compile-time
// default is best-effort - a checkout with no installed module tree still
// runs scripts that import nothing - so it is never validated to exist.
func (s Sysmoddir) Validate(stat func(string) (os.FileInfo, error)) error {
	if s.Source == SourceCompile {
		return nil
	}
	fi, err := stat(s.Dir)
	if err != nil {
		// Distinguish "not there" from "there but unreadable" (EACCES etc.):
		// reporting a permission error as "does not exist" is misleading.
		if os.IsNotExist(err) {
			return fmt.Errorf("%s module directory %q does not exist", s.Source, s.Dir)
		}
		return fmt.Errorf("%s module directory %q is not accessible: %v", s.Source, s.Dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s module directory %q is not a directory", s.Source, s.Dir)
	}
	return nil
}
