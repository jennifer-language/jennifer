// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// emitNode writes the JSON form of any AST node into b, with the given
// indent level (each level = two spaces). The format mirrors the AST
// struct shapes so the output is one-to-one with what the parser built;
// every node carries a "type" tag and source position. Hand-rolled (no
// reflect / no encoding/json) so the dump path stays TinyGo-friendly.
func emitNode(b *strings.Builder, n parser.Node, indent int) {
	switch v := n.(type) {
	case *parser.Program:
		startObj(b, indent)
		emitTypeAndPos(b, "Program", v, indent+1)
		emitNodeListField(b, "imports", asNodes(v.Imports), indent+1)
		emitNodeListField(b, "methods", asNodes(v.Methods), indent+1)
		emitStmtListField(b, "topLevel", v.TopLevel, indent+1)
		endObj(b, indent)

	case *parser.ImportStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "ImportStmt", v, indent+1)
		emitStringField(b, "name", v.Name, indent+1)
		if v.AsName != "" {
			emitStringField(b, "as", v.AsName, indent+1)
		}
		endObj(b, indent)

	case *parser.MethodDef:
		startObj(b, indent)
		emitTypeAndPos(b, "MethodDef", v, indent+1)
		emitStringField(b, "name", v.Name, indent+1)
		emitParamsField(b, "params", v.Params, indent+1)
		emitNodeField(b, "body", v.Body, indent+1)
		endObj(b, indent)

	case *parser.Block:
		startObj(b, indent)
		emitTypeAndPos(b, "Block", v, indent+1)
		emitStmtListField(b, "stmts", v.Stmts, indent+1)
		endObj(b, indent)

	case *parser.DefineStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "DefineStmt", v, indent+1)
		emitBoolField(b, "isConst", v.IsConst, indent+1)
		emitStringField(b, "varName", v.VarName, indent+1)
		emitStringField(b, "varType", v.VarType.String(), indent+1)
		if v.InitExpr != nil {
			emitNodeField(b, "init", v.InitExpr, indent+1)
		} else {
			emitNullField(b, "init", indent+1)
		}
		endObj(b, indent)

	case *parser.AssignStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "AssignStmt", v, indent+1)
		emitStringField(b, "varName", v.VarName, indent+1)
		emitNodeField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.IfStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "IfStmt", v, indent+1)
		emitNodeField(b, "cond", v.Cond, indent+1)
		emitNodeField(b, "then", v.Then, indent+1)
		emitNodeListField(b, "elseIfConds", asNodes(v.ElseIfs), indent+1)
		emitNodeListField(b, "elseIfBodies", asNodes(v.ElseIfBodies), indent+1)
		if v.Else != nil {
			emitNodeField(b, "else", v.Else, indent+1)
		} else {
			emitNullField(b, "else", indent+1)
		}
		endObj(b, indent)

	case *parser.WhileStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "WhileStmt", v, indent+1)
		emitNodeField(b, "cond", v.Cond, indent+1)
		emitNodeField(b, "body", v.Body, indent+1)
		endObj(b, indent)

	case *parser.ForStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "ForStmt", v, indent+1)
		emitOptionalNodeField(b, "init", v.Init, indent+1)
		emitOptionalNodeField(b, "cond", v.Cond, indent+1)
		emitOptionalNodeField(b, "step", v.Step, indent+1)
		emitNodeField(b, "body", v.Body, indent+1)
		endObj(b, indent)

	case *parser.ReturnStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "ReturnStmt", v, indent+1)
		emitOptionalNodeField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.ExprStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "ExprStmt", v, indent+1)
		emitNodeField(b, "expr", v.Expr, indent+1)
		endObj(b, indent)

	case *parser.IntLit:
		startObj(b, indent)
		emitTypeAndPos(b, "IntLit", v, indent+1)
		emitField(b, "value", strconv.FormatInt(v.Value, 10), indent+1)
		endObj(b, indent)

	case *parser.FloatLit:
		startObj(b, indent)
		emitTypeAndPos(b, "FloatLit", v, indent+1)
		emitField(b, "value", strconv.FormatFloat(v.Value, 'g', -1, 64), indent+1)
		endObj(b, indent)

	case *parser.StringLit:
		startObj(b, indent)
		emitTypeAndPos(b, "StringLit", v, indent+1)
		emitStringField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.BoolLit:
		startObj(b, indent)
		emitTypeAndPos(b, "BoolLit", v, indent+1)
		emitBoolField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.NullLit:
		startObj(b, indent)
		emitTypeAndPos(b, "NullLit", v, indent+1)
		endObj(b, indent)

	case *parser.VarExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "VarExpr", v, indent+1)
		emitStringField(b, "name", v.Name, indent+1)
		endObj(b, indent)

	case *parser.ConstRefExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "ConstRefExpr", v, indent+1)
		emitStringField(b, "name", v.Name, indent+1)
		endObj(b, indent)

	case *parser.CallExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "CallExpr", v, indent+1)
		emitStringField(b, "callee", v.Callee, indent+1)
		emitNodeListField(b, "args", asNodes(v.Args), indent+1)
		endObj(b, indent)

	case *parser.QualifiedCallExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "QualifiedCallExpr", v, indent+1)
		emitStringField(b, "prefix", v.Prefix, indent+1)
		emitStringField(b, "callee", v.Callee, indent+1)
		emitNodeListField(b, "args", asNodes(v.Args), indent+1)
		endObj(b, indent)

	case *parser.QualifiedConstRefExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "QualifiedConstRefExpr", v, indent+1)
		emitStringField(b, "prefix", v.Prefix, indent+1)
		emitStringField(b, "name", v.Name, indent+1)
		endObj(b, indent)

	case *parser.BinaryExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "BinaryExpr", v, indent+1)
		emitStringField(b, "op", v.Op.String(), indent+1)
		emitNodeField(b, "left", v.Left, indent+1)
		emitNodeField(b, "right", v.Right, indent+1)
		endObj(b, indent)

	case *parser.UnaryExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "UnaryExpr", v, indent+1)
		emitStringField(b, "op", v.Op.String(), indent+1)
		emitNodeField(b, "operand", v.Operand, indent+1)
		endObj(b, indent)

	case *parser.ListLit:
		startObj(b, indent)
		emitTypeAndPos(b, "ListLit", v, indent+1)
		emitNodeListField(b, "elements", asNodes(v.Elements), indent+1)
		endObj(b, indent)

	case *parser.MapLit:
		startObj(b, indent)
		emitTypeAndPos(b, "MapLit", v, indent+1)
		emitNodeListField(b, "keys", asNodes(v.Keys), indent+1)
		emitNodeListField(b, "values", asNodes(v.Values), indent+1)
		endObj(b, indent)

	case *parser.IndexExpr:
		startObj(b, indent)
		emitTypeAndPos(b, "IndexExpr", v, indent+1)
		emitNodeField(b, "target", v.Target, indent+1)
		emitNodeField(b, "index", v.Index, indent+1)
		endObj(b, indent)

	case *parser.AppendStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "AppendStmt", v, indent+1)
		emitNodeField(b, "target", v.Target, indent+1)
		emitNodeField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.IndexAssignStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "IndexAssignStmt", v, indent+1)
		emitNodeField(b, "target", v.Target, indent+1)
		emitNodeField(b, "value", v.Value, indent+1)
		endObj(b, indent)

	case *parser.ForEachStmt:
		startObj(b, indent)
		emitTypeAndPos(b, "ForEachStmt", v, indent+1)
		emitStringField(b, "varName", v.VarName, indent+1)
		emitNodeField(b, "coll", v.Coll, indent+1)
		emitNodeField(b, "body", v.Body, indent+1)
		endObj(b, indent)

	default:
		// Forward-compat: unknown node kind shows up as a small object with
		// just its Go type name. We hit this if a new AST node lands without
		// a case here; the test suite should fail loudly long before this
		// ever ships, but having a graceful fallback beats a panic.
		startObj(b, indent)
		writeIndent(b, indent+1)
		fmt.Fprintf(b, "\"type\": %q\n", fmt.Sprintf("%T", n))
		endObj(b, indent)
	}
}

