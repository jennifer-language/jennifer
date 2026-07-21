# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Terminal user interfaces: an explicit `screen`, not a GUI framework. Two
 * layers. The output-only layer is pure strings over ANSI control sequences -
 * a cell `Buffer` you draw into (`text` / `box` / `fill`) and paint with
 * `render`, plus `diff` for a flicker-free update loop (dashboards, progress,
 * self-updating tables); it needs no host capability and runs on both binaries.
 * The interactive layer decodes a raw byte stream into `Key` events
 * (`decodeKey`, pure and testable) and drives an event loop over the `term`
 * library's raw mode (`begin` / `nextKey` / `end`) for menus, forms, and key
 * navigation; that layer needs `term`, so it works on the default `jennifer`
 * (a friendly error on `jennifer-tiny`, which stubs `term`).
 *
 * Coordinates are 0-based with the origin at the top-left `(0, 0)`; `x` is the
 * column, `y` the row. Drawing that runs past an edge is clipped, not an error.
 * @module screen
 * @example
 * use io;
 * import "screen.j" as screen;
 * def s as screen.Buffer init screen.newScreen(3, 20);
 * def s2 as screen.Buffer init screen.box($s, 0, 0, 20, 3);
 * io.printf("%s%s", screen.clear(), screen.render(screen.text($s2, 2, 1, "hi")));
 */
use convert;
use strings;
use maps;
use io;
use term;

# The ESC control byte (27) has no string-literal escape in Jennifer, so it is
# built from a one-byte `bytes`; CSI is the "ESC [" control-sequence introducer.
def const ESC as string init charOf(27);
def const CSI as string init ESC + "[";

# SGR foreground colour codes for `textColor` (per-cell, so the diff stays
# correct); mirrors the `ansi` module's names.
def const FG as map of string to string init {
    "black": "30", "red": "31", "green": "32", "yellow": "33",
    "blue": "34", "magenta": "35", "cyan": "36", "white": "37",
    "gray": "90", "grey": "90"
};

/**
 * A rectangular grid of character cells, drawn into then painted to the
 * terminal. `cells` is row-major (`y * cols + x`); each cell is a one-column
 * string, optionally wrapped in an SGR colour sequence. Value-semantic like any
 * struct: the drawing functions return a fresh `Buffer`.
 * @field rows {int} the number of rows (height)
 * @field cols {int} the number of columns (width)
 * @field cells {list of string} row-major cell contents, length `rows * cols`
 */
export def struct Buffer {
    rows as int,
    cols as int,
    cells as list of string
};

/**
 * A decoded key event from `nextKey` / `decodeKey`. `name` is a symbolic key
 * name: a printable key is `"char"` (with the character in `char`); the rest
 * are named - `"up"` / `"down"` / `"left"` / `"right"`, `"enter"` / `"tab"` /
 * `"escape"` / `"backspace"` / `"delete"` / `"insert"`, `"home"` / `"end"` /
 * `"pageup"` / `"pagedown"`, `"f1"`..`"f12"`, `"ctrl-a"`..`"ctrl-z"`,
 * `"alt-<c>"`, `"eof"` at end of input, and `"unknown"`.
 * @field name {string} the symbolic key name
 * @field char {string} the character for a `"char"` / `"alt-"` key, else empty
 */
export def struct Key {
    name as string,
    char as string
};

# charOf builds a one-character string from a byte value (ASCII / Latin-1).
func charOf(n as int) {
    def b as bytes;
    $b[] = $n;
    return convert.stringFromBytes($b, "utf-8");
}

# ---- Stage 1: ANSI control-sequence builders (pure strings) ----

/**
 * The sequence that clears the whole screen and homes the cursor.
 * @return {string} the clear-screen escape sequence
 */
export func clear() {
    return CSI + "2J" + CSI + "H";
}
/**
 * The sequence that clears the current line.
 * @return {string} the clear-line escape sequence
 */
export func clearLine() {
    return CSI + "2K";
}
/**
 * The sequence that moves the cursor to `(x, y)` (0-based; origin top-left).
 * @param x {int} the target column (0-based)
 * @param y {int} the target row (0-based)
 * @return {string} the cursor-position escape sequence
 */
export func moveTo(x as int, y as int) {
    return CSI + convert.toString($y + 1) + ";" + convert.toString($x + 1) + "H";
}
/**
 * The sequence that moves the cursor up `n` rows.
 * @param n {int} the number of rows
 * @return {string} the cursor-up escape sequence
 */
