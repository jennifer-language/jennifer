// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package encodinglib

// Request-text parsers: form-urlencoded bodies / query strings
// (`parseForm`), a Cookie header lookup (`cookieValue`) and
// multipart/form-data bodies (`parseMultipart`). They are the Go
// halves of web.parseForm / web.parseCookie, uri.parseQuery and
// multipart.parse, which split untrusted input into single-character or
// single-pair Jennifer values: a 3 MB `&&&...` body or a 1 MiB part header
// cost gigabytes there. Here the scan walks the input in place and only the
// results are allocated, so memory is a small multiple of the input, and the
// result count is capped. Pure parsing: both builds, no capability.
//
// Malformed input raises kind "encoding" (the modules rethrow it under their
// own kind), a reached cap kind "limit". There is no lenient mode.

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

const (
	// MaxFormFields caps the non-empty `&`-separated pairs parseForm decodes
	// (Go's net/url default). Empty pairs are skipped without cost.
	MaxFormFields = 10000
	// MaxMultipartParts caps the parts parseMultipart returns.
	MaxMultipartParts = 1000
	// MaxPartHeaderBytes caps one part's header block (before CRLFCRLF).
	MaxPartHeaderBytes = 16 << 10
	// MaxPartParamBytes caps a part's decoded name and filename.
	MaxPartParamBytes = 1024
)

// inputError is malformed input: raised as Error{kind: "encoding"}, the
// library's own kind for bad data.
type inputError struct{ msg string }

func (e *inputError) Error() string     { return e.msg }
func (e *inputError) ErrorKind() string { return "encoding" }

func inputErrf(format string, a ...any) error {
	return &inputError{msg: fmt.Sprintf(format, a...)}
}

// named puts the function's name in front of a malformed-input message.
func named(fn string, err error) error {
	if ie, ok := err.(*inputError); ok {
		return &inputError{msg: fn + ": " + ie.msg}
	}
	return err
}

var stringType = parser.PrimitiveType(parser.TypeString)

// installForm registers the parsers and their result struct.
func installForm(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "FormPart", []parser.StructField{
		{Name: "name", Type: stringType},
		{Name: "filename", Type: stringType},
		{Name: "contentType", Type: stringType},
		{Name: "data", Type: parser.PrimitiveType(parser.TypeBytes)},
	})
	in.RegisterNamespaced(LibraryName, "parseForm", parseFormFn)
	in.RegisterNamespaced(LibraryName, "parseCookies", parseCookiesFn)
	in.RegisterNamespaced(LibraryName, "parseMultipart", parseMultipartFn)
}

// ----- application/x-www-form-urlencoded ----------------------------------

// parseFormFn: parseForm(text) -> map of string to string.
func parseFormFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("encoding.parseForm expects 1 argument (text), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.parseForm: argument must be string, got %s", args[0].Kind)
	}
	v, err := parseForm(args[0].Str)
	return v, named("encoding.parseForm", err)
}

// parseForm splits text on `&`, skips empty pairs, splits each pair at its
// first `=` (no `=`: the value is ""), and form-decodes both halves. A
// repeated key keeps its first position and its last value.
func parseForm(text string) (interpreter.Value, error) {
	out := interpreter.MapVal(stringType, stringType, nil)
	fields := 0
	for rest := text; ; {
		amp := strings.IndexByte(rest, '&')
		pair := rest
		if amp >= 0 {
			pair = rest[:amp]
		}
		if pair != "" {
			fields++
			if fields > MaxFormFields {
				return interpreter.Null(), limits.Capf("encoding.parseForm: more than %d fields", MaxFormFields)
			}
			k, v := pair, ""
			if eq := strings.IndexByte(pair, '='); eq >= 0 {
				k, v = pair[:eq], pair[eq+1:]
			}
			dk, err := formComponent(k)
			if err != nil {
				return interpreter.Null(), err
			}
			dv, err := formComponent(v)
			if err != nil {
				return interpreter.Null(), err
			}
			out.UpsertKey(interpreter.StringVal(dk), interpreter.StringVal(dv))
		}
		if amp < 0 {
			return out, nil
		}
		rest = rest[amp+1:]
	}
}

// formComponent decodes one key or value: `+` is a space, `%XX` a byte. Any
// other `%` and a result that is not UTF-8 are errors.
func formComponent(s string) (string, error) {
	if strings.IndexByte(s, '%') < 0 && strings.IndexByte(s, '+') < 0 {
		if utf8.ValidString(s) {
			return s, nil
		}
		return "", inputErrf("text is not valid UTF-8")
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '+':
			out = append(out, ' ')
			i++
			continue
		case '%':
			if i+2 < len(s) {
				hi, ok1 := pctHexVal(s[i+1])
				lo, ok2 := pctHexVal(s[i+2])
				if ok1 && ok2 {
					out = append(out, hi<<4|lo)
					i += 3
					continue
				}
			}
			return "", inputErrf("invalid %% escape \"%s\"", escapeAt(s, i))
		}
		out = append(out, c)
		i++
	}
	if !utf8.Valid(out) {
		return "", inputErrf("decoded bytes are not valid UTF-8")
	}
	return string(out), nil
}

