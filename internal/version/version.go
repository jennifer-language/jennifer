// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package version exposes the interpreter's build version as a single
// string. The value is written at build time by a generated init() in
// version_gen.go (see the Makefile and scripts/gen-version.sh; codegen rather than
// `-ldflags -X`, which TinyGo silently ignores); a plain `go build` / `go run` /
// `go test` skips codegen and leaves it at the default of "dev".
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the project's build version. Format:
//
//   - "<tag>"                       when HEAD is exactly on a semver tag
//   - "<tag>-dev+<N>.<shortsha>"    when HEAD is N commits past a tag
//   - "0.0.0-dev+<N>.<shortsha>"    when no tag exists yet
//   - "dev"                         when built without the version ldflag
//
// The string surfaces to users as the CLI's `version` output and the `meta.VERSION`
// constant, and backs the `# pragma-jennifer-version:` floor check via AtLeast.
var Version = "dev"

// AtLeast reports whether the running interpreter satisfies a minimum version floor
// (a "major.minor.patch" string, e.g. "0.25.0"). It backs the
// `# pragma-jennifer-version:` header check.
//
// Any development build satisfies every floor: the literal "dev" default (a plain
// `go build` / `go test`) and any "X.Y.Z-dev+<n>.<sha>" build produced between
// releases are ahead of the last tag, so they are treated as new enough - only a
// clean release tag is actually compared. That is also what keeps `go test` and
// `make build` from failing the interpreter's own in-tree modules whose floor is
// the release being worked toward.
//
// A `min` that is not a valid "major.minor.patch" is an error (a malformed pragma
// value), reported regardless of build so `lint` / the read-time check catch it.
func AtLeast(min string) (bool, error) {
	reqMaj, reqMin, reqPat, err := parseCore(min)
	if err != nil {
		return false, err
	}
	if isDevVersion(Version) {
		return true, nil
	}
	curMaj, curMin, curPat, err := parseCore(Version)
	if err != nil {
		// A real release build always has a clean core; a non-dev version that
		// does not parse is unexpected. Fail open (treat as satisfying) rather than
		// break a legitimately-built binary over an unparseable self-version.
		return true, nil
	}
	if curMaj != reqMaj {
		return curMaj > reqMaj, nil
	}
	if curMin != reqMin {
		return curMin > reqMin, nil
	}
	return curPat >= reqPat, nil
}

// Below reports whether the running interpreter is below an exclusive upper bound
// (a "major.minor.patch" string): the `<0.26.0` of a `# pragma-jennifer-version:
// >=0.25.0 <0.26.0` header. It is the mirror of AtLeast for development builds:
// a dev build counts as newer than any release, so it satisfies every floor and
// fails every upper bound. A dev build cannot know whether the next tag is a
// patch or a minor, and `main` already carries the next release's breaking
// changes.
//
// A `max` that is not a valid "major.minor.patch" is an error, reported
// regardless of build.
func Below(max string) (bool, error) {
	if _, _, _, err := parseCore(max); err != nil {
		return false, err
	}
	if isDevVersion(Version) {
		return false, nil
	}
	if _, _, _, err := parseCore(Version); err != nil {
		// As in AtLeast: fail open on an unparseable non-dev self-version.
		return true, nil
	}
	c, _ := Compare(Version, max)
	return c < 0, nil
}

// Compare orders two versions by their "major.minor.patch" core (anything after a
// '-' or '+' is ignored): -1, 0 or 1. Either side failing to parse is an error.
func Compare(a, b string) (int, error) {
	aMaj, aMin, aPat, err := parseCore(a)
	if err != nil {
		return 0, err
	}
	bMaj, bMin, bPat, err := parseCore(b)
	if err != nil {
		return 0, err
	}
	for _, d := range [][2]int{{aMaj, bMaj}, {aMin, bMin}, {aPat, bPat}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

// isDevVersion reports whether v is a development (non-release) build - the "dev"
// default or any "-dev" build between tags.
func isDevVersion(v string) bool {
	return v == "dev" || strings.Contains(v, "-dev")
}

// parseCore parses the leading "major.minor.patch" of a version string, stopping at
// the first '-' or '+' (so "0.24.0-dev+7.abc" -> 0,24,0).
func parseCore(v string) (maj, min, pat int, err error) {
	core := v
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("version %q is not major.minor.patch", v)
	}
	for i, p := range parts {
		n, e := strconv.Atoi(p)
		if e != nil || n < 0 {
			return 0, 0, 0, fmt.Errorf("version %q has a non-numeric component", v)
		}
		switch i {
		case 0:
			maj = n
		case 1:
			min = n
		case 2:
			pat = n
		}
	}
	return maj, min, pat, nil
}