export func up(n as int) {
    return CSI + convert.toString($n) + "A";
}
/**
 * The sequence that moves the cursor down `n` rows.
 * @param n {int} the number of rows
 * @return {string} the cursor-down escape sequence
 */
export func down(n as int) {
    return CSI + convert.toString($n) + "B";
}
/**
 * The sequence that moves the cursor right `n` columns.
 * @param n {int} the number of columns
 * @return {string} the cursor-forward escape sequence
 */
export func right(n as int) {
    return CSI + convert.toString($n) + "C";
}
/**
 * The sequence that moves the cursor left `n` columns.
 * @param n {int} the number of columns
 * @return {string} the cursor-back escape sequence
 */
export func left(n as int) {
    return CSI + convert.toString($n) + "D";
}
/**
 * The sequence that homes the cursor to the top-left.
 * @return {string} the cursor-home escape sequence
 */
export func home() {
    return CSI + "H";
}
/**
 * The sequence that hides the text cursor.
 * @return {string} the hide-cursor escape sequence
 */
export func hideCursor() {
    return CSI + "?25l";
}
/**
 * The sequence that shows the text cursor.
 * @return {string} the show-cursor escape sequence
 */
export func showCursor() {
    return CSI + "?25h";
}
/**
 * The sequence that switches to the alternate screen buffer (so the app's
 * output does not scroll the user's shell history).
 * @return {string} the enter-alternate-screen escape sequence
 */
export func enterAlt() {
    return CSI + "?1049h";
}
/**
 * The sequence that leaves the alternate screen buffer, restoring the prior
 * terminal contents.
 * @return {string} the leave-alternate-screen escape sequence
 */
export func exitAlt() {
    return CSI + "?1049l";
}

# ---- Stage 1: the cell buffer ----

/**
 * A new blank `Buffer` of `rows` by `cols` cells, every cell a space.
 * @param rows {int} the number of rows (must be positive)
 * @param cols {int} the number of columns (must be positive)
 * @return {Buffer} the blank buffer
 * @throws {Error} when `rows` or `cols` is not positive
 */
export func newScreen(rows as int, cols as int) {
    if ($rows <= 0 or $cols <= 0) {
        throw Error{kind: "value", message: "screen.newScreen: rows and cols must be positive", file: "", line: 0, col: 0};
    }
    def cells as list of string init [];
    def total as int init $rows * $cols;
    for (def i as int init 0; $i < $total; $i = $i + 1) {
        $cells[] = " ";
    }
    return Buffer{rows: $rows, cols: $cols, cells: $cells};
}

/**
 * A copy of `buf` with the single cell at `(x, y)` set to `cell` (one column).
 * Out-of-range coordinates are clipped (returns `buf` unchanged).
 * @param buf {Buffer} the source buffer
 * @param x {int} the column (0-based)
 * @param y {int} the row (0-based)
 * @param cell {string} the cell contents (one visible column)
 * @return {Buffer} the updated buffer
 */
export func set(buf as Buffer, x as int, y as int, cell as string) {
    def out as Buffer init $buf;
    if ($x >= 0 and $x < $out.cols and $y >= 0 and $y < $out.rows) {
        $out.cells[$y * $out.cols + $x] = $cell;
    }
    return $out;
}

/**
 * The contents of the cell at `(x, y)`, or an empty string if out of range.
 * @param buf {Buffer} the buffer
 * @param x {int} the column (0-based)
 * @param y {int} the row (0-based)
 * @return {string} the cell contents
 */
export func get(buf as Buffer, x as int, y as int) {
    if ($x >= 0 and $x < $buf.cols and $y >= 0 and $y < $buf.rows) {
        return $buf.cells[$y * $buf.cols + $x];
    }
    return "";
}

/**
 * A copy of `buf` with `s` written left-to-right starting at `(x, y)`, one rune
 * per cell, clipped at the row's right edge (no wrapping).
 * @param buf {Buffer} the source buffer
 * @param x {int} the starting column (0-based)
 * @param y {int} the row (0-based)
 * @param s {string} the text to write
 * @return {Buffer} the updated buffer
 */
export func text(buf as Buffer, x as int, y as int, s as string) {
    return writeRunes($buf, $x, $y, strings.chars($s), "");
}

/**
 * A copy of `buf` with `s` written from `(x, y)` in the named foreground
 * colour, one rune per cell, clipped at the row's right edge.
 * @param buf {Buffer} the source buffer
 * @param x {int} the starting column (0-based)
 * @param y {int} the row (0-based)
 * @param s {string} the text to write
 * @param color {string} the colour name (e.g. `"red"`, `"cyan"`)
 * @return {Buffer} the updated buffer
 * @throws {Error} when `color` is not a known colour
 */
