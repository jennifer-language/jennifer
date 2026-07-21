// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package iolib

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// FormatSpec is the parsed shape of one `%verb[|key=value]*` site in a
// format string. The zero value of each field encodes "modifier not
// present" so the renderer can branch on whether a key was set. Verbs
// share one struct rather than carrying a per-verb spec type, because
// modifier keys are partitioned by verb at parse time and never collide.
type FormatSpec struct {
	Verb byte

	NullSet bool
	Null    nullMode
	NullLit string

	HasPad bool
	Pad    int
	HasMax bool
	Max    int
	Align  alignMode
	Fill   byte

	Mode stringMode

	Base   int
	Sign   signMode
	HasGrp bool
	Group  int
	HasSep bool
	Sep    byte

	HasPrec bool
	Prec    int
	Trim    bool
	Sci     bool

	Case caseMode

	// %a (aggregate) modifiers. Defaults reproduce Jennifer's
	// literal syntax for the kind being rendered: `[a, b, c]` for
	// lists, `{k: v, k: v}` for maps. Override via:
	//   sep="..."   element separator (default ", ")
	//   kv="..."    map key/value separator (default ": ")
	//   open="..."  opener bracket (default "[" / "{")
	//   close="..." closer bracket (default "]" / "}")
	//   depth=N     max recursion depth; deeper levels collapse to
	//               "[...]" / "{...}" (default unlimited; 0 = collapse
	//               at the top, useful for "size only" renderings)
	//   null=skip   per-element null handling; omits null list elements
	//               and null map values entirely. Other null= modes
	//               (empty / null / literal) are rejected for %a.
	HasAggSep   bool
	AggSep      string
	HasAggKV    bool
	AggKV       string
	HasAggOpen  bool
	AggOpen     string
	HasAggClose bool
	AggClose    string
	HasDepth    bool
	Depth       int
	NullSkipA   bool // null=skip set on %a

	seen map[string]bool
}

type nullMode uint8

const (
	nullNone nullMode = iota
	nullEmpty
	nullNull
	nullLiteral
)

type alignMode uint8

const (
	alignDefault alignMode = iota
	alignLeft
	alignRight
	alignCenter // %s only - splits padding around the value
)

type stringMode uint8

const (
	stringRaw stringMode = iota
	stringQuote
	stringEscape
)

type signMode uint8

const (
	signNegative signMode = iota
	signAlways
	signSpace
)

type caseMode uint8

const (
	caseLower caseMode = iota
	caseUpper
	caseTitle
)

// parseFormatSpec consumes any `|key=value` modifiers starting at pos
// (which must point at the first byte after the verb char). Returns the
// populated spec and the index of the first byte that is not part of a
// modifier (or len(src) at end of string).
func parseFormatSpec(verb byte, src string, pos int) (FormatSpec, int, error) {
	spec := FormatSpec{Verb: verb, Base: 10, seen: map[string]bool{}}
	for pos < len(src) && src[pos] == '|' {
		if pos+1 < len(src) && src[pos+1] == '|' {
			// `||` is the escape for a literal `|` immediately after a
			// verb. Consume only the first `|` here; the second falls
			// through to the outer format-string scanner as a regular
			// literal byte.
			pos++
			break
		}
		pos++
		key, np, err := readKey(src, pos)
		if err != nil {
			return spec, pos, err
		}
		pos = np
		if pos >= len(src) || src[pos] != '=' {
			return spec, pos, fmt.Errorf("modifier %q is missing `=`", key)
		}
		pos++
		if spec.seen[key] {
			return spec, pos, fmt.Errorf("modifier %q specified twice", key)
		}
		spec.seen[key] = true
		var value string
		var afterVal int
		if key == "null" {
			value, afterVal, err = readNullValue(src, pos)
		} else {
			value, afterVal, err = readValue(src, pos)
		}
		if err != nil {
			return spec, pos, fmt.Errorf("modifier %q: %v", key, err)
		}
		pos = afterVal
		if err := applyModifier(&spec, key, value); err != nil {
			return spec, pos, err
		}
	}
	if err := validateSpec(&spec); err != nil {
		return spec, pos, err
	}
	return spec, pos, nil
}

