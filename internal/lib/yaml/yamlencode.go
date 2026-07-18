// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// YAML encode: render an interpreter Value (or a yaml.Value handle) back to YAML
// text. A yaml.Node tree is built from the value so key order is preserved and
// each scalar carries its resolved tag, then gopkg.in/yaml.v3 marshals it.
// `yaml.encode` emits compact flow style (`{a: 1, b: [x, y]}`); `yaml.encodePretty`
// emits the readable block style - the flow / block distinction is YAML's own
// analogue of json's compact / pretty pair.
package yamllib

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	stdtime "time"

	"gopkg.in/yaml.v3"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// encodeYaml renders v as a YAML document. flow selects compact flow style over
// block style.
func encodeYaml(v interpreter.Value, flow bool) (string, error) {
	if inner, ok := v.AsObject(LibraryName, "Value"); ok {
		v = inner
	}
	node, err := valueToNode(v, flow)
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// scalarNode builds a scalar yaml.Node with an explicit tag and value.
func scalarNode(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

// valueToNode converts an interpreter Value into a yaml.Node. A user struct is
// treated as a mapping over its fields (a time.Time becomes a timestamp), and
// `bytes` becomes a `!!binary` scalar.
func valueToNode(v interpreter.Value, flow bool) (*yaml.Node, error) {
	if inner, ok := v.AsObject(LibraryName, "Value"); ok {
		v = inner
	}
	switch v.Kind {
	case interpreter.KindNull:
		return scalarNode("!!null", "null"), nil
	case interpreter.KindBool:
		if v.Bool {
			return scalarNode("!!bool", "true"), nil
		}
		return scalarNode("!!bool", "false"), nil
	case interpreter.KindInt:
		return scalarNode("!!int", strconv.FormatInt(v.Int, 10)), nil
	case interpreter.KindFloat:
		return scalarNode("!!float", formatYamlFloat(v.Float)), nil
	case interpreter.KindString:
		// No tag: let yaml.v3 pick quoting so a string that looks like another
		// type (`"true"`, `"42"`) is quoted and round-trips as a string.
		return &yaml.Node{Kind: yaml.ScalarNode, Value: v.Str}, nil
	case interpreter.KindBytes:
		return scalarNode("!!binary", base64.StdEncoding.EncodeToString(v.Bytes)), nil
	case interpreter.KindList:
		style := yaml.Style(0)
		if flow {
			style = yaml.FlowStyle
		}
		n := &yaml.Node{Kind: yaml.SequenceNode, Style: style}
		for _, e := range v.List {
			child, err := valueToNode(e, flow)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, child)
		}
		return n, nil
	case interpreter.KindMap:
		return mapToNode(v.Map, flow)
	case interpreter.KindStruct:
		if text, ok := isDatetimeNode(v); ok {
			return scalarNode("!!timestamp", text), nil
		}
		if text, ok := timeToTimestamp(v); ok {
			return scalarNode("!!timestamp", text), nil
		}
		entries := make([]interpreter.MapEntry, len(v.Fields))
		for i, f := range v.Fields {
			entries[i] = interpreter.MapEntry{Key: interpreter.StringVal(f.Name), Value: f.Value}
		}
		return mapToNode(entries, flow)
	case interpreter.KindTask:
		return nil, fmt.Errorf("cannot encode a task value")
	default:
		return nil, fmt.Errorf("cannot encode value of kind %s", v.Kind)
	}
}

func mapToNode(entries []interpreter.MapEntry, flow bool) (*yaml.Node, error) {
	style := yaml.Style(0)
	if flow {
		style = yaml.FlowStyle
	}
	n := &yaml.Node{Kind: yaml.MappingNode, Style: style}
	for _, e := range entries {
		if e.Key.Kind != interpreter.KindString {
			return nil, fmt.Errorf("map key must be string, got %s", e.Key.Kind)
		}
		val, err := valueToNode(e.Value, flow)
		if err != nil {
			return nil, err
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: e.Key.Str}, val)
	}
	return n, nil
}

// timeToTimestamp turns a `time.Time` struct (nanos + offset) into a YAML
// timestamp text (RFC 3339).
func timeToTimestamp(v interpreter.Value) (string, bool) {
	if v.StructNS != "time" || v.StructName != "Time" {
		return "", false
	}
	var nanos, offset int64
	for _, f := range v.Fields {
		switch f.Name {
		case "nanos":
			nanos = f.Value.Int
		case "offset":
			offset = f.Value.Int
		}
	}
	loc := stdtime.FixedZone("", int(offset))
	return stdtime.Unix(0, nanos).In(loc).Format(stdtime.RFC3339Nano), true
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
