// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package testinglib implements Jennifer's `testing` library:
// the irreducible system-side surface a Jennifer-coded
// test framework needs. Five verbs (run / results / reset / report)
// plus the `testing.Result` struct. The Jennifer-coded assertion
// vocabulary and CLI harness live in the testing module and
// build on top of these primitives.
//
// Why it lives here (not in .j):
//
//   - `testing.run(name)` invokes a user method by name. Jennifer
//     has no function references / first-class methods today; a
//     pure .j runner can't take "the test body" as a value. This
//     library uses the interpreter's method registry directly via
//     Interpreter.CallByName.
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

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "testing"

// Value alias.
type Value = interpreter.Value

// -------- Registry --------

// Record mirrors testing.Result. Kept in Go form so serializing
// to text/tap/junit doesn't have to walk struct-field slices for
// every access.
type Record struct {
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
	results   []Record
)

// ResetForTest wipes the accumulator; exported for the _test package.
func ResetForTest() {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = nil
}

// -------- Install --------

// Install registers the testing surface. Unlike most
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
	in.RegisterNamespaced(LibraryName, "runWith", makeRunWithFn(in))
	in.RegisterNamespaced(LibraryName, "results", resultsFn)
	in.RegisterNamespaced(LibraryName, "reset", resetFn)
	in.RegisterNamespaced(LibraryName, "report", reportFn)

	// Assertion vocabulary. All throw a canonical Error{kind: "assertion"} on
	// failure, positioned at the assertion call site, which testing.run
	// catches and classifies. See assertions.go.
	in.RegisterNamespaced(LibraryName, "assertEqual", assertEqualFn)
	in.RegisterNamespaced(LibraryName, "assertNotEqual", assertNotEqualFn)
	in.RegisterNamespaced(LibraryName, "assertTrue", assertTrueFn)
	in.RegisterNamespaced(LibraryName, "assertFalse", assertFalseFn)
	in.RegisterNamespaced(LibraryName, "assertContains", assertContainsFn)
	in.RegisterNamespaced(LibraryName, "assertThrows", makeAssertThrowsFn(in))
}

// -------- Helpers --------

func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

func recToValue(r Record) Value {
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

func valueToRec(fnName string, v Value) (Record, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Result" {
		return Record{}, fmt.Errorf("%s: expected a testing.Result, got %s", fnName, v.Kind)
	}
	var r Record
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

// timeAndRecord runs call, times it, classifies any error into a Record,
// appends it to the accumulator, and returns the Record as a testing.Result.
// Shared by testing.run (zero-arg) and testing.runWith (arg-taking).
func timeAndRecord(name string, call func() (Value, error)) Value {
	start := stdtime.Now()
	_, callErr := call()
	rec := Record{Name: name, Ms: stdtime.Since(start).Milliseconds()}
	if callErr == nil {
		rec.Passed = true
	} else {
		rec.ErrorKind, rec.ErrorMessage, rec.File, rec.Line, rec.Col = interpreter.ClassifyError(callErr)
	}
	resultsMu.Lock()
	results = append(results, rec)
	resultsMu.Unlock()
	return recToValue(rec)
}

// makeRunFn captures the interpreter reference so the returned builtin can
// invoke a zero-arg user method via CallByName.
func makeRunFn(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("testing.run expects 1 argument (name), got %d", len(args))
		}
		name, err := takeStringArg("testing.run", args, 0, "name")
		if err != nil {
			return interpreter.Null(), err
		}
		return timeAndRecord(name, func() (Value, error) { return in.CallByName(name) }), nil
	}
}

