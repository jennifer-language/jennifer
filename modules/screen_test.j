# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# screen_test.j - white-box tests for screen.j. Run with:
#
#     jennifer test modules/screen_test.j
#
# The overlay splices screen.j in front of this file, so the tests reach its
# private helpers (charOf, csiParam, tildeKey, finalKey, writeRunes) by bare
# identifier as well as its exported surface. Only the pure output-only layer
# and the pure decodeKey are tested here; the term-backed event loop (nextKey /
# begin / end / size) needs a real TTY and is exercised by the demo.
use testing;

# The ESC control byte, for exact escape-string assertions.
func esc() {
    def b as bytes;
    $b[] = 27;
    return convert.stringFromBytes($b, "utf-8");
}

func testNewScreenDimsAndBlank() {
    def buf as Buffer init newScreen(4, 6);
    testing.assertEqual($buf.rows, 4);
    testing.assertEqual($buf.cols, 6);
    testing.assertEqual(len($buf.cells), 24);
    testing.assertEqual($buf.cells[0], " ");
    testing.assertEqual(get($buf, 5, 3), " ");
}

func zeroRows() { newScreen(0, 5); }
func negativeCols() { newScreen(5, -1); }
func testNewScreenRejectsNonPositive() {
    testing.assertThrows("zeroRows", "value");
    testing.assertThrows("negativeCols", "value");
}

func testSetGet() {
    def buf as Buffer init set(newScreen(2, 3), 1, 1, "X");
    testing.assertEqual(get($buf, 1, 1), "X");
    testing.assertEqual(get($buf, 0, 0), " ");
}

func testSetClipsOutOfRange() {
    def buf as Buffer init newScreen(2, 2);
    # Out-of-range writes are no-ops, not errors.
    testing.assertEqual(get(set($buf, 5, 5, "Z"), 0, 0), " ");
    testing.assertEqual(get(set($buf, -1, 0, "Z"), 0, 0), " ");
    # Out-of-range read returns "".
    testing.assertEqual(get($buf, 9, 9), "");
}

func testSetDoesNotMutateInput() {
    def buf as Buffer init newScreen(1, 3);
    def other as Buffer init set($buf, 0, 0, "A");
    testing.assertEqual(get($buf, 0, 0), " ");        # original untouched
    testing.assertEqual(get($other, 0, 0), "A");
}

func testTextWritesRunes() {
    def buf as Buffer init text(newScreen(1, 5), 1, 0, "hi");
    testing.assertEqual(get($buf, 0, 0), " ");
    testing.assertEqual(get($buf, 1, 0), "h");
    testing.assertEqual(get($buf, 2, 0), "i");
}

func testTextClipsAtRowEnd() {
    # "hello" from col 3 of a 5-wide row keeps only "he".
    def buf as Buffer init text(newScreen(1, 5), 3, 0, "hello");
    testing.assertEqual(get($buf, 3, 0), "h");
    testing.assertEqual(get($buf, 4, 0), "e");
    testing.assertEqual(len($buf.cells), 5);          # nothing wrapped/appended
}

func testTextOffScreenRowIsNoop() {
    def buf as Buffer init text(newScreen(1, 5), 0, 9, "hi");
    testing.assertEqual(get($buf, 0, 0), " ");
}

func testTextColorWrapsEachCell() {
    def buf as Buffer init textColor(newScreen(1, 3), 0, 0, "ab", "red");
    # Each cell is one visible column wrapped in SGR 31 ... 0.
    testing.assertEqual(get($buf, 0, 0), esc() + "[31m" + "a" + esc() + "[0m");
    testing.assertEqual(get($buf, 1, 0), esc() + "[31m" + "b" + esc() + "[0m");
}

func unknownColor() { textColor(newScreen(1, 3), 0, 0, "x", "mauve"); }
func testTextColorRejectsUnknownColor() {
    testing.assertThrows("unknownColor", "value");
}

func testFill() {
    def buf as Buffer init fill(newScreen(3, 3), 0, 0, 2, 2, "#");
    testing.assertEqual(get($buf, 0, 0), "#");
    testing.assertEqual(get($buf, 1, 1), "#");
    testing.assertEqual(get($buf, 2, 2), " ");        # outside the rect
}

