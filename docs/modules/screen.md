# `screen` - terminal user interfaces

Import with `import "screen.j" as screen;`. An *explicit* terminal UI - a cell
buffer you draw into and paint to the terminal, plus a key-event decoder and
event loop - not a GUI framework. Two layers:

- **Output-only** (no host capability, runs on both binaries): ANSI control
  strings, a `Buffer` of character cells, and a `render` / `diff` update loop
  for live dashboards, progress bars, and self-updating tables. Pure strings.
- **Interactive** (needs the [`term`](../libraries/term.md) library, so the
  default `jennifer`): decode a raw byte stream into named `Key` events and read
  them in a loop over raw mode - menus, forms, key navigation. On
  `jennifer-tiny` these raise the `term` friendly-error stub.

Coordinates are **0-based**, origin at the top-left `(0, 0)`; `x` is the column,
`y` the row. Drawing that runs past an edge is clipped, never an error.

```jennifer
use io;
import "screen.j" as screen;

def buf as screen.Buffer init screen.newScreen(3, 20);
def out as screen.Buffer init screen.text(screen.box($buf, 0, 0, 20, 3), 2, 1, "hello");
io.printf("%s%s", screen.clear(), screen.render($out));       # draw it
```

Runnable: [`examples/modules/screen_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/screen_demo.j).

## The cell buffer

`screen.Buffer { rows as int, cols as int, cells as list of string }` is a
row-major grid; each cell is a one-column string (optionally wrapped in an SGR
colour). It is value-semantic like any struct, so every drawing call returns a
**fresh** buffer - assign it back (`$buf = screen.text($buf, ...);`).

| Call                                   | Returns          | Notes                                                                     |
| -------------------------------------- | ---------------- | ------------------------------------------------------------------------- |
| `screen.newScreen(rows, cols)`         | `screen.Buffer`  | A blank buffer, every cell a space. Non-positive dimensions error.        |
| `screen.set(buf, x, y, cell)`          | `screen.Buffer`  | Set one cell (one column). Out-of-range is a no-op (clipped).             |
| `screen.get(buf, x, y)`                | `string`         | The cell contents, or `""` out of range.                                  |
| `screen.text(buf, x, y, s)`            | `screen.Buffer`  | Write `s` one rune per cell from `(x, y)`, clipped at the row's edge.     |
| `screen.textColor(buf, x, y, s, color)`| `screen.Buffer`  | Same, in a named foreground colour. Unknown colour errors.                |
| `screen.fill(buf, x, y, w, h, ch)`     | `screen.Buffer`  | Fill a `w`-by-`h` rectangle with the glyph `ch`.                          |
| `screen.hline(buf, x, y, n, ch)`       | `screen.Buffer`  | A horizontal run of `n` cells of `ch`.                                    |
| `screen.vline(buf, x, y, n, ch)`       | `screen.Buffer`  | A vertical run of `n` cells of `ch`.                                      |
| `screen.box(buf, x, y, w, h)`          | `screen.Buffer`  | A single-line box border (Unicode box-drawing), interior untouched.       |
| `screen.clearBuffer(buf)`              | `screen.Buffer`  | Reset every cell to a space (keeps the dimensions).                       |

Colour names for `textColor`: `black`, `red`, `green`, `yellow`, `blue`,
`magenta`, `cyan`, `white`, `gray` / `grey`. Each coloured cell carries its own
SGR reset, so it still occupies exactly one column and the diff stays correct.

For efficiency, prefer `text` / `box` / `fill` (each copies the buffer once) over
a per-cell `set` in a loop (each copies the whole buffer, so a loop is O(n^2)).

## Painting: `render` and `diff`

| Call                    | Returns  | Notes                                                                                       |
| ----------------------- | -------- | ------------------------------------------------------------------------------------------- |
| `screen.render(buf)`    | `string` | The escape string that paints the whole buffer. Print it with `io.printf`.                  |
| `screen.diff(old, new)` | `string` | The **minimal** escape string that turns `old` into `new` - only changed cells, in row runs. Falls back to a full `render(new)` when the sizes differ. |

The update loop keeps the previously-shown buffer and diffs the next one against
it, so only the cells that changed are repositioned and rewritten - no flicker,
no full repaint:

```jennifer
use io;
import "screen.j" as screen;

def prev as screen.Buffer init frame(0);
io.printf("%s%s", screen.clear(), screen.render($prev));
for (def i as int init 1; $i <= 100; $i = $i + 1) {
    def next as screen.Buffer init frame($i);
    io.printf("%s", screen.diff($prev, $next));       # only the deltas
    $prev = $next;
}
```

## ANSI control sequences

Each returns the escape string; the caller prints it. All pure, so they work on
both binaries.

| Call                    | Effect                                             |
| ----------------------- | -------------------------------------------------- |
| `screen.clear()`        | Clear the whole screen and home the cursor.        |
| `screen.clearLine()`    | Clear the current line.                             |
| `screen.moveTo(x, y)`   | Move the cursor to `(x, y)` (0-based).             |
| `screen.up(n)` / `down(n)` / `left(n)` / `right(n)` | Move the cursor `n` cells.            |
| `screen.home()`         | Move the cursor to the top-left.                   |
| `screen.hideCursor()` / `showCursor()` | Hide / show the text cursor.        |
| `screen.enterAlt()` / `exitAlt()` | Switch to / from the alternate screen buffer (so the app does not scroll the shell's scrollback). |

## Interactive input (needs `term`)

The key decoder is a pure function so it is testable without a terminal; the
event loop reads through the [`term`](../libraries/term.md) library.

| Call                    | Returns       | Notes                                                                     |
| ----------------------- | ------------- | ------------------------------------------------------------------------- |
| `screen.decodeKey(seq)` | `screen.Key`  | Decode one raw byte sequence (`list of int`) into a key. Pure and total.  |
| `screen.nextKey()`      | `screen.Key`  | Read and decode the next key from the terminal (blocking).                |
| `screen.begin()`        | `term.State`  | Enter raw mode + alternate screen, hide the cursor, clear. Pair with `end`. |
| `screen.end(state)`     | `null`        | Show the cursor, leave the alternate screen, restore the terminal.        |
| `screen.size()`         | `term.Size`   | The terminal dimensions (`{rows, cols}`), a passthrough to `term.size`.   |
| `screen.startInput()`   | `screen.Input`| Start a background reader that drains decoded keys into a buffered channel (for a free-running loop). |
| `screen.hasKey(input)`  | `bool`        | Whether a decoded key is waiting (non-blocking).                          |
| `screen.pollKey(input)` | `screen.Key`  | The next key if one is waiting, else a `"none"` sentinel (non-blocking).  |
| `screen.waitKey(input)` | `screen.Key`  | Block until the next decoded key (the buffered equivalent of `nextKey`).  |

`screen.Key { name as string, char as string }`. `name` is symbolic: a printable
key is `"char"` (the character in `char`); the rest are named - `"up"` / `"down"`
/ `"left"` / `"right"`, `"enter"` / `"tab"` / `"escape"` / `"backspace"` /
`"delete"` / `"insert"`, `"home"` / `"end"` / `"pageup"` / `"pagedown"`,
`"f1"`..`"f12"`, `"ctrl-a"`..`"ctrl-z"`, `"alt-<c>"`, `"eof"` at end of input,
and `"unknown"`.

```jennifer
use io;
import "screen.j" as screen;

def state as term.State init screen.begin();
defer screen.end($state);                 # restored on every exit path
repeat {
    def key as screen.Key init screen.nextKey();
    if ($key.name == "ctrl-c" or $key.name == "char" and $key.char == "q") {
        break;
    }
    # ... redraw based on $key ...
} until (false);
```

A lone **Escape** press is only reported once the next byte arrives:
`term.readByte` has no timeout to tell a bare `ESC` from the start of an escape
sequence. Prefer a named key (or `Ctrl-C`) to quit a loop. A multi-byte UTF-8
character (an accented letter, a symbol, an emoji) is read whole and decodes to a
single `"char"` key with the full rune in `char`; malformed or truncated bytes
decode to `"unknown"` (never a throw).

### Free-running loops (non-blocking input)

`nextKey` blocks, which stalls a loop that must keep ticking without a keypress -
a game, a live dashboard, anything also watching a clock or a socket. `startInput`
runs the reader in a background task that drains decoded keys into a buffered
channel, so the loop polls (`hasKey` / `pollKey`) and keeps rendering:

```jennifer
def state as term.State init screen.begin();
defer screen.end($state);
def input as screen.Input init screen.startInput();
repeat {
    while (screen.hasKey($input)) {              # drain all input this tick
        def key as screen.Key init screen.pollKey($input);
        # ... handle $key ...
    }
    # ... advance the world, screen.render(...) ...
    time.sleep(time.fromMilliseconds(16));
} until ($quit);
```

The background reader blocks in `term.readByte` and **cannot be interrupted
mid-read** (neither `task.cancel` nor closing the channel reaches a syscall read),
so it runs until end of input or program exit, when the OS reaps it; `startInput`
discards the task so the program exits cleanly instead of hanging on it. One
reader owns stdin - do not also call `nextKey` while it runs. Stopping and
resuming raw input mid-program needs a lower-level timed read (a horizon item).

## Layering and platforms

The output-only layer is pure Jennifer over ANSI strings and runs on either
binary. The interactive layer's `nextKey` / `begin` / `end` / `size` call
`term`, which is real on the default `jennifer` and a friendly-error stub on
`jennifer-tiny`. Colour output should still gate on a real terminal - use
[`ansi`](ansi.md)'s `enabled` check or `os.isTerminal` before entering
full-screen mode.
