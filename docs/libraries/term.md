# `term` - terminal control

Enable with `use term;`. The terminal host capabilities an interactive TUI needs
and pure `.j` cannot reach: **raw mode** (unbuffered, no-echo input), the
**terminal size**, and **raw single-byte reads** from stdin. This is the low
level - screen control, key decoding, and rendering belong in a `.j` layer on
top; `term` is only the primitives that require the host.

An **output-only** TUI (a dashboard, a progress bar) needs none of this - just
[`ansi`](../modules/ansi.md) escape codes and [`os.isTerminal`](os.md). Reach
for `term` when you need to read keypresses as they happen.

`term` is a **system library** built on `golang.org/x/term` (the package the
REPL's own line editor uses). Build-tag split like [`net`](net.md): the default
`jennifer` binary has the real implementation; `jennifer-tiny` returns a friendly
error (a minimal / embedded target may have no controlling terminal).

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `term.makeRaw(stream)` | `term.State` | Put the terminal into raw mode; `stream` must be `"stdin"` (raw mode governs input). Returns a handle for `restore`. |
| `term.restore(state)` | `null` | Undo `makeRaw`, restoring the terminal. The handle is single-use. |
| `term.size(stream)` | `term.Size` | The terminal's `{rows, cols}` (query `"stdout"`). |
| `term.readByte()` | `int` | The next raw byte from stdin (`0`-`255`), or `-1` at end of input. |

`stream` is `"stdin"` / `"stdout"` / `"stderr"` (the same names as
`os.isTerminal`). A stream that is not a terminal - a pipe or a redirect - is a
catchable error for `makeRaw` / `size`.

## Raw mode

In the terminal's default (cooked) mode the OS buffers a whole line, echoes it,
and only hands it over on Enter. **Raw mode** turns that off: every keypress is
delivered immediately, unechoed - which is what an editor, a pager, or a game
loop needs. `term.makeRaw` enters it and returns a `term.State` handle; pass that
handle to `term.restore` to put the terminal back:

```jennifer
use io;
use os;
use term;

if (not os.isTerminal("stdin")) {
    io.printf("needs an interactive terminal\n");
} else {
    def state as term.State init term.makeRaw("stdin");
    defer term.restore($state);                  # runs however this block exits
    def running as bool init true;
    while ($running) {
        def b as int init term.readByte();       # blocks until a key is pressed
        if ($b == -1 or $b == 113) {             # -1 = end of input, 113 = 'q'
            $running = false;
        } else {
            io.printf("byte %d\r\n", $b);         # raw mode: newline is not cooked
        }
    }
}
```

**Always restore.** Raw mode is a property of the terminal device, not the
process, so a program that exits still in raw mode would leave the shell there.
Put `defer term.restore($state);` right after the `makeRaw` (as above) so the
restore runs on **every** exit path - including a `throw` unwinding through the
block ([control-flow > `defer`](../user-guide/control-flow.md#defer-deterministic-cleanup)).
Note that in raw mode the newline is no longer cooked, so prints need an explicit
`\r\n`. The `term.State` handle is **single-use**: a second `term.restore` of the
same handle is an error, so a live terminal is never clobbered by a stale handle.

As a **backstop**, the `jennifer` CLI cooks the terminal back on the way out even
when your program doesn't - a normal exit, an `exit`, an uncaught error, a panic,
or an uncaught terminating signal (`SIGINT` / `SIGTERM` / `SIGHUP`, re-raised so
the process still dies as usual). This is a safety net for a crash, not a
substitute for `restore`: it only runs as the process ends, so a long-running
program that means to leave raw mode mid-run must still call `restore` itself, and
a `kill -9` (`SIGKILL`) is uncatchable and bypasses it.

## Reading keys

`term.readByte` returns one raw byte from stdin (`0`-`255`), or `-1` at end of
input. In raw mode it returns the instant a key is pressed. It reads **bytes**,
not decoded keys: a plain key is one byte, but an arrow key or a function key
arrives as a multi-byte escape sequence (`ESC [ A` for Up, ...). Decoding those
into key events is the job of a TUI layer on top - `term` deliberately stops at
the raw byte.

`term.readByte` is refused inside the REPL, which owns the terminal for its own
line editor.

## Terminal size

`term.size(stream)` returns a `term.Size{rows, cols}` - the character grid of the
terminal behind `stream` (usually `"stdout"`):

```jennifer
def dim as term.Size init term.size("stdout");
io.printf("%d rows x %d cols\n", $dim.rows, $dim.cols);
```

The size is a snapshot; a terminal can be resized after the call, so re-query it
when you need the current value.

## See also

- [`os`](os.md) - `os.isTerminal(stream)` to gate on an interactive terminal
  before entering raw mode.
- [`ansi`](../modules/ansi.md) - escape codes for colour and cursor control,
  which pair with `term` for a full-screen TUI.
- [`screen`](../modules/screen.md) - the full-screen TUI module (cell buffer,
  diff rendering, key decoding) built on top of `term`.