// escapeAt is the bad escape at s[i] for an error message: the `%` and up to
// two following runes, stopping at the next `%`.
func escapeAt(s string, i int) string {
	end := i + 1
	for n := 0; n < 2 && end < len(s) && s[end] != '%'; n++ {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}
	return s[i:end]
}

// ----- Cookie header ------------------------------------------------------

// MaxCookies caps the cookies parseCookies returns.
const MaxCookies = 1000

// parseCookiesFn: parseCookies(header) -> map of string to string. The header
// is split on `;`, each piece trimmed of white space; a piece with a name
// before its first `=` gives name -> the text after it, as sent (no
// decoding). The first of a repeated name wins, as browsers send the most
// specific cookie first. Pieces without a name are skipped. An absent cookie
// is an absent key: test with maps.has.
func parseCookiesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("encoding.parseCookies expects 1 argument (header), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.parseCookies: argument must be string, got %s", args[0].Kind)
	}
	return parseCookies(args[0].Str)
}

func parseCookies(header string) (interpreter.Value, error) {
	out := interpreter.MapVal(stringType, stringType, nil)
	seen := map[string]bool{}
	for rest := header; ; {
		semi := strings.IndexByte(rest, ';')
		piece := rest
		if semi >= 0 {
			piece = rest[:semi]
		}
		kv := strings.TrimSpace(piece)
		if eq := strings.IndexByte(kv, '='); eq > 0 && !seen[kv[:eq]] {
			if len(seen) == MaxCookies {
				return interpreter.Null(), limits.Capf("encoding.parseCookies: more than %d cookies", MaxCookies)
			}
			seen[kv[:eq]] = true
			out.UpsertKey(interpreter.StringVal(kv[:eq]), interpreter.StringVal(kv[eq+1:]))
		}
		if semi < 0 {
			return out, nil
		}
		rest = rest[semi+1:]
	}
}

// ----- multipart/form-data ------------------------------------------------

type formPart struct {
	name, filename, contentType string
	data                        []byte
}

// parseMultipartFn: parseMultipart(contentType, body) -> list of
// encoding.FormPart.
func parseMultipartFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("encoding.parseMultipart expects 2 arguments (contentType, body), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.parseMultipart: first argument must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("encoding.parseMultipart: second argument must be bytes, got %s", args[1].Kind)
	}
	parts, err := parseMultipart(args[0].Str, args[1].Bytes)
	if err != nil {
		return interpreter.Null(), named("encoding.parseMultipart", err)
	}
	out := make([]interpreter.Value, len(parts))
	for i, p := range parts {
		out[i] = interpreter.NamespacedStructVal(LibraryName, "FormPart", []interpreter.StructField{
			{Name: "name", Value: interpreter.StringVal(p.name)},
			{Name: "filename", Value: interpreter.StringVal(p.filename)},
			{Name: "contentType", Value: interpreter.StringVal(p.contentType)},
			{Name: "data", Value: interpreter.BytesVal(p.data)},
		})
	}
	return interpreter.ListVal(parser.NamespacedStructType(LibraryName, "FormPart"), out), nil
}

// parseMultipart splits body on "\r\n--boundary" as if body were preceded by
// a CRLF (so a body opening with the delimiter line matches). The segment
// before the first delimiter (the preamble) is dropped; each later one is a
// close delimiter ("--", required: a torn body is malformed, the epilogue after
// it is dropped), a part (optional SP / HT padding, CRLF, then the part), or
// else a line that only starts with the boundary, which RFC 2046 forbids.
func parseMultipart(contentType string, body []byte) ([]formPart, error) {
	boundary := ""
	if semi := strings.IndexByte(contentType, ';'); semi >= 0 {
		b, _, err := headerParam(contentType[semi+1:], "boundary")
		if err != nil {
			return nil, err
		}
		boundary = b
	}
	if boundary == "" {
		return nil, inputErrf("no boundary in Content-Type")
	}
	delim := []byte("\r\n--" + boundary)
	var start int
	if lead := delim[2:]; bytes.HasPrefix(body, lead) {
		start = len(lead)
	} else if m := bytes.Index(body, delim); m >= 0 {
		start = m + len(delim)
	} else {
		return nil, inputErrf("missing closing boundary (truncated body)")
	}
	var parts []formPart
	for {
		seg := body[start:]
		m := bytes.Index(seg, delim)
		if m >= 0 {
			seg = seg[:m]
		}
		if len(seg) >= 2 && seg[0] == '-' && seg[1] == '-' {
			return parts, nil
		}
		i := 0
		for i < len(seg) && (seg[i] == ' ' || seg[i] == '\t') {
			i++
		}
		if !(i+1 < len(seg) && seg[i] == '\r' && seg[i+1] == '\n') {
			return nil, inputErrf("a line starts with the boundary but is no delimiter")
		}
		if len(parts) >= MaxMultipartParts {
			return nil, limits.Capf("encoding.parseMultipart: more than %d parts", MaxMultipartParts)
		}
		p, err := parsePart(seg[i+2:])
		if err != nil {
			return nil, err
		}
		parts = append(parts, p)
		if m < 0 {
			return nil, inputErrf("missing closing boundary (truncated body)")
		}
		start += m + len(delim)
	}
}

