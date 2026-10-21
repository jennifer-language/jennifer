# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

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
use lists;
use regex;
use maps;
use io;
use term;
use channel;
use task;

# The ESC control byte (27); CSI is the "ESC [" control-sequence introducer.
def const ESC as string init "\u001b";
def const CSI as string init ESC + "[";

# SGR foreground colour codes for `textColor` (per-cell, so the diff stays
# correct); mirrors the `ansi` module's names.
def const FG as map of string to string init {
    "black": "30",
    "red": "31",
    "green": "32",
    "yellow": "33",
    "blue": "34",
    "magenta": "35",
    "cyan": "36",
    "white": "37",
    "gray": "90",
    "grey": "90"
};

/**
 * A rectangular grid of character cells, drawn into then painted to the
 * terminal. `cells` is row-major (`y * cols + x`); each cell is a one-column
 * string, optionally wrapped in an SGR colour sequence. Value-semantic like any
 * struct: a drawing function returns the updated `Buffer`. Written as
 * `$buf = screen.set($buf, ...);` in a function, a call updates the grid in place;
 * anywhere else (top-level code, a global, inside a `try`, or keeping the old
 * buffer too) it copies the whole grid, so batch such writes with `setMany` /
 * `textMany`.
 *
 * A cell is assumed to be one terminal column wide. A wide glyph (CJK, most
 * emoji) takes two columns on screen and an East-Asian-ambiguous one (such as
 * `♥`) takes two on some CJK-locale terminals, shifting the rest of its row out
 * of step with the buffer; keep cell text to single-width characters.
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

/**
 * One cell write for `setMany`: the glyph `cell` at `(x, y)`.
 * @field x {int} the column (0-based)
 * @field y {int} the row (0-based)
 * @field cell {string} the cell contents (one visible column)
 */
export def struct Plot {
    x as int,
    y as int,
    cell as string
};

/**
 * One text run for `textMany`: `text` written from `(x, y)`, in the named
 * foreground `color`, or uncoloured when `color` is empty.
 * @field x {int} the starting column (0-based)
 * @field y {int} the row (0-based)
 * @field text {string} the text to write
 * @field color {string} a colour name (as for `textColor`), or `""` for none
 */
export def struct Span {
    x as int,
    y as int,
    text as string,
    color as string
};

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
    return CSI + "{$y + 1};{$x + 1}H";
}
/**
 * The sequence that moves the cursor up `n` rows.
 * @param n {int} the number of rows
 * @return {string} the cursor-up escape sequence
 */
export func up(n as int) {
    return CSI + "{$n}A";
}
/**
 * The sequence that moves the cursor down `n` rows.
 * @param n {int} the number of rows
 * @return {string} the cursor-down escape sequence
 */
export func down(n as int) {
    return CSI + "{$n}B";
}
/**
 * The sequence that moves the cursor right `n` columns.
 * @param n {int} the number of columns
 * @return {string} the cursor-forward escape sequence
 */
export func right(n as int) {
    return CSI + "{$n}C";
}
/**
 * The sequence that moves the cursor left `n` columns.
 * @param n {int} the number of columns
 * @return {string} the cursor-back escape sequence
 */
export func left(n as int) {
    return CSI + "{$n}D";
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
        throw Error{kind: "value", message: "screen.newScreen: rows and cols must be positive"};
    }
    # Bound the grid so a huge rows*cols cannot overflow the product or allocate a
    # multi-gigabyte cell list (a real terminal is far below this ceiling).
    if ($rows > 65535 or $cols > 65535 or $rows * $cols > 16777216) {
        throw Error{kind: "value", message: "screen.newScreen: dimensions too large"};
    }
    # One native repeat + split instead of rows*cols appends from .j.
    def cells as list of string init strings.chars(strings.repeat(" ", $rows * $cols));
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
    if ($x >= 0 and $x < $buf.cols and $y >= 0 and $y < $buf.rows) {
        $buf.cells[$y * $buf.cols + $x] = $cell;
    }
    return $buf;
}

