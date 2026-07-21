// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package devio holds the argument- and handle-plumbing shared by the
// handle-based libraries - born with the four device-I/O libraries
// (serial / spi / iic / gpio), also used by `sql`. It is not a Jennifer library
// itself - no namespace, no Install - just the boilerplate those libraries would
// otherwise each repeat: integer-handle structs (`serial.Port{id}`, ...) and
// typed positional-argument extraction with uniform, positioned error messages.
package devio

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Value keeps signatures short.
type Value = interpreter.Value

// Handle builds a `ns.name{id: id}` handle value (the integer-registry idiom).
func Handle(ns, name string, id int64) Value {
	return interpreter.NamespacedStructVal(ns, name, []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// HandleID pulls the integer id out of a `ns.name{...}` handle value, with a
// uniform boundary error when the argument is the wrong kind of struct.
func HandleID(fn string, v Value, ns, name string) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != ns || v.StructName != name {
		return 0, fmt.Errorf("%s: argument must be a %s.%s, got %s", fn, ns, name, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: %s.%s.id is not int", fn, ns, name)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: %s.%s has no id field", fn, ns, name)
}

// WantArgs checks the argument count.
func WantArgs(fn string, args []Value, n int) error {
	if len(args) != n {
		return fmt.Errorf("%s expects %d argument(s), got %d", fn, n, len(args))
	}
	return nil
}

// MaxRead caps a single caller-requested device read so a huge `n` cannot force
// a multi-gigabyte up-front allocation and OOM the whole process (the builtin
// panic is not catchable). Read in chunks for more. Matches fs.maxHandleRead.
const MaxRead = 256 << 20

// ReadSize validates a requested read length - non-negative and within MaxRead -
// and returns it as an int for make([]byte, ...). A huge length is a catchable
// positioned error rather than an uncatchable allocation crash.
func ReadSize(fn string, n int64) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("%s: read length must be non-negative, got %d", fn, n)
	}
	if n > MaxRead {
		return 0, fmt.Errorf("%s: read length %d exceeds the %d-byte cap; read in chunks", fn, n, MaxRead)
	}
	return int(n), nil
}

// StringArg / IntArg / BytesArg extract a typed positional argument.
func StringArg(fn string, args []Value, i int, name string) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("%s: missing argument %d (%s)", fn, i+1, name)
	}
	if args[i].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fn, name, args[i].Kind)
	}
	return args[i].Str, nil
}

func IntArg(fn string, args []Value, i int, name string) (int64, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("%s: missing argument %d (%s)", fn, i+1, name)
	}
	if args[i].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fn, name, args[i].Kind)
	}
	return args[i].Int, nil
}

func BytesArg(fn string, args []Value, i int, name string) ([]byte, error) {
	if i >= len(args) {
		return nil, fmt.Errorf("%s: missing argument %d (%s)", fn, i+1, name)
	}
	if args[i].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: %s must be bytes, got %s", fn, name, args[i].Kind)
	}
	return args[i].Bytes, nil
}
