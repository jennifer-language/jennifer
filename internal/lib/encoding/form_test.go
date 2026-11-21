// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package encodinglib

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func errKind(err error) string {
	var k interface{ ErrorKind() string }
	if errors.As(err, &k) {
		return k.ErrorKind()
	}
	return ""
}

// formPairs renders a parseForm result as "k=v" lines in map order.
func formPairs(t *testing.T, text string) []string {
	t.Helper()
	v, err := parseFormFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(text)})
	if err != nil {
		t.Fatalf("parseForm(%q): %v", text, err)
	}
	var out []string
	for _, e := range v.Map {
		out = append(out, e.Key.Str+"="+e.Value.Str)
	}
	return out
}

func TestParseForm(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"name=Jane+Doe&city=New%20York&flag=", "name=Jane Doe|city=New York|flag="},
		{"b=1&a=2&b=3", "b=3|a=2"},  // first position, last value
		{"&&a&&=x&", "a=|=x"},       // empty pairs skipped; bare key; empty key
		{"k=a=b", "k=a=b"},          // split at the first "="
		{"%C3%A9=%e2%82%ac", "é=€"}, // both hex cases
		{"a%2Bb=c%26d", "a+b=c&d"},  // escaped separators stay data
		{"x=100%25", "x=100%"},
	}
	for _, c := range cases {
		if got := strings.Join(formPairs(t, c.in), "|"); got != c.want {
			t.Errorf("parseForm(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every "%" needs two hex digits and the result must be UTF-8, else kind
// "encoding" with the function named in the message. There is no lenient mode.
func TestParseFormRefusesMalformed(t *testing.T) {
	for _, in := range []string{"a=%zz", "%4=1", "a=1&b=%", "a=%FF", "a=%C3", "a=%%41", "a=\xff"} {
		_, err := parseFormFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(in)})
		if errKind(err) != "encoding" || !strings.HasPrefix(err.Error(), "encoding.parseForm: ") {
			t.Errorf("%q: %v (kind %q), want kind encoding from encoding.parseForm", in, err, errKind(err))
		}
	}
	if _, err := parseFormFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("a"), interpreter.BoolVal(true)}); err == nil {
		t.Error("a second argument must be refused")
	}
}

func TestParseFormFieldCap(t *testing.T) {
	ok := strings.Repeat("a=1&", MaxFormFields)
	if got := len(formPairs(t, ok)); got != 1 {
		t.Fatalf("repeated key: %d entries", got)
	}
	_, err := parseFormFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(ok + "a=1")})
	if errKind(err) != "limit" {
		t.Errorf("%v, want kind limit", err)
	}
}

// allocDuring reports the bytes f allocates.
func allocDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// A 3 MB `&&&...` body (gigabytes in the old .j split) allocates next to
// nothing; so does a 64 KiB Cookie header of ";".
func TestRequestParsersStayBounded(t *testing.T) {
	amps := strings.Repeat("&", 3<<20)
	if n := allocDuring(func() {
		if got := formPairs(t, amps); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	}); n > 1<<20 {
		t.Errorf("parseForm of 3 MB of '&' allocated %d bytes", n)
	}
	semis := strings.Repeat(";", 64<<10)
	if n := allocDuring(func() {
		if got, err := parseCookies(semis); err != nil || len(got.Map) != 0 {
			t.Fatalf("got %v, %v", got, err)
		}
	}); n > 64<<10 {
		t.Errorf("parseCookies allocated %d bytes", n)
	}
}

func TestParseCookies(t *testing.T) {
	cases := []struct{ header, want string }{
		{"sid=abc; theme=dark", "sid=abc|theme=dark"},
		{"", ""},
		{"=x; sid=1", "sid=1"},         // an empty name is skipped
		{"sid = x", "sid = x"},         // only the whole piece is trimmed: name "sid ", value " x"
		{" sid=a b ;sid=2", "sid=a b"}, // the first of a repeated name wins
		{"sid=a=b", "sid=a=b"},         // split at the first "="
		{"sid; a=1", "a=1"},            // a piece without "=" is skipped
		{"a=%41", "a=%41"},             // values are not decoded
	}
	for _, c := range cases {
		v, err := parseCookiesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(c.header)})
		if err != nil {
			t.Fatalf("%q: %v", c.header, err)
		}
		var got []string
		for _, e := range v.Map {
			got = append(got, e.Key.Str+"="+e.Value.Str)
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("parseCookies(%q) = %q, want %q", c.header, strings.Join(got, "|"), c.want)
		}
	}
	many := ""
	for i := 0; i <= MaxCookies; i++ {
		many += fmt.Sprintf("c%d=1;", i)
	}
	if _, err := parseCookies(many); errKind(err) != "limit" {
		t.Errorf("more than %d cookies: %v, want kind limit", MaxCookies, err)
	}
}

const mpCT = "multipart/form-data; boundary=B"

