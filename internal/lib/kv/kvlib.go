// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package kv is an in-process key/value store with per-key TTL: the local,
// no-server backend that parallels the `memcache` / `redis` clients. A store is
// an integer handle into a per-interpreter registry (the pattern `net.Conn` /
// `hash.Stream` use), so a `kv.Store` value shares its backing map across
// value-copies and across `spawn`ed tasks - the shared mutable state a pure `.j`
// module cannot hold itself. Pure Go stdlib (`sync`, `time`), so TinyGo-clean and
// available on both binaries.
//
// The verb set deliberately mirrors `memcache` (set / add / get / delete / touch
// / incr) so the `session` / `ratelimit` backend selector can dispatch to any of
// the three uniformly.
package kv

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "kv"

// entry is one stored value with an optional absolute expiry (zero = no expiry).
type entry struct {
	value   string
	expires time.Time
}

// store is one key/value map guarded by its own lock (spawned tasks sharing a
// handle can touch it concurrently). `path` is "" for a memory-only store, or a
// file the store is flushed to after every mutation (openFile), so its contents
// survive across `jennifer run` invocations.
type store struct {
	mu     sync.Mutex
	data   map[string]entry
	path   string
	writes int // mutations since the last active-expiry sweep
}

// sweepInterval is how many mutations pass between full expired-entry sweeps.
// TTL eviction is otherwise lazy (on access), so without this a flood of
// distinct short-lived keys - a rate limiter keyed by an attacker-controlled IP -
// would accumulate expired entries that are never accessed again and so never
// evicted. The sweep bounds memory to the *unexpired* working set (plus at most
// this many recently-expired entries). There is no cap on unexpired live data:
// that is the program's own responsibility (like an unbounded list), and a
// distributed backend with a server-side maxmemory policy is the answer for an
// adversarial, unbounded working set.
const sweepInterval = 512

// maybeSweepLocked drops every expired entry once `sweepInterval` mutations have
// accumulated. Amortised O(1) per mutation (an O(n) sweep every n writes). The
// caller holds s.mu.
func (s *store) maybeSweepLocked(now time.Time) {
	s.writes++
	if s.writes < sweepInterval {
		return
	}
	s.writes = 0
	for k, e := range s.data {
		if !e.expires.IsZero() && !now.Before(e.expires) {
			delete(s.data, k)
		}
	}
}

// flushLocked writes the (unexpired) store to its file, atomically (temp +
// rename). A no-op for a memory-only store. The caller holds s.mu. Values and
// keys are base64-encoded so an arbitrary string cannot break the line format;
// the expiry is stored as a unix-nano deadline (0 = no expiry).
func (s *store) flushLocked() error {
	if s.path == "" {
		return nil
	}
	now := time.Now()
	var b strings.Builder
	for k, e := range s.data {
		if !e.expires.IsZero() && !now.Before(e.expires) {
			continue // drop expired entries from the persisted set
		}
		exp := int64(0)
		if !e.expires.IsZero() {
			exp = e.expires.UnixNano()
		}
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(k)))
		b.WriteByte(' ')
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(e.value)))
		b.WriteByte(' ')
		b.WriteString(strconv.FormatInt(exp, 10))
		b.WriteByte('\n')
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// loadFile reads a store file written by flushLocked, dropping entries that have
// already expired. A missing file is an empty store.
func loadFile(path string) (map[string]entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]entry{}, nil
		}
		return nil, err
	}
	m := map[string]entry{}
	now := time.Now()
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 3 {
			continue
		}
		kb, e1 := base64.StdEncoding.DecodeString(parts[0])
		vb, e2 := base64.StdEncoding.DecodeString(parts[1])
		exp, e3 := strconv.ParseInt(parts[2], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		var expires time.Time
		if exp != 0 {
			expires = time.Unix(0, exp)
			if !now.Before(expires) {
				continue // already expired
			}
		}
		m[string(kb)] = entry{value: string(vb), expires: expires}
	}
	return m, nil
}

// registry holds the live stores keyed by integer handle, per interpreter (a
// fresh registry is closed over in Install, so nothing leaks across runs).
type registry struct {
	mu     sync.Mutex
	stores map[int64]*store
	nextID int64
}