// readKey reads an identifier (lowercase letters) used as a modifier key.
func readKey(src string, pos int) (string, int, error) {
	start := pos
	for pos < len(src) {
		c := src[pos]
		if c >= 'a' && c <= 'z' {
			pos++
			continue
		}
		break
	}
	if pos == start {
		return "", pos, fmt.Errorf("expected modifier key after `|` at byte %d", start)
	}
	return src[start:pos], pos, nil
}

// readValue reads a modifier value: either a bare token (letters,
// digits, underscore, or single-char punctuation from the `sep=` set)
// or a Jennifer-style double-quoted string. The quoted form is what
// makes `%a|sep=", "` and `%a|open="<"` work - values containing
// spaces, brackets, or other reserved characters can be expressed
// without limiting the modifier-list grammar.
func readValue(src string, pos int) (string, int, error) {
	if pos < len(src) && src[pos] == '"' {
		return readQuotedValue(src, pos)
	}
	start := pos
	for pos < len(src) && isValueByte(src[pos]) {
		pos++
	}
	if pos == start {
		return "", pos, fmt.Errorf("empty value after `=`")
	}
	return src[start:pos], pos, nil
}

// readQuotedValue parses a `"..."` modifier value with the standard
// `\n \r \t \\ \"` escape set. Stops at the closing `"`; returns the
// position just past it.
func readQuotedValue(src string, pos int) (string, int, error) {
	pos++ // consume opening "
	var b strings.Builder
	for pos < len(src) {
		c := src[pos]
		if c == '"' {
			return b.String(), pos + 1, nil
		}
		if c == '\\' && pos+1 < len(src) {
			switch src[pos+1] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				return "", pos, fmt.Errorf("unknown escape `\\%c` in quoted modifier value", src[pos+1])
			}
			pos += 2
			continue
		}
		b.WriteByte(c)
		pos++
	}
	return "", pos, fmt.Errorf("unterminated quoted modifier value")
}

func isValueByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == ',' || c == '.' || c == '-' || c == ':':
		return true
	}
	return false
}

// readNullValue reads a `null=` value, which is either a bare identifier
// (`empty`, `null`) or `literal(STR)` where STR is a Jennifer-style
// double-quoted string literal with escape sequences.
func readNullValue(src string, pos int) (string, int, error) {
	if strings.HasPrefix(src[pos:], "literal(") {
		i := pos + len("literal(")
		if i >= len(src) || src[i] != '"' {
			return "", pos, fmt.Errorf("literal(...) needs a double-quoted string")
		}
		i++
		var b strings.Builder
		for i < len(src) {
			c := src[i]
			if c == '"' {
				if i+1 >= len(src) || src[i+1] != ')' {
					return "", pos, fmt.Errorf("literal(...) is missing closing `)`")
				}
				return "literal:" + b.String(), i + 2, nil
			}
			if c == '\\' {
				if i+1 >= len(src) {
					return "", pos, fmt.Errorf("literal(...) has dangling backslash")
				}
				esc := src[i+1]
				switch esc {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				case '\'':
					b.WriteByte('\'')
				case '0':
					b.WriteByte(0)
				default:
					return "", pos, fmt.Errorf("literal(...) has unknown escape `\\%c`", esc)
				}
				i += 2
				continue
			}
			b.WriteByte(c)
			i++
		}
		return "", pos, fmt.Errorf("literal(...) is missing closing `\"`")
	}
	return readValue(src, pos)
}

// applyModifier writes one key=value pair into the spec, after checking it
// is valid for the spec's verb.
func applyModifier(spec *FormatSpec, key, value string) error {
	if key == "null" {
		return setNull(spec, value)
	}
	switch spec.Verb {
	case 's':
		return setStringMod(spec, key, value)
	case 'd':
		return setIntMod(spec, key, value)
	case 'f':
		return setFloatMod(spec, key, value)
	case 't':
		return setBoolMod(spec, key, value)
	case 'a':
		return setAggMod(spec, key, value)
	case 'v':
		return fmt.Errorf("verb `%%v` takes no modifiers (got %q)", key)
	}
	return fmt.Errorf("unknown verb `%%%c`", spec.Verb)
}

