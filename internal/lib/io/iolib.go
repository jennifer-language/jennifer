// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package iolib implements Jennifer's `io` library: printf / eprintf / sprintf
// and stdin input (readLine, readLines, readBytes, readChars, eof). The Go
// package name is `iolib` to avoid colliding with the standard `io` package
// the implementation depends on.
package iolib

import (
	"fmt"
	"io"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "io"

// dataKind is the kind of stdin text that is not valid UTF-8 (M29.21).
const dataKind = "io"

// Install registers io library functions on an interpreter.
// Call this before Interpreter.Run(prog).
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "printf", printf)
	in.RegisterNamespaced(LibraryName, "eprintf", eprintf)
	in.RegisterNamespaced(LibraryName, "sprintf", sprintf)
	in.RegisterNamespaced(LibraryName, "readLine", readLine)
	in.RegisterNamespaced(LibraryName, "eof", eofFn)
	in.RegisterNamespaced(LibraryName, "readBytes", readBytes)
	in.RegisterNamespaced(LibraryName, "readChars", readChars)
	in.RegisterNamespaced(LibraryName, "readLines", readLines)
}

// printf writes formatted output to stdout. Two forms:
//   - printf(value)                 -> writes value's display form
//   - printf(format, args...)       -> format must be string; substitutes verbs
//
// Verbs: %d (int), %f (float), %s (string), %t (bool), %v (any/display), %%.
func printf(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs("io.printf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, err := io.WriteString(ctx.Out, s); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

// eprintf is printf's stderr sibling: same formatting, writes to the error
// stream instead of stdout. For diagnostics / logs that must not pollute a
// program's stdout data.
func eprintf(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs("io.eprintf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, err := io.WriteString(ctx.Err, s); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

// sprintf is like printf but returns the formatted string instead of writing.
func sprintf(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs("io.sprintf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(s), nil
}

// formatArgs implements the shared single-arg / format-string semantics:
//   - 0 args: error.
//   - First arg is a string: treat as format string and consume the rest.
//     Single-arg string form is a format string with 0 substitutions, so
//     `%%` is interpreted and stray `%` errors.
//   - Otherwise: error. The one-value form that printed a non-string's
//     display form was removed in 0.28.0 (M29.35): write `"%v"`.
//
// Errors are prefixed with fn (`io.printf` / `io.eprintf` / `io.sprintf`).
func formatArgs(fn string, args []interpreter.Value) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("%s: expects at least 1 argument", fn)
	}
	if args[0].Kind == interpreter.KindString {
		s, err := formatString(args[0].Str, args[1:])
		if err != nil {
			return "", fmt.Errorf("%s: %w", fn, err)
		}
		return s, nil
	}
	return "", fmt.Errorf("%s: the first argument is the format and must be a string, got %s; to print a value, write %s(\"%%v\", value)", fn, args[0].Kind, fn)
}

func formatString(fmtStr string, args []interpreter.Value) (string, error) {
	var b strings.Builder
	argIdx := 0
	err := scanFormat(fmtStr, b.WriteByte, func(spec FormatSpec) error {
		if argIdx >= len(args) {
			return fmt.Errorf("not enough arguments for format string")
		}
		out, err := renderValue(spec, args[argIdx])
		if err != nil {
			return err
		}
		b.WriteString(out)
		argIdx++
		return nil
	})
	if err != nil {
		return "", err
	}
	if argIdx != len(args) {
		return "", fmt.Errorf("too many arguments for format string (used %d of %d)", argIdx, len(args))
	}
	return b.String(), nil
}

// scanFormat walks a format string: each literal byte (`%%` as one '%') goes
// to lit, each verb's parsed spec to spec, in order. It stops at the first
// malformed verb, or the first error spec returns.
func scanFormat(fmtStr string, lit func(byte) error, spec func(FormatSpec) error) error {
	i := 0
	for i < len(fmtStr) {
		c := fmtStr[i]
		if c != '%' {
			_ = lit(c)
			i++
			continue
		}
		if i+1 >= len(fmtStr) {
			return fmt.Errorf("dangling `%%` at end of format string")
		}
		verb := fmtStr[i+1]
		if verb == '%' {
			_ = lit('%')
			i += 2
			continue
		}
		if !isVerb(verb) {
			return fmt.Errorf("unknown format verb `%%%c`", verb)
		}
		sp, after, err := parseFormatSpec(verb, fmtStr, i+2)
		if err != nil {
			return err
		}
		if err := spec(sp); err != nil {
			return err
		}
		i = after
	}
	return nil
}

// FormatVerb is one verb of a format string: the verb letter, and whether a
// `null=` modifier lets it take a null.
type FormatVerb struct {
	Verb   byte
	NullOK bool
}

// ParseFormat parses a printf format string as printf does at call time,
// without the values: its verbs in order, or the error printf would raise for
// the format itself. For `jennifer lint` (L117), which checks a literal format.
func ParseFormat(fmtStr string) ([]FormatVerb, error) {
	var verbs []FormatVerb
	err := scanFormat(fmtStr, func(byte) error { return nil }, func(sp FormatSpec) error {
		verbs = append(verbs, FormatVerb{Verb: sp.Verb, NullOK: sp.NullSet})
		return nil
	})
	return verbs, err
}

func isVerb(c byte) bool {
	return c == 'd' || c == 'f' || c == 's' || c == 't' || c == 'v' || c == 'a'
}
