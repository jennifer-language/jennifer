# Installing & running

## Which binary?

On **Linux** (the supported platform) Jennifer ships as two binaries.
Same source, same language; only the compiler differs. Pick by use
case:

| Binary          | Build                  | Pick when                                                                                                                                          |
| --------------- | ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `jennifer`      | standard Go (default)  | **What most users want.** Full host-feature surface; competitive on compute-heavy work (the two builds are now within ~1.5x either way per workload on the serial benchmark, and the Go binary wins the end-to-end wall clock once `spawn` parallelism is involved; see [technical/benchmark.md](../technical/benchmark.md)) and the reliable choice for multi-core parallel `spawn`. Required for `os.run` / `os.spawn` / `os.wait` / `os.poll` / `os.kill` and the whole `net` library. |
| `jennifer-tiny` | TinyGo                 | Constrained variant. Smaller binary, embeddable in minimal-footprint deployments (embedded systems, minimal containers, small-footprint scripting hosts). Missing `os/exec` (TinyGo runtime gap) and the network stack (no netdev driver). Also run-only: the `tokens` / `ast` / `fmt` / `lint` / `profile` / `test` development subcommands live only in the default binary. Calls into any of these surfaces return a friendly error pointing back at `jennifer`. |

Both binaries install side by side and never overlap. The packaged
distributions below install both; for tarball or from-source builds
you get both binaries in one go too. (The best-effort **macOS /
Windows** builds ship the standard `jennifer` only - see
[macOS / Windows](#macos--windows-unsupported) below.)

## Install

### Debian / Ubuntu (`.deb`)

Pick the right `.deb` for your architecture from the latest
[Releases page](https://github.com/jennifer-language/jennifer/releases),
verify the checksum, and install:

```sh
# Replace X.Y.Z with the release version, e.g. 0.14.0
ARCH=$(dpkg --print-architecture)   # amd64 or arm64
curl -LO "https://github.com/jennifer-language/jennifer/releases/download/X.Y.Z/jennifer_X.Y.Z_${ARCH}.deb"
curl -LO "https://github.com/jennifer-language/jennifer/releases/download/X.Y.Z/jennifer_X.Y.Z_${ARCH}.deb.sha256"
sha256sum -c "jennifer_X.Y.Z_${ARCH}.deb.sha256"
sudo dpkg -i "jennifer_X.Y.Z_${ARCH}.deb"
```

Installs `/usr/bin/jennifer` + `/usr/bin/jennifer-tiny`, man pages
under `/usr/share/man/man1/`, bash completion, the XDG MIME definition
that registers `.j` as `text/x-jennifer` with file managers and
editors, and Vim + Neovim syntax highlighting (dropped in both
`/usr/share/vim/vimfiles` and `/usr/share/nvim/site` so `.j` files
highlight with no per-user setup in either editor). A Sublime Text /
[`bat`](https://github.com/sharkdp/bat) syntax also ships at
`/usr/share/jennifer/syntaxes/jennifer.sublime-syntax`; `bat` needs a
one-time activation (copy it into `$(bat --config-dir)/syntaxes/` and run
`bat cache --build`) since it compiles syntaxes into a per-user cache.

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
from the [Releases page](https://github.com/jennifer-language/jennifer/releases):

```sh
# Replace X.Y.Z and ARCH with the release version + your arch
curl -LO "https://github.com/jennifer-language/jennifer/releases/download/X.Y.Z/jennifer-X.Y.Z-linux-${ARCH}.tar.gz"
tar -xzf "jennifer-X.Y.Z-linux-${ARCH}.tar.gz"
cd "jennifer-X.Y.Z-linux-${ARCH}"
./jennifer version
./jennifer-tiny version
```

The tarball lays out as:

```
jennifer-X.Y.Z-linux-ARCH/
├── jennifer               # standard-Go binary (default)
├── jennifer-tiny          # TinyGo binary (constrained)
├── README.md
├── JENNIFER.md
└── share/
    ├── man/man1/          # jennifer.1, jennifer-tiny.1
    ├── mime/packages/     # jennifer.xml (XDG MIME)
    ├── bash-completion/   # completions/jennifer (+ jennifer-tiny symlink)
    ├── vim/vimfiles/      # syntax/ + ftdetect/ (Vim highlighting)
    ├── nvim/site/         # syntax/ + ftdetect/ (Neovim highlighting)
    └── jennifer/
        ├── modules/       # importable .j modules (import "name.j";)
        └── syntaxes/      # jennifer.sublime-syntax (Sublime Text / bat)
```

To install system-wide:

```sh
sudo install -m 0755 jennifer      /usr/local/bin/
sudo install -m 0755 jennifer-tiny /usr/local/bin/
sudo install -m 0644 share/mime/packages/jennifer.xml /usr/local/share/mime/packages/
sudo update-mime-database /usr/local/share/mime || true
# System modules, so a bare `import "name.j";` resolves. /usr/share/jennifer/modules
# is the built-in default; install elsewhere and point JENNIFER_SYSMODDIR (or
# --sysmoddir) at it instead. To print the path in effect:
#   echo 'use io; use meta; io.printf("%s\n", meta.SYSMODDIR);' | jennifer run -
sudo mkdir -p /usr/share/jennifer/modules
sudo install -m 0644 share/jennifer/modules/*.j /usr/share/jennifer/modules/
```

### macOS / Windows (unsupported)

**Linux is the only supported platform.** As a convenience, best-effort
**unsupported** binaries for macOS (Intel + Apple Silicon) and Windows
(64- and 32-bit) are attached to each
[release](https://github.com/jennifer-language/jennifer/releases), named
`...-UNSUPPORTED`. Read the caveats before relying on them:

- **Best-effort, may be absent.** They come from a pipeline step that is
  allowed to fail; if a build breaks, that release simply won't have them,
  and it does not hold up the Linux release.
- **Standard `jennifer` only.** No `jennifer-tiny` - TinyGo's macOS /
  Windows host support is too limited to ship. This is the full-featured
  build, so `os.run` / `os.spawn`, the `net` library, and the rest of the
  surface all work.
- **Unsigned.** On macOS, Gatekeeper quarantines the download - clear it
  with `xattr -d com.apple.quarantine ./jennifer` (or right-click ->
  Open). On Windows, SmartScreen warns about an unknown publisher - choose
  "More info" -> "Run anyway".
- **No support.** Bugs specific to macOS / Windows may not be fixed;
  supported development and testing happen on Linux. Fully supported
  builds for these platforms are separate future work (see
  [milestones.md](../milestones.md)).
- **Just the binary (macOS).** No installer, man pages, MIME registration,
  or shell completion - the archive holds the executable plus `JENNIFER.md`,
  `README.md`, and the licence. Windows additionally gets an installer (below).

#### Windows installer

Windows releases also ship a `jennifer-<version>-setup.exe` - the same
best-effort **unsupported** build wrapped in an [Inno Setup](https://jrsoftware.org/isinfo.php)
installer. It is still unsigned (SmartScreen: "More info" -> "Run anyway") and
still unsupported, but it saves the manual setup:

- Installs `jennifer.exe` per-user (no admin) to
  `%LOCALAPPDATA%\Programs\Jennifer` and adds it to your `PATH`, so `jennifer`
  works in a fresh terminal.
- Bundles the Jennifer-coded system modules and sets `JENNIFER_SYSMODDIR`, so a
  bare `import "name.j";` resolves (on Windows the built-in module path is a
  Unix path that does not exist, so the plain `.zip` cannot import modules
  without setting this yourself).
- Optionally associates `.j` files (opt-in): double-click opens the source in
  Notepad; a "Run with Jennifer" right-click action runs it.
- Uninstall from **Apps & Features**, which reverses the `PATH`, the
  environment variable, and the association.

Prefer the plain `-UNSUPPORTED.zip` if you want a portable, no-registry copy (or
you are on 32-bit Windows - the installer is 64-bit only); set
`JENNIFER_SYSMODDIR` yourself to use modules from the zip.

Windows 8.1 and earlier are not possible: this project's Go toolchain
(Go 1.21+) produces binaries that require **Windows 10 or newer** (or
Windows Server 2016+). Go discontinued support for older releases in Go
1.21, so Windows 7, 8, and 8.1 - as well as Vista and XP - are all
excluded, not just XP. The 32-bit build targets 32-bit Windows 10 / 11.

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

```jennifer
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