func setNull(spec *FormatSpec, value string) error {
	// `null=skip` is the per-element mode that only makes sense for
	// the aggregate verb (`%a`) - it omits null list elements and null
	// map values from the rendered output. Reject it on every other
	// verb so a stray `%s|null=skip` produces a clear error rather
	// than silently doing the wrong thing.
	if value == "skip" {
		if spec.Verb != 'a' {
			return fmt.Errorf("null=skip is only valid on %%a, not %%%c", spec.Verb)
		}
		spec.NullSkipA = true
		spec.NullSet = true
		return nil
	}
	spec.NullSet = true
	switch {
	case value == "empty":
		spec.Null = nullEmpty
	case value == "null":
		spec.Null = nullNull
	case strings.HasPrefix(value, "literal:"):
		spec.Null = nullLiteral
		spec.NullLit = value[len("literal:"):]
	default:
		return fmt.Errorf("null=%q: expected `empty`, `null`, or `literal(\"...\")`", value)
	}
	return nil
}

// setAggMod handles modifiers for the `%a` aggregate verb.
// The modifiers shape the *render*, not the values: separators,
// brackets, recursion depth, and the per-element null-handling.
func setAggMod(spec *FormatSpec, key, value string) error {
	switch key {
	case "sep":
		spec.HasAggSep = true
		spec.AggSep = value
	case "kv":
		spec.HasAggKV = true
		spec.AggKV = value
	case "open":
		spec.HasAggOpen = true
		spec.AggOpen = value
	case "close":
		spec.HasAggClose = true
		spec.AggClose = value
	case "depth":
		return setIntField(value, &spec.Depth, &spec.HasDepth)
	default:
		return fmt.Errorf("unknown modifier %q for `%%a`", key)
	}
	return nil
}

func setStringMod(spec *FormatSpec, key, value string) error {
	switch key {
	case "pad":
		return setIntField(value, &spec.Pad, &spec.HasPad)
	case "max":
		return setIntField(value, &spec.Max, &spec.HasMax)
	case "align":
		return setAlign(spec, value)
	case "mode":
		switch value {
		case "raw":
			spec.Mode = stringRaw
		case "quote":
			spec.Mode = stringQuote
		case "escape":
			spec.Mode = stringEscape
		default:
			return fmt.Errorf("mode=%q: expected `raw`, `quote`, or `escape`", value)
		}
		return nil
	}
	return fmt.Errorf("option %q not valid for verb `%%s`", key)
}

func setIntMod(spec *FormatSpec, key, value string) error {
	switch key {
	case "pad":
		return setIntField(value, &spec.Pad, &spec.HasPad)
	case "fill":
		if value != "0" {
			return fmt.Errorf("fill=%q: only `0` is supported", value)
		}
		spec.Fill = '0'
		return nil
	case "align":
		return setAlign(spec, value)
	case "base":
		switch value {
		case "2", "8", "10", "16":
			b, _ := strconv.Atoi(value)
			spec.Base = b
		default:
			return fmt.Errorf("base=%q: expected `2`, `8`, `10`, or `16`", value)
		}
		return nil
	case "sign":
		return setSign(spec, value)
	case "group":
		if err := setIntField(value, &spec.Group, &spec.HasGrp); err != nil {
			return err
		}
		if spec.Group < 1 {
			return fmt.Errorf("group=%q: must be >= 1", value)
		}
		return nil
	case "sep":
		if len(value) != 1 || !strings.ContainsRune("_,.-:", rune(value[0])) {
			return fmt.Errorf("sep=%q: expected one of `_,.-:`", value)
		}
		spec.Sep = value[0]
		spec.HasSep = true
		return nil
	}
	return fmt.Errorf("option %q not valid for verb `%%d`", key)
}

func setFloatMod(spec *FormatSpec, key, value string) error {
	switch key {
	case "prec":
		if err := setIntField(value, &spec.Prec, &spec.HasPrec); err != nil {
			return err
		}
		if spec.Prec < 0 {
			return fmt.Errorf("prec=%q: must be >= 0", value)
		}
		return nil
	case "trim":
		return setBoolField(value, &spec.Trim)
	case "sci":
		return setBoolField(value, &spec.Sci)
	case "pad":
		return setIntField(value, &spec.Pad, &spec.HasPad)
	case "align":
		return setAlign(spec, value)
	case "sign":
		return setSign(spec, value)
	}
	return fmt.Errorf("option %q not valid for verb `%%f`", key)
}

