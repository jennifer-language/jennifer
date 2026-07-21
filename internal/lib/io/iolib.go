// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package iolib implements Jennifer's `io` library: printf, sprintf, and
// (later) other I/O primitives. The Go package name is `iolib` to avoid
// colliding with the standard `io` package the implementation depends on.
package iolib

import (
	"fmt"
	"io"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "io"

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
}

// printf writes formatted output to stdout. Two forms:
//   - printf(value)                 -> writes value's display form
//   - printf(format, args...)       -> format must be string; substitutes verbs
//
// Verbs: %d (int), %f (float), %s (string), %t (bool), %v (any/display), %%.
func printf(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs(args)
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
	s, err := formatArgs(args)
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
	s, err := formatArgs(args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(s), nil
}

// formatArgs implements the shared single-arg / format-string semantics:
//   - 0 args: error.
//   - 1 non-string arg: print its display form (preserves ergonomics for
//     printing a single int/float/bool/null without a format string).
//   - First arg is a string: treat as format string and consume the rest.
//     Single-arg string form is a format string with 0 substitutions, so
//     `%%` is interpreted and stray `%` errors.
//   - Otherwise (multi-arg, non-string first): error.
func formatArgs(args []interpreter.Value) (string, error) {
	switch len(args) {
	case 0:
		return "", fmt.Errorf("expects at least 1 argument")
	}
	if args[0].Kind == interpreter.KindString {
		return formatString(args[0].Str, args[1:])
	}
	if len(args) == 1 {
		return args[0].Display(), nil
	}
	return "", fmt.Errorf("with multiple arguments, the first argument must be a string, got %s", args[0].Kind)
}

func formatString(fmtStr string, args []interpreter.Value) (string, error) {
	var b strings.Builder
	argIdx := 0
	i := 0
	for i < len(fmtStr) {
		c := fmtStr[i]
		if c != '%' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(fmtStr) {
			return "", fmt.Errorf("dangling `%%` at end of format string")
		}
		verb := fmtStr[i+1]
		if verb == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		if !isVerb(verb) {
			return "", fmt.Errorf("unknown format verb `%%%c`", verb)
		}
		spec, after, err := parseFormatSpec(verb, fmtStr, i+2)
		if err != nil {
			return "", err
		}
		if argIdx >= len(args) {
			return "", fmt.Errorf("not enough arguments for format string")
		}
		out, err := renderValue(spec, args[argIdx])
		if err != nil {
			return "", err
		}
		b.WriteString(out)
		argIdx++
		i = after
	}
	if argIdx != len(args) {
		return "", fmt.Errorf("too many arguments for format string (used %d of %d)", argIdx, len(args))
	}
	return b.String(), nil
}

func isVerb(c byte) bool {
	return c == 'd' || c == 'f' || c == 's' || c == 't' || c == 'v' || c == 'a'
}
