// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package mathlib implements Jennifer's `math` library: a small set of
// frequently-needed numeric functions plus the constants PI and E. Strict
// error policy: anything mathematically undefined (sqrt of negative, NaN/Inf
// results) is rejected at runtime rather than letting NaN propagate.
//
// The Go package is named mathlib to avoid colliding with Go's standard
// `math` package, which this implementation depends on.
package mathlib

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	mathrand "math/rand"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "math"

// Install registers math library functions and constants on an interpreter.
// Every name lives behind the `math.` prefix.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "abs", absFn)
	in.RegisterNamespaced(LibraryName, "min", minFn)
	in.RegisterNamespaced(LibraryName, "max", maxFn)
	in.RegisterNamespaced(LibraryName, "sqrt", sqrtFn)
	in.RegisterNamespaced(LibraryName, "pow", powFn)
	in.RegisterNamespaced(LibraryName, "floor", floorFn)
	in.RegisterNamespaced(LibraryName, "ceil", ceilFn)
	in.RegisterNamespaced(LibraryName, "round", roundFn)

	in.RegisterNamespaced(LibraryName, "rand", randFn)
	in.RegisterNamespaced(LibraryName, "randInt", randIntFn)
	in.RegisterNamespaced(LibraryName, "randSeed", randSeedFn)

	in.RegisterNamespacedConst(LibraryName, "PI", interpreter.FloatVal(math.Pi))
	in.RegisterNamespacedConst(LibraryName, "E", interpreter.FloatVal(math.E))
}

func arityOne(name string, args []interpreter.Value) error {
	if len(args) != 1 {
		return fmt.Errorf("%s expects 1 argument, got %d", name, len(args))
	}
	return nil
}

func arityTwo(name string, args []interpreter.Value) error {
	if len(args) != 2 {
		return fmt.Errorf("%s expects 2 arguments, got %d", name, len(args))
	}
	return nil
}

// absFn returns the absolute value, preserving the operand's type.
//   - int    -> int
//   - float  -> float
func absFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("abs", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		if v.Int == math.MinInt64 {
			// -MinInt64 overflows back to MinInt64 (still negative); the true
			// magnitude does not fit in a signed int64, so error rather than
			// return a wrong (negative) result.
			return interpreter.Null(), fmt.Errorf("abs(): |%d| does not fit in an int", v.Int)
		}
		if v.Int < 0 {
			return interpreter.IntVal(-v.Int), nil
		}
		return v, nil
	case interpreter.KindFloat:
		return interpreter.FloatVal(math.Abs(v.Float)), nil
	}
	return interpreter.Null(), fmt.Errorf("abs(): requires int or float, got %s", v.Kind)
}

// minFn returns the lesser of two numeric values. (int,int) -> int;
// mixed or (float,float) -> float (matches the `+` promotion rule).
func minFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("min", args); err != nil {
		return interpreter.Null(), err
	}
	return cmpPick(args[0], args[1], true, "min")
}

// maxFn is symmetric with minFn.
func maxFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("max", args); err != nil {
		return interpreter.Null(), err
	}
	return cmpPick(args[0], args[1], false, "max")
}

// cmpPick selects a or b based on `wantSmaller`. Same numeric-promotion rule
// as the `+` operator.
func cmpPick(a, b interpreter.Value, wantSmaller bool, name string) (interpreter.Value, error) {
	// Same-int fast path keeps the int type.
	if a.Kind == interpreter.KindInt && b.Kind == interpreter.KindInt {
		if wantSmaller {
			if a.Int <= b.Int {
				return a, nil
			}
			return b, nil
		}
		if a.Int >= b.Int {
			return a, nil
		}
		return b, nil
	}
	af, aok := a.AsFloat()
	bf, bok := b.AsFloat()
	if !aok || !bok {
		return interpreter.Null(), fmt.Errorf("%s(): requires numeric operands, got %s and %s", name, a.Kind, b.Kind)
	}
	if wantSmaller {
		if af <= bf {
			return interpreter.FloatVal(af), nil
		}
		return interpreter.FloatVal(bf), nil
	}
	if af >= bf {
		return interpreter.FloatVal(af), nil
	}
	return interpreter.FloatVal(bf), nil
}

// sqrtFn returns the square root as a float. Errors on negative input.
func sqrtFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("sqrt", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	f, ok := v.AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("sqrt(): requires int or float, got %s", v.Kind)
	}
	if f < 0 {
		return interpreter.Null(), fmt.Errorf("sqrt(): undefined for negative input %s", interpreter.DisplayFloat(f))
	}
	return interpreter.FloatVal(math.Sqrt(f)), nil
}

// powFn returns x**y as a float (always - avoids "is this int or float"
// ambiguity). NaN or Infinity results are rejected.
func powFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("pow", args); err != nil {
		return interpreter.Null(), err
	}
	a, aok := args[0].AsFloat()
	b, bok := args[1].AsFloat()
	if !aok || !bok {
		return interpreter.Null(), fmt.Errorf("pow(): requires numeric operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	r := math.Pow(a, b)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return interpreter.Null(), fmt.Errorf("pow(%s, %s): result is undefined or infinite", interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
	}
	return interpreter.FloatVal(r), nil
}

