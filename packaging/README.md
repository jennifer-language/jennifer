# Packaging

Distribution-specific metadata for Jennifer. Each distro gets its
own subdirectory; shared assets (man pages, MIME definitions) live
at this level.

```
packaging/
  README.md             - this file
  debian/               - .deb control files (control, copyright, postinst, postrm)
  arch/                 - AUR PKGBUILDs (-bin downloads release, -git builds from source)
  mime/jennifer.xml       - XDG shared-mime-info; both .deb and AUR install it
  man/jennifer.1          - man page for the default (standard-Go) binary
  man/jennifer-tiny.1     - man page for the constrained (TinyGo) binary
  completions/jennifer.bash - bash completion; installed as
                            share/bash-completion/completions/jennifer,
                            with jennifer-tiny symlinked to it
```

The actual `.deb` is built by `scripts/build-deb.sh` (invoked
by `.github/workflows/release.yml`) and attached to the GitHub
Release. AUR packages are published manually after each tagged
release (the PKGBUILDs in `arch/` are the canonical source).

## Adding a new distro

Add a subdirectory `packaging/<distro>/` with the platform-native
recipe and reference the shared assets in `mime/`, `man/`, and
`completions/` so the MIME registration, man pages, and shell
completion stay consistent across distros.
Update the release pipeline if the build needs CI integration; the
parallel "Path to 1.0.0 distribution" track in
[../docs/milestones.md](../docs/milestones.md) lists planned future
formats (Homebrew, Snap, Nix flake).