/**
 * A copy of `buf` with every plot applied in order, as a run of `set` calls
 * would. Where a drawing call copies the grid (see `Buffer`), this costs one copy
 * for the whole batch instead of one per cell. Out-of-range plots are clipped.
 * @param buf {Buffer} the source buffer
 * @param plots {list of Plot} the cells to write
 * @return {Buffer} the updated buffer
 */
export func setMany(buf as Buffer, plots as list of Plot) {
    for (def p in $plots) {
        if ($p.x >= 0 and $p.x < $buf.cols and $p.y >= 0 and $p.y < $buf.rows) {
            $buf.cells[$p.y * $buf.cols + $p.x] = $p.cell;
        }
    }
    return $buf;
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
    $buf = textMany($buf, [Span{x: $x, y: $y, text: $s, color: ""}]);
    return $buf;
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
        throw Error{kind: "value", message: "screen.textColor: unknown colour: " + $color};
    }
    $buf = textMany($buf, [Span{x: $x, y: $y, text: $s, color: $color}]);
    return $buf;
}

# CONTROL_CHARS matches a C0 control (codepoint < 32) or DEL (127).
def const CONTROL_CHARS as string init '[\x00-\x1f\x7f]';

# controlsToSpaces replaces every C0 control or DEL in s with a space, so
# untrusted text written into a cell cannot smuggle a terminal escape (a CSI /
# OSC sequence - window title, cursor move, OSC 52 clipboard write) through
# render. One regex pass per span, not a check per glyph; multi-byte runes are
# untouched, and the module's own SGR codes are added after this, so styling is
# unaffected.
func controlsToSpaces(s as string) {
    return regex.replace(CONTROL_CHARS, $s, " ");
}

/**
 * A copy of `buf` with every span written in order, as a run of `text` /
 * `textColor` calls would. Where a drawing call copies the grid (see `Buffer`),
 * this costs one copy for the whole batch instead of one per call. Each span
 * writes one rune per cell from its `(x, y)`, clipped at the row's right edge,
 * in its colour (none when `color` is empty). A control
 * character is written as a space, so untrusted text cannot smuggle a terminal
 * escape through `render`.
 * @param buf {Buffer} the source buffer
 * @param spans {list of Span} the text runs to write
 * @return {Buffer} the updated buffer
 * @throws {Error} when a span names an unknown colour
 */
export func textMany(buf as Buffer, spans as list of Span) {
    for (def sp in $spans) {
        def code as string init spanCode($sp.color);
        if ($sp.y < 0 or $sp.y >= $buf.rows) {
            continue;
        }
        def col as int init $sp.x;
        for (def r in strings.chars(controlsToSpaces($sp.text))) {
            if ($col >= $buf.cols) {
                break;
            }
            if ($col >= 0) {
                $buf.cells[$sp.y * $buf.cols + $col] = styledCell($r, $code);
            }
            $col = $col + 1;
        }
    }
    return $buf;
}

# spanCode is the SGR colour code for a span's colour name, "" for none.
func spanCode(color as string) {
    if (len($color) == 0) {
        return "";
    }
    if (not maps.has(FG, $color)) {
        throw Error{kind: "value", message: "screen.textMany: unknown colour: " + $color};
    }
    return FG[$color];
}

