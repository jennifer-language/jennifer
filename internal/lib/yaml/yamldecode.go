// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// YAML decode: parse text into the opaque yaml.Value tree. The parse itself is
// delegated to gopkg.in/yaml.v3 (anchors / aliases, flow and block styles,
// implicit typing, and multi-document streams are impractical to hand-roll and
// have no Go stdlib equivalent - the one place a config parser earns a
// dependency), then the resulting yaml.Node tree is walked into the shared
// compact document tree the json / toml libraries use (internal/lib/doctree),
// so the read / write surface is literally shared. Mapping order is preserved (Jennifer's
// map is insertion-ordered); scalar keys are taken in their source text form.
package yamllib

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	stdtime "time"

	"gopkg.in/yaml.v3"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// maxNestingDepth caps container recursion so a deeply-nested (or alias-nested)
// document cannot overflow the Go stack - a Go stack overflow is fatal, not a
// catchable Jennifer error. It is the limit every decoder shares, set per
// binary (internal/limits): aliases can nest a tree past what guardDepth saw
// in the text, and the tree's other walkers (encode, ==) rely on it.
const maxNestingDepth = limits.MaxNestingDepth

// maxNodes caps the number of tree nodes materialised from one decode. An alias
// bomb (a chain of anchors that each reference the previous one twice) expands
// exponentially when followed by value; this budget turns that into a bounded,
// catchable error rather than an out-of-memory kill. Shared with the other
// decoders. A var so tests can lower it.
var maxNodes = limits.MaxDecodedNodes

// maxDecodeBytes caps the raw input one decode accepts, checked before yaml.v3
// runs. The converter's maxNodes budget bounds the tree, but yaml.v3 builds
// its own yaml.Node tree for the whole document first, so without a byte cap a
// large flat document reaches multi-GB RSS (and a possible OOM kill) before the
// node budget is ever consulted. Sized at two bytes per node so the densest
// input (~2 bytes per node) cannot build far past maxNodes yaml.Nodes before the
// converter stops it: 4 MiB on the default binary, 512 KiB on jennifer-tiny; a
// larger document is a catchable error. Config YAML is KB-MB, so this never
// rejects a real one.
var maxDecodeBytes = 2 * limits.MaxDecodedNodes

// converter carries the per-decode budget counters.
type converter struct {
	nodes int
	// anchored holds each anchored node's conversion, so an alias shares it
	// instead of converting the subtree again: the tree is immutable, and a
	// document that reuses a block of defaults stays one copy in memory.
	anchored map[*yaml.Node]node
}

// decodeYaml parses a single-document YAML string into its root value. A stream
// with more than one document is an error pointing at decodeAll; an empty stream
// yields a null value.
func decodeYaml(src string) (node, error) {
	docs, err := decodeStream(src)
	if err != nil {
		return node{}, err
	}
	if len(docs) == 0 {
		return doctree.Null(), nil
	}
	if len(docs) > 1 {
		return node{}, fmt.Errorf("yaml.decode: input has %d documents; use yaml.decodeAll for a multi-document stream", len(docs))
	}
	return docs[0], nil
}

// decodeAllYaml parses every document of a YAML stream into a slice of root
// values (each later wrapped as a yaml.Value).
func decodeAllYaml(src string) ([]node, error) {
	return decodeStream(src)
}

// decodeStream runs the yaml.v3 streaming decoder and converts each document.
func decodeStream(src string) ([]node, error) {
	if len(src) > maxDecodeBytes {
		return nil, fmt.Errorf("yaml.decode: input is %d bytes, over the %d-byte limit (input rejected as a decode bomb)", len(src), maxDecodeBytes)
	}
	if err := guardDepth(src); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(src)))
	var out []node
	// One converter for the whole stream: its node budget bounds the decode,
	// not each document (a many-document file of alias bombs would otherwise
	// multiply the cap).
	c := &converter{}
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("yaml.decode: %v", err)
		}
		v, cerr := c.node(&doc, 0)
		if cerr != nil {
			return nil, cerr
		}
		out = append(out, v)
	}
	return out, nil
}

