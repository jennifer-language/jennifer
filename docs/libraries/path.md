# `path` - filesystem path manipulation

`use path;` enables `path.`-prefixed functions for taking paths apart and
building them back up: base name, directory, extension, join, clean. Every
function is a **pure string transform** - it never touches the disk (that is
`fs`'s job) - so `path` is the string layer that pairs with `fs`'s I/O, the same
way `strings` pairs with text.

Paths use the **host separator** (`/` on Linux, `\` on the best-effort Windows
build), via Go's `path/filepath`. Build paths with `path.join` instead of
hardcoding `"/"` and they stay portable across platforms; the separator itself is
[`os.DIRSEP`](os.md).

## Functions

| Call                     | Returns          | Notes                                                                                   |
| ------------------------ | ---------------- | --------------------------------------------------------------------------------------- |
| `path.base(p)`           | string           | The last element of `p`. `""` -> `"."`, `"/"` -> `"/"`, a trailing slash is dropped.    |
| `path.dir(p)`            | string           | Everything but the last element, cleaned. `"c.txt"` -> `"."`.                           |
| `path.ext(p)`            | string           | The extension including the leading dot (`".txt"`), or `""` when there is none.         |
| `path.stem(p)`           | string           | The base name without its extension (`"/a/c.txt"` -> `"c"`).                            |
| `path.join(a, b, ...)`   | string           | Join any number (>= 1) of elements with the separator and clean the result; empty elements are dropped. |
| `path.clean(p)`          | string           | The shortest path equivalent to `p` (collapses `.`, `..`, and repeated separators). `""` -> `"."`. |
| `path.isAbs(p)`          | bool             | Whether `p` is absolute.                                                                |
| `path.split(p)`          | list of string   | `[dir, file]`, where `dir` keeps its trailing separator so `dir + file == p`.           |

## Not a sanitizer

`path.base` is path **logic**, not a way to sanitize an untrusted filename. It is
OS-aware, so on Linux `path.base("a\b")` is `"a\b"` unchanged (backslash is a
legal Unix filename byte). To neutralize an attacker-controlled name - an email
attachment's filename, an upload field - before writing it, strip **both**
separators yourself and reject `.` / `..`, rather than reaching for `path.base`.

## Examples

```jennifer
use path;
use io;

io.printf("%s\n", path.base("/var/log/app.txt"));      # app.txt
io.printf("%s\n", path.dir("/var/log/app.txt"));       # /var/log
io.printf("%s\n", path.ext("/var/log/app.txt"));       # .txt
io.printf("%s\n", path.stem("/var/log/app.txt"));      # app
io.printf("%s\n", path.join("var", "log", "app.txt")); # var/log/app.txt
io.printf("%s\n", path.clean("a//b/../c"));            # a/c
io.printf("%t\n", path.isAbs("/etc"));                 # true

def parts as list of string init path.split("/var/log/app.txt");
io.printf("[%s][%s]\n", $parts[0], $parts[1]);      # [/var/log/][app.txt]
```

Building a destination path portably (pairs with `fs`):

```jennifer
use path;
use os;
use fs;
def dest as string init path.join(os.homeDir(), "downloads", $name);
fs.mkdirAll(path.dir($dest));
fs.writeBytes($dest, $data);
```

## TinyGo

The manipulation subset does no I/O, so `path` is TinyGo-clean and ships in
**both** binaries (unlike `fs` / `net`, which need a build-tag split).
