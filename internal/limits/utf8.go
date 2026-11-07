// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package limits

import (
	"fmt"
	"unicode/utf8"
)

// Text entering the interpreter as a `string` (stdin, file, process and HTTP
// request reads) must be valid UTF-8: the string functions assume it, and an
// invalid byte would otherwise be silently replaced or mis-cased later. Reads
// that return `bytes` are not checked.

// InvalidUTF8At returns the byte offset of the first byte of s that does not
// start a valid UTF-8 sequence, or -1 when s is valid.
func InvalidUTF8At(s string) int {
	if utf8.ValidString(s) {
		return -1
	}
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// CheckUTF8 returns `<what>: invalid UTF-8 at byte <offset>` when s is not
// valid UTF-8, else nil. what names the function (and stream or path).
func CheckUTF8(what, s string) error {
	if off := InvalidUTF8At(s); off >= 0 {
		return fmt.Errorf("%s: invalid UTF-8 at byte %d", what, off)
	}
	return nil
}