func testHlineVline() {
    def h as Buffer init hline(newScreen(2, 4), 1, 0, 2, "-");
    testing.assertEqual(get($h, 1, 0), "-");
    testing.assertEqual(get($h, 2, 0), "-");
    testing.assertEqual(get($h, 3, 0), " ");
    def v as Buffer init vline(newScreen(3, 2), 0, 0, 2, "|");
    testing.assertEqual(get($v, 0, 0), "|");
    testing.assertEqual(get($v, 0, 1), "|");
    testing.assertEqual(get($v, 0, 2), " ");
}

func testBoxBorderAndInterior() {
    def buf as Buffer init box(newScreen(3, 4), 0, 0, 4, 3);
    testing.assertEqual(get($buf, 0, 0), "┌");
    testing.assertEqual(get($buf, 3, 0), "┐");
    testing.assertEqual(get($buf, 0, 2), "└");
    testing.assertEqual(get($buf, 3, 2), "┘");
    testing.assertEqual(get($buf, 1, 0), "─");        # top edge
    testing.assertEqual(get($buf, 0, 1), "│");        # left edge
    testing.assertEqual(get($buf, 1, 1), " ");        # interior untouched
}

func testBoxTooSmallIsNoop() {
    def buf as Buffer init box(newScreen(2, 2), 0, 0, 1, 1);
    testing.assertEqual(get($buf, 0, 0), " ");
}

func testClearBuffer() {
    def buf as Buffer init clearBuffer(text(newScreen(1, 3), 0, 0, "abc"));
    testing.assertEqual(get($buf, 0, 0), " ");
    testing.assertEqual(get($buf, 2, 0), " ");
}

func testControlSequences() {
    testing.assertEqual(clear(), esc() + "[2J" + esc() + "[H");
    testing.assertEqual(clearLine(), esc() + "[2K");
    testing.assertEqual(home(), esc() + "[H");
    testing.assertEqual(hideCursor(), esc() + "[?25l");
    testing.assertEqual(showCursor(), esc() + "[?25h");
    testing.assertEqual(enterAlt(), esc() + "[?1049h");
    testing.assertEqual(exitAlt(), esc() + "[?1049l");
}

func testMoveToIsOneBasedFromZeroBasedInput() {
    # moveTo(x, y) -> CSI (y+1) ; (x+1) H
    testing.assertEqual(moveTo(0, 0), esc() + "[1;1H");
    testing.assertEqual(moveTo(2, 3), esc() + "[4;3H");
}

func testCursorMoves() {
    testing.assertEqual(up(2), esc() + "[2A");
    testing.assertEqual(down(3), esc() + "[3B");
    testing.assertEqual(right(4), esc() + "[4C");
    testing.assertEqual(left(5), esc() + "[5D");
}

func testRenderPositionsAndPaints() {
    def buf as Buffer init text(newScreen(1, 3), 0, 0, "ab");
    # Row 0 starts with a move to (0,0) then the three cells.
    testing.assertEqual(render($buf), moveTo(0, 0) + "a" + "b" + " ");
}

func testDiffEmptyWhenIdentical() {
    def buf as Buffer init text(newScreen(2, 3), 0, 0, "hi");
    testing.assertEqual(diff($buf, $buf), "");
}

func testDiffSingleCell() {
    def before as Buffer init newScreen(1, 5);
    def after as Buffer init set($before, 2, 0, "X");
    # One changed cell -> one move + the cell.
    testing.assertEqual(diff($before, $after), moveTo(2, 0) + "X");
}

func testDiffGroupsAContiguousRun() {
    def before as Buffer init newScreen(1, 5);
    def after as Buffer init text($before, 1, 0, "abc");
    # Cells 1..3 changed contiguously -> a single move, then the run.
    testing.assertEqual(diff($before, $after), moveTo(1, 0) + "a" + "b" + "c");
}

func testDiffTwoSeparateRuns() {
    def before as Buffer init newScreen(1, 5);
    def after as Buffer init set(set($before, 0, 0, "A"), 4, 0, "B");
    # Two non-adjacent changes -> two moves.
    testing.assertEqual(diff($before, $after),
        moveTo(0, 0) + "A" + moveTo(4, 0) + "B");
}

