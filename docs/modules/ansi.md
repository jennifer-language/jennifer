# `ansi` - terminal styling

Import with `import "ansi.j" as ansi;`. Wraps a string in ANSI SGR escape
codes for colour, background colour, text style, the 256-colour palette,
and 24-bit truecolor - and strips them back off. Pure Jennifer (no Go), so
it runs on either binary.

Styling is **TTY-aware**: it suppresses itself when stdout is not a
terminal (redirected to a file or a pipe), so the wrapped text stays clean
either way. The [`NO_COLOR`](https://no-color.org) / `FORCE_COLOR`
environment convention overrides the gate.

```jennifer
use io;
import "ansi.j" as ansi;

io.printf("%s\n", ansi.bold(ansi.red("error:")) + " something broke");
io.printf("%s\n", ansi.green("ok") + " / " + ansi.yellow("warn"));
io.printf("%s\n", ansi.rgb("truecolor orange", 255, 128, 0));
io.printf("%s\n", ansi.underline(ansi.cyan("nested + underlined")));
```

Runnable: [`examples/modules/ansi_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ansi_demo.j).

## Surface

| Call                       | Returns  | Notes                                                                          |
| -------------------------- | -------- | ------------------------------------------------------------------------------ |
| `ansi.color(s, name)`      | `string` | Wrap `s` in the named foreground colour. Unknown name `throw`s.                |
| `ansi.bgColor(s, name)`    | `string` | Wrap `s` in the named background colour.                                       |
| `ansi.style(s, name)`      | `string` | Wrap `s` in a text style: `bold` / `dim` / `italic` / `underline` / `reverse` / `strike`. |
| `ansi.color256(s, n)`      | `string` | 256-colour palette foreground (`38;5;n`); `n` clamped to `0`-`255`.            |
| `ansi.bgColor256(s, n)`    | `string` | 256-colour palette background (`48;5;n`); `n` clamped to `0`-`255`.            |
| `ansi.rgbToColor256(r, g, b)` | `int`  | Nearest palette index for an RGB triple, for a 256-colour terminal.            |
| `ansi.rgb(s, r, g, b)`     | `string` | 24-bit truecolor foreground; each channel `0`-`255`.                           |
| `ansi.strip(s)`            | `string` | Remove every SGR escape - the inverse of the wrappers.                         |

Wrapping composes and nests (`ansi.bold(ansi.red(s))`); each wrapper emits
its own code and a reset, so an inner reset never truncates an outer style.

### Colour and style names

- **Foreground / background** (`color` / `bgColor`): `black`, `red`,
  `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`. `color` also
  accepts the bright `gray` (alias `grey`).
- **Styles** (`style`): `bold`, `dim`, `italic`, `underline`, `reverse`, `strike`.

An unrecognized name is a thrown `Error` (`kind: "value"`), catchable with
`try` / `catch`.

### 256-colour palette

`color256` / `bgColor256` are the indexed middle rung between the named
colours and `rgb`. The index `n` follows the xterm layout: `0`-`15` the
base colours, `16`-`231` a 6x6x6 colour cube, `232`-`255` a 24-step
grayscale ramp. Out-of-range values clamp to `0`-`255`.

`rgbToColor256(r, g, b)` quantizes a truecolor triple to the nearest
palette index (the closer of the cube and the grayscale ramp), so an app
holding RGB can degrade gracefully on a terminal without 24-bit support:

```jennifer
def n as int init ansi.rgbToColor256(255, 128, 0);   # orange -> cube index
io.printf("%s\n", ansi.color256("orange-ish", $n));
```

### Shortcuts

Each common colour and style has a one-argument shortcut - `ansi.NAME(s)`
is exactly `ansi.color(s, "NAME")` (or `ansi.style` for a style name):

- Colours: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`,
  `white`, `gray`.
- Styles: `bold`, `dim`, `italic`, `underline`, `reverse`, `strike`.

## When styling is emitted

`ansi` decides per call (it is stateless - there is no toggle to store):

1. `NO_COLOR` set (to anything) - **off**, always.
2. else `FORCE_COLOR` set - **on**, always.
3. else on when `os.isTerminal("stdout")` is true; off when it is false.
4. If the host cannot tell whether stdout is a terminal, defaults **on**.

`strip` ignores this gate - it removes escapes unconditionally, so it
cleans up already-styled text (or a captured subprocess's output)
regardless of the current terminal state.

```jennifer
def styled as string init ansi.bold(ansi.blue("styled"));
io.printf("stripped: [%s]\n", ansi.strip($styled));   # stripped: [styled]
```

## See also

- [os.md](../libraries/os.md) - `os.isTerminal`, the TTY gate, and
  `os.getEnv` for the `NO_COLOR` / `FORCE_COLOR` reads.
- [regex.md](../libraries/regex.md) - `strip` uses `regex.replace` to
  drop escape sequences.
- [modules/index.md](index.md) - the module catalog and import rules.
