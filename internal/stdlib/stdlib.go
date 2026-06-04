// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package stdlib

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// Install registers stdlib functions on an interpreter.
// Call this before Interpreter.Run(prog).
func Install(in *interpreter.Interpreter) {
	in.Builtins["printf"] = printf
	in.Builtins["sprintf"] = sprintf
}

// printf writes formatted output to stdout. Two forms:
//   - printf(value)                 -> writes value's display form (M2 behavior)
//   - printf(format, args...)       -> format must be string; substitutes verbs (M3)
//
// Verbs: %d (int), %f (float), %s (string), %t (bool), %v (any/display), %%.
func printf(out io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs(args)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, err := io.WriteString(out, s); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

// sprintf is like printf but returns the formatted string instead of writing.
func sprintf(_ io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	s, err := formatArgs(args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(s), nil
}

// formatArgs implements the shared single-arg / format-string semantics:
//   - 0 args: error.
//   - 1 non-string arg: print its display form (preserves M2 ergonomics for
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
		if argIdx >= len(args) {
			return "", fmt.Errorf("not enough arguments for format string")
		}
		v := args[argIdx]
		switch verb {
		case 'd':
			if v.Kind != interpreter.KindInt {
				return "", fmt.Errorf("`%%d` requires int, got %s", v.Kind)
			}
			b.WriteString(strconv.FormatInt(v.Int, 10))
		case 'f':
			if v.Kind != interpreter.KindFloat {
				return "", fmt.Errorf("`%%f` requires float, got %s", v.Kind)
			}
			b.WriteString(strconv.FormatFloat(v.Float, 'g', -1, 64))
		case 's':
			if v.Kind != interpreter.KindString {
				return "", fmt.Errorf("`%%s` requires string, got %s", v.Kind)
			}
			b.WriteString(v.Str)
		case 't':
			if v.Kind != interpreter.KindBool {
				return "", fmt.Errorf("`%%t` requires bool, got %s", v.Kind)
			}
			if v.Bool {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case 'v':
			b.WriteString(v.Display())
		default:
			return "", fmt.Errorf("unknown format verb `%%%c`", verb)
		}
		argIdx++
		i += 2
	}
	if argIdx != len(args) {
		return "", fmt.Errorf("too many arguments for format string (used %d of %d)", argIdx, len(args))
	}
	return b.String(), nil
}
