// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// editWith runs the line editor against a synthetic byte stream so the
// editor's state machine can be exercised in isolation - no terminal, no
// raw mode. The returned `line` is what the editor would have submitted
// on Enter; `err` is the editor's terminating error (typically nil on
// Enter, io.EOF on Ctrl+D, errLineCancelled on Ctrl+C).
//
// The output buffer is captured but tests rarely need to assert on it -
// what we care about is the final logical line.
func editWith(t *testing.T, history *replHistory, input string) (string, error) {
	t.Helper()
	if history == nil {
		history = newReplHistory()
	}
	e := &lineEditor{
		in:      bufio.NewReader(strings.NewReader(input)),
		out:     &bytes.Buffer{},
		history: history,
	}
	return e.readLine(">>> ")
}

func TestEditorBasicInsertAndEnter(t *testing.T) {
	line, err := editWith(t, nil, "hello\r")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if line != "hello" {
		t.Errorf("got %q, want %q", line, "hello")
	}
}

func TestEditorBackspace(t *testing.T) {
	// Type "abcd", backspace twice, Enter -> "ab"
	line, _ := editWith(t, nil, "abcd\x7f\x7f\r")
	if line != "ab" {
		t.Errorf("got %q, want %q", line, "ab")
	}
}

func TestEditorLeftRightArrows(t *testing.T) {
	// Type "ad", Left, Left, insert "bc", Enter -> "abcad"... wait, left
	// twice puts cursor at position 0 of "ad", then "bc" gets inserted at
	// position 0 producing "bcad". Adjust expectation accordingly.
	line, _ := editWith(t, nil, "ad\x1b[D\x1b[Dbc\r")
	if line != "bcad" {
		t.Errorf("got %q, want %q", line, "bcad")
	}
}

func TestEditorHomeEnd(t *testing.T) {
	// Type "world", Home, insert "hello ", End, insert "!", Enter
	line, _ := editWith(t, nil, "world\x1b[Hhello \x1b[F!\r")
	if line != "hello world!" {
		t.Errorf("got %q, want %q", line, "hello world!")
	}
}

func TestEditorCtrlAE(t *testing.T) {
	// Same as Home/End via Ctrl+A / Ctrl+E
	line, _ := editWith(t, nil, "world\x01hello \x05!\r")
	if line != "hello world!" {
		t.Errorf("got %q, want %q", line, "hello world!")
	}
}

func TestEditorCtrlW(t *testing.T) {
	// "foo bar baz" + Ctrl+W -> "foo bar "
	line, _ := editWith(t, nil, "foo bar baz\x17\r")
	if line != "foo bar " {
		t.Errorf("got %q, want %q", line, "foo bar ")
	}
}

func TestEditorCtrlWMultiSpaceBoundary(t *testing.T) {
	// Trailing whitespace should be consumed along with the word to its
	// left: "foo bar   " + Ctrl+W -> "foo " (drops "bar" + spaces).
	line, _ := editWith(t, nil, "foo bar   \x17\r")
	if line != "foo " {
		t.Errorf("got %q, want %q", line, "foo ")
	}
}

func TestEditorCtrlULineKill(t *testing.T) {
	// "abc def" Home (move to start), insert "X" -> "Xabc def"? No,
	// Ctrl+U kills from start to cursor. Easier test: type "abc", Ctrl+U,
	// type "xyz" -> "xyz".
	line, _ := editWith(t, nil, "abc\x15xyz\r")
	if line != "xyz" {
		t.Errorf("got %q, want %q", line, "xyz")
	}
}

func TestEditorCtrlKKillToEnd(t *testing.T) {
	// Type "hello world", Home (\x01), Ctrl+Right (CSI 1;5C), Ctrl+K,
	// Enter -> "hello" (cursor after "hello", everything to right killed).
	line, _ := editWith(t, nil, "hello world\x01\x1b[1;5C\x0b\r")
	if line != "hello" {
		t.Errorf("got %q, want %q", line, "hello")
	}
}

func TestEditorCtrlLeftRightWordMotions(t *testing.T) {
	// Type "alpha beta gamma", Ctrl+Left twice -> cursor at start of
	// "beta", insert "X" -> "alpha Xbeta gamma".
	line, _ := editWith(t, nil, "alpha beta gamma\x1b[1;5D\x1b[1;5DX\r")
	if line != "alpha Xbeta gamma" {
		t.Errorf("got %q, want %q", line, "alpha Xbeta gamma")
	}
}

func TestEditorAltBAltFAlsoWordMotions(t *testing.T) {
	// Some terminals (notably macOS) send ESC b / ESC f for option-arrow.
	// Same semantics as Ctrl+Left / Ctrl+Right.
	line, _ := editWith(t, nil, "alpha beta gamma\x1bb\x1bbX\r")
	if line != "alpha Xbeta gamma" {
		t.Errorf("got %q, want %q", line, "alpha Xbeta gamma")
	}
}

func TestEditorForwardDelete(t *testing.T) {
	// "abcd", Home, two Forward-Deletes (CSI 3~), Enter -> "cd"
	line, _ := editWith(t, nil, "abcd\x01\x1b[3~\x1b[3~\r")
	if line != "cd" {
		t.Errorf("got %q, want %q", line, "cd")
	}
}

func TestEditorCtrlCCancels(t *testing.T) {
	_, err := editWith(t, nil, "abc\x03")
	if !errors.Is(err, errLineCancelled) {
		t.Errorf("got %v, want errLineCancelled", err)
	}
}

