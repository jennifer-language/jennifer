// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
	"strings"

	"gopkg.in/yaml.v3"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// maxNestingDepth caps container recursion so a deeply-nested (or alias-nested)
// document cannot overflow the Go stack - a Go stack overflow is fatal, not a
// catchable Jennifer error. 1000 is far beyond any legitimate document.
const maxNestingDepth = 1000

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

// maxParseDepth caps structural nesting *before* the input reaches yaml.v3.
// yaml.v3 is a recursive-descent parser that recurses once per nesting level as
// it builds the Node tree - and it does so on the Go stack of whichever
// goroutine calls Decode. Under jennifer-tiny (a fixed 2 MiB stack) that
// overflows and a Go stack overflow is a fatal, uncatchable SIGSEGV, not a
// recoverable error - so a merely deeply-nested config file could crash the
// whole interpreter (measured: ~350-400 levels). The converter's own
// maxNestingDepth guard is too late (it runs after the parse), so guardDepth
// pre-scans the raw text and rejects excessive nesting with a normal, catchable
// decode error. 128 is far above any legitimate document and comfortably below
// the crash threshold.
const maxParseDepth = 128

// converter carries the per-decode budget counters.
type converter struct {
	nodes int
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

// guardDepth pre-scans raw YAML text for structural nesting that could overflow
// yaml.v3's parser stack, rejecting it with a normal decode error before the
// parse runs. It is a single non-recursive pass that tracks three depth sources:
//
//   - flow-collection depth: running count of unmatched `[` / `{`;
//   - block-mapping / nested-scalar depth: the number of strictly-increasing
//     indentation levels currently open (an indent stack);
//   - compact block-sequence depth: leading `- ` markers on a line, each of
//     which nests a sequence without any extra indentation (`- - - x`).
//
// It is deliberately conservative: it counts every `[` / `{` outside a quoted
// scalar or comment, so it can only ever *over*-estimate depth (rejecting a
// pathological document), never under-estimate it (letting a crash through).
func guardDepth(src string) error {
	flow := 0               // running [ { depth (multi-line flow persists)
	var indents []int       // indentation columns of open block levels (increasing)
	blockLevels := 0        // depth contribution of the current line (indents + dashes)
	blockScalarIndent := -1 // header indent while inside a |/> block scalar, else -1
	atLineStart := true
	i, n := 0, len(src)

	tooDeep := fmt.Errorf("yaml.decode: nesting too deep (limit %d); refusing to parse to avoid a stack overflow", maxParseDepth)

	for i < n {
		if atLineStart && flow == 0 {
			indent := 0
			for i < n && src[i] == ' ' {
				indent++
				i++
			}
			// Inside a |/> block scalar, its more-indented lines (and blank lines)
			// are literal content, not structure: skip them without counting, so
			// a scalar whose text happens to deepen in indentation is not mistaken
			// for nesting. A non-blank line no deeper than the header ends it. This
			// cannot hide real nesting: yaml.v3 never parses structure in the lines
			// following a |/> indicator - they are scalar content or continuation.
			if blockScalarIndent >= 0 {
				blank := i >= n || src[i] == '\n' || src[i] == '\r'
				if blank || indent > blockScalarIndent {
					for i < n && src[i] != '\n' {
						i++
					}
					if i < n {
						i++
					}
					continue
				}
				blockScalarIndent = -1
			}
			// A blank or comment-only line does not change block structure.
			if i >= n || src[i] == '\n' || src[i] == '\r' || src[i] == '#' {
				for i < n && src[i] != '\n' {
					i++
				}
				if i < n {
					i++
				}
				continue
			}
			// Adjust the indent stack to this line's indentation.
			for len(indents) > 0 && indents[len(indents)-1] > indent {
				indents = indents[:len(indents)-1]
			}
			if len(indents) == 0 || indents[len(indents)-1] < indent {
				indents = append(indents, indent)
			}
			blockLevels = len(indents)
			// Count leading compact block-sequence markers (`- ` each nests one
			// more sequence without further indentation).
			for i < n && src[i] == '-' && (i+1 >= n || src[i+1] == ' ' || src[i+1] == '\n' || src[i+1] == '\r') {
				blockLevels++
				i++
				for i < n && src[i] == ' ' {
					i++
				}
			}
			if blockLevels+flow > maxParseDepth {
				return tooDeep
			}
			// If this line's value is a |/> block scalar, its indented content
			// follows; enter block-scalar mode so those lines are skipped.
			if introducesBlockScalar(src, i, n) {
				blockScalarIndent = indent
				for i < n && src[i] != '\n' {
					i++
				}
				if i < n {
					i++
				}
				continue
			}
			atLineStart = false
			continue
		}

		switch src[i] {
		case '\n':
			atLineStart = true
			i++
		case '#':
			// A comment runs to end of line, but `#` is a comment only at line
			// start or after whitespace (elsewhere it is scalar content).
			if i == 0 || src[i-1] == ' ' || src[i-1] == '\t' || src[i-1] == '\n' {
				for i < n && src[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '\'':
			i++
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '"':
			i++
			for i < n {
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
		case '[', '{':
			flow++
			if blockLevels+flow > maxParseDepth {
				return tooDeep
			}
			i++
		case ']', '}':
			if flow > 0 {
				flow--
			}
			i++
		default:
			i++
		}
	}
	return nil
}

// introducesBlockScalar reports whether the line beginning at `start` (already
// past its indentation and any block-sequence dashes) ends with a block-scalar
// header indicator: `|` or `>`, optionally followed by an indentation digit and
// a chomping `+`/`-`, then optional trailing comment. It skips over quoted
// scalars and a trailing comment so a `|` inside a value is not mistaken for the
// indicator. Used only to mark a scalar's following lines as content; it never
// needs to be exact, because a line ending in `|`/`>` never has real nesting
// after it, so a loose match cannot let deep nesting slip past the guard.
func introducesBlockScalar(src string, start, n int) bool {
	i := start
	contentEnd := start // one past the last non-space, non-comment character
	for i < n && src[i] != '\n' && src[i] != '\r' {
		switch src[i] {
		case '\'':
			i++
			for i < n && src[i] != '\n' {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			contentEnd = i
		case '"':
			i++
			for i < n && src[i] != '\n' {
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			contentEnd = i
		case '#':
			if i == start || src[i-1] == ' ' || src[i-1] == '\t' {
				for i < n && src[i] != '\n' {
					i++
				}
			} else {
				i++
				contentEnd = i
			}
		case ' ', '\t':
			i++
		default:
			i++
			contentEnd = i
		}
	}
	j := contentEnd - 1
	if j < start {
		return false
	}
	for j > start && (src[j] >= '0' && src[j] <= '9' || src[j] == '+' || src[j] == '-') {
		j--
	}
	if src[j] != '|' && src[j] != '>' {
		return false
	}
	if j == start {
		return true
	}
	p := src[j-1]
	return p == ' ' || p == '\t' || p == ':'
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
		return c.node(n.Alias, depth+1)
	case yaml.MappingNode:
		return c.mapping(n, depth)
	case yaml.SequenceNode:
		return c.sequence(n, depth)
	case yaml.ScalarNode:
		return c.scalar(n)
	case 0:
		// A zero Node comes from an empty document.
		return doctree.Null(), nil
	default:
		return node{}, fmt.Errorf("yaml.decode: unsupported node kind %d", n.Kind)
	}
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

// scalar resolves a scalar node to a typed tree node using the tag yaml.v3 resolved
// (implicit typing). Unknown / custom tags fall back to the verbatim string.
func (c *converter) scalar(n *yaml.Node) (node, error) {
	switch n.ShortTag() {
	case "!!null":
		return doctree.Null(), nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return doctree.String(n.Value), nil
		}
		return doctree.Bool(b), nil
	case "!!int":
		var i int64
		if err := n.Decode(&i); err == nil {
			return doctree.Int(i), nil
		}
		// Too big for int64: keep the verbatim digits as a string rather than a
		// float. A float64 cannot hold a 20+-digit integer exactly, so a float
		// fallback would silently corrupt an id / counter; the string keeps every
		// digit and the caller can convert deliberately if it wants a number.
		return doctree.String(n.Value), nil
	case "!!float":
		// yaml.v3 re-tags an integer too large for int64 as !!float. Such a value
		// is all digits (no '.' / exponent); keep its exact text as a string
		// rather than a lossy float64 - a 20+-digit id or counter must not
		// silently change value. Genuine floats (with '.' / 'e') decode normally.
		if isIntegerText(n.Value) {
			return doctree.String(n.Value), nil
		}
		var f float64
		if err := n.Decode(&f); err != nil {
			return doctree.String(n.Value), nil
		}
		return doctree.Float(f), nil
	case "!!timestamp":
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
			return doctree.String(n.Value), nil
		}
		return doctree.Bytes(data), nil
	default:
		return doctree.String(n.Value), nil
	}
}