// --- emitter primitives ---

const indentUnit = "  "

// fieldBuf records a pending field so we know whether to add a trailing
// comma to the previous entry when the next one is added. Each
// emit*Field call goes through emitField / multi-line helpers, all of
// which manage commas via b's tail.
//
// We keep this simple by always writing fields in a fixed order per node
// and tracking comma state with a helper that strips/appends as needed.
// startObj resets the per-object pending state.

func startObj(b *strings.Builder, indent int) {
	b.WriteByte('{')
	b.WriteByte('\n')
}

func endObj(b *strings.Builder, indent int) {
	// Drop the trailing ",\n" that the last field left behind, if any.
	trimTrailingComma(b)
	writeIndent(b, indent)
	b.WriteByte('}')
}

// trimTrailingComma removes a trailing ",\n" (the suffix every field
// emitter writes) so the closing `}` doesn't produce invalid JSON.
func trimTrailingComma(b *strings.Builder) {
	s := b.String()
	if strings.HasSuffix(s, ",\n") {
		// strings.Builder doesn't have a truncate method - rebuild.
		shortened := s[:len(s)-2] + "\n"
		b.Reset()
		b.WriteString(shortened)
	}
}

func writeIndent(b *strings.Builder, indent int) {
	for i := 0; i < indent; i++ {
		b.WriteString(indentUnit)
	}
}