func setBoolMod(spec *FormatSpec, key, value string) error {
	if key != "case" {
		return fmt.Errorf("option %q not valid for verb `%%t`", key)
	}
	switch value {
	case "lower":
		spec.Case = caseLower
	case "upper":
		spec.Case = caseUpper
	case "title":
		spec.Case = caseTitle
	default:
		return fmt.Errorf("case=%q: expected `lower`, `upper`, or `title`", value)
	}
	return nil
}

func setIntField(value string, dst *int, has *bool) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%q is not an integer", value)
	}
	*dst = n
	*has = true
	return nil
}

func setBoolField(value string, dst *bool) error {
	switch value {
	case "true":
		*dst = true
	case "false":
		*dst = false
	default:
		return fmt.Errorf("%q must be `true` or `false`", value)
	}
	return nil
}

func setAlign(spec *FormatSpec, value string) error {
	switch value {
	case "left":
		spec.Align = alignLeft
	case "right":
		spec.Align = alignRight
	case "center":
		// `center` is only meaningful for `%s` - splitting padding
		// around a number breaks columnar alignment for numeric output.
		if spec.Verb != 's' {
			return fmt.Errorf("align=center is only valid on %%s, not %%%c", spec.Verb)
		}
		spec.Align = alignCenter
	default:
		return fmt.Errorf("align=%q: expected `left`, `right`, or `center`", value)
	}
	return nil
}

func setSign(spec *FormatSpec, value string) error {
	switch value {
	case "negative":
		spec.Sign = signNegative
	case "always":
		spec.Sign = signAlways
	case "space":
		spec.Sign = signSpace
	default:
		return fmt.Errorf("sign=%q: expected `negative`, `always`, or `space`", value)
	}
	return nil
}

func validateSpec(spec *FormatSpec) error {
	if spec.HasGrp != spec.HasSep {
		return fmt.Errorf("`group=` and `sep=` must be specified together")
	}
	if spec.HasPad && spec.Pad < 0 {
		return fmt.Errorf("pad must be >= 0")
	}
	if spec.HasMax && spec.Max < 0 {
		return fmt.Errorf("max must be >= 0")
	}
	if spec.Fill == '0' && spec.Verb == 'd' && spec.Align == alignLeft {
		return fmt.Errorf("`fill=0` requires `align=right` (the default)")
	}
	return nil
}

// renderValue produces the substituted text for one verb+spec+value.
// Null handling runs first: a null value with `null=` set returns the
// configured replacement, padded/truncated by any layout modifiers but
// otherwise skipping verb-specific rendering. Without `null=`, a null
// value is a type-mismatch error against any verb except `%v`.
func renderValue(spec FormatSpec, v interpreter.Value) (string, error) {
	if v.Kind == interpreter.KindNull && spec.NullSet {
		return layoutOnly(spec, nullText(spec)), nil
	}
	switch spec.Verb {
	case 'd':
		if v.Kind != interpreter.KindInt {
			return "", fmt.Errorf("`%%d` requires int, got %s", v.Kind)
		}
		return applyLayout(spec, renderInt(spec, v.Int)), nil
	case 'f':
		if v.Kind != interpreter.KindFloat {
			return "", fmt.Errorf("`%%f` requires float, got %s", v.Kind)
		}
		return applyLayout(spec, renderFloat(spec, v.Float)), nil
	case 's':
		if v.Kind != interpreter.KindString {
			return "", fmt.Errorf("`%%s` requires string, got %s", v.Kind)
		}
		return applyLayout(spec, renderString(spec, v.Str)), nil
	case 't':
		if v.Kind != interpreter.KindBool {
			return "", fmt.Errorf("`%%t` requires bool, got %s", v.Kind)
		}
		return applyLayout(spec, renderBool(spec, v.Bool)), nil
	case 'v':
		return v.Display(), nil
	case 'a':
		if v.Kind != interpreter.KindList && v.Kind != interpreter.KindMap {
			return "", fmt.Errorf("`%%a` requires list or map, got %s", v.Kind)
		}
		return renderAggregate(spec, v, 0), nil
	}
	return "", fmt.Errorf("unknown format verb `%%%c`", spec.Verb)
}