// makeRunWithFn mirrors testing.run for methods that take arguments: it binds
// the list's elements to the method's parameters via CallByNameWith.
func makeRunWithFn(in *interpreter.Interpreter) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		if len(args) != 2 {
			return interpreter.Null(), fmt.Errorf("testing.runWith expects 2 arguments (name, args), got %d", len(args))
		}
		name, err := takeStringArg("testing.runWith", args, 0, "name")
		if err != nil {
			return interpreter.Null(), err
		}
		if args[1].Kind != interpreter.KindList {
			return interpreter.Null(), fmt.Errorf("testing.runWith: args must be a list, got %s", args[1].Kind)
		}
		callArgs := args[1].List
		return timeAndRecord(name, func() (Value, error) { return in.CallByNameWith(name, callArgs...) }), nil
	}
}

// RenderReport formats records as "text" | "tap" | "junit". Exported so the
// `jennifer test` CLI can render results it collected at the Go level without
// round-tripping through the Jennifer-level testing.report.
func RenderReport(recs []Record, format string) (string, error) {
	switch format {
	case "text":
		return renderText(recs), nil
	case "tap":
		return renderTAP(recs), nil
	case "junit":
		return renderJUnit(recs), nil
	default:
		return "", fmt.Errorf("unknown format %q; known: text, tap, junit", format)
	}
}

func resultsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("testing.results expects 0 arguments, got %d", len(args))
	}
	resultsMu.Lock()
	snapshot := make([]Record, len(results))
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

	recs := make([]Record, len(args[0].List))
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
func renderText(recs []Record) string {
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
		fmt.Fprintf(&sb, "%s %s (%d ms)\n", status, oneLine(r.Name), r.Ms)
		if !r.Passed {
			pos := ""
			if r.File != "" {
				pos = fmt.Sprintf(" %s:%d:%d", r.File, r.Line, r.Col)
			} else if r.Line != 0 {
				pos = fmt.Sprintf(" %d:%d", r.Line, r.Col)
			}
			fmt.Fprintf(&sb, "     [%s]%s %s\n", oneLine(r.ErrorKind), pos, oneLine(r.ErrorMessage))
		}
	}
	fmt.Fprintf(&sb, "\n%d passed, %d failed, %d total\n", passed, failed, len(recs))
	return sb.String()
}

// oneLine renders a value on a single line: control characters become escapes
// so a newline in a test name or message can't split one report line into two
// (which makes a TAP/text harness miscount).
func oneLine(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(s)
}

// tapDescription is oneLine plus a backslash before any `#`, so a `#` in a name
// isn't read as a TAP directive (`# TODO` / `# SKIP`) and mis-marked.
func tapDescription(s string) string {
	return strings.ReplaceAll(oneLine(s), "#", "\\#")
}

// tapMessage writes a `message:` YAML field that is valid regardless of the
// message's contents: a multi-line message uses a literal block scalar, a
// single-line one a double-quoted string (so an embedded `#`/`:` can't be
// misread as a YAML comment or mapping).
func tapMessage(sb *strings.Builder, msg string) {
	if strings.ContainsAny(msg, "\n\r") {
		sb.WriteString("  message: |\n")
		norm := strings.ReplaceAll(msg, "\r\n", "\n")
		norm = strings.ReplaceAll(norm, "\r", "\n")
		for _, line := range strings.Split(norm, "\n") {
			sb.WriteString("    ")
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		return
	}
	quoted := strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(msg)
	fmt.Fprintf(sb, "  message: \"%s\"\n", quoted)
}

// renderTAP: Test Anything Protocol v14. Machine-readable, works
// with `prove` and CI harnesses.
func renderTAP(recs []Record) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "TAP version 14\n1..%d\n", len(recs))
	for i, r := range recs {
		if r.Passed {
			fmt.Fprintf(&sb, "ok %d - %s\n", i+1, tapDescription(r.Name))
		} else {
			fmt.Fprintf(&sb, "not ok %d - %s\n", i+1, tapDescription(r.Name))
			fmt.Fprintf(&sb, "  ---\n")
			fmt.Fprintf(&sb, "  kind: %s\n", oneLine(r.ErrorKind))
			tapMessage(&sb, r.ErrorMessage)
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

func renderJUnit(recs []Record) string {
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