# styledCell wraps one cell's glyph in an SGR colour sequence, or returns it as is
# when code is empty.
func styledCell(cell as string, code as string) {
    if (len($code) == 0) {
        return $cell;
    }
    return CSI + $code + "m" + $cell + CSI + "0m";
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
    for (def row as int init $y; $row < $y + $h; $row = $row + 1) {
        for (def col as int init $x; $col < $x + $w; $col = $col + 1) {
            if ($col >= 0 and $col < $buf.cols and $row >= 0 and $row < $buf.rows) {
                $buf.cells[$row * $buf.cols + $col] = $ch;
            }
        }
    }
    return $buf;
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
    $buf = fill($buf, $x, $y, $n, 1, $ch);
    return $buf;
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
    $buf = fill($buf, $x, $y, 1, $n, $ch);
    return $buf;
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
    if ($w < 2 or $h < 2) {
        return $buf;
    }
    def right as int init $x + $w - 1;
    def bottom as int init $y + $h - 1;
    $buf = hline($buf, $x + 1, $y, $w - 2, "─");
    $buf = hline($buf, $x + 1, $bottom, $w - 2, "─");
    $buf = vline($buf, $x, $y + 1, $h - 2, "│");
    $buf = vline($buf, $right, $y + 1, $h - 2, "│");
    $buf = set($buf, $x, $y, "┌");
    $buf = set($buf, $right, $y, "┐");
    $buf = set($buf, $x, $bottom, "└");
    $buf = set($buf, $right, $bottom, "┘");
    return $buf;
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
    for (def y in 0..$buf.rows) {
        $parts[] = moveTo(0, $y);
        def rowStart as int init $y * $buf.cols;
        for (def x in 0..$buf.cols) {
            $parts[] = $buf.cells[$rowStart + $x];
        }
    }
    return strings.join($parts, "");
}

/**
 * The minimal escape string that turns the terminal showing `old` into `new` -
 * only the changed cells are repositioned and rewritten, in row runs. When the
 * two buffers differ in size (a resized terminal) it falls back to clearing the
 * screen and a full `render(new)`, so no cell of the old frame is left behind
 * outside the new one. This is the flicker-free update path: keep the previous
 * buffer, `diff` against the next.
 * @param old {Buffer} the buffer currently on screen
 * @param new {Buffer} the buffer to display
 * @return {string} the minimal-update escape string
 */
export func diff(old as Buffer, new as Buffer) {
    if ($old.rows != $new.rows or $old.cols != $new.cols) {
        return clear() + render($new);
    }
    def parts as list of string init [];
    def cols as int init $new.cols;
    # lists.mismatch skips every unchanged cell in one native call, so an
    # unchanged stretch - most of a typical frame - costs nothing per cell.
    def i as int init lists.mismatch($old.cells, $new.cells);
    while ($i >= 0) {
        # Start of a changed run: emit one move, then every changed cell
        # until the buffers agree again or the row ends.
        def y as int init $i // $cols;
        def row as int init $y * $cols;
        def x as int init $i - $row;
        def end as int init runEnd($old, $new, $row, $x, $cols);
        $parts[] = moveTo($x, $y);
        $parts[] = strings.join($new.cells[$row + $x..$row + $end], "");
        $i = lists.mismatch($old.cells, $new.cells, $row + $end);
    }
    return strings.join($parts, "");
}

# runEnd returns the column where the run of changed cells starting at x in the
# row beginning at cell index row ends: the first column from x whose cell is the
# same in both buffers, or cols.
func runEnd(old as Buffer, new as Buffer, row as int, x as int, cols as int) {
    def end as int init $x;
    while ($end < $cols and $old.cells[$row + $end] != $new.cells[$row + $end]) {
        $end = $end + 1;
    }
    return $end;
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
    match ($param) {
        when 1 { return "home"; }
        when 2 { return "insert"; }
        when 3 { return "delete"; }
        when 4 { return "end"; }
        when 5 { return "pageup"; }
        when 6 { return "pagedown"; }
        when 15 { return "f5"; }
        when 17 { return "f6"; }
        when 18 { return "f7"; }
        when 19 { return "f8"; }
        when 20 { return "f9"; }
        when 21 { return "f10"; }
        when 23 { return "f11"; }
        when 24 { return "f12"; }
        else { return "unknown"; }
    }
}

# finalKey maps a CSI / SS3 final byte (arrows, home/end, F1-F4) to a name.
func finalKey(b as int) {
    match ($b) {
        when 65 { return "up"; }
        when 66 { return "down"; }
        when 67 { return "right"; }
        when 68 { return "left"; }
        when 72 { return "home"; }
        when 70 { return "end"; }
        when 80 { return "f1"; }
        when 81 { return "f2"; }
        when 82 { return "f3"; }
        when 83 { return "f4"; }
        else { return "unknown"; }
    }
}

