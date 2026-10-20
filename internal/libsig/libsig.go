// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package libsig is the static signature table of the standard-library
// builtins: for each `lib.name` its arity, argument types, and return type, as
// far as they can be stated without running the call. Builtins validate their
// own arguments at runtime and carry no signature of their own, so `jennifer
// lint` reads this table to check a library call's arity (L109) and argument
// types (L110), and to know the type a library call returns (L110 / L111).
//
// The table is a declarative side table rather than part of
// RegisterNamespaced: one file per library, each registering a text block of
// one signature per line, so the registration call stays a name and a function
// and the table compiles into nothing but strings. It depends only on the
// parser's Type, so it costs the tiny binary nothing (only the lint subcommand
// reads it). A test in cmd/jennifer keeps it in lock-step with what InstallAll
// actually registers: every builtin has a signature, every signature names a
// builtin, and every line parses.
//
// Line grammar (blank lines and `#` comments are skipped):
//
//	name(T1, T2?, T3...) R
//
// Each T is a type in Jennifer's own syntax (`int`, `list of string`, `map of
// string to int`, `time.Time`, `Error`, `func`), `any` for "any value", or
// several such types joined by `|` (`string|bytes`). Inside a collection
// `any` leaves the element type open (`list of any` accepts every list). A
// trailing `?` marks an optional parameter (only trailing parameters may be
// optional); a trailing `...` marks a variadic last parameter taking zero or
// more arguments. R is the return type, `any` when it depends on the
// arguments, `null` when the call returns nothing. A builtin whose accepted
// shapes cannot be one line (it dispatches on its first argument) gets one
// line per shape; a call is accepted when any line accepts it.
//
// A signature must never be stricter than the builtin: lint reports a call
// only when no line could accept it, so an over-narrow type would turn a
// working call into a false finding. When in doubt, widen to `any`.
package libsig

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// Param is one parameter of a signature. Alts lists the accepted types; nil
// means any value. Optional and Variadic follow the `?` / `...` suffixes.
type Param struct {
	Alts     []parser.Type
	Optional bool
	Variadic bool
}

// Sig is one accepted call shape of a builtin. Ret lists the possible return
// types; nil means the return type is not statically known.
type Sig struct {
	Params []Param
	Ret    []parser.Type
}

// Accepts reports whether a call with n arguments fits this shape's arity.
func (s Sig) Accepts(n int) bool {
	min, max := 0, 0
	for _, p := range s.Params {
		switch {
		case p.Variadic:
			max = -1
		case p.Optional:
			if max >= 0 {
				max++
			}
		default:
			min++
			if max >= 0 {
				max++
			}
		}
	}
	return n >= min && (max < 0 || n <= max)
}

// ParamFor returns the parameter that the i-th argument binds to (the variadic
// last parameter absorbs every argument past it), or ok=false past the end.
func (s Sig) ParamFor(i int) (Param, bool) {
	if i < len(s.Params) {
		return s.Params[i], true
	}
	if n := len(s.Params); n > 0 && s.Params[n-1].Variadic {
		return s.Params[n-1], true
	}
	return Param{}, false
}

// ArityText renders the accepted argument count for a diagnostic: "2",
// "1 to 3", "at least 1".
func (s Sig) ArityText() string {
	min, max := 0, 0
	for _, p := range s.Params {
		switch {
		case p.Variadic:
			max = -1
		case p.Optional:
			if max >= 0 {
				max++
			}
		default:
			min++
			if max >= 0 {
				max++
			}
		}
	}
	switch {
	case max < 0:
		return fmt.Sprintf("at least %d", min)
	case min == max:
		return fmt.Sprintf("%d", min)
	default:
		return fmt.Sprintf("%d to %d", min, max)
	}
}

// sources holds each library's raw signature text, appended by the per-library
// files' init functions.
var sources = map[string][]string{}

// add registers a library's signature block. Called from init only.
func add(lib, text string) {
	sources[lib] = append(sources[lib], text)
}

var (
	once   sync.Once
	table  map[string]map[string][]Sig
	errs   []error
	libSet []string
)

func load() {
	once.Do(func() {
		table = map[string]map[string][]Sig{}
		for lib, texts := range sources {
			fns := map[string][]Sig{}
			for _, text := range texts {
				for n, line := range strings.Split(text, "\n") {
					if i := strings.IndexByte(line, '#'); i >= 0 {
						line = line[:i]
					}
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					name, sig, err := parseLine(lib, line)
					if err != nil {
						errs = append(errs, fmt.Errorf("libsig %s line %d: %q: %v", lib, n+1, line, err))
						continue
					}
					fns[name] = append(fns[name], sig)
				}
			}
			table[lib] = fns
			libSet = append(libSet, lib)
		}
		sort.Strings(libSet)
	})
}

// Lookup returns the call shapes of `lib.name`, or nil when the table has no
// entry (an unknown name, or a library the table does not cover).
func Lookup(lib, name string) []Sig {
	load()
	return table[lib][name]
}

// HasLibrary reports whether the table covers lib.
func HasLibrary(lib string) bool {
	load()
	_, ok := table[lib]
	return ok
}

// Libraries returns the covered library names, sorted.
func Libraries() []string {
	load()
	return libSet
}

