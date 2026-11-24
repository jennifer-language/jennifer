// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// DecodeCaps are the caps of one decode: the nesting depth and the node count.
type DecodeCaps struct {
	Depth int
	Nodes int
}

// DecodeOptionFields are the fields every `NAME.DecodeOptions` has (json,
// toml, yaml, xml, asn1); a library appends its own (xml's namespaces).
func DecodeOptionFields(extra ...parser.StructField) []parser.StructField {
	return append([]parser.StructField{
		{Name: "maxDepth", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "maxNodes", Type: parser.PrimitiveType(parser.TypeInt)},
	}, extra...)
}

// ReadDecodeOptions reads a `ns.DecodeOptions` argument of fnName (a
// `decodeWith`) over the defaults def. The options only lower a cap: 0 keeps
// the default, a value above it or a negative one is an error ("runtime")
// naming the allowed range. Fields other than maxDepth / maxNodes are left to
// the caller.
func ReadDecodeOptions(fnName, ns string, o interpreter.Value, def DecodeCaps) (DecodeCaps, error) {
	if o.Kind != interpreter.KindStruct || o.StructNS != ns || o.StructName != "DecodeOptions" {
		return DecodeCaps{}, fmt.Errorf("%s: argument 2 must be a %s.DecodeOptions, got %s", fnName, ns, o.Kind)
	}
	caps := def
	for _, f := range o.Fields {
		var limit *int
		switch f.Name {
		case "maxDepth":
			limit = &caps.Depth
		case "maxNodes":
			limit = &caps.Nodes
		default:
			continue
		}
		if f.Value.Kind != interpreter.KindInt {
			return DecodeCaps{}, fmt.Errorf("%s: %s must be int, got %s", fnName, f.Name, f.Value.Kind)
		}
		v := f.Value.Int
		if v < 0 || v > int64(*limit) {
			return DecodeCaps{}, fmt.Errorf("%s: %s must be 0 (the default) to %d, got %d", fnName, f.Name, *limit, v)
		}
		if v > 0 {
			*limit = int(v)
		}
	}
	return caps, nil
}