// isIntegerText reports whether s is a plain optionally-signed run of decimal
// digits (an integer literal, no '.' / exponent). Used to spot an integer that
// overflowed int64 and was re-tagged !!float, so its exact digits are kept.
func isIntegerText(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// node converts one yaml.Node into a tree node.
func (c *converter) node(n *yaml.Node, depth int) (node, error) {
	if depth > maxNestingDepth {
		return node{}, fmt.Errorf("yaml.decode: nesting too deep (limit %d)", maxNestingDepth)
	}
	c.nodes++
	if c.nodes > maxNodes {
		return node{}, fmt.Errorf("yaml.decode: document expands to too many nodes (limit %d); possible alias bomb", maxNodes)
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return doctree.Null(), nil
		}
		return c.node(n.Content[0], depth+1)
	case yaml.AliasNode:
		if n.Alias == nil {
			return node{}, fmt.Errorf("yaml.decode: alias %q has no anchor", n.Value)
		}
		if v, ok := c.anchored[n.Alias]; ok {
			// The node budget counts the expansion, as though it were
			// converted again (one node is already counted above).
			if size := v.ExpandedSize(); size > uint64(maxNodes) || c.nodes+int(size)-1 > maxNodes {
				return node{}, fmt.Errorf("yaml.decode: document expands to too many nodes (limit %d); possible alias bomb", maxNodes)
			} else {
				c.nodes += int(size) - 1
			}
			if depth+v.Height() > maxNestingDepth {
				return node{}, fmt.Errorf("yaml.decode: nesting too deep (limit %d)", maxNestingDepth)
			}
			return v, nil
		}
		return c.node(n.Alias, depth+1)
	case yaml.MappingNode, yaml.SequenceNode, yaml.ScalarNode:
		v, err := c.value(n, depth)
		if err == nil && n.Anchor != "" {
			if c.anchored == nil {
				c.anchored = map[*yaml.Node]node{}
			}
			c.anchored[n] = v
		}
		return v, err
	case 0:
		// A zero Node comes from an empty document.
		return doctree.Null(), nil
	default:
		return node{}, fmt.Errorf("yaml.decode: unsupported node kind %d", n.Kind)
	}
}

// value converts a mapping, sequence or scalar. A collection carrying a core
// tag of another kind (`!!map [1]`, `!!str {a: 1}`) is an error, not a value
// of whichever kind the text happened to be.
func (c *converter) value(n *yaml.Node, depth int) (node, error) {
	tag := n.ShortTag()
	switch n.Kind {
	case yaml.MappingNode:
		if strings.HasPrefix(tag, "!!") && tag != "!!map" && tag != "!!set" {
			return node{}, fmt.Errorf("yaml.decode: line %d: a mapping tagged %s", n.Line, tag)
		}
		return c.mapping(n, depth)
	case yaml.SequenceNode:
		if strings.HasPrefix(tag, "!!") && tag != "!!seq" && tag != "!!omap" && tag != "!!pairs" {
			return node{}, fmt.Errorf("yaml.decode: line %d: a sequence tagged %s", n.Line, tag)
		}
		return c.sequence(n, depth)
	}
	if tag == "!!map" || tag == "!!seq" || tag == "!!set" || tag == "!!omap" || tag == "!!pairs" {
		return node{}, fmt.Errorf("yaml.decode: line %d: a scalar tagged %s", n.Line, tag)
	}
	return c.scalar(n)
}

func (c *converter) mapping(n *yaml.Node, depth int) (node, error) {
	keys := make([]string, 0, len(n.Content)/2)
	elems := make([]node, 0, len(n.Content)/2)
	seen := map[string]bool{}
	var merges []*yaml.Node // deferred `<<` merge-source value nodes, in order
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		// A `<<` merge key pulls another mapping's entries into this one; its
		// value is deferred so the mapping's own keys always take precedence.
		if keyNode.ShortTag() == "!!merge" {
			merges = append(merges, n.Content[i+1])
			continue
		}
		if keyNode.Kind == yaml.AliasNode && keyNode.Alias != nil {
			keyNode = keyNode.Alias
		}
		if keyNode.Kind != yaml.ScalarNode {
			return node{}, fmt.Errorf("yaml.decode: mapping key must be scalar, got node kind %d", keyNode.Kind)
		}
		// Duplicate keys would produce a map with a repeated key, breaking the
		// unique-key invariant; reject them (as json / toml / xml decode do).
		if seen[keyNode.Value] {
			return node{}, fmt.Errorf("yaml.decode: duplicate mapping key %q", keyNode.Value)
		}
		val, err := c.node(n.Content[i+1], depth+1)
		if err != nil {
			return node{}, err
		}
		seen[keyNode.Value] = true
		keys = append(keys, keyNode.Value)
		elems = append(elems, val)
	}
	// Apply merges: an explicit key always wins over a merged one, and an
	// earlier merge source wins over a later one.
	for _, m := range merges {
		sources, err := c.mergeSources(m, depth)
		if err != nil {
			return node{}, err
		}
		for _, srcMap := range sources {
			for i, e := range srcMap.Elems() {
				k := srcMap.Key(i)
				if seen[k] {
					continue
				}
				seen[k] = true
				keys = append(keys, k)
				elems = append(elems, e)
			}
		}
	}
	return doctree.Map(keys, elems), nil
}