/**
 * Decode one raw key byte sequence into a `Key`. Pure and total: `seq` is the
 * bytes of a single key press (`[65]` for `A`, `[27, 91, 65]` for Up,
 * `[27, 91, 51, 126]` for Delete, `[195, 182]` for a two-byte UTF-8 character).
 * A multi-byte UTF-8 sequence decodes to a `"char"` key holding the full rune.
 * Unrecognized input decodes to `"unknown"`;
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
        if ($b == 13 or $b == 10) {
            return Key{name: "enter", char: ""};
        }
        if ($b == 9) {
            return Key{name: "tab", char: ""};
        }
        if ($b == 27) {
            return Key{name: "escape", char: ""};
        }
        if ($b == 127 or $b == 8) {
            return Key{name: "backspace", char: ""};
        }
        if ($b == 0) {
            return Key{name: "ctrl-space", char: ""};
        }
        if ($b >= 1 and $b <= 26) {
            return Key{name: "ctrl-" + convert.fromCodepoint($b + 96), char: ""};
        }
        if ($b >= 32 and $b <= 126) {
            return Key{name: "char", char: convert.fromCodepoint($b)};
        }
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
            return Key{name: "alt-" + convert.fromCodepoint($c), char: convert.fromCodepoint($c)};
        }
        # ESC then a lone control byte (Ctrl-C, another Escape, Enter, ...) is the
        # blocking-read merge of a lone Escape with the user's *next* key: with no
        # timed read, nextKey reads the following key into this escape sequence (see
        # the nextKey docblock). The two cannot be un-merged, but decoding the second
        # byte as its own key keeps a working exit - ESC then Ctrl-C yields ctrl-c,
        # ESC then Escape yields escape - instead of collapsing the pair to an
        # ignored "unknown" and leaving the user no way out.
        if (len($seq) == 2) {
            return decodeKey([$c]);
        }
    }
    # A UTF-8 encoded character: a lead byte 0xC0-0xF7 plus its continuation
    # bytes (an accented letter, the section sign, an emoji). Decode the whole
    # sequence to its rune. Kept total - malformed or truncated bytes fall through
    # to "unknown" rather than throwing.
    if ($b >= 192) {
        def bs as bytes;
        for (def i in 0..len($seq)) {
            $bs[] = $seq[$i];
        }
        try {
            return Key{name: "char", char: convert.stringFromBytes($bs, "utf-8")};
        } catch (e) {
            return Key{name: "unknown", char: ""};
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
 * A lone Escape is told apart from the start of an escape sequence by timing:
 * after `ESC`, the next byte is awaited for `ESCAPE_WAIT_MS` (50 ms). A
 * terminal sends a sequence's bytes together, so nothing arriving in that window
 * is a bare Escape, returned as `"escape"` without consuming the key pressed
 * after it. A key pressed within the window still merges with the Escape (`ESC`
 * then `q` decodes as `alt-q`, the way terminals encode Alt).
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
        def c as int init term.readByteTimeout(ESCAPE_WAIT_MS);
        if ($c == -2) {
            return Key{name: "escape", char: ""};
        }
        if ($c == -1) {
            return decodeKey($seq);
        }
        $seq[] = $c;
        if ($c == 91 or $c == 79) {
            for (def d in readSequenceTail()) {
                $seq[] = $d;
            }
        }
    } elseif ($b >= 192) {
        # A UTF-8 lead byte: read the continuation bytes so a multi-byte character
        # arrives whole - 0xC0-0xDF is 2 bytes total, 0xE0-0xEF is 3, 0xF0-0xF7 is
        # 4. decodeKey turns the assembled sequence into a "char" key.
        def extra as int init 0;
        if ($b <= 223) {
            $extra = 1;
        } elseif ($b <= 239) {
            $extra = 2;
        } elseif ($b <= 247) {
            $extra = 3;
        }
        for (def i in 0..$extra) {
            def c as int init term.readByte();
            if ($c == -1) {
                break;
            }
            $seq[] = $c;
        }
    }
    return decodeKey($seq);
}

# readSequenceTail reads the rest of a CSI / SS3 sequence after its `ESC [` or
# `ESC O` opener: bytes up to and including the final byte (a letter, or `~` for
# the numeric-parameter forms), or up to end of input.
func readSequenceTail() {
    def tail as list of int init [];
    repeat {
        def d as int init term.readByte();
        if ($d == -1) {
            return $tail;
        }
        $tail[] = $d;
        if (($d >= 65 and $d <= 90) or ($d >= 97 and $d <= 122) or $d == 126) {
            return $tail;
        }
    } until (false);
    return $tail;
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
 * A live keyboard reader for a free-running event loop. A background task drains
 * the terminal into a buffered channel of decoded `Key` events, so the loop can
 * poll for input (`hasKey` / `pollKey`) without blocking the way `nextKey` does.
 * Build one with `startInput`.
 * @field keys {channel of Key} the buffered decoded-key stream
 */