// renderAggregate handles `%a`. Recurses into nested lists and
// maps using the same spec, so a single `%a|sep=" | "` on the top
// level applies consistently all the way down. `level` tracks the
// current recursion depth so `depth=N` can collapse deeper trees to
// "[...]"/"{...}". Per-element rendering uses Value.Display(), which
// matches `%v` semantics (so primitive values inside an aggregate
// look the way they would in a print statement).
func renderAggregate(spec FormatSpec, v interpreter.Value, level int) string {
	switch v.Kind {
	case interpreter.KindList:
		open, close := "[", "]"
		if spec.HasAggOpen {
			open = spec.AggOpen
		}
		if spec.HasAggClose {
			close = spec.AggClose
		}
		if spec.HasDepth && level >= spec.Depth {
			return open + "..." + close
		}
		sep := ", "
		if spec.HasAggSep {
			sep = spec.AggSep
		}
		var parts []string
		for _, elem := range v.List {
			if elem.Kind == interpreter.KindNull && spec.NullSkipA {
				continue
			}
			parts = append(parts, aggElement(spec, elem, level+1))
		}
		return open + strings.Join(parts, sep) + close
	case interpreter.KindMap:
		open, close := "{", "}"
		if spec.HasAggOpen {
			open = spec.AggOpen
		}
		if spec.HasAggClose {
			close = spec.AggClose
		}
		if spec.HasDepth && level >= spec.Depth {
			return open + "..." + close
		}
		sep := ", "
		if spec.HasAggSep {
			sep = spec.AggSep
		}
		kv := ": "
		if spec.HasAggKV {
			kv = spec.AggKV
		}
		var parts []string
		for _, entry := range v.Map {
			if entry.Value.Kind == interpreter.KindNull && spec.NullSkipA {
				continue
			}
			parts = append(parts, aggElement(spec, entry.Key, level+1)+kv+aggElement(spec, entry.Value, level+1))
		}
		return open + strings.Join(parts, sep) + close
	}
	return v.Display()
}

// aggElement renders one element/value/key inside an aggregate. Nested
// collections recurse through renderAggregate; primitives use Display
// (the same shape `%v` produces).
func aggElement(spec FormatSpec, v interpreter.Value, level int) string {
	if v.Kind == interpreter.KindList || v.Kind == interpreter.KindMap {
		return renderAggregate(spec, v, level)
	}
	return v.Display()
}

func nullText(spec FormatSpec) string {
	switch spec.Null {
	case nullEmpty:
		return ""
	case nullNull:
		return "null"
	case nullLiteral:
		return spec.NullLit
	}
	return ""
}

func renderString(spec FormatSpec, s string) string {
	switch spec.Mode {
	case stringQuote:
		return `"` + escapeString(s) + `"`
	case stringEscape:
		return escapeString(s)
	}
	return s
}