export func textColor(buf as Buffer, x as int, y as int, s as string, color as string) {
    if (not maps.has(FG, $color)) {
        throw Error{kind: "value", message: "screen.textColor: unknown colour: " + $color, file: "", line: 0, col: 0};
    }
    return writeRunes($buf, $x, $y, strings.chars($s), FG[$color]);
}

# writeRunes places each rune of `runes` into consecutive cells from (x, y),
# wrapping each in an SGR sequence when `code` is non-empty. Clips at the row
# edge. One local copy, mutated in place - O(cells + len(runes)), not O(len^2).
func writeRunes(buf as Buffer, x as int, y as int, runes as list of string, code as string) {
    def out as Buffer init $buf;
    if ($y < 0 or $y >= $out.rows) {
        return $out;
    }
    def col as int init $x;
    for (def i as int init 0; $i < len($runes); $i = $i + 1) {
        if ($col >= 0 and $col < $out.cols) {
            def cell as string init $runes[$i];
            if (len($code) > 0) {
                $cell = CSI + $code + "m" + $cell + CSI + "0m";
            }
            $out.cells[$y * $out.cols + $col] = $cell;
        }
        $col = $col + 1;
    }
    return $out;
}

/**
 * A copy of `buf` with the rectangle at `(x, y)` of size `w` by `h` filled with
 * the glyph `ch`. Clipped to the buffer.
 * @param buf {Buffer} the source buffer
 * @param x {int} the left column (0-based)
 * @param y {int} the top row (0-based)
 * @param w {int} the width in columns
 * @param h {int} the height in rows
 * @param ch {string} the fill glyph (one column)
 * @return {Buffer} the updated buffer
 */
export func fill(buf as Buffer, x as int, y as int, w as int, h as int, ch as string) {
    def out as Buffer init $buf;
    for (def row as int init $y; $row < $y + $h; $row = $row + 1) {
        for (def col as int init $x; $col < $x + $w; $col = $col + 1) {
            if ($col >= 0 and $col < $out.cols and $row >= 0 and $row < $out.rows) {
                $out.cells[$row * $out.cols + $col] = $ch;
            }
        }
    }
    return $out;
}

/**
 * A copy of `buf` with a horizontal line of `n` cells of `ch` from `(x, y)`.
 * @param buf {Buffer} the source buffer
 * @param x {int} the starting column (0-based)
 * @param y {int} the row (0-based)
 * @param n {int} the length in columns
 * @param ch {string} the line glyph (one column)
 * @return {Buffer} the updated buffer
 */
export func hline(buf as Buffer, x as int, y as int, n as int, ch as string) {
    return fill($buf, $x, $y, $n, 1, $ch);
}

/**
 * A copy of `buf` with a vertical line of `n` cells of `ch` from `(x, y)`.
 * @param buf {Buffer} the source buffer
 * @param x {int} the column (0-based)
 * @param y {int} the starting row (0-based)
 * @param n {int} the length in rows
 * @param ch {string} the line glyph (one column)
 * @return {Buffer} the updated buffer
 */
export func vline(buf as Buffer, x as int, y as int, n as int, ch as string) {
    return fill($buf, $x, $y, 1, $n, $ch);
}

/**
 * A copy of `buf` with a single-line box border drawn at `(x, y)` of outer
 * size `w` by `h`, using Unicode box-drawing glyphs. The interior is untouched.
 * @param buf {Buffer} the source buffer
 * @param x {int} the left column (0-based)
 * @param y {int} the top row (0-based)
 * @param w {int} the outer width in columns (>= 2)
 * @param h {int} the outer height in rows (>= 2)
 * @return {Buffer} the updated buffer
 */
export func box(buf as Buffer, x as int, y as int, w as int, h as int) {
    def out as Buffer init $buf;
    if ($w < 2 or $h < 2) {
        return $out;
    }
    def right as int init $x + $w - 1;
    def bottom as int init $y + $h - 1;
    $out = hline($out, $x + 1, $y, $w - 2, "─");
    $out = hline($out, $x + 1, $bottom, $w - 2, "─");
    $out = vline($out, $x, $y + 1, $h - 2, "│");
    $out = vline($out, $right, $y + 1, $h - 2, "│");
    $out = set($out, $x, $y, "┌");
    $out = set($out, $right, $y, "┐");
    $out = set($out, $x, $bottom, "└");
    $out = set($out, $right, $bottom, "┘");
    return $out;
}