export def struct Input {
    keys as channel of Key
};

# INPUT_BUFFER is the reader's channel capacity: decoded keys queue here between
# polls. When it is full the reader blocks on send (backpressure), so keys are
# delayed, never dropped; 64 is far more than one render tick accumulates.
def const INPUT_BUFFER as int init 64;

# ESCAPE_WAIT_MS is how long nextKey waits for the byte after an `ESC` before
# deciding it was a lone Escape key. A terminal writes an escape sequence in one
# burst, so even over SSH its bytes arrive well within it, while a human cannot
# press a second key that fast on purpose.
def const ESCAPE_WAIT_MS as int init 50;

/**
 * Start a background keyboard reader for a free-running loop (a game, a live
 * dashboard, anything that must keep ticking without a keypress). It spawns a
 * task that reads and decodes keys through `nextKey` into a buffered channel;
 * poll it with `hasKey` / `pollKey` (non-blocking) or drain it with `waitKey`
 * (blocking) while the loop renders. Requires raw mode (`begin`) and the `term`
 * library (default binary).
 *
 * The reader task is deliberately **discarded**: it blocks in `term.readByte` and
 * cannot be interrupted mid-read (neither `task.cancel` nor closing the channel
 * reaches a syscall read), so it runs until end of input or program exit, when
 * the OS reaps it. Discarding it is what lets the program exit instead of hanging
 * on the never-returning reader. One reader owns stdin; do not also call
 * `nextKey` while it runs.
 * @return {Input} the live input handle
 */
export func startInput() {
    def keys as channel of Key init channel.make(INPUT_BUFFER);
    def reader as task of null init spawn {
        def running as bool init true;
        while ($running) {
            def k as Key init nextKey();
            channel.send($keys, $k);
            if ($k.name == "eof") {
                $running = false;
            }
        }
    };
    task.discard($reader);
    return Input{keys: $keys};
}

/**
 * Whether a decoded key is waiting (non-blocking): true when `pollKey` would
 * return a real key rather than the `"none"` sentinel.
 * @param input {Input} the live input handle (from `startInput`)
 * @return {bool} true if a key is buffered
 */
export func hasKey(input as Input) {
    return channel.len($input.keys) > 0;
}

/**
 * The next decoded key if one is waiting, else a `"none"` sentinel key
 * (non-blocking) - the poll a free-running loop calls each tick.
 * @param input {Input} the live input handle (from `startInput`)
 * @return {Key} the next key, or `Key{name: "none", char: ""}` when none is ready
 */
export func pollKey(input as Input) {
    if (channel.len($input.keys) > 0) {
        return channel.recv($input.keys);
    }
    return Key{name: "none", char: ""};
}

/**
 * Block until the next decoded key and return it - the buffered equivalent of
 * `nextKey`, for a loop that waits rather than spins.
 * @param input {Input} the live input handle (from `startInput`)
 * @return {Key} the next key event
 */
export func waitKey(input as Input) {
    return channel.recv($input.keys);
}

/**
 * The terminal size as a `term.Size` (`{rows, cols}`), a convenience passthrough
 * to `term.size` so an app stays in one namespace. Requires the `term` library.
 * @return {term.Size} the terminal dimensions
 */
export func size() {
    return term.size("stdout");
}
