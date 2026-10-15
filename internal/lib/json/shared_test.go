// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

// The shared accessor and write builtins (internal/lib/doctree), bound under
// the json namespace, as this package's tests call them.
var (
	typeOfFn   = lib.Builtin("typeOf")
	getFn      = lib.Builtin("get")
	hasFn      = lib.Builtin("has")
	keysFn     = lib.Builtin("keys")
	lengthFn   = lib.Builtin("length")
	asIntFn    = lib.Builtin("asInt")
	asFloatFn  = lib.Builtin("asFloat")
	asStringFn = lib.Builtin("asString")
	asBoolFn   = lib.Builtin("asBool")
	isNullFn   = lib.Builtin("isNull")
	listFn     = lib.Builtin("list")
	mapFn      = lib.Builtin("map")
	setFn      = lib.Builtin("set")
	insertFn   = lib.Builtin("insert")
	appendFn   = lib.Builtin("append")
	removeFn   = lib.Builtin("remove")
	moveFn     = lib.Builtin("move")
)