func testDiffFallsBackToFullRenderOnSizeChange() {
    def small as Buffer init newScreen(1, 2);
    def big as Buffer init newScreen(2, 2);
    testing.assertEqual(diff($small, $big), render($big));
}

# ---- decodeKey (pure) ----

func testDecodePrintableChar() {
    def k as Key init decodeKey([65]);
    testing.assertEqual($k.name, "char");
    testing.assertEqual($k.char, "A");
    def space as Key init decodeKey([32]);
    testing.assertEqual($space.name, "char");
    testing.assertEqual($space.char, " ");
}

func testDecodeControlBytes() {
    testing.assertEqual(decodeKey([13]).name, "enter");
    testing.assertEqual(decodeKey([10]).name, "enter");
    testing.assertEqual(decodeKey([9]).name, "tab");
    testing.assertEqual(decodeKey([27]).name, "escape");
    testing.assertEqual(decodeKey([127]).name, "backspace");
    testing.assertEqual(decodeKey([8]).name, "backspace");
    testing.assertEqual(decodeKey([0]).name, "ctrl-space");
}

func testDecodeCtrlLetters() {
    testing.assertEqual(decodeKey([1]).name, "ctrl-a");
    testing.assertEqual(decodeKey([3]).name, "ctrl-c");
    testing.assertEqual(decodeKey([26]).name, "ctrl-z");
}

func testDecodeArrows() {
    testing.assertEqual(decodeKey([27, 91, 65]).name, "up");
    testing.assertEqual(decodeKey([27, 91, 66]).name, "down");
    testing.assertEqual(decodeKey([27, 91, 67]).name, "right");
    testing.assertEqual(decodeKey([27, 91, 68]).name, "left");
}

func testDecodeHomeEndBothForms() {
    testing.assertEqual(decodeKey([27, 91, 72]).name, "home");
    testing.assertEqual(decodeKey([27, 91, 70]).name, "end");
    testing.assertEqual(decodeKey([27, 79, 72]).name, "home");     # SS3 form
    testing.assertEqual(decodeKey([27, 79, 70]).name, "end");
}

func testDecodeNavTildeKeys() {
    testing.assertEqual(decodeKey([27, 91, 49, 126]).name, "home");
    testing.assertEqual(decodeKey([27, 91, 50, 126]).name, "insert");
    testing.assertEqual(decodeKey([27, 91, 51, 126]).name, "delete");
    testing.assertEqual(decodeKey([27, 91, 52, 126]).name, "end");
    testing.assertEqual(decodeKey([27, 91, 53, 126]).name, "pageup");
    testing.assertEqual(decodeKey([27, 91, 54, 126]).name, "pagedown");
}

func testDecodeFunctionKeys() {
    testing.assertEqual(decodeKey([27, 79, 80]).name, "f1");
    testing.assertEqual(decodeKey([27, 79, 83]).name, "f4");
    testing.assertEqual(decodeKey([27, 91, 49, 53, 126]).name, "f5");
    testing.assertEqual(decodeKey([27, 91, 50, 52, 126]).name, "f12");
}

func testDecodeAltAndEdges() {
    def alt as Key init decodeKey([27, 120]);
    testing.assertEqual($alt.name, "alt-x");
    testing.assertEqual($alt.char, "x");
    testing.assertEqual(decodeKey([]).name, "eof");
    testing.assertEqual(decodeKey([200]).name, "unknown");
    testing.assertEqual(decodeKey([27, 91, 122]).name, "unknown");   # unknown final
}

# ---- private helpers ----

func testCharOf() {
    testing.assertEqual(charOf(65), "A");
    testing.assertEqual(charOf(97), "a");
    testing.assertEqual(charOf(32), " ");
}

func testCsiParam() {
    testing.assertEqual(csiParam([27, 91, 51, 126]), 3);
    testing.assertEqual(csiParam([27, 91, 50, 52, 126]), 24);
    testing.assertEqual(csiParam([27, 91, 65]), -1);        # no digits
}

func testTildeAndFinalHelpers() {
    testing.assertEqual(tildeKey(3), "delete");
    testing.assertEqual(tildeKey(99), "unknown");
    testing.assertEqual(finalKey(65), "up");
    testing.assertEqual(finalKey(48), "unknown");
}
