// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

// lineEditor reads a single logical line from a terminal with the small set
// of editing affordances expected of a modern REPL: cursor movement, word
// motions, history navigation, and a handful of standard control-key
// shortcuts. It is intentionally small and TinyGo-friendly - no readline
// dependency, no plugin system, just a state machine over an in-memory
// rune buffer.
//
// The editor activates only when stdin is a terminal (see runRepl). When
// stdin is piped or redirected (`echo ... | jennifer repl`, integration
// tests) the REPL falls back to bufio-style line reading so non-interactive
// inputs keep working unchanged.
type lineEditor struct {
	in       *bufio.Reader // raw byte source (terminal stdin in raw mode)
	out      io.Writer     // where prompt/redraws are written (os.Stdout)
	fd       int           // file descriptor of stdin, used for term.MakeRaw
	history  *replHistory
	buf      []rune
	cur      int    // 0..len(buf), insertion point
	prompt   string // currently-active prompt (>>> or ...)
	color    bool   // recolour the committed line with syntax highlighting
	browsing int    // history position; 0 = "below the bottom" (current buffer); positive walks back
	stashed  []rune // edited buffer saved when user starts walking history
	stashedC int    // saved cursor position to restore when leaving history
}

// errLineCancelled is returned when the user hits Ctrl+C. The REPL treats
// it as "discard the in-progress line and continue with a fresh prompt."
var errLineCancelled = errors.New("line cancelled")

// readLine reads, edits, and returns one line of user input. The trailing
// newline is not included. Returns io.EOF when the user hits Ctrl+D on an
// empty buffer.
func (e *lineEditor) readLine(prompt string) (string, error) {
	e.prompt = prompt
	e.buf = e.buf[:0]
	e.cur = 0
	e.browsing = 0
	e.stashed = nil
	e.stashedC = 0
	// If previous output (printf, error message, REPL value) didn't end
	// in a newline, emit one now. Otherwise the redraw's `\r ESC[K` would
	// erase the previous content on this line.
	if cr, ok := e.out.(*crlfWriter); ok && !cr.lastByteIsNL {
		// Direct write to the underlying terminal - bypass the wrapper so
		// we don't accidentally bump the tracker for our own setup byte.
		cr.w.Write([]byte("\r\n"))
		cr.lastByteIsNL = true
	}
	e.redraw()

	for {
		r, sz, err := e.in.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) && len(e.buf) == 0 {
				return "", io.EOF
			}
			return "", err
		}
		_ = sz

		// Most keys arrive as a single rune; arrow keys and friends arrive
		// as a multi-byte escape sequence starting with 0x1b. Branch on
		// the first byte to keep the hot path simple.
		switch r {
		case 0x1b: // ESC - start of a CSI sequence (probably)
			e.handleEscape()
			continue
		case '\r', '\n':
			// Commit the line. Terminal raw mode produces \r on Enter;
			// either way we want to leave the cursor on a fresh line. When
			// colour is on, redraw the committed line one last time with
			// syntax highlighting before the newline, so the just-entered
			// source shows coloured above its output.
			if e.color {
				e.redrawCommitted()
			}
			e.out.Write([]byte("\r\n"))
			s := string(e.buf)
			return s, nil
		case 0x7f, 0x08: // Backspace (DEL on most terminals, BS on some)
			e.backspaceChar()
		case 0x01: // Ctrl+A
			e.toLineStart()
		case 0x05: // Ctrl+E
			e.toLineEnd()
		case 0x02: // Ctrl+B
			e.moveLeft()
		case 0x06: // Ctrl+F
			e.moveRight()
		case 0x03: // Ctrl+C
			e.out.Write([]byte("^C\r\n"))
			return "", errLineCancelled
		case 0x04: // Ctrl+D
			if len(e.buf) == 0 {
				e.out.Write([]byte("\r\n"))
				return "", io.EOF
			}
			// Non-empty buffer: behave like forward-delete.
			e.deleteChar()
		case 0x0b: // Ctrl+K - kill from cursor to end of line
			e.killToEnd()
		case 0x15: // Ctrl+U - kill from start of line to cursor
			e.killToStart()
		case 0x17: // Ctrl+W - kill word backward
			e.killWordLeft()
		case 0x09:
			// Tab is reserved for future autocomplete; for now ignore.
		default:
			if r < 0x20 {
				// Unhandled control character; swallow.
				continue
			}
			e.insertRune(r)
		}
	}
}

