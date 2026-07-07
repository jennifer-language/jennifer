// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package testinglib implements Jennifer's `testing` library
// (M16.4): the irreducible system-side surface a Jennifer-coded
// test framework needs. Five verbs (run / results / reset / report)
// plus the `testing.Result` struct. The Jennifer-coded assertion
// vocabulary and CLI harness live in the M18.x testing module and
// build on top of these primitives.
//
// Why it lives here (not in .j):
//
//   - `testing.run(name)` invokes a user method by name. Jennifer
//     has no function references / first-class methods today; a
//     pure .j runner can't take "the test body" as a value. This
//     library uses the interpreter's method registry directly via
//     Interpreter.CallByName (M16.4).
//   - `testing.run` also catches `exit` at the Go level. Language-
//     level `try`/`catch` deliberately does NOT catch `exit` (spec:
//     exit is program-level escape). Testing is the one place where
//     a runaway `exit` in a test body needs to be contained;
//     capturing it in Go keeps the escape scoped to the runner
//     without weakening the language guarantee.
package testinglib

import (
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	stdtime "time"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "testing"

// Value alias.
type Value = interpreter.Value

// -------- Registry --------

// resultRec mirrors testing.Result. Kept in Go form so serializing
// to text/tap/junit doesn't have to walk struct-field slices for
// every access.
type resultRec struct {
	Name         string
	Ms           int64
	Passed       bool
	ErrorKind    string
	ErrorMessage string
	File         string
	Line         int
	Col          int
}

var (
	resultsMu sync.Mutex
	results   []resultRec
)

// ResetForTest wipes the accumulator; exported for the _test package.
func ResetForTest() {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = nil
}

// -------- Install --------

// Install registers the M16.4 testing surface. Unlike most
// libraries, `testing.run` needs to invoke user methods; capture
// the interpreter reference so runFn can dispatch.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Result", []parser.StructField{
		{Name: "name", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "ms", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "passed", Type: parser.PrimitiveType(parser.TypeBool)},
		{Name: "errorKind", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "errorMessage", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "file", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "line", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "col", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	in.RegisterNamespaced(LibraryName, "run", makeRunFn(in))
	in.RegisterNamespaced(LibraryName, "results", resultsFn)
	in.RegisterNamespaced(LibraryName, "reset", resetFn)
	in.RegisterNamespaced(LibraryName, "report", reportFn)
}

// -------- Helpers --------

func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

func recToValue(r resultRec) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Result", []interpreter.StructField{
		{Name: "name", Value: interpreter.StringVal(r.Name)},
		{Name: "ms", Value: interpreter.IntVal(r.Ms)},
		{Name: "passed", Value: interpreter.BoolVal(r.Passed)},
		{Name: "errorKind", Value: interpreter.StringVal(r.ErrorKind)},
		{Name: "errorMessage", Value: interpreter.StringVal(r.ErrorMessage)},
		{Name: "file", Value: interpreter.StringVal(r.File)},
		{Name: "line", Value: interpreter.IntVal(int64(r.Line))},
		{Name: "col", Value: interpreter.IntVal(int64(r.Col))},
	})
}

func valueToRec(fnName string, v Value) (resultRec, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Result" {
		return resultRec{}, fmt.Errorf("%s: expected a testing.Result, got %s", fnName, v.Kind)
	}
	var r resultRec
	for _, f := range v.Fields {
		switch f.Name {
		case "name":
			r.Name = f.Value.Str
		case "ms":
			r.Ms = f.Value.Int
		case "passed":
			r.Passed = f.Value.Bool
		case "errorKind":
			r.ErrorKind = f.Value.Str
		case "errorMessage":
			r.ErrorMessage = f.Value.Str
		case "file":
			r.File = f.Value.Str
		case "line":
			r.Line = int(f.Value.Int)
		case "col":
			r.Col = int(f.Value.Int)
		}
	}
	return r, nil
}

// -------- Verbs --------

// makeRunFn captures the interpreter reference so the returned
// builtin can invoke user methods via CallByName.
func makeRunFn(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("testing.run expects 1 argument (name), got %d", len(args))
		}
		name, err := takeStringArg("testing.run", args, 0, "name")
		if err != nil {
			return interpreter.Null(), err
		}

		start := stdtime.Now()
		_, callErr := in.CallByName(name)
		elapsed := stdtime.Since(start).Milliseconds()

		rec := resultRec{
			Name: name,
			Ms:   elapsed,
		}
		if callErr == nil {
			rec.Passed = true
		} else {
			rec.Passed = false
			kind, msg, file, line, col := interpreter.ClassifyError(callErr)
			rec.ErrorKind = kind
			rec.ErrorMessage = msg
			rec.File = file
			rec.Line = line
			rec.Col = col
		}

		resultsMu.Lock()
		results = append(results, rec)
		resultsMu.Unlock()

		return recToValue(rec), nil
	}
}

func resultsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("testing.results expects 0 arguments, got %d", len(args))
	}
	resultsMu.Lock()
	snapshot := make([]resultRec, len(results))
	copy(snapshot, results)
	resultsMu.Unlock()

	out := make([]Value, len(snapshot))
	for i, r := range snapshot {
		out[i] = recToValue(r)
	}
	return interpreter.ListVal(
		parser.NamespacedStructType(LibraryName, "Result"), out,
	), nil
}

func resetFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("testing.reset expects 0 arguments, got %d", len(args))
	}
	resultsMu.Lock()
	results = nil
	resultsMu.Unlock()
	return interpreter.Null(), nil
}

func reportFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("testing.report expects 2 arguments (results, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("testing.report: results must be a list of testing.Result, got %s", args[0].Kind)
	}
	format, err := takeStringArg("testing.report", args, 1, "format")
	if err != nil {
		return interpreter.Null(), err
	}

	recs := make([]resultRec, len(args[0].List))
	for i, v := range args[0].List {
		r, err := valueToRec("testing.report", v)
		if err != nil {
			return interpreter.Null(), err
		}
		recs[i] = r
	}

	switch format {
	case "text":
		return interpreter.StringVal(renderText(recs)), nil
	case "tap":
		return interpreter.StringVal(renderTAP(recs)), nil
	case "junit":
		return interpreter.StringVal(renderJUnit(recs)), nil
	default:
		return interpreter.Null(), fmt.Errorf(`testing.report: unknown format %q; known: "text", "tap", "junit"`, format)
	}
}

// -------- Renderers --------

// renderText: human-readable terminal output. One line per test
// with pass/fail status, timing, and failure context on the next
// line. Totals at the bottom.
func renderText(recs []resultRec) string {
	var sb strings.Builder
	var passed, failed int
	for _, r := range recs {
		status := "FAIL"
		if r.Passed {
			status = "PASS"
			passed++
		} else {
			failed++
		}
		fmt.Fprintf(&sb, "%s %s (%d ms)\n", status, r.Name, r.Ms)
		if !r.Passed {
			pos := ""
			if r.File != "" {
				pos = fmt.Sprintf(" %s:%d:%d", r.File, r.Line, r.Col)
			} else if r.Line != 0 {
				pos = fmt.Sprintf(" %d:%d", r.Line, r.Col)
			}
			fmt.Fprintf(&sb, "     [%s]%s %s\n", r.ErrorKind, pos, r.ErrorMessage)
		}
	}
	fmt.Fprintf(&sb, "\n%d passed, %d failed, %d total\n", passed, failed, len(recs))
	return sb.String()
}

// renderTAP: Test Anything Protocol v14. Machine-readable, works
// with `prove` and CI harnesses.
func renderTAP(recs []resultRec) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "TAP version 14\n1..%d\n", len(recs))
	for i, r := range recs {
		if r.Passed {
			fmt.Fprintf(&sb, "ok %d - %s\n", i+1, r.Name)
		} else {
			fmt.Fprintf(&sb, "not ok %d - %s\n", i+1, r.Name)
			fmt.Fprintf(&sb, "  ---\n")
			fmt.Fprintf(&sb, "  kind: %s\n", r.ErrorKind)
			fmt.Fprintf(&sb, "  message: %s\n", r.ErrorMessage)
			if r.File != "" {
				fmt.Fprintf(&sb, "  file: %s\n", r.File)
			}
			if r.Line != 0 {
				fmt.Fprintf(&sb, "  line: %d\n", r.Line)
				fmt.Fprintf(&sb, "  col: %d\n", r.Col)
			}
			fmt.Fprintf(&sb, "  ...\n")
		}
	}
	return sb.String()
}

// -------- JUnit --------

// junitTestSuite is the minimal JUnit-XML shape most CI systems
// consume. One suite; one testcase per result.
type junitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name    string        `xml:"name,attr"`
	Time    string        `xml:"time,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Type    string `xml:"type,attr"`
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func renderJUnit(recs []resultRec) string {
	suite := junitTestSuite{
		Name:  "jennifer",
		Tests: len(recs),
	}
	for _, r := range recs {
		tc := junitTestCase{
			Name: r.Name,
			Time: fmt.Sprintf("%.3f", float64(r.Ms)/1000.0),
		}
		if !r.Passed {
			suite.Failures++
			body := r.ErrorMessage
			if r.File != "" {
				body = fmt.Sprintf("%s\n  at %s:%d:%d", r.ErrorMessage, r.File, r.Line, r.Col)
			}
			tc.Failure = &junitFailure{
				Type:    r.ErrorKind,
				Message: r.ErrorMessage,
				Body:    body,
			}
		}
		suite.Cases = append(suite.Cases, tc)
	}
	buf, err := xml.MarshalIndent(&suite, "", "  ")
	if err != nil {
		return "<!-- junit render error: " + err.Error() + " -->"
	}
	return xml.Header + string(buf) + "\n"
}
