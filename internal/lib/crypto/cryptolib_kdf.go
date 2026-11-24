// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// crypto.deriveKey(algo, secret, salt, length, crypto.KdfParams) -> bytes:
// one function for every key derivation, the algorithm first, like encrypt,
// sign and keypair. The hash of HKDF and PBKDF2 is part of the name
// ("hkdf-sha256"), as in sign("rsa-pss-sha256", ...); the costs go in KdfParams.
//
// There are no defaults. A cost a release could raise or lower under a caller
// is the wrong thing to leave implicit in a KDF, so every parameter the
// algorithm needs must be set, and one it does not use must stay zero.
//
// The checks and caps are those of the four functions this replaces
// (hkdf, pbkdf2, argon2id, scrypt, deprecated in 0.27.0): each algorithm runs
// through their cores, which report under "crypto.deriveKey (<algo>)".

package cryptolib

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// kdfParamFields is crypto.KdfParams in declaration order.
var kdfParamFields = []parser.StructField{
	{Name: "info", Type: parser.PrimitiveType(parser.TypeBytes)},
	{Name: "iterations", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "time", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "memoryKiB", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "threads", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "n", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "r", Type: parser.PrimitiveType(parser.TypeInt)},
	{Name: "p", Type: parser.PrimitiveType(parser.TypeInt)},
}

// kdfSpec says which KdfParams an algorithm takes: required ones must be set,
// optional ones may be, every other field must stay zero.
type kdfSpec struct {
	hash     string // HKDF / PBKDF2 hash, "" for argon2id and scrypt
	required []string
	optional []string
}

// kdfSpecs holds every deriveKey name: "hkdf-<hash>" and "pbkdf2-<hash>" for
// each of kdfHashNames, then argon2id and scrypt.
var kdfSpecs = func() map[string]kdfSpec {
	m := map[string]kdfSpec{
		"argon2id": {required: []string{"time", "memoryKiB", "threads"}},
		"scrypt":   {required: []string{"n", "r", "p"}},
	}
	for _, h := range kdfHashNames {
		m["hkdf-"+h] = kdfSpec{hash: h, optional: []string{"info"}}
		m["pbkdf2-"+h] = kdfSpec{hash: h, required: []string{"iterations"}}
	}
	return m
}()

// kdfAlgoNames is the known-name list of deriveKey: the two hash families in
// one entry each, then the hashes they take.
func kdfAlgoNames() string {
	return `"hkdf-<hash>", "pbkdf2-<hash>", "argon2id", "scrypt"; <hash> is one of ` + quoteList(kdfHashNames)
}

func deriveKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	const verb = "crypto.deriveKey"
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("%s expects 5 arguments (algo, secret, salt, length, crypto.KdfParams), got %d", verb, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("%s: algo must be string, got %s", verb, args[0].Kind)
	}
	algo := args[0].Str
	spec, ok := kdfSpecs[algo]
	if algo == "hkdf-md5" || algo == "pbkdf2-md5" {
		return interpreter.Null(), fmt.Errorf("%s: %q: %s", verb, algo, md5KDF)
	}
	if !ok {
		return interpreter.Null(), fmt.Errorf("%s: unknown algorithm %q; known: %s", verb, algo, kdfAlgoNames())
	}
	fn := fmt.Sprintf("%s (%s)", verb, algo)
	if args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: secret and salt must be bytes", fn)
	}
	if args[3].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("%s: length must be int, got %s", fn, args[3].Kind)
	}
	pv := args[4]
	if pv.Kind != interpreter.KindStruct || pv.StructNS != LibraryName || pv.StructName != "KdfParams" {
		return interpreter.Null(), fmt.Errorf("%s: params must be a crypto.KdfParams, got %s", fn, pv.Kind)
	}
	params := map[string]interpreter.Value{}
	for _, f := range pv.Fields {
		params[f.Name] = f.Value
	}
	isSet := func(name string) bool {
		v := params[name]
		if v.Kind == interpreter.KindBytes {
			return len(v.Bytes) > 0
		}
		return v.Kind == interpreter.KindInt && v.Int != 0
	}
	takes := map[string]bool{}
	for _, n := range spec.required {
		takes[n] = true
		if !isSet(n) {
			return interpreter.Null(), fmt.Errorf("%s: needs KdfParams.%s (there is no default)", fn, n)
		}
	}
	for _, n := range spec.optional {
		takes[n] = true
	}
	for _, f := range kdfParamFields {
		if !takes[f.Name] && isSet(f.Name) {
			return interpreter.Null(), fmt.Errorf("%s: does not take KdfParams.%s; leave it out", fn, f.Name)
		}
	}
	secret, salt, length := args[1], args[2], args[3]
	switch {
	case strings.HasPrefix(algo, "hkdf-"):
		info := params["info"]
		if info.Kind != interpreter.KindBytes {
			info = interpreter.BytesVal(nil)
		}
		return hkdfCore(fn, []interpreter.Value{interpreter.StringVal(spec.hash), secret, salt, info, length})
	case strings.HasPrefix(algo, "pbkdf2-"):
		return pbkdf2Core(fn, []interpreter.Value{interpreter.StringVal(spec.hash), secret, salt, params["iterations"], length})
	case algo == "argon2id":
		return argon2idCore(fn, []interpreter.Value{secret, salt, params["time"], params["memoryKiB"], params["threads"], length})
	default: // scrypt
		return scryptCore(fn, []interpreter.Value{secret, salt, params["n"], params["r"], params["p"], length})
	}
}