// mergeSources resolves a `<<` merge value into the mappings it contributes: a
// single mapping (an alias or an inline map), or a sequence of them.
func (c *converter) mergeSources(n *yaml.Node, depth int) ([]node, error) {
	nn := n
	if nn.Kind == yaml.AliasNode && nn.Alias != nil {
		nn = nn.Alias
	}
	if nn.Kind == yaml.SequenceNode {
		out := make([]node, 0, len(nn.Content))
		for _, e := range nn.Content {
			v, err := c.node(e, depth+1)
			if err != nil {
				return nil, err
			}
			if v.Kind() != doctree.KindMap {
				return nil, fmt.Errorf("yaml.decode: merge (<<) source must be a mapping, got a %s", v.TypeName())
			}
			out = append(out, v)
		}
		return out, nil
	}
	v, err := c.node(nn, depth+1)
	if err != nil {
		return nil, err
	}
	if v.Kind() != doctree.KindMap {
		return nil, fmt.Errorf("yaml.decode: merge (<<) source must be a mapping, got a %s", v.TypeName())
	}
	return []node{v}, nil
}

func (c *converter) sequence(n *yaml.Node, depth int) (node, error) {
	elems := make([]node, 0, len(n.Content))
	for _, child := range n.Content {
		v, err := c.node(child, depth+1)
		if err != nil {
			return node{}, err
		}
		elems = append(elems, v)
	}
	return doctree.List(elems), nil
}

// scalar resolves a scalar node to a typed tree node using the tag yaml.v3
// resolved (implicit typing) or the one written on it. A value whose text does
// not fit its explicit tag (`!!int abc`, `!!binary @@@`) is an error, not the
// text as a string. A custom tag (`!Ref x`) has no meaning here: its scalar
// is the plain string.
func (c *converter) scalar(n *yaml.Node) (node, error) {
	bad := func(what string) (node, error) {
		return node{}, fmt.Errorf("yaml.decode: line %d: %q is not %s", n.Line, n.Value, what)
	}
	switch n.ShortTag() {
	case "!!null":
		switch n.Value {
		case "", "~", "null", "Null", "NULL":
			return doctree.Null(), nil
		}
		return bad("a null")
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return bad("a bool")
		}
		return doctree.Bool(b), nil
	case "!!int":
		if err := checkCoreNumber(n); err != nil {
			return node{}, err
		}
		var i int64
		if err := n.Decode(&i); err == nil {
			return doctree.Int(i), nil
		}
		// Too big for int64: keep the verbatim digits as a string rather than a
		// float. A float64 cannot hold a 20+-digit integer exactly, so a float
		// fallback would silently corrupt an id / counter; the string keeps every
		// digit and the caller can convert deliberately if it wants a number.
		if isIntegerText(n.Value) {
			return doctree.String(n.Value), nil
		}
		return bad("an integer that fits 64 bits")
	case "!!float":
		if err := checkCoreNumber(n); err != nil {
			return node{}, err
		}
		// yaml.v3 re-tags an integer too large for int64 as !!float. Such a value
		// is all digits (no '.' / exponent); keep its exact text as a string
		// rather than a lossy float64 - a 20+-digit id or counter must not
		// silently change value. Genuine floats (with '.' / 'e') decode normally.
		if isIntegerText(n.Value) {
			if _, err := strconv.ParseInt(n.Value, 10, 64); err != nil {
				return doctree.String(n.Value), nil
			}
		}
		var f float64
		if err := n.Decode(&f); err != nil {
			return bad("a float")
		}
		return doctree.Float(f), nil
	case "!!timestamp":
		var t stdtime.Time
		if err := n.Decode(&t); err != nil {
			return bad("a timestamp")
		}
		return doctree.Datetime(n.Value, 0), nil
	case "!!binary":
		// A single !!binary node cannot be decoded through Node.Decode, so
		// base64-decode the payload directly (whitespace in a block scalar is
		// stripped first).
		payload := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				return -1
			}
			return r
		}, n.Value)
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return bad("base64 (for !!binary)")
		}
		return doctree.Bytes(data), nil
	default:
		return doctree.String(n.Value), nil
	}
}

// checkCoreNumber refuses the number spellings yaml.v3 still reads the YAML
// 1.1 way although YAML 1.2 (which this library follows) reads them
// differently or not at all: a leading-zero integer (`012` is octal 10 in 1.1,
// decimal 12 in 1.2), `0b` binary, `_` digit separators, an upper-case or
// signed base prefix. Picking either reading silently would make the same file
// mean different things to different parsers - a zip code `01234` became 668 -
// so it is an error that names the fix.
func checkCoreNumber(n *yaml.Node) error {
	s := n.Value
	body := strings.TrimLeft(s, "+-")
	signed := len(body) != len(s)
	switch {
	case strings.Contains(s, "_"):
	case len(body) > 1 && body[0] == '0' && body[1] >= '0' && body[1] <= '9' && !strings.ContainsAny(body, ".eE"):
	case len(body) > 1 && body[0] == '0' && strings.ContainsRune("bBXO", rune(body[1])):
	case signed && len(body) > 1 && body[0] == '0' && (body[1] == 'x' || body[1] == 'o'):
	default:
		return nil
	}
	return fmt.Errorf("yaml.decode: line %d: %q is a number only in YAML 1.1 and reads differently in YAML 1.2; "+
		"quote it for a string, or write it as a plain decimal, 0x hex or 0o octal number", n.Line, s)
}