func TestEditorCtrlDOnEmptyEOF(t *testing.T) {
	_, err := editWith(t, nil, "\x04")
	if !errors.Is(err, io.EOF) {
		t.Errorf("got %v, want io.EOF", err)
	}
}

func TestEditorCtrlDOnNonEmptyDeletes(t *testing.T) {
	// "abc", Home, Ctrl+D (which becomes forward-delete on non-empty),
	// Enter -> "bc".
	line, _ := editWith(t, nil, "abc\x01\x04\r")
	if line != "bc" {
		t.Errorf("got %q, want %q", line, "bc")
	}
}

func TestEditorHistoryUpDown(t *testing.T) {
	h := newReplHistory()
	h.Add("io.printf(1);")
	h.Add("io.printf(2);")
	h.Add("io.printf(3);")

	// Press Up three times to step back to the oldest, Enter -> "io.printf(1);"
	line, err := editWith(t, h, "\x1b[A\x1b[A\x1b[A\r")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if line != "io.printf(1);" {
		t.Errorf("got %q, want %q", line, "io.printf(1);")
	}

	// Up twice goes back 2 (io.printf(2);), Down once steps forward by 1
	// (toward the present) -> the most recent entry "io.printf(3);".
	line, _ = editWith(t, h, "\x1b[A\x1b[A\x1b[B\r")
	if line != "io.printf(3);" {
		t.Errorf("got %q, want %q", line, "io.printf(3);")
	}
}

func TestEditorHistoryDownReturnsEditedBuffer(t *testing.T) {
	h := newReplHistory()
	h.Add("oldest;")

	// Type "in-progress", press Up (replaces with "oldest;"), Down
	// (returns to "in-progress"), Enter.
	line, _ := editWith(t, h, "in-progress\x1b[A\x1b[B\r")
	if line != "in-progress" {
		t.Errorf("got %q, want %q", line, "in-progress")
	}
}

func TestHistoryAddSkipsEmpty(t *testing.T) {
	h := newReplHistory()
	h.Add("")
	if len(h.entries) != 0 {
		t.Errorf("empty entry was added: %v", h.entries)
	}
}

func TestHistoryAddCollapsesAdjacentDuplicates(t *testing.T) {
	h := newReplHistory()
	h.Add("a;")
	h.Add("a;")
	h.Add("b;")
	h.Add("a;")
	if len(h.entries) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(h.entries), h.entries)
	}
}

func TestHistoryRingTruncates(t *testing.T) {
	h := newReplHistory()
	h.max = 3
	for _, s := range []string{"a;", "b;", "c;", "d;", "e;"} {
		h.Add(s)
	}
	if len(h.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(h.entries))
	}
	if h.entries[0] != "c;" || h.entries[2] != "e;" {
		t.Errorf("ring contents wrong: %v", h.entries)
	}
}

// TestCrlfWriterTranslatesNewlines covers the small wrapper used to keep
// multi-line output readable while the terminal is in raw mode.
func TestCrlfWriterTranslatesNewlines(t *testing.T) {
	var buf bytes.Buffer
	w := crlfWriter{w: &buf}
	w.Write([]byte("first\nsecond\n"))
	if buf.String() != "first\r\nsecond\r\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestCrlfWriterPassesThroughWithoutNewline(t *testing.T) {
	var buf bytes.Buffer
	w := crlfWriter{w: &buf}
	w.Write([]byte("no newline"))
	if buf.String() != "no newline" {
		t.Errorf("got %q", buf.String())
	}
}

func TestCrlfWriterTracksTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	w := &crlfWriter{w: &buf}
	w.Write([]byte("hello"))
	if w.lastByteIsNL {
		t.Error("after 'hello' lastByteIsNL should be false")
	}
	w.Write([]byte("\n"))
	if !w.lastByteIsNL {
		t.Error("after '\\n' lastByteIsNL should be true")
	}
	w.Write([]byte("more"))
	if w.lastByteIsNL {
		t.Error("after 'more' lastByteIsNL should be false again")
	}
}

// TestEditorEmitsFreshLineAfterRawOutput reproduces the bug where printf
// output without a trailing newline got wiped by the next prompt's
// `\r ESC[K` redraw. The fix: when the shared crlfWriter shows
// lastByteIsNL=false at the start of readLine, the editor first emits
// `\r\n` so the redraw lands on a clean line. The assertion is that the
// editor's output (terminal byte stream) preserves the "5.0" payload -
// it shouldn't be erased before the prompt appears.
func TestEditorEmitsFreshLineAfterRawOutput(t *testing.T) {
	var buf bytes.Buffer
	out := &crlfWriter{w: &buf}
	// Simulate a program writing "5.0" (no newline) before the next
	// prompt cycle starts.
	out.Write([]byte("5.0"))
	if out.lastByteIsNL {
		t.Fatal("setup wrong: tracker should be false after no-newline write")
	}

	e := &lineEditor{
		in:      bufio.NewReader(strings.NewReader("\r")),
		out:     out,
		history: newReplHistory(),
	}
	_, _ = e.readLine(">>> ")

	got := buf.String()
	if !strings.Contains(got, "5.0") {
		t.Errorf("payload '5.0' missing from output: %q", got)
	}
	if !strings.Contains(got, "\r\n") {
		t.Errorf("expected a fresh-line emission before redraw, got %q", got)
	}
}
