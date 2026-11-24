// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// YAML encode: render a yaml.Value (or any value that converts to one) back to
// YAML text. A yaml.Node tree is built from the document so key order is
// preserved and each scalar carries its resolved tag, then gopkg.in/yaml.v3
// marshals it. `yaml.encode` emits compact flow style (`{a: 1, b: [x, y]}`);
// `yaml.encodePretty` emits the readable block style - the flow / block
// distinction is YAML's own analogue of json's compact / pretty pair.
package yamllib

import (
	"encoding/base64"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// encodeYaml renders v as a YAML document. A plain value converts as it would
// be stored (a struct as a mapping over its fields, a time.Time as a
// timestamp), except that `bytes` stays binary and becomes a `!!binary`
// scalar. flow selects compact flow style over block style.
func encodeYaml(v interpreter.Value, flow bool) (string, error) {
	n, err := lib.ToNode(v, true)
	if err != nil {
		return "", err
	}
	return encodeNode(n, flow)
}

// encodeNode renders a tree node as a YAML document.
func encodeNode(n node, flow bool) (string, error) {
	out, err := yaml.Marshal(toYaml(n, flow))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// scalarNode builds a scalar yaml.Node with an explicit tag and value.
func scalarNode(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

// toYaml converts a tree node into a yaml.Node. Recursion is bounded by the
// tree's nesting cap (decode and every write enforce it).
func toYaml(n node, flow bool) *yaml.Node {
	style := yaml.Style(0)
	if flow {
		style = yaml.FlowStyle
	}
	switch n.Kind() {
	case doctree.KindBool:
		if n.BoolVal() {
			return scalarNode("!!bool", "true")
		}
		return scalarNode("!!bool", "false")
	case doctree.KindInt:
		return scalarNode("!!int", strconv.FormatInt(n.IntVal(), 10))
	case doctree.KindBigInt:
		// Plain untagged digits: a YAML 1.2 core-schema integer of any size,
		// as other parsers read it. yaml.v3 itself resolves such digits to
		// !!float or !!str, so neither a tag nor its resolution may be used
		// (an !!int tag would be written out); decode reads them back exactly.
		return scalarNode("", n.Str())
	case doctree.KindFloat:
		return scalarNode("!!float", formatYamlFloat(n.FloatVal()))
	case doctree.KindString:
		// Tag `!!str` explicitly. Without it yaml.v3 emits a plain scalar, and a
		// string whose text resolves to another type (`"true"`, `"42"`, `"null"`)
		// comes back mistyped on the next decode. With the tag, yaml.v3 quotes
		// exactly those ambiguous values (and leaves ordinary strings plain), so
		// the round-trip preserves the string type.
		y := scalarNode("!!str", n.Str())
		if literalUnsafe(y.Value) {
			y.Style = yaml.DoubleQuotedStyle
		}
		return y
	case doctree.KindBytes:
		return scalarNode("!!binary", base64.StdEncoding.EncodeToString([]byte(n.Str())))
	case doctree.KindDatetime:
		y := scalarNode("!!timestamp", n.Str())
		if flow && strings.IndexByte(y.Value, ':') >= 0 {
			// A ':' keeps yaml.v3 from writing a plain scalar in flow
			// context, and a quoted one untagged reads back as a string:
			// write the tag (`!!timestamp '2001-12-14T21:59:43Z'`).
			y.Style = yaml.TaggedStyle
		}
		return y
	case doctree.KindList:
		y := &yaml.Node{Kind: yaml.SequenceNode, Style: style, Content: make([]*yaml.Node, n.Len())}
		for i, e := range n.Elems() {
			y.Content[i] = toYaml(e, flow)
		}
		return y
	case doctree.KindMap:
		y := &yaml.Node{Kind: yaml.MappingNode, Style: style, Content: make([]*yaml.Node, 0, 2*n.Len())}
		for i, e := range n.Elems() {
			k := &yaml.Node{Kind: yaml.ScalarNode, Value: n.Key(i)}
			if k.Value == "<<" {
				// A plain `<<` key is a merge key on decode (a tag does not
				// change how yaml.v3 emits it); quoted, it stays a string.
				k.Style = yaml.DoubleQuotedStyle
			}
			y.Content = append(y.Content, k, toYaml(e, flow))
		}
		return y
	}
	return scalarNode("!!null", "null")
}

// literalUnsafe reports a multi-line string that yaml.v3 would emit as a
// literal block scalar it cannot read back: one starting with a line break
// (the leading empty line is dropped), with a space (the indentation
// indicator it writes is wrong below the top level), or holding a tab (a
// whitespace-only line with a tab is refused as indentation). Such a string
// is double-quoted instead, which round-trips any text.
func literalUnsafe(s string) bool {
	return strings.IndexByte(s, '\n') >= 0 && (s[0] == '\n' || s[0] == ' ' || strings.IndexByte(s, '\t') >= 0)
}

// formatYamlFloat renders a float in YAML core-schema syntax: `.inf` / `-.inf`
// / `.nan`, else doctree.FormatFloat (`1000000.0`, `1e-7`).
func formatYamlFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	case math.IsNaN(f):
		return ".nan"
	}
	return doctree.FormatFloat(f)
}
