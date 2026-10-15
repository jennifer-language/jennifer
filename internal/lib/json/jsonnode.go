// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// The json.Value tree is the shared compact document tree (internal/lib/
// doctree): a 40-byte node per JSON value, map members in document order with
// a sorted key index above 8 members, strings sliced from the input. doctree
// also supplies the accessors (`typeOf get has keys length as* isNull`) and the
// non-mutating write surface (`map list set insert append remove move`),
// addressed by JSON Pointer (RFC 6901); this package adds only the decoder and
// the encoder.
package jsonlib

import (
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// node is a json.Value tree node.
type node = doctree.Node

// lib is the json library's handle on the shared surface. A time.Time is an
// ordinary struct to JSON (no TimeNode), stored as a map of its fields.
var lib = &doctree.Lib{Name: LibraryName, HasNull: true, Display: display}

// display renders a json.Value as its compact JSON, so `$v` at the REPL and
// `%v` show the document rather than an opaque `<json.Value>`.
func display(n node) (string, error) {
	var sb strings.Builder
	if err := encodeNode(&sb, n, false, 0); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// wrap makes a json.Value handle over n.
func wrap(n node) interpreter.Value { return lib.Wrap(n) }

// asNode returns the node a json.Value handle holds.
func asNode(v interpreter.Value) (node, bool) { return lib.AsNode(v) }