/**
 * A copy of `buf` with every cell reset to a space.
 * @param buf {Buffer} the source buffer
 * @return {Buffer} the cleared buffer
 */
export func clearBuffer(buf as Buffer) {
    return newScreen($buf.rows, $buf.cols);
}

/**
 * The escape string that paints the whole `buf` to the terminal (positions the
 * cursor at the start of each row and writes its cells). Print it with
 * `io.printf`; usually preceded once by `clear`.
 * @param buf {Buffer} the buffer to paint
 * @return {string} the full-repaint escape string
 */
export func render(buf as Buffer) {
    def parts as list of string init [];
    for (def y as int init 0; $y < $buf.rows; $y = $y + 1) {
        $parts[] = moveTo(0, $y);
        def rowStart as int init $y * $buf.cols;
        for (def x as int init 0; $x < $buf.cols; $x = $x + 1) {
            $parts[] = $buf.cells[$rowStart + $x];
        }
    }
    return strings.join($parts, "");
}

/**
 * The minimal escape string that turns the terminal showing `old` into `new` -
 * only the changed cells are repositioned and rewritten, in row runs. When the
 * two buffers differ in size it falls back to a full `render(new)`. This is the
 * flicker-free update path: keep the previous buffer, `diff` against the next.
 * @param old {Buffer} the buffer currently on screen
 * @param new {Buffer} the buffer to display
 * @return {string} the minimal-update escape string
 */
export func diff(old as Buffer, new as Buffer) {
    if ($old.rows != $new.rows or $old.cols != $new.cols) {
        return render($new);
    }
    def parts as list of string init [];
    def cols as int init $new.cols;
    for (def y as int init 0; $y < $new.rows; $y = $y + 1) {
        def x as int init 0;
        for (; $x < $cols; ) {
            def idx as int init $y * $cols + $x;
            if ($old.cells[$idx] == $new.cells[$idx]) {
                $x = $x + 1;
            } else {
                # Start of a changed run: emit one move, then every changed
                # cell until the buffers agree again.
                $parts[] = moveTo($x, $y);
                for (; $x < $cols and $old.cells[$y * $cols + $x] != $new.cells[$y * $cols + $x]; $x = $x + 1) {
                    $parts[] = $new.cells[$y * $cols + $x];
                }
            }
        }
    }
    return strings.join($parts, "");
}

# ---- Stage 2: key decoding (pure) + the event loop (over `term`) ----

# csiParam reads the decimal parameter of a CSI sequence: the digits between
# index 2 and the final byte. Returns -1 when there are no digits.
func csiParam(seq as list of int) {
    def n as int init 0;
    def seen as bool init false;
    for (def i as int init 2; $i < len($seq) - 1; $i = $i + 1) {
        def b as int init $seq[$i];
        if ($b >= 48 and $b <= 57) {
            $n = $n * 10 + ($b - 48);
            $seen = true;
        }
    }
    if ($seen) {
        return $n;
    }
    return -1;
}

# tildeKey maps a CSI "<n>~" parameter to its key name.
func tildeKey(param as int) {
    if ($param == 1) { return "home"; }
    if ($param == 2) { return "insert"; }
    if ($param == 3) { return "delete"; }
    if ($param == 4) { return "end"; }
    if ($param == 5) { return "pageup"; }
    if ($param == 6) { return "pagedown"; }
    if ($param == 15) { return "f5"; }
    if ($param == 17) { return "f6"; }
    if ($param == 18) { return "f7"; }
    if ($param == 19) { return "f8"; }
    if ($param == 20) { return "f9"; }
    if ($param == 21) { return "f10"; }
    if ($param == 23) { return "f11"; }
    if ($param == 24) { return "f12"; }
    return "unknown";
}

# finalKey maps a CSI / SS3 final byte (arrows, home/end, F1-F4) to a name.
func finalKey(b as int) {
    if ($b == 65) { return "up"; }
    if ($b == 66) { return "down"; }
    if ($b == 67) { return "right"; }
    if ($b == 68) { return "left"; }
    if ($b == 72) { return "home"; }
    if ($b == 70) { return "end"; }
    if ($b == 80) { return "f1"; }
    if ($b == 81) { return "f2"; }
    if ($b == 82) { return "f3"; }
    if ($b == 83) { return "f4"; }
    return "unknown";
}