// handleEscape consumes the bytes following an ESC and dispatches the key.
// Recognises CSI sequences (`ESC [ ...`) plus the few alt-prefixed forms
// some terminals send for ctrl-arrow.
func (e *lineEditor) handleEscape() {
	b, err := e.in.ReadByte()
	if err != nil {
		return
	}
	if b != '[' && b != 'O' {
		// Alt-<char> form (less common). Map alt-b / alt-f to word moves
		// so Mac terminals that send those for option-arrow still work.
		switch b {
		case 'b':
			e.moveWordLeft()
		case 'f':
			e.moveWordRight()
		}
		return
	}

	// CSI sequence body: zero or more parameters (digits and `;`), then a
	// final byte that names the action. Read until we hit a final byte.
	var params strings.Builder
	for {
		nb, err := e.in.ReadByte()
		if err != nil {
			return
		}
		if (nb >= '0' && nb <= '9') || nb == ';' {
			params.WriteByte(nb)
			continue
		}
		e.dispatchCSI(params.String(), nb)
		return
	}
}

// dispatchCSI handles the CSI body once both parameters and the final byte
// have been read. The two cases we care about:
//   - `A`/`B`/`C`/`D` with no modifier: arrow keys.
//   - `A`/`B`/`C`/`D` with `;5` modifier: Ctrl+arrow word motions.
//   - `H`/`F` and `1~`/`4~`/`7~`/`8~`: Home/End in various terminal flavors.
//   - `3~`: forward Delete.
func (e *lineEditor) dispatchCSI(params string, final byte) {
	ctrlMod := strings.HasSuffix(params, ";5")
	switch final {
	case 'A':
		e.historyBack()
	case 'B':
		e.historyForward()
	case 'C':
		if ctrlMod {
			e.moveWordRight()
		} else {
			e.moveRight()
		}
	case 'D':
		if ctrlMod {
			e.moveWordLeft()
		} else {
			e.moveLeft()
		}
	case 'H':
		e.toLineStart()
	case 'F':
		e.toLineEnd()
	case '~':
		switch params {
		case "1", "7":
			e.toLineStart()
		case "4", "8":
			e.toLineEnd()
		case "3":
			e.deleteChar()
		}
	}
}

// --- editing primitives ---

func (e *lineEditor) insertRune(r rune) {
	e.buf = append(e.buf, 0)
	copy(e.buf[e.cur+1:], e.buf[e.cur:])
	e.buf[e.cur] = r
	e.cur++
	e.redraw()
}

func (e *lineEditor) backspaceChar() {
	if e.cur == 0 {
		return
	}
	e.buf = append(e.buf[:e.cur-1], e.buf[e.cur:]...)
	e.cur--
	e.redraw()
}

func (e *lineEditor) deleteChar() {
	if e.cur >= len(e.buf) {
		return
	}
	e.buf = append(e.buf[:e.cur], e.buf[e.cur+1:]...)
	e.redraw()
}

func (e *lineEditor) moveLeft() {
	if e.cur > 0 {
		e.cur--
		e.redraw()
	}
}

func (e *lineEditor) moveRight() {
	if e.cur < len(e.buf) {
		e.cur++
		e.redraw()
	}
}

func (e *lineEditor) toLineStart() {
	if e.cur != 0 {
		e.cur = 0
		e.redraw()
	}
}

func (e *lineEditor) toLineEnd() {
	if e.cur != len(e.buf) {
		e.cur = len(e.buf)
		e.redraw()
	}
}

// moveWordLeft / moveWordRight walk by "word" boundaries. The convention is
// the one most users expect: skip any whitespace adjacent to the cursor in
// the direction of travel, then stop at the first whitespace boundary.
func (e *lineEditor) moveWordLeft() {
	if e.cur == 0 {
		return
	}
	i := e.cur
	// skip whitespace immediately to the left
	for i > 0 && isWordSep(e.buf[i-1]) {
		i--
	}
	// then skip the word characters
	for i > 0 && !isWordSep(e.buf[i-1]) {
		i--
	}
	e.cur = i
	e.redraw()
}

func (e *lineEditor) moveWordRight() {
	if e.cur >= len(e.buf) {
		return
	}
	i := e.cur
	// skip whitespace immediately to the right
	for i < len(e.buf) && isWordSep(e.buf[i]) {
		i++
	}
	// then skip the word characters
	for i < len(e.buf) && !isWordSep(e.buf[i]) {
		i++
	}
	e.cur = i
	e.redraw()
}

func (e *lineEditor) killWordLeft() {
	if e.cur == 0 {
		return
	}
	end := e.cur
	i := e.cur
	for i > 0 && isWordSep(e.buf[i-1]) {
		i--
	}
	for i > 0 && !isWordSep(e.buf[i-1]) {
		i--
	}
	e.buf = append(e.buf[:i], e.buf[end:]...)
	e.cur = i
	e.redraw()
}

