# Installing & running

## Which binary?

Jennifer ships as two binaries per platform. Same source, same
language; only the compiler differs. Pick by use case:

| Binary          | Build                  | Pick when                                                                                                                                          |
| --------------- | ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `jennifer`      | standard Go (default)  | **What most users want.** Full host-feature surface; fastest on compute-heavy work (2-5x over TinyGo on tight numeric / recursive loops; see [technical/tinygo.md > Single-binary benchmark results](../technical/tinygo.md#single-binary-benchmark-results)). Required for `os.run` / `os.spawn` / `os.wait` / `os.poll` / `os.kill` and the whole `net` library. |
| `jennifer-tiny` | TinyGo                 | Constrained variant. Smaller binary, embeddable in minimal-footprint deployments (embedded systems, minimal containers, small-footprint scripting hosts). Missing `os/exec` (TinyGo runtime gap) and the network stack (no netdev driver). Calls into those surfaces return a friendly error pointing back at `jennifer`. |

Both binaries install side by side and never overlap. The packaged
distributions below install both; for tarball or from-source builds
you get both binaries in one go too.

## Install

### Debian / Ubuntu (`.deb`)

Pick the right `.deb` for your architecture from the latest
[Releases page](https://github.com/mplx/jennifer-lang/releases),
verify the checksum, and install:

```sh
# Replace X.Y.Z with the release version, e.g. 0.14.0
ARCH=$(dpkg --print-architecture)   # amd64 or arm64
curl -LO "https://github.com/mplx/jennifer-lang/releases/download/X.Y.Z/jennifer_X.Y.Z_${ARCH}.deb"
curl -LO "https://github.com/mplx/jennifer-lang/releases/download/X.Y.Z/jennifer_X.Y.Z_${ARCH}.deb.sha256"
sha256sum -c "jennifer_X.Y.Z_${ARCH}.deb.sha256"
sudo dpkg -i "jennifer_X.Y.Z_${ARCH}.deb"
```

Installs `/usr/bin/jennifer` + `/usr/bin/jennifer-tiny`, man pages
under `/usr/share/man/man1/`, and the XDG MIME definition that
registers `.j` as `text/x-jennifer` with file managers and editors.

### Arch Linux (AUR)

Two packages, take whichever fits:

```sh
# Prebuilt binary, downloads the release tarball (fast install):
yay -S jennifer-bin
# or paru -S jennifer-bin, or any other AUR helper.

# Builds from source on each install, tracks main:
yay -S jennifer-git
```

Both install the same set of files as the `.deb`. The `jennifer-bin`
package is on par with each release; `jennifer-git` rebuilds against
the latest commit on `main` whenever you ask your AUR helper to
upgrade.

### Linux (tarball)

For distros without a native package, grab the per-arch tarball
from the [Releases page](https://github.com/mplx/jennifer-lang/releases):

```sh
# Replace X.Y.Z and ARCH with the release version + your arch
curl -LO "https://github.com/mplx/jennifer-lang/releases/download/X.Y.Z/jennifer-X.Y.Z-linux-${ARCH}.tar.gz"
tar -xzf "jennifer-X.Y.Z-linux-${ARCH}.tar.gz"
cd "jennifer-X.Y.Z-linux-${ARCH}"
./jennifer version
./jennifer-tiny version
```

The tarball lays out as:

```
jennifer-X.Y.Z-linux-ARCH/
├── jennifer              # standard-Go binary (default)
├── jennifer-tiny         # TinyGo binary (constrained)
├── README.md
└── share/
    ├── man/man1/         # jennifer.1, jennifer-tiny.1
    └── mime/packages/    # jennifer.xml (XDG MIME)
```

To install system-wide:

```sh
sudo install -m 0755 jennifer      /usr/local/bin/
sudo install -m 0755 jennifer-tiny /usr/local/bin/
sudo install -m 0644 share/mime/packages/jennifer.xml /usr/local/share/mime/packages/
sudo update-mime-database /usr/local/share/mime || true
```

### macOS / Windows

Not supported yet. The interpreter is pure Go and *should* build
on either, but no CI coverage and no release artifacts ship. See
[milestones.md](../milestones.md) ("Future directions") for the
platform-portability work that gates these.

### Build from source

For development, or any platform without a prebuilt artifact. You
need a working [TinyGo](https://tinygo.org/) toolchain plus
standard Go. From the repository root:

```sh
# Build both binaries:
make build

# Or just one:
make build-go      # produces ./jennifer      (standard Go, default)
make build-tinygo  # produces ./jennifer-tiny (TinyGo, constrained)

# Quick iteration without rebuilding:
go run ./cmd/jennifer run examples/hello.j
```

The `make` targets regenerate `internal/version/version_gen.go`
from git state before invoking the toolchain, so `./jennifer
version` always reflects the current commit. See
[../libraries/meta.md](../libraries/meta.md) for the
`meta.VERSION` string format.

## Running

```sh
# Run a Jennifer source file (.j extension required):
jennifer run examples/hello.j

# Print the build version:
jennifer version
```

You can also pipe source in on stdin by passing `-` as the
filename:

```sh
echo 'use io; io.printf("hi\n");' | jennifer run -
jennifer run - < program.j
cat program.j | jennifer run -
```

When reading from stdin, error messages identify the source as
`<stdin>` and file imports (`include "name.j";`) resolve relative
to the current working directory.

## Interactive REPL

For experimenting with the language, start an interactive session
with `jennifer repl`:

```text
$ jennifer repl
jennifer - Jennifer programming language interpreter
type :quit (or Ctrl-D) to exit; :help for help
>>> use io;
>>> def x as int init 21;
>>> $x + $x;
42
>>> io.printf("hi\n");
hi
>>> func dbl(n as int) {
...   return $n * 2;
... }
>>> dbl(7);
14
>>> :quit
```

A few notes:

- Statements still end with `;`. If a line ends with an unclosed
  `{` or `(`, the prompt switches to `... ` and waits for you to
  finish the block.
- A bare expression at the end of an input (like `$x + $x;`)
  prints its value. `null` results (including the return value of
  `printf`) are suppressed.
- String results are printed with surrounding double quotes so
  they're distinguishable from numbers (`"hello"`, not `hello`).
- Variables, constants, methods, and library imports persist for
  the whole session. Methods can be redefined freely as you
  iterate.
- File splices (`include "lib.j";`) work in the REPL and resolve
  relative to the directory you launched `jennifer repl` from.
- `:quit`, `:exit`, or Ctrl-D end the session; `:help` shows a
  reminder.

The prompt supports the standard line-editing keys you'd expect
from a modern shell:

| Key                    | Action                   |
| ---------------------- | ------------------------ |
| Left / Right           | Move cursor              |
| Home / End             | Jump to line start / end |
| Ctrl+A / Ctrl+E        | Same as Home / End       |
| Ctrl+Left / Ctrl+Right | Move by word             |
| Backspace, Delete      | Delete character         |
| Ctrl+W, Ctrl+Backspace | Delete word backward     |
| Ctrl+U / Ctrl+K        | Kill to line start / end |
| Up / Down              | Browse history           |
| Ctrl+C                 | Cancel the current line  |

History is in-memory only (no on-disk persistence yet) and holds
up to 100 entries. When stdin is piped (e.g. `echo ... | jennifer
repl` in a test harness) the editor is bypassed and the REPL
reads lines normally, so non-interactive uses keep working.

## Inspection and formatting

Three commands help you see what Jennifer is doing under the hood
and keep your source in canonical shape:

```sh
# Print the lexer's token stream, one per line
jennifer tokens examples/hello.j

# Print the parsed (and preprocessed) AST as JSON
jennifer ast examples/hello.j

# Reformat the source to canonical style (see style-guide.md)
jennifer fmt examples/hello.j
```

`fmt` writes the formatted source to stdout. To rewrite in place,
use your shell: `jennifer fmt foo.j > foo.j.new && mv foo.j.new
foo.j`. The formatter is idempotent (`fmt` of `fmt` output equals
`fmt` output) and preserves runtime behavior - every example in
this repo is checked both ways by the test suite. See
[style-guide.md](style-guide.md) for the full style rules.