// Install registers the kv surface with a fresh per-interpreter registry.
func Install(in *interpreter.Interpreter) {
	r := &registry{stores: map[int64]*store{}}
	in.RegisterNamespacedStruct(LibraryName, "Store", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "open", r.openFn)
	in.RegisterNamespaced(LibraryName, "openFile", r.openFileFn)
	in.RegisterNamespaced(LibraryName, "set", r.setFn)
	in.RegisterNamespaced(LibraryName, "add", r.addFn)
	in.RegisterNamespaced(LibraryName, "get", r.getFn)
	in.RegisterNamespaced(LibraryName, "has", r.hasFn)
	in.RegisterNamespaced(LibraryName, "delete", r.deleteFn)
	in.RegisterNamespaced(LibraryName, "touch", r.touchFn)
	in.RegisterNamespaced(LibraryName, "incr", r.incrFn)
	in.RegisterNamespaced(LibraryName, "close", r.closeFn)
}

// makeStore builds the Jennifer-side `kv.Store{id}` value.
func makeStore(id int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Store", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// resolve pulls the store out of a `kv.Store` argument.
func (r *registry) resolve(fnName string, v interpreter.Value) (*store, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Store" {
		return nil, fmt.Errorf("%s: argument must be a kv.Store, got %s", fnName, v.Kind)
	}
	var id int64
	found := false
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return nil, fmt.Errorf("%s: kv.Store.id is not int", fnName)
			}
			id = f.Value.Int
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("%s: kv.Store has no id field", fnName)
	}
	r.mu.Lock()
	s, ok := r.stores[id]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s: kv.Store is closed or invalid", fnName)
	}
	return s, nil
}

// live reports the entry at key if present and not expired, evicting it lazily
// on expiry. The caller holds s.mu.
func live(s *store, key string, now time.Time) (entry, bool) {
	e, ok := s.data[key]
	if !ok {
		return entry{}, false
	}
	if !e.expires.IsZero() && !now.Before(e.expires) {
		delete(s.data, key)
		return entry{}, false
	}
	return e, true
}

// expiryFor turns a ttl in seconds (0 = no expiry) into an absolute deadline.
func expiryFor(ttl int64, now time.Time) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(ttl) * time.Second)
}

func stringArg(fnName, what string, v interpreter.Value) (string, error) {
	if v.Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, what, v.Kind)
	}
	return v.Str, nil
}

func intArg(fnName, what string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fnName, what, v.Kind)
	}
	return v.Int, nil
}

// openFn implements `kv.open() -> kv.Store`: a fresh, empty store.
func (r *registry) openFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("kv.open expects 0 arguments, got %d", len(args))
	}
	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.stores[id] = &store{data: map[string]entry{}}
	r.mu.Unlock()
	return makeStore(id), nil
}

// openFileFn implements `kv.openFile(path) -> kv.Store`: a store persisted to
// `path`. Its contents are loaded on open and rewritten after every mutation, so
// state survives across `jennifer run` invocations (unlike `open`, which is reset
// each run). Single-process persistence: within a process, `spawn`ed tasks share
// it safely, but concurrent *separate* processes on one file can lose writes -
// use `redis` / `memcache` for cross-process coordination.
func (r *registry) openFileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("kv.openFile expects 1 argument (path), got %d", len(args))
	}
	path, err := stringArg("kv.openFile", "path", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	data, lerr := loadFile(path)
	if lerr != nil {
		return interpreter.Null(), fmt.Errorf("kv.openFile: %v", lerr)
	}
	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.stores[id] = &store{data: data, path: path}
	r.mu.Unlock()
	return makeStore(id), nil
}

// setFn implements `kv.set(store, key, value, ttl)`: store value with a ttl-second
// expiry (0 = never expires), replacing any existing value.
func (r *registry) setFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("kv.set expects 4 arguments (store, key, value, ttl), got %d", len(args))
	}
	s, err := r.resolve("kv.set", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.set", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	value, err := stringArg("kv.set", "value", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	ttl, err := intArg("kv.set", "ttl", args[3])
	if err != nil {
		return interpreter.Null(), err
	}
	now := time.Now()
	s.mu.Lock()
	s.data[key] = entry{value: value, expires: expiryFor(ttl, now)}
	s.maybeSweepLocked(now)
	ferr := s.flushLocked()
	s.mu.Unlock()
	if ferr != nil {
		return interpreter.Null(), fmt.Errorf("kv.set: %v", ferr)
	}
	return interpreter.Null(), nil
}

