# Homebrew tap (macOS, best-effort unsupported)

[`jennifer.rb`](jennifer.rb) is a [Homebrew](https://brew.sh) formula that builds
the standard-Go `jennifer` **from source**. Linux stays the only supported
platform; this is a best-effort, **unsupported** macOS convenience - the same
status as the `-UNSUPPORTED` tarball, but installed the way macOS users expect a
CLI to arrive.

Building from source is deliberate: it works on both Intel and Apple Silicon with
**no code-signing / Gatekeeper friction** (Homebrew does not quarantine what it
builds, and the Go linker ad-hoc-signs the arm64 binary it produces), and the
formula bakes the version and the module search path at link time, so a bare
`import "name.j";` resolves with no environment variable.

## Install (for users)

**Not published yet:** the `homebrew-tap` repo does not exist, so the commands
below work only once the formula is first published (see "One-time tap setup").
The formula goes to a tap repo named `homebrew-tap` under the project org, so
the tap is `jennifer-language/tap`:

```sh
brew install jennifer-language/tap/jennifer
# or:
brew tap jennifer-language/tap
brew install jennifer
```

`brew install --HEAD jennifer-language/tap/jennifer` builds from `main`.

`jennifer-tiny` is **not** shipped here (TinyGo's macOS host support is too
limited); the tap installs the full standard-Go `jennifer` only.

## One-time tap setup (for the maintainer)

Create a public repo `jennifer-language/homebrew-tap` (the `homebrew-` prefix is
what lets `brew tap jennifer-language/tap` find it). The formula lives at
`Formula/jennifer.rb` inside it; `publish.sh` writes it there.

## Publishing a release

`publish.sh` mirrors the AUR flow (`packaging/arch/publish-bin.sh`): copy it into
your `homebrew-tap` clone and run it there after a release is tagged.

```sh
cp /path/to/jennifer-lang/packaging/homebrew/publish.sh ~/src/homebrew-tap/
cd ~/src/homebrew-tap
./publish.sh
```

It resolves the newest release tag, downloads the GitHub source tarball, computes
its `sha256`, renders `Formula/jennifer.rb` from this canonical template (the
`url` + `sha256` are the only per-release values), then commits and pushes. It
refuses to run outside a `homebrew-` clone and no-ops if already up to date.

The committed [`jennifer.rb`](jennifer.rb) here is the **template**: its `url`
tag (`0.0.0`) and `sha256` (all zeros) are placeholders that `publish.sh` fills
in - the same template-plus-render split the AUR `PKGBUILD-bin` uses.

## CI-automation option

The manual `publish.sh` needs no secrets (matching the AUR convention). To
automate it instead, add a job that renders the formula on a release tag and
pushes to the tap repo using a personal-access-token secret with write access to
`homebrew-tap` (the built-in `GITHUB_TOKEN` cannot push to a different repo).
Local testing before publishing:

```sh
brew install --build-from-source ./packaging/homebrew/jennifer.rb   # with real url/sha
brew test jennifer
brew audit --strict --new jennifer
```