/**
 * Decode one raw key byte sequence into a `Key`. Pure and total: `seq` is the
 * bytes of a single key press (`[65]` for `A`, `[27, 91, 65]` for Up,
 * `[27, 91, 51, 126]` for Delete). Unrecognized input decodes to `"unknown"`;
 * an empty sequence to `"eof"`. This is what `nextKey` calls after reading the
 * bytes, exposed separately so key handling is testable without a terminal.
 * @param seq {list of int} the raw byte values of one key event
 * @return {Key} the decoded key
 */
export func decodeKey(seq as list of int) {
    if (len($seq) == 0) {
        return Key{name: "eof", char: ""};
    }
    def b as int init $seq[0];
    if (len($seq) == 1) {
        if ($b == 13 or $b == 10) { return Key{name: "enter", char: ""}; }
        if ($b == 9) { return Key{name: "tab", char: ""}; }
        if ($b == 27) { return Key{name: "escape", char: ""}; }
        if ($b == 127 or $b == 8) { return Key{name: "backspace", char: ""}; }
        if ($b == 0) { return Key{name: "ctrl-space", char: ""}; }
        if ($b >= 1 and $b <= 26) { return Key{name: "ctrl-" + charOf($b + 96), char: ""}; }
        if ($b >= 32 and $b <= 126) { return Key{name: "char", char: charOf($b)}; }
        return Key{name: "unknown", char: ""};
    }
    # Multi-byte: an ESC-introduced sequence.
    if ($b == 27) {
        def c as int init $seq[1];
        if ($c == 91 or $c == 79) {
            def last as int init $seq[len($seq) - 1];
            if ($last == 126) {
                return Key{name: tildeKey(csiParam($seq)), char: ""};
            }
            return Key{name: finalKey($last), char: ""};
        }
        # ESC then a printable byte is Alt+<char>.
        if ($c >= 32 and $c <= 126) {
            return Key{name: "alt-" + charOf($c), char: charOf($c)};
        }
    }
    return Key{name: "unknown", char: ""};
}

/**
 * Read and decode the next key from the terminal (blocking). Reads raw bytes
 * through the `term` library, assembling a full escape sequence before
 * decoding, and returns the `Key`. At end of input the name is `"eof"`.
 * Requires raw mode (see `begin`) and the `term` library (default binary).
 *
 * A lone Escape press is only reported once the next byte arrives, because
 * `term.readByte` has no timeout to distinguish it from the start of an escape
 * sequence; prefer a named key (or Ctrl-C) to quit an event loop.
 * @return {Key} the next key event
 */
export func nextKey() {
    def b as int init term.readByte();
    if ($b == -1) {
        return Key{name: "eof", char: ""};
    }
    def seq as list of int init [];
    $seq[] = $b;
    if ($b == 27) {
        def c as int init term.readByte();
        if ($c == -1) {
            return decodeKey($seq);
        }
        $seq[] = $c;
        if ($c == 91 or $c == 79) {
            # Read the rest of the CSI / SS3 sequence up to its final byte
            # (a letter, or `~` for the numeric-parameter forms).
            repeat {
                def d as int init term.readByte();
                if ($d == -1) {
                    break;
                }
                $seq[] = $d;
                if (($d >= 65 and $d <= 90) or ($d >= 97 and $d <= 122) or $d == 126) {
                    break;
                }
            } until (false);
        }
    }
    return decodeKey($seq);
}

/**
 * Enter full-screen interactive mode: put the terminal in raw mode and switch
 * to the alternate screen with the cursor hidden and the screen cleared. Pair
 * with `end`, ideally via `defer screen.end($state);`. Requires the `term`
 * library (default binary).
 * @return {term.State} the raw-mode handle to pass to `end`
 */
export func begin() {
    def state as term.State init term.makeRaw("stdin");
    io.printf("%s%s%s", enterAlt(), hideCursor(), clear());
    return $state;
}

/**
 * Leave interactive mode: show the cursor, leave the alternate screen, and
 * restore the terminal from the handle `begin` returned.
 * @param state {term.State} the handle returned by `begin`
 * @return {null} nothing
 */
export func end(state as term.State) {
    io.printf("%s%s", showCursor(), exitAlt());
    term.restore($state);
    return;
}

/**
 * The terminal size as a `term.Size` (`{rows, cols}`), a convenience passthrough
 * to `term.size` so an app stays in one namespace. Requires the `term` library.
 * @return {term.Size} the terminal dimensions
 */
export func size() {
    return term.size("stdout");
}