// escapeString renders unprintable characters and the quote/backslash
// escape set the Jennifer lexer recognises. Used by `mode=quote` and
// `mode=escape` for `%s`. The character set mirrors the lexer.
func escapeString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case 0:
			b.WriteString(`\0`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func renderInt(spec FormatSpec, n int64) string {
	neg := n < 0
	// Take the magnitude in uint64: `-n` overflows for MinInt64 (staying
	// negative), which would make FormatInt emit a second minus sign.
	mag := uint64(n)
	if neg {
		mag = -mag
	}
	digits := strconv.FormatUint(mag, spec.Base)
	sign := signPrefix(neg, spec.Sign)
	if spec.Fill == '0' && spec.HasPad {
		// Zero-pad the raw digits *before* grouping so the fill lines up
		// columnar with the separators (`0,001,234`), not jammed ahead of the
		// first one (`000001,234`). The target width counts the sign and the
		// separators grouping will insert.
		digits = zeroPadForGroup(digits, spec, len(sign))
	}
	if spec.HasGrp {
		digits = groupDigits(digits, spec.Group, spec.Sep)
	}
	return sign + digits
}

// zeroPadForGroup grows the raw digit string until the fully rendered field
// (sign + digits + grouping separators) reaches spec.Pad, so a `fill=0` with a
// `group=` produces valid columnar output. With no grouping active it reduces
// to padding to `spec.Pad - signLen` digits, matching the plain zero-fill.
func zeroPadForGroup(digits string, spec FormatSpec, signLen int) string {
	width := func(nd int) int {
		seps := 0
		if spec.HasGrp && spec.Group > 0 && nd > 0 {
			seps = (nd - 1) / spec.Group
		}
		return signLen + nd + seps
	}
	n := len(digits)
	for width(n) < spec.Pad {
		n++
	}
	if n > len(digits) {
		digits = strings.Repeat("0", n-len(digits)) + digits
	}
	return digits
}

func groupDigits(digits string, every int, sep byte) string {
	if every < 1 || len(digits) <= every {
		return digits
	}
	var b strings.Builder
	first := len(digits) % every
	if first > 0 {
		b.WriteString(digits[:first])
		if len(digits) > first {
			b.WriteByte(sep)
		}
	}
	for i := first; i < len(digits); i += every {
		b.WriteString(digits[i : i+every])
		if i+every < len(digits) {
			b.WriteByte(sep)
		}
	}
	return b.String()
}

func signPrefix(neg bool, mode signMode) string {
	if neg {
		return "-"
	}
	switch mode {
	case signAlways:
		return "+"
	case signSpace:
		return " "
	}
	return ""
}

func renderFloat(spec FormatSpec, f float64) string {
	neg := math.Signbit(f) && f != 0
	mag := f
	if neg {
		mag = -f
	}
	prec := -1
	if spec.HasPrec {
		prec = spec.Prec
	}
	var body string
	if spec.Sci {
		if prec < 0 {
			prec = 6
		}
		body = strconv.FormatFloat(mag, 'e', prec, 64)
	} else {
		if prec < 0 {
			body = interpreter.DisplayFloat(mag)
		} else {
			body = strconv.FormatFloat(mag, 'f', prec, 64)
		}
	}
	if spec.Trim {
		body = trimFloatZeros(body)
	}
	return signPrefix(neg, spec.Sign) + body
}

// trimFloatZeros drops trailing zeros from a float's fractional part and,
// if the fraction is empty afterward, the decimal point too. Handles both
// fixed-point (`3.1400` -> `3.14`) and scientific (`3.1400e+03` -> `3.14e+03`).
func trimFloatZeros(s string) string {
	mant, exp := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, exp = s[:i], s[i:]
	}
	if !strings.ContainsRune(mant, '.') {
		return mant + exp
	}
	mant = strings.TrimRight(mant, "0")
	mant = strings.TrimRight(mant, ".")
	return mant + exp
}

func renderBool(spec FormatSpec, b bool) string {
	switch spec.Case {
	case caseUpper:
		if b {
			return "TRUE"
		}
		return "FALSE"
	case caseTitle:
		if b {
			return "True"
		}
		return "False"
	}
	if b {
		return "true"
	}
	return "false"
}

// applyLayout truncates by `max` then pads to `pad` using the verb's
// default alignment unless one was requested. For ints the pad char may
// be `0` (via `fill=0`); otherwise pads are spaces.
func applyLayout(spec FormatSpec, s string) string {
	if spec.HasMax {
		s = truncateRunes(s, spec.Max)
	}
	if !spec.HasPad || runeLen(s) >= spec.Pad {
		return s
	}
	if spec.Fill == '0' && spec.Verb == 'd' {
		return s
	}
	return padTo(s, spec.Pad, alignOf(spec))
}

// layoutOnly is the variant used for the `null=` replacement string:
// `max` and `pad` still apply (so columnar output keeps aligning), but
// the verb's render is skipped.
func layoutOnly(spec FormatSpec, s string) string {
	if spec.HasMax {
		s = truncateRunes(s, spec.Max)
	}
	if !spec.HasPad || runeLen(s) >= spec.Pad {
		return s
	}
	return padTo(s, spec.Pad, alignOf(spec))
}

// padTo extends s with spaces to `width` runes, placing s on the
// requested side. Centered alignment splits leftover space with the
// extra rune (when the gap is odd) going to the right so column
// headers line up over even/odd-length values.
func padTo(s string, width int, align alignMode) string {
	need := width - runeLen(s)
	if need <= 0 {
		return s
	}
	switch align {
	case alignLeft:
		return s + strings.Repeat(" ", need)
	case alignCenter:
		left := need / 2
		right := need - left
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
	}
	return strings.Repeat(" ", need) + s
}

// alignOf falls back to the verb's default when the spec didn't ask for
// a specific side: `%s` left-aligns, `%d` and `%f` right-align.
func alignOf(spec FormatSpec) alignMode {
	if spec.Align != alignDefault {
		return spec.Align
	}
	if spec.Verb == 's' {
		return alignLeft
	}
	return alignRight
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i, c := 0, 0
	for i < len(s) {
		if c == n {
			return s[:i]
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		c++
	}
	return s
}