func (e *lineEditor) killToEnd() {
	if e.cur >= len(e.buf) {
		return
	}
	e.buf = e.buf[:e.cur]
	e.redraw()
}

func (e *lineEditor) killToStart() {
	if e.cur == 0 {
		return
	}
	e.buf = e.buf[e.cur:]
	e.cur = 0
	e.redraw()
}

// isWordSep treats runs of spaces / tabs / punctuation as breaks. Sticking
// to whitespace + a small punctuation set keeps the behavior predictable
// for source-code editing without needing a full Unicode word-break
// implementation.
func isWordSep(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '(', ')', '[', ']', '{', '}', ',', ';', '.', ':', '"', '\'', '=', '+', '-', '*', '/', '%', '<', '>':
		return true
	}
	return false
}

// --- history ---

func (e *lineEditor) historyBack() {
	if e.history == nil || len(e.history.entries) == 0 {
		return
	}
	if e.browsing == 0 {
		// Save the buffer we were editing so we can come back to it.
		e.stashed = append(e.stashed[:0], e.buf...)
		e.stashedC = e.cur
	}
	if e.browsing >= len(e.history.entries) {
		return
	}
	e.browsing++
	entry := e.history.entries[len(e.history.entries)-e.browsing]
	e.buf = append(e.buf[:0], []rune(entry)...)
	e.cur = len(e.buf)
	e.redraw()
}

func (e *lineEditor) historyForward() {
	if e.history == nil || e.browsing == 0 {
		return
	}
	e.browsing--
	if e.browsing == 0 {
		e.buf = append(e.buf[:0], e.stashed...)
		e.cur = e.stashedC
	} else {
		entry := e.history.entries[len(e.history.entries)-e.browsing]
		e.buf = append(e.buf[:0], []rune(entry)...)
		e.cur = len(e.buf)
	}
	e.redraw()
}

// --- rendering ---

// redraw rewrites the current input line in place. It uses three ANSI
// escapes:
//
//	\r        carriage return (cursor back to column 0)
//	ESC[K     clear from cursor to end of line
//	ESC[<n>C  move cursor right N columns (used to place the cursor inside
//	          the line when the user has moved away from the end)
//
// This is the smallest set of sequences that gives a stable, flicker-free
// edit experience for single-line input. Wide / combining characters
// aren't handled - the editor assumes one rune = one column, which is
// true for ASCII source code (Jennifer doesn't allow non-ASCII identifiers
// anyway).
func (e *lineEditor) redraw() {
	var b strings.Builder
	b.WriteByte('\r')
	b.WriteString("\x1b[K")
	b.WriteString(e.prompt)
	b.WriteString(string(e.buf))
	if e.cur < len(e.buf) {
		// Move cursor back to the insertion point.
		back := len(e.buf) - e.cur
		b.WriteString("\x1b[")
		b.WriteString(itoa(back))
		b.WriteString("D")
	}
	e.out.Write([]byte(b.String()))
}

// redrawCommitted rewrites the line one final time with syntax highlighting,
// used at commit (Enter) just before the newline. There is no cursor-back
// step: the cursor is about to move to a fresh line, so the colour spans (all
// zero-width ANSI escapes) don't disturb the layout.
func (e *lineEditor) redrawCommitted() {
	var b strings.Builder
	b.WriteByte('\r')
	b.WriteString("\x1b[K")
	b.WriteString(e.prompt)
	b.WriteString(highlightLine(string(e.buf)))
	e.out.Write([]byte(b.String()))
}

// itoa is a tiny stack-only int-to-string used only for small CSI offsets.
// Avoids pulling strconv for one usage and keeps the redraw path cheap.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// --- raw mode + TTY detection ---

// isTTY reports whether stdin is connected to a terminal. The line editor
// only activates in this case; piped or redirected stdin falls back to the
// bufio reader so non-interactive runs (e.g. tests) keep working.
func isTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// withRawMode installs raw-mode termios for stdin and returns a restore
// function that the caller must defer. If the fd isn't a terminal the
// returned restore is a no-op and ok is false.
func withRawMode(f *os.File) (restore func(), ok bool) {
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return func() {}, false
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}, false
	}
	return func() { _ = term.Restore(fd, state) }, true
}

// newLineEditor builds an editor over stdin. The caller is responsible for
// switching the terminal into raw mode beforehand (see withRawMode).
func newLineEditor(stdin *os.File, stdout io.Writer, history *replHistory) *lineEditor {
	return &lineEditor{
		in:      bufio.NewReader(stdin),
		out:     stdout,
		fd:      int(stdin.Fd()),
		history: history,
	}
}