// parsePart decodes one part (headers, CRLFCRLF, data). RFC 7578 4.2: every
// part carries `Content-Disposition: form-data` with a `name` parameter;
// `name` and `filename` are read from that header only.
func parsePart(pb []byte) (formPart, error) {
	hEnd := bytes.Index(pb, []byte("\r\n\r\n"))
	if hEnd < 0 {
		return formPart{}, inputErrf("malformed part: no header terminator")
	}
	if hEnd > MaxPartHeaderBytes {
		return formPart{}, limits.Capf("encoding.parseMultipart: part header over %d bytes", MaxPartHeaderBytes)
	}
	if !utf8.Valid(pb[:hEnd]) {
		return formPart{}, inputErrf("part header is not valid UTF-8")
	}
	headers := string(pb[:hEnd])
	disp, err := partHeader(headers, "content-disposition")
	if err != nil {
		return formPart{}, err
	}
	dtype, params := disp, ""
	if semi := strings.IndexByte(disp, ';'); semi >= 0 {
		dtype, params = disp[:semi], disp[semi+1:]
	}
	if strings.ToLower(strings.TrimSpace(dtype)) != "form-data" {
		return formPart{}, inputErrf("part without Content-Disposition: form-data")
	}
	name, found, err := headerParam(params, "name")
	if err != nil {
		return formPart{}, err
	}
	if !found {
		return formPart{}, inputErrf("part without a name parameter")
	}
	filename, _, err := headerParam(params, "filename")
	if err != nil {
		return formPart{}, err
	}
	if len(name) > MaxPartParamBytes || len(filename) > MaxPartParamBytes {
		return formPart{}, limits.Capf("encoding.parseMultipart: part name or filename over %d bytes", MaxPartParamBytes)
	}
	ct, err := partHeader(headers, "content-type")
	if err != nil {
		return formPart{}, err
	}
	return formPart{
		name:        name,
		filename:    filename,
		contentType: strings.Clone(ct),
		data:        bytes.Clone(pb[hEnd+4:]),
	}, nil
}

// partHeader is the trimmed value of the first `Name: value` line whose
// trimmed, lower-cased name is name (lower case), or "". A line without a
// colon before that one is malformed.
func partHeader(headers, name string) (string, error) {
	for rest := headers; ; {
		end := strings.Index(rest, "\r\n")
		line := rest
		if end >= 0 {
			line = rest[:end]
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			return "", inputErrf("malformed part header line (no colon)")
		}
		if strings.ToLower(strings.TrimSpace(line[:colon])) == name {
			return strings.TrimSpace(line[colon+1:]), nil
		}
		if end < 0 {
			return "", nil
		}
		rest = rest[end+2:]
	}
}

// headerParam finds a `key="value"` (or bare `key=value`) parameter in a
// `;`-separated list, matching the whole key case-insensitively (`name` is
// not found inside `filename`), honouring quotes (a `;` inside one is no
// separator) and backslash escapes. found is false when the key is absent. An
// unterminated quoted value met before the key is malformed. The value is
// returned trimmed, quoted or not.
func headerParam(h, key string) (value string, found bool, err error) {
	target := strings.ToLower(key)
	n := len(h)
	i := 0
	for i < n {
		for i < n && (h[i] == ';' || h[i] == ' ' || h[i] == '\t') {
			i++
		}
		ks := i
		for i < n && h[i] != '=' && h[i] != ';' {
			i++
		}
		k := h[ks:i]
		if i >= n || h[i] != '=' {
			continue
		}
		i++
		var v string
		if i < n && h[i] == '"' {
			i++
			var sb []byte
			for i < n && h[i] != '"' {
				if h[i] == '\\' && i+1 < n {
					i++
				}
				sb = append(sb, h[i])
				i++
			}
			if i >= n {
				return "", false, inputErrf("unterminated quoted parameter value")
			}
			i++
			v = string(sb)
		} else {
			vs := i
			for i < n && h[i] != ';' {
				i++
			}
			v = h[vs:i]
		}
		if strings.ToLower(strings.TrimSpace(k)) == target {
			return strings.Clone(strings.TrimSpace(v)), true, nil
		}
	}
	return "", false, nil
}