func mpField(name, value string) string {
	return "--B\r\nContent-Disposition: form-data; name=\"" + name + "\"\r\n\r\n" + value + "\r\n"
}

func TestParseMultipart(t *testing.T) {
	body := "preamble\r\n" + mpField("a", "1") +
		"--B \t\r\nContent-Disposition: form-data; filename=\"x\\\".bin\"; name=\"up\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\n\x00\xff\r\n--B--\r\nepilogue"
	parts, err := parseMultipart(mpCT, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("%d parts", len(parts))
	}
	if p := parts[0]; p.name != "a" || p.filename != "" || p.contentType != "" || string(p.data) != "1" {
		t.Errorf("part 0: %+v", p)
	}
	if p := parts[1]; p.name != "up" || p.filename != "x\".bin" || p.contentType != "application/octet-stream" || string(p.data) != "\x00\xff" {
		t.Errorf("part 1: %+v", p)
	}
	// The data is a copy, not a view of the caller's body.
	b := []byte(mpField("a", "v") + "--B--\r\n")
	parts, _ = parseMultipart(mpCT, b)
	b[strings.Index(string(b), "\r\n\r\nv")+4] = 'X'
	if string(parts[0].data) != "v" {
		t.Error("part data aliases the body")
	}
	// A quoted boundary, matched as a whole parameter name.
	parts, err = parseMultipart(`multipart/form-data; xboundary=fake; boundary="a b"`,
		[]byte("--a b\r\nContent-Disposition: form-data; name=\"n\"\r\n\r\nv\r\n--a b--"))
	if err != nil || len(parts) != 1 {
		t.Errorf("quoted boundary: %v %v", parts, err)
	}
}

func TestParseMultipartKinds(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	cases := []struct {
		name, ct, body, kind string
	}{
		{"no boundary", "multipart/form-data", mpField("a", "1") + "--B--", "encoding"},
		{"torn", mpCT, mpField("a", "1"), "encoding"},
		{"no delimiter", mpCT, "hello", "encoding"},
		{"phantom", mpCT, "--Bx\r\n" + "--B--", "encoding"},
		{"no terminator", mpCT, "--B\r\nContent-Disposition: form-data; name=\"a\"\r\n--B--", "encoding"},
		{"no colon", mpCT, "--B\r\nbad\r\n\r\nv\r\n--B--", "encoding"},
		{"not form-data", mpCT, "--B\r\nContent-Disposition: attachment; name=\"a\"\r\n\r\nv\r\n--B--", "encoding"},
		{"no name", mpCT, "--B\r\nContent-Disposition: form-data\r\n\r\nv\r\n--B--", "encoding"},
		{"open quote", mpCT, "--B\r\nContent-Disposition: form-data; name=\"a\r\n\r\nv\r\n--B--", "encoding"},
		{"header not UTF-8", mpCT, "--B\r\nContent-Disposition: form-data; name=\"\xff\"\r\n\r\nv\r\n--B--", "encoding"},
		{"header cap", mpCT, "--B\r\nContent-Disposition: form-data; name=\"a\"; x=" + long(MaxPartHeaderBytes) + "\r\n\r\nv\r\n--B--", "limit"},
		{"name cap", mpCT, mpField(long(MaxPartParamBytes+1), "v") + "--B--", "limit"},
		{"filename cap", mpCT, "--B\r\nContent-Disposition: form-data; name=\"a\"; filename=\"" + long(MaxPartParamBytes+1) + "\"\r\n\r\nv\r\n--B--", "limit"},
		{"part cap", mpCT, strings.Repeat(mpField("a", "1"), MaxMultipartParts+1) + "--B--", "limit"},
	}
	for _, c := range cases {
		_, err := parseMultipartFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(c.ct), interpreter.BytesVal([]byte(c.body))})
		if errKind(err) != c.kind {
			t.Errorf("%s: %v (kind %q), want kind %s", c.name, err, errKind(err), c.kind)
		}
	}
	// At the caps exactly, it parses.
	ok := strings.Repeat(mpField(long(MaxPartParamBytes), "1"), MaxMultipartParts) + "--B--"
	if parts, err := parseMultipart(mpCT, []byte(ok)); err != nil || len(parts) != MaxMultipartParts {
		t.Errorf("at the caps: %d parts, %v", len(parts), err)
	}
}

// A 1 MiB part header (1.6 GB in the old .j scan) is refused after one pass.
func TestParseMultipartHugeHeaderBounded(t *testing.T) {
	body := []byte("--B\r\nContent-Disposition: form-data; name=\"a\"; " + strings.Repeat("x", 1<<20) + "\r\n\r\nv\r\n--B--\r\n")
	var err error
	if n := allocDuring(func() { _, err = parseMultipart(mpCT, body) }); n > 1<<20 {
		t.Errorf("allocated %d bytes", n)
	}
	if errKind(err) != "limit" {
		t.Errorf("%v, want kind limit", err)
	}
}