// Names returns the builtin names the table lists for lib, sorted.
func Names(lib string) []string {
	load()
	var out []string
	for n := range table[lib] {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Errors returns every malformed line found while loading the table. Empty in
// a correct build; the coverage test asserts it.
func Errors() []error {
	load()
	return errs
}

// parseLine parses `name(params) ret`.
func parseLine(lib, line string) (string, Sig, error) {
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open <= 0 || close < open {
		return "", Sig{}, fmt.Errorf("want name(params) ret")
	}
	name := strings.TrimSpace(line[:open])
	var sig Sig
	if inner := strings.TrimSpace(line[open+1 : close]); inner != "" {
		parts := strings.Split(inner, ",")
		for i, part := range parts {
			part = strings.TrimSpace(part)
			var p Param
			switch {
			case strings.HasSuffix(part, "..."):
				if i != len(parts)-1 {
					return "", Sig{}, fmt.Errorf("only the last parameter may be variadic")
				}
				p.Variadic = true
				part = strings.TrimSpace(strings.TrimSuffix(part, "..."))
			case strings.HasSuffix(part, "?"):
				p.Optional = true
				part = strings.TrimSpace(strings.TrimSuffix(part, "?"))
			default:
				if i > 0 && (sig.Params[i-1].Optional || sig.Params[i-1].Variadic) {
					return "", Sig{}, fmt.Errorf("a required parameter follows an optional one")
				}
			}
			alts, err := parseAlts(lib, part)
			if err != nil {
				return "", Sig{}, err
			}
			p.Alts = alts
			sig.Params = append(sig.Params, p)
		}
	}
	ret := strings.TrimSpace(line[close+1:])
	if ret == "" {
		return "", Sig{}, fmt.Errorf("missing return type")
	}
	alts, err := parseAlts(lib, ret)
	if err != nil {
		return "", Sig{}, err
	}
	sig.Ret = alts
	return name, sig, nil
}

// parseAlts parses `T` or `T1|T2|...`; a top-level `any` anywhere in the
// union makes the whole set nil (any value).
func parseAlts(lib, s string) ([]parser.Type, error) {
	var out []parser.Type
	anyVal := false
	for _, alt := range strings.Split(s, "|") {
		words := strings.Fields(alt)
		if len(words) == 1 && words[0] == "any" {
			anyVal = true
			continue
		}
		t, rest, err := parseType(lib, words)
		if err != nil {
			return nil, err
		}
		if len(rest) > 0 {
			return nil, fmt.Errorf("unexpected %q after a type", strings.Join(rest, " "))
		}
		if t == nil {
			return nil, fmt.Errorf("`any` is only a whole parameter or an element type")
		}
		out = append(out, *t)
	}
	if anyVal {
		return nil, nil
	}
	return out, nil
}

// parseType parses one type from words and returns the rest. A nil type is
// `any` (only meaningful as a collection element).
func parseType(lib string, words []string) (*parser.Type, []string, error) {
	if len(words) == 0 {
		return nil, nil, fmt.Errorf("missing type")
	}
	w, rest := words[0], words[1:]
	prim := map[string]parser.TypeKind{
		"int": parser.TypeInt, "float": parser.TypeFloat, "string": parser.TypeString,
		"bool": parser.TypeBool, "null": parser.TypeNull, "bytes": parser.TypeBytes,
		"func": parser.TypeFunc,
	}
	if k, ok := prim[w]; ok {
		return &parser.Type{Kind: k}, rest, nil
	}
	switch w {
	case "any":
		return nil, rest, nil
	case "list", "task", "channel":
		if len(rest) == 0 || rest[0] != "of" {
			return nil, nil, fmt.Errorf("`%s` needs `of T`", w)
		}
		el, rest2, err := parseType(lib, rest[1:])
		if err != nil {
			return nil, nil, err
		}
		kind := map[string]parser.TypeKind{"list": parser.TypeList, "task": parser.TypeTask, "channel": parser.TypeChannel}[w]
		return &parser.Type{Kind: kind, Element: el}, rest2, nil
	case "map":
		if len(rest) == 0 || rest[0] != "of" {
			return nil, nil, fmt.Errorf("`map` needs `of K to V`")
		}
		k, rest2, err := parseType(lib, rest[1:])
		if err != nil {
			return nil, nil, err
		}
		if len(rest2) == 0 || rest2[0] != "to" {
			return nil, nil, fmt.Errorf("`map of K` needs `to V`")
		}
		v, rest3, err := parseType(lib, rest2[1:])
		if err != nil {
			return nil, nil, err
		}
		if k == nil || v == nil {
			// An open key or value leaves the whole map open.
			return &parser.Type{Kind: parser.TypeMap}, rest3, nil
		}
		return &parser.Type{Kind: parser.TypeMap, KeyType: k, ValType: v}, rest3, nil
	}
	if ns, name, ok := strings.Cut(w, "."); ok {
		if !isIdent(ns) || !isIdent(name) {
			return nil, nil, fmt.Errorf("bad struct type %q", w)
		}
		return &parser.Type{Kind: parser.TypeStruct, StructNS: ns, StructName: name}, rest, nil
	}
	if w == "Error" {
		return &parser.Type{Kind: parser.TypeStruct, StructName: "Error"}, rest, nil
	}
	return nil, nil, fmt.Errorf("unknown type %q (write a library type as `%s.%s`)", w, lib, w)
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}