// addFn implements `kv.add(store, key, value, ttl) -> bool`: store only if the
// key is absent (or expired); returns whether it stored.
func (r *registry) addFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("kv.add expects 4 arguments (store, key, value, ttl), got %d", len(args))
	}
	s, err := r.resolve("kv.add", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.add", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	value, err := stringArg("kv.add", "value", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	ttl, err := intArg("kv.add", "ttl", args[3])
	if err != nil {
		return interpreter.Null(), err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := live(s, key, now); ok {
		return interpreter.BoolVal(false), nil
	}
	s.data[key] = entry{value: value, expires: expiryFor(ttl, now)}
	s.maybeSweepLocked(now)
	if err := s.flushLocked(); err != nil {
		return interpreter.Null(), fmt.Errorf("kv.add: %v", err)
	}
	return interpreter.BoolVal(true), nil
}

// getFn implements `kv.get(store, key) -> string`: the value, or "" when absent
// or expired.
func (r *registry) getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("kv.get expects 2 arguments (store, key), got %d", len(args))
	}
	s, err := r.resolve("kv.get", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.get", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := live(s, key, time.Now()); ok {
		return interpreter.StringVal(e.value), nil
	}
	return interpreter.StringVal(""), nil
}

// hasFn implements `kv.has(store, key) -> bool`: whether the key is present and
// unexpired (so "" stored is distinguishable from absent).
func (r *registry) hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("kv.has expects 2 arguments (store, key), got %d", len(args))
	}
	s, err := r.resolve("kv.has", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.has", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := live(s, key, time.Now())
	return interpreter.BoolVal(ok), nil
}

// deleteFn implements `kv.delete(store, key) -> bool`: remove the key; returns
// whether it existed (and was unexpired).
func (r *registry) deleteFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("kv.delete expects 2 arguments (store, key), got %d", len(args))
	}
	s, err := r.resolve("kv.delete", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.delete", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := live(s, key, time.Now())
	if ok {
		delete(s.data, key)
		if err := s.flushLocked(); err != nil {
			return interpreter.Null(), fmt.Errorf("kv.delete: %v", err)
		}
	}
	return interpreter.BoolVal(ok), nil
}

// touchFn implements `kv.touch(store, key, ttl) -> bool`: re-arm the key's expiry
// without changing its value; returns whether it existed.
func (r *registry) touchFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("kv.touch expects 3 arguments (store, key, ttl), got %d", len(args))
	}
	s, err := r.resolve("kv.touch", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.touch", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	ttl, err := intArg("kv.touch", "ttl", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := live(s, key, now)
	if !ok {
		return interpreter.BoolVal(false), nil
	}
	e.expires = expiryFor(ttl, now)
	s.data[key] = e
	s.maybeSweepLocked(now)
	if err := s.flushLocked(); err != nil {
		return interpreter.Null(), fmt.Errorf("kv.touch: %v", err)
	}
	return interpreter.BoolVal(true), nil
}

// incrFn implements `kv.incr(store, key, delta) -> int`: atomically add delta to
// the numeric value at key and return the new value, or -1 when the key is absent
// (it is not created - mirroring memcache). `delta` is signed, so a negative
// delta decrements (there is no separate `decr` - one signed verb). Unlike
// memcached's DECR it does NOT floor at 0; a counter may go negative. A
// non-numeric stored value errors.
func (r *registry) incrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("kv.incr expects 3 arguments (store, key, delta), got %d", len(args))
	}
	s, err := r.resolve("kv.incr", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := stringArg("kv.incr", "key", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	delta, err := intArg("kv.incr", "delta", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := live(s, key, now)
	if !ok {
		return interpreter.IntVal(-1), nil
	}
	cur, perr := strconv.ParseInt(e.value, 10, 64)
	if perr != nil {
		return interpreter.Null(), fmt.Errorf("kv.incr: value at %q is not a number", key)
	}
	// Reject int64 overflow rather than wrapping silently - Jennifer's integer
	// arithmetic errors on overflow, and kv.incr must match that stance.
	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return interpreter.Null(), fmt.Errorf("kv.incr: %q overflows int64", key)
	}
	cur += delta
	e.value = strconv.FormatInt(cur, 10)
	s.data[key] = e
	s.maybeSweepLocked(now)
	if err := s.flushLocked(); err != nil {
		return interpreter.Null(), fmt.Errorf("kv.incr: %v", err)
	}
	return interpreter.IntVal(cur), nil
}

// closeFn implements `kv.close(store)`: drop the store and free its handle.
// Idempotent (closing an already-closed store is a no-op).
func (r *registry) closeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("kv.close expects 1 argument (store), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindStruct || args[0].StructNS != LibraryName || args[0].StructName != "Store" {
		return interpreter.Null(), fmt.Errorf("kv.close: argument must be a kv.Store, got %s", args[0].Kind)
	}
	for _, f := range args[0].Fields {
		if f.Name == "id" && f.Value.Kind == interpreter.KindInt {
			r.mu.Lock()
			delete(r.stores, f.Value.Int)
			r.mu.Unlock()
		}
	}
	return interpreter.Null(), nil
}