// floatToInt converts an integral float to int64, rejecting the values int64
// cannot hold: NaN, +/-Inf, and magnitudes at or beyond 2^63. Keeps math's
// strict stance - a mathematically-undefined result errors rather than yielding
// the platform-defined garbage a bare int64(f) conversion produces. fn names
// the caller for the error text.
func floatToInt(fn string, f float64) (interpreter.Value, error) {
	if math.IsNaN(f) {
		return interpreter.Null(), fmt.Errorf("%s(): result is not a number", fn)
	}
	if math.IsInf(f, 0) || f >= 9223372036854775808.0 || f < -9223372036854775808.0 {
		return interpreter.Null(), fmt.Errorf("%s(): result %g does not fit in an int", fn, f)
	}
	return interpreter.IntVal(int64(f)), nil
}

// floorFn rounds toward negative infinity. Accepts int (identity) or float
// (returns int).
func floorFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("floor", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	if v.Kind == interpreter.KindInt {
		return v, nil
	}
	if v.Kind != interpreter.KindFloat {
		return interpreter.Null(), fmt.Errorf("floor(): requires int or float, got %s", v.Kind)
	}
	return floatToInt("floor", math.Floor(v.Float))
}

// ceilFn rounds toward positive infinity. Accepts int (identity) or float
// (returns int).
func ceilFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("ceil", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	if v.Kind == interpreter.KindInt {
		return v, nil
	}
	if v.Kind != interpreter.KindFloat {
		return interpreter.Null(), fmt.Errorf("ceil(): requires int or float, got %s", v.Kind)
	}
	return floatToInt("ceil", math.Ceil(v.Float))
}

// roundFn rounds half-away-from-zero (Go's math.Round). Accepts int
// (identity) or float (returns int).
func roundFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("round", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	if v.Kind == interpreter.KindInt {
		return v, nil
	}
	if v.Kind != interpreter.KindFloat {
		return interpreter.Null(), fmt.Errorf("round(): requires int or float, got %s", v.Kind)
	}
	return floatToInt("round", math.Round(v.Float))
}

// Non-crypto pseudo-random helpers. Shared seeded source; protected with a
// mutex so concurrent interpreter instances don't race even though Jennifer
// itself has no concurrency primitives. The default source is seeded from
// OS entropy at startup so every process gets a distinct stream; randSeed()
// is the deterministic opt-in for reproducible runs.
var (
	randMu  sync.Mutex
	randSrc = mathrand.New(mathrand.NewSource(entropySeed()))
)

// entropySeed returns a startup seed for the shared source: 8 bytes from
// the OS CSPRNG, falling back to the wall clock if that read fails. The
// stream itself stays math/rand (seedable, non-crypto - documented); only
// the default seed is entropy so fresh processes don't repeat each other.
func entropySeed() int64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err == nil {
		return int64(binary.LittleEndian.Uint64(b[:]))
	}
	return time.Now().UnixNano()
}

// randFn returns a float in [0, 1).
func randFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("rand expects 0 arguments, got %d", len(args))
	}
	randMu.Lock()
	defer randMu.Unlock()
	return interpreter.FloatVal(randSrc.Float64()), nil
}

// randIntFn returns an int in [lo, hi] inclusive. Requires lo <= hi.
func randIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("randInt", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("randInt(): requires int operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	lo, hi := args[0].Int, args[1].Int
	if lo > hi {
		return interpreter.Null(), fmt.Errorf("randInt(%d, %d): lo must be <= hi", lo, hi)
	}
	randMu.Lock()
	defer randMu.Unlock()
	// Int63n needs span > 0; we add 1 because the range is inclusive.
	span := hi - lo + 1
	if span > 0 {
		return interpreter.IntVal(lo + randSrc.Int63n(span)), nil
	}
	// span overflowed int64 (the range is wider than 2^63, e.g.
	// randInt(0, MaxInt64)). Draw a full-width value and fold it into
	// [lo, hi]. uspan is the true span mod 2^64; 0 means the whole int64 range.
	uspan := uint64(hi) - uint64(lo) + 1
	if uspan == 0 {
		return interpreter.IntVal(int64(randSrc.Uint64())), nil
	}
	// Rejection-sample to avoid modulo bias: a raw `Uint64() % uspan` would
	// over-weight the lowest 2^64 mod uspan values. Discard draws in the short
	// top interval so every residue is equally likely.
	limit := (0xFFFFFFFFFFFFFFFF/uspan)*uspan - 1
	u := randSrc.Uint64()
	for u > limit {
		u = randSrc.Uint64()
	}
	return interpreter.IntVal(lo + int64(u%uspan)), nil
}

// randSeedFn sets the deterministic seed.
func randSeedFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("randSeed", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("randSeed(): requires int operand, got %s", args[0].Kind)
	}
	randMu.Lock()
	defer randMu.Unlock()
	randSrc = mathrand.New(mathrand.NewSource(args[0].Int))
	return interpreter.Null(), nil
}

// SharedIntN returns a uniformly-distributed int64 in [0, n) using the
// shared random source. n must be positive. Exposed so sibling
// libraries (e.g. lists.shuffle) can draw from the same `math.randSeed`
// stream as `math.rand` / `math.randInt`. Goroutine-safe via the same
// mutex as the random functions.
//
// Library-implementation detail: a user program does NOT need
// `use math;` for the function to work - the dependency is at the Go
// level, not the Jennifer level.
func SharedIntN(n int64) int64 {
	randMu.Lock()
	defer randMu.Unlock()
	return randSrc.Int63n(n)
}
