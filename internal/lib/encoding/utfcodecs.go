// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// The Unicode encoding forms: UTF-8, UTF-16 and UTF-32, the last two in
// both byte orders and with a byte-order mark. Go's unicode/utf16 does the
// surrogate pairs; no dependency, so both builds have them.
//
// Decoding is strict: an odd length, a truncated unit, an unpaired
// surrogate or a UTF-32 value outside Unicode is an error naming its byte
// offset. Encoding cannot fail, as a string holds only valid code points.
//
// "utf-16" / "utf-32" (no byte order in the name) write big-endian with a
// BOM, and on decode let a BOM choose the order, big-endian without one
// (RFC 2781 section 4.3). In "utf-16le" / "utf-16be" / "utf-32le" /
// "utf-32be" the order is the name's and a leading U+FEFF is a character,
// not a mark (RFC 2781 section 4.2).

package encodinglib

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func init() {
	utf := []*codec{
		{canonical: "utf-8", encodeFn: func(s string) ([]byte, error) { return []byte(s), nil }, decodeFn: decodeUTF8},
		{canonical: "utf-16", encodeFn: utf16Encoder(binary.BigEndian, true), decodeFn: decodeUTF16Marked},
		{canonical: "utf-16le", encodeFn: utf16Encoder(binary.LittleEndian, false), decodeFn: utf16Decoder(binary.LittleEndian)},
		{canonical: "utf-16be", encodeFn: utf16Encoder(binary.BigEndian, false), decodeFn: utf16Decoder(binary.BigEndian)},
		{canonical: "utf-32", encodeFn: utf32Encoder(binary.BigEndian, true), decodeFn: decodeUTF32Marked},
		{canonical: "utf-32le", encodeFn: utf32Encoder(binary.LittleEndian, false), decodeFn: utf32Decoder(binary.LittleEndian)},
		{canonical: "utf-32be", encodeFn: utf32Encoder(binary.BigEndian, false), decodeFn: utf32Decoder(binary.BigEndian)},
	}
	names := make([]string, len(utf))
	for i, c := range utf {
		codecs[c.canonical] = c
		names[i] = c.canonical
	}
	// The Unicode forms first: they are the ones most programs want.
	canonicalCodecOrder = append(names, canonicalCodecOrder...)
}

func decodeUTF8(b []byte) (string, error) {
	if utf8.Valid(b) {
		return string(b), nil
	}
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			return "", fmt.Errorf("invalid UTF-8 at byte %d", i)
		}
		i += size
	}
	return string(b), nil // not reached: utf8.Valid found an error above
}

func utf16Encoder(order binary.AppendByteOrder, mark bool) func(string) ([]byte, error) {
	return func(s string) ([]byte, error) {
		units := utf16.Encode([]rune(s))
		out := make([]byte, 0, 2*len(units)+2)
		if mark {
			out = order.AppendUint16(out, 0xFEFF)
		}
		for _, u := range units {
			out = order.AppendUint16(out, u)
		}
		return out, nil
	}
}

func utf16Decoder(order binary.ByteOrder) func([]byte) (string, error) {
	return func(b []byte) (string, error) { return decodeUTF16(b, order, 0) }
}

// decodeUTF16Marked reads a BOM if there is one, else big-endian.
func decodeUTF16Marked(b []byte) (string, error) {
	if len(b) >= 2 {
		switch {
		case b[0] == 0xFE && b[1] == 0xFF:
			return decodeUTF16(b[2:], binary.BigEndian, 2)
		case b[0] == 0xFF && b[1] == 0xFE:
			return decodeUTF16(b[2:], binary.LittleEndian, 2)
		}
	}
	return decodeUTF16(b, binary.BigEndian, 0)
}

// decodeUTF16 decodes code units, pairing surrogates; base is the offset of
// b in the caller's input, for messages.
func decodeUTF16(b []byte, order binary.ByteOrder, base int) (string, error) {
	if len(b)%2 != 0 {
		return "", fmt.Errorf("%d bytes is not a whole number of 16-bit units", len(b)+base)
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for i := 0; i < len(b); i += 2 {
		u := order.Uint16(b[i:])
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			if i+3 >= len(b) {
				return "", fmt.Errorf("high surrogate 0x%04X at byte %d has no low surrogate", u, base+i)
			}
			lo := order.Uint16(b[i+2:])
			if lo < 0xDC00 || lo > 0xDFFF {
				return "", fmt.Errorf("high surrogate 0x%04X at byte %d has no low surrogate", u, base+i)
			}
			sb.WriteRune(utf16.DecodeRune(rune(u), rune(lo)))
			i += 2
		case u >= 0xDC00 && u <= 0xDFFF:
			return "", fmt.Errorf("unpaired low surrogate 0x%04X at byte %d", u, base+i)
		default:
			sb.WriteRune(rune(u))
		}
	}
	return sb.String(), nil
}

func utf32Encoder(order binary.AppendByteOrder, mark bool) func(string) ([]byte, error) {
	return func(s string) ([]byte, error) {
		out := make([]byte, 0, 4*utf8.RuneCountInString(s)+4)
		if mark {
			out = order.AppendUint32(out, 0xFEFF)
		}
		for _, r := range s {
			out = order.AppendUint32(out, uint32(r))
		}
		return out, nil
	}
}

func utf32Decoder(order binary.ByteOrder) func([]byte) (string, error) {
	return func(b []byte) (string, error) { return decodeUTF32(b, order, 0) }
}

// decodeUTF32Marked reads a BOM if there is one, else big-endian.
func decodeUTF32Marked(b []byte) (string, error) {
	if len(b) >= 4 {
		switch {
		case b[0] == 0 && b[1] == 0 && b[2] == 0xFE && b[3] == 0xFF:
			return decodeUTF32(b[4:], binary.BigEndian, 4)
		case b[0] == 0xFF && b[1] == 0xFE && b[2] == 0 && b[3] == 0:
			return decodeUTF32(b[4:], binary.LittleEndian, 4)
		}
	}
	return decodeUTF32(b, binary.BigEndian, 0)
}

func decodeUTF32(b []byte, order binary.ByteOrder, base int) (string, error) {
	if len(b)%4 != 0 {
		return "", fmt.Errorf("%d bytes is not a whole number of 32-bit units", len(b)+base)
	}
	var sb strings.Builder
	sb.Grow(len(b) / 2)
	for i := 0; i < len(b); i += 4 {
		v := order.Uint32(b[i:])
		if v > utf8.MaxRune || (v >= 0xD800 && v <= 0xDFFF) {
			return "", fmt.Errorf("0x%08X at byte %d is not a Unicode scalar value", v, base+i)
		}
		sb.WriteRune(rune(v))
	}
	return sb.String(), nil
}
