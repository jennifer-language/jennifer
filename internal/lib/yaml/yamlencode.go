// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
	case doctree.KindFloat:
		return scalarNode("!!float", formatYamlFloat(n.FloatVal()))
	case doctree.KindString:
		// Tag `!!str` explicitly. Without it yaml.v3 emits a plain scalar, and a
		// string whose text resolves to another type (`"true"`, `"42"`, `"null"`)
		// comes back mistyped on the next decode. With the tag, yaml.v3 quotes
		// exactly those ambiguous values (and leaves ordinary strings plain), so
		// the round-trip preserves the string type.
		return scalarNode("!!str", n.Str())
	case doctree.KindBytes:
		return scalarNode("!!binary", base64.StdEncoding.EncodeToString([]byte(n.Str())))
	case doctree.KindDatetime:
		return scalarNode("!!timestamp", n.Str())
	case doctree.KindList:
		y := &yaml.Node{Kind: yaml.SequenceNode, Style: style, Content: make([]*yaml.Node, n.Len())}
		for i, e := range n.Elems() {
			y.Content[i] = toYaml(e, flow)
		}
		return y
	case doctree.KindMap:
		y := &yaml.Node{Kind: yaml.MappingNode, Style: style, Content: make([]*yaml.Node, 0, 2*n.Len())}
		for i, e := range n.Elems() {
			y.Content = append(y.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: n.Key(i)}, toYaml(e, flow))
		}
		return y
	}
	return scalarNode("!!null", "null")
}

// formatYamlFloat renders a float in YAML core-schema syntax (`.inf` / `-.inf` /
// `.nan`; a whole-valued float keeps a `.0` so it re-resolves as a float).
func formatYamlFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	case math.IsNaN(f):
		return ".nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !containsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func containsAny(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return true
			}
		}
	}
	return false
}
