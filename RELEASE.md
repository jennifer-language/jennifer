# Release checklist

Cutting a new Jennifer release. The CI pipeline does most of the work
automatically; this document covers the steps a human still has to do.

## What CI handles automatically

Triggered by pushing a bare-semver git tag (e.g. `0.14.1`, no `v`
prefix per project convention), `.github/workflows/release.yml`:

1. Cross-compiles both binaries (`jennifer` standard Go - default;
   `jennifer-tiny` TinyGo - constrained) for `linux/amd64` and
   `linux/arm64` from a single `ubuntu-latest` runner. QEMU
   user-mode emulation smoke-tests each non-native artifact.
2. Runs `examples/benchmark.j` on the native (`linux/amd64`) entry
   so the release notes carry fresh numbers.
3. Packages per arch:
   - `jennifer-<TAG>-linux-<ARCH>.tar.gz` (binaries + man pages +
     MIME definition + README), plus a sidecar `.sha256`.
   - `jennifer_<TAG>_<ARCH>.deb` via `scripts/build-deb.sh`. Sidecar
     `.sha256` next to the `.deb`.
4. Generates a release-ready `PKGBUILD-bin` with the real `pkgver`
   and `sha256sums_*` filled in, attaches it to the release draft.
5. Composes release notes from a template that embeds the benchmark
   output and the artifact list.
6. Publishes a **draft** GitHub Release with every artifact attached
   and the notes pre-filled.

The release goes live only after a human reviews and clicks Publish.

## What you still have to do

### Before tagging

1. **Check the milestone is actually shippable.**
   - `go test ./...` is green locally.
   - `make build` produces both binaries cleanly.
   - `examples/benchmark.j` runs to completion under both binaries
     and the numbers look healthy.
   - `grep -rPn '\xe2\x80\x94|\xe2\x80\x93'` is empty (CI also
     enforces this; doing it locally first avoids a wasted CI run).
2. **Refresh user-visible version markers** if any are pinned by
   hand. The README's `**Milestone N**` banner is the main one;
   bump it if this release closes a milestone you want callout for.
   The Jennifer-side `meta.VERSION` derives from `git describe`
   automatically, no source edits required.
3. **Land any release-notes prose** you want appended to the
   CI-generated template (workload commentary, breaking changes,
   migration notes for pre-1.0 breaks). Drafting in a tracking
   issue or as a checklist commit before tagging avoids
   last-minute thrash.

### Tagging

```sh
git tag -a 0.14.1 -m "0.14.1"
git push origin 0.14.1
```

(Project convention: bare semver tags, no `v` prefix.)

The annotated tag triggers `release.yml`. Watch the run on GitHub
Actions; it takes around 10 minutes (most of it the benchmark run
plus QEMU smoke tests).

### After CI completes

1. **Review the draft Release on GitHub.** It will have every
   artifact attached and the auto-generated notes filled in. Edit
   the notes if you want extra prose; verify the artifact list
   covers what you expect.
2. **Publish the draft** (Edit -> uncheck "Save as draft" -> Publish
   release).
3. **Publish the AUR packages** with the publish scripts. The canonical
   copies live in this repo at `packaging/arch/publish-bin.sh` and
   `packaging/arch/publish-git.sh`; each is copied into its AUR clone as
   `publish.sh` and run there. This is the one step that genuinely can't
   run from GitHub Actions: the AUR publishes via SSH-key-authenticated
   `git push` to `aur.archlinux.org`, and storing that key in CI secrets
   adds blast-radius for limited benefit at our scale.

   The scripts expect the two AUR clones as siblings of this repo (they
   sync `jennifer.install` from `../jennifer-lang/packaging/arch/`, and
   refuse to run outside an AUR clone):

   ```
   <workdir>/
     jennifer-lang/   # this repo
     jennifer-bin/    # AUR clone; publish.sh copied from packaging/arch/
     jennifer-git/    # AUR clone; publish.sh copied from packaging/arch/
   ```

   First time only, from your `jennifer-lang` checkout: clone both AUR
   repos as siblings and copy the publish scripts in.

   ```sh
   git clone ssh://aur@aur.archlinux.org/jennifer-bin.git ../jennifer-bin
   git clone ssh://aur@aur.archlinux.org/jennifer-git.git ../jennifer-git
   cp packaging/arch/publish-bin.sh ../jennifer-bin/publish.sh
   cp packaging/arch/publish-git.sh ../jennifer-git/publish.sh
   chmod +x ../jennifer-bin/publish.sh ../jennifer-git/publish.sh
   ```

   (Re-copy after either canonical script changes, to keep the clones
   current.)

   Each release, refresh `jennifer-bin`:

   ```sh
   ../jennifer-bin/publish.sh
   ```

   The script resolves the newest release from the GitHub API (the
   `/releases/latest/` alias can't be used: every `0.x` tag is a
   pre-release and that alias excludes pre-releases), downloads its
   `PKGBUILD-bin`, syncs `jennifer.install`, regenerates `.SRCINFO`,
   then commits and pushes to `master`. It is a clean no-op if the
   release is unchanged. Re-run it after every release, because `-bin`
   pins `pkgver` / `sha256sums` to the tarball. To smoke-test the
   actual install first, run `makepkg -si` in the clone (the script
   itself does not build).

   `jennifer-git` does **not** need a per-release update: its
   `pkgver()` derives from `git describe`, so AUR users get the new
   version automatically on their next rebuild. Its `publish.sh` is
   optional storefront polish, run only to refresh the version shown on
   the AUR page:

   ```sh
   ../jennifer-git/publish.sh
   ```

### Post-release

- Move the milestone log entry from "in progress" to "done" + the
  compaction pass we use for every shipped milestone (see prior
  M15.x entries for the shape).
- Open the next release-cycle tracking issue if relevant.

## Smoke test commands

Quick checks the artifacts work as advertised, run on a clean
machine after the release is live:

```sh
# Debian / Ubuntu
sudo dpkg -i jennifer_0.14.1_amd64.deb
jennifer version
jennifer-tiny version
jennifer run /usr/share/doc/jennifer/examples/hello.j 2>/dev/null \
    || echo 'hi' | jennifer run -

# Arch
yay -S jennifer-bin   # or paru, or any AUR helper
# (jennifer-git for the built-from-source variant)
jennifer version

# Tarball
tar -xzf jennifer-0.14.1-linux-amd64.tar.gz
cd jennifer-0.14.1-linux-amd64
./jennifer run -
```

## Known manual gaps (not currently CI-automated)

- AUR push (see above).
- Homebrew tap / Snap / Nix flake / `.pacman` standalone artefact -
  on the "Path to 1.0.0 distribution" parallel track in
  [docs/milestones.md](docs/milestones.md), not gated on this
  release process.
- macOS / Windows builds - waits on the platform-portability
  work tracked separately.
