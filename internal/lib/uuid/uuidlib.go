// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package uuidlib is the `uuid` library: generate (`uuid.v4()` / `uuid.v7()`)
// and parse RFC 9562 UUIDs (v4 random, v7 time-ordered). Sixteen bytes,
// version / variant bits, and the 8-4-4-4-12 hex form. Self-contained and
// TinyGo-clean: hex is hand-formatted, randomness draws from the `crypto`
// library's crypto-grade source, and v7's timestamp is the wall clock. v7 is
// monotonic per process (RFC 9562 6.2 method 3).
package uuidlib

import (
	"encoding/binary"
	"fmt"
	"sync"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	cryptolib "jennifer-lang.dev/jennifer/internal/lib/crypto"
)

// LibraryName is the namespace prefix (`uuid.`) and the `use` name.
const LibraryName = "uuid"

// NIL is the all-zero UUID.
const NIL = "00000000-0000-0000-0000-000000000000"

const hexDigits = "0123456789abcdef"

// nowFunc is the wall-clock source for v7 timestamps. Tests swap it to get
// deterministic, monotonic values (mirrors timelib.nowFunc).
var nowFunc = stdtime.Now

// Install registers v4 / v7 / parse / isValid / version and the NIL constant.
// The version is a real method (`uuid.v4()` / `uuid.v7()`) now that identifiers
// admit interior digits, rather than a string argument to a `generate` verb.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "v4", v4Fn)
	in.RegisterNamespaced(LibraryName, "v7", v7Fn)
	in.RegisterNamespaced(LibraryName, "parse", parseFn)
	in.RegisterNamespaced(LibraryName, "isValid", isValidFn)
	in.RegisterNamespaced(LibraryName, "version", versionFn)
	in.RegisterNamespacedConst(LibraryName, "NIL", interpreter.StringVal(NIL))
}

// randFill fills b with crypto-grade random bytes in one shared-lock
// acquisition (rather than one per byte). An unavailable
// random source is an error, never a predictable UUID.
func randFill(b []byte) error { return cryptolib.RandFill(b) }

func v4Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := noArgs("v4", args); err != nil {
		return interpreter.Null(), err
	}
	s, err := genV4()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("uuid.v4: %v", err)
	}
	return interpreter.StringVal(s), nil
}

func v7Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := noArgs("v7", args); err != nil {
		return interpreter.Null(), err
	}
	s, err := genV7()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("uuid.v7: %v", err)
	}
	return interpreter.StringVal(s), nil
}

// noArgs rejects any argument to a nullary generator.
func noArgs(fn string, args []interpreter.Value) error {
	if len(args) != 0 {
		return fmt.Errorf("uuid.%s expects no arguments, got %d", fn, len(args))
	}
	return nil
}

// genV4 builds a random (version 4) UUID string.
func genV4() (string, error) {
	var b [16]byte
	if err := randFill(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return format(b), nil
}

// v7 state: the last timestamp and 74-bit random tail (rand_a 12 bits, rand_b
// 62 bits) handed out, shared by every task so v7s are strictly increasing
// process-wide.
var (
	v7Mu     sync.Mutex
	v7LastMs int64 = -1
	v7RandA  uint64
	v7RandB  uint64
)

const (
	randAMax = 1 << 12
	randBMax = 1 << 62
)

// genV7 builds a time-ordered (version 7) UUID string: 48-bit big-endian
// millisecond timestamp in the leading six bytes, random elsewhere. Within one
// millisecond (or when the clock steps back) it reuses the last timestamp and
// adds a random step of 1..2^32 to the 74-bit tail (RFC 9562 6.2 method 3),
// carrying into the timestamp on overflow, so each v7 sorts after the last.
func genV7() (string, error) {
	ms := nowFunc().UnixMilli()
	var r [10]byte
	if err := randFill(r[:]); err != nil {
		return "", err
	}
	v7Mu.Lock()
	if ms > v7LastMs {
		v7RandA = uint64(binary.BigEndian.Uint16(r[0:2])) & (randAMax - 1)
		v7RandB = binary.BigEndian.Uint64(r[2:10]) & (randBMax - 1)
	} else {
		ms = v7LastMs
		v7RandB += uint64(binary.BigEndian.Uint32(r[0:4])) + 1
		if v7RandB >= randBMax {
			v7RandB -= randBMax
			v7RandA++
			if v7RandA >= randAMax {
				v7RandA = 0
				ms++
			}
		}
	}
	v7LastMs = ms
	ra, rb := v7RandA, v7RandB
	v7Mu.Unlock()

	var b [16]byte
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = 0x70 | byte(ra>>8) // version 7
	b[7] = byte(ra)
	binary.BigEndian.PutUint64(b[8:16], rb)
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return format(b), nil
}

func parseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := strArg("parse", args)
	if err != nil {
		return interpreter.Null(), err
	}
	b, ok := parse(s)
	if !ok {
		return interpreter.Null(), errInvalid("parse", s)
	}
	out := make([]byte, 16)
	copy(out, b[:])
	return interpreter.BytesVal(out), nil
}

func isValidFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := strArg("isValid", args)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ok := parse(s)
	return interpreter.BoolVal(ok), nil
}

func versionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	s, err := strArg("version", args)
	if err != nil {
		return interpreter.Null(), err
	}
	b, ok := parse(s)
	if !ok {
		return interpreter.Null(), errInvalid("version", s)
	}
	return interpreter.IntVal(int64(b[6] >> 4)), nil
}

// format renders the 16 bytes as lowercase 8-4-4-4-12 hex.
func format(b [16]byte) string {
	var out [36]byte
	pos := 0
	for i, by := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hexDigits[by>>4]
		out[pos+1] = hexDigits[by&0x0f]
		pos += 2
	}
	return string(out[:])
}

// parse validates the 8-4-4-4-12 form (case-insensitive hex) and decodes it to
// 16 bytes. ok=false for any malformed input.
func parse(s string) (b [16]byte, ok bool) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return b, false
	}
	bi := 0
	for i := 0; i < 36; {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			i++
			continue
		}
		hi, ok1 := nibble(s[i])
		lo, ok2 := nibble(s[i+1])
		if !ok1 || !ok2 {
			return [16]byte{}, false
		}
		b[bi] = hi<<4 | lo
		bi++
		i += 2
	}
	return b, bi == 16
}

// strArg validates a single string argument.
func strArg(fn string, args []interpreter.Value) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("uuid.%s expects 1 argument (string), got %d", fn, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return "", fmt.Errorf("uuid.%s: argument must be string, got %s", fn, args[0].Kind)
	}
	return args[0].Str, nil
}

// errInvalid reports a malformed UUID string.
func errInvalid(fn, s string) error {
	return fmt.Errorf("uuid.%s: %q is not a valid UUID", fn, s)
}

// nibble decodes one hex digit.
func nibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