// emitField writes `"key": rawValue,\n` at the given indent. `rawValue`
// is already in JSON form (e.g. a quoted string, a number, true/false,
// null, or a serialized object/array).
func emitField(b *strings.Builder, key, rawValue string, indent int) {
	writeIndent(b, indent)
	fmt.Fprintf(b, "%q: %s,\n", key, rawValue)
}

func emitStringField(b *strings.Builder, key, value string, indent int) {
	emitField(b, key, jsonString(value), indent)
}

func emitBoolField(b *strings.Builder, key string, value bool, indent int) {
	if value {
		emitField(b, key, "true", indent)
	} else {
		emitField(b, key, "false", indent)
	}
}

func emitNullField(b *strings.Builder, key string, indent int) {
	emitField(b, key, "null", indent)
}

func emitTypeAndPos(b *strings.Builder, kind string, n parser.Node, indent int) {
	line, col := n.Pos()
	emitStringField(b, "type", kind, indent)
	if file := n.Filename(); file != "" {
		emitStringField(b, "file", file, indent)
	}
	emitField(b, "line", strconv.Itoa(line), indent)
	emitField(b, "col", strconv.Itoa(col), indent)
}

// emitNodeField writes a "key": <object> pair for a single node value.
func emitNodeField(b *strings.Builder, key string, n parser.Node, indent int) {
	writeIndent(b, indent)
	fmt.Fprintf(b, "%q: ", key)
	emitNode(b, n, indent)
	// emitNode wrote the value but not the trailing comma+newline that
	// the field-emitter contract expects. Add them so endObj's trim works.
	b.WriteString(",\n")
}

// emitOptionalNodeField writes either the node's object form or "null".
// Used for optional AST positions (return.Value, for.Init/Cond/Step, etc.)
// where the parser may have left nil.
func emitOptionalNodeField(b *strings.Builder, key string, n parser.Node, indent int) {
	if n == nil {
		emitNullField(b, key, indent)
		return
	}
	// A typed-nil (e.g. (*parser.IfStmt)(nil)) survives the `== nil` test
	// above only for interface receivers; in our AST, optional fields are
	// declared as concrete pointer types, so the simple check is enough.
	emitNodeField(b, key, n, indent)
}

// emitNodeListField writes a JSON array of node objects. Empty arrays
// emit as `[]` on one line; non-empty arrays are pretty-printed.
func emitNodeListField(b *strings.Builder, key string, items []parser.Node, indent int) {
	writeIndent(b, indent)
	fmt.Fprintf(b, "%q: ", key)
	if len(items) == 0 {
		b.WriteString("[],\n")
		return
	}
	b.WriteString("[\n")
	for i, it := range items {
		writeIndent(b, indent+1)
		emitNode(b, it, indent+1)
		if i < len(items)-1 {
			b.WriteString(",\n")
		} else {
			b.WriteByte('\n')
		}
	}
	writeIndent(b, indent)
	b.WriteString("],\n")
}

func emitStmtListField(b *strings.Builder, key string, items []parser.Stmt, indent int) {
	nodes := make([]parser.Node, len(items))
	for i, s := range items {
		nodes[i] = s
	}
	emitNodeListField(b, key, nodes, indent)
}

func emitParamsField(b *strings.Builder, key string, params []parser.Param, indent int) {
	writeIndent(b, indent)
	fmt.Fprintf(b, "%q: ", key)
	if len(params) == 0 {
		b.WriteString("[],\n")
		return
	}
	b.WriteString("[\n")
	for i, p := range params {
		writeIndent(b, indent+1)
		b.WriteString("{\n")
		emitStringField(b, "name", p.Name, indent+2)
		emitStringField(b, "type", p.Type.String(), indent+2)
		if p.File != "" {
			emitStringField(b, "file", p.File, indent+2)
		}
		emitField(b, "line", strconv.Itoa(p.Line), indent+2)
		emitField(b, "col", strconv.Itoa(p.Col), indent+2)
		trimTrailingComma(b)
		writeIndent(b, indent+1)
		b.WriteByte('}')
		if i < len(params)-1 {
			b.WriteString(",\n")
		} else {
			b.WriteByte('\n')
		}
	}
	writeIndent(b, indent)
	b.WriteString("],\n")
}

// asNodes adapts a typed slice of AST nodes (e.g. []*parser.ImportStmt) to
// []parser.Node for the generic emitNodeListField path.
func asNodes[T parser.Node](items []T) []parser.Node {
	out := make([]parser.Node, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

// jsonString formats a Go string as a JSON-safe quoted literal. We don't
// reach for `encoding/json` here for TinyGo safety; the supported set
// matches what a Jennifer source can hold (control chars, quotes,
// backslashes; UTF-8 passes through verbatim).
func jsonString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
