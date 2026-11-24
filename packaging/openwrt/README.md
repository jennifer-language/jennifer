# OpenWrt package

An `.ipk` of the standard `jennifer` interpreter for OpenWrt, plus the feed
`Makefile` that builds it.

## Why this works at all

The default `jennifer` binary is built `CGO_ENABLED=0`, so it is a fully static
ELF that links no libc. That is the whole story for OpenWrt: a static Go binary
runs on musl exactly as it runs on glibc, so there is no separate
"aarch64-linux-musl" build - the ordinary `linux/arm64` (and `linux/amd64`,
`linux/armv6`, `linux/armv7`, `linux/386`, `linux/riscv64`) release binary *is* the OpenWrt
binary. The `.ipk` just wraps it in opkg's
package format.

## Install

Download the `.ipk` for your CPU from the
[GitHub Release](https://github.com/jennifer-language/jennifer/releases) and
install the local file:

```sh
# copy jennifer_<version>_<arch>.ipk to the device, then:
opkg install ./jennifer_<version>_x86_64.ipk                    # x86-64 targets
opkg install ./jennifer_<version>_aarch64_generic.ipk           # ARMv8 targets
opkg install ./jennifer_<version>_arm_cortex-a7_neon-vfpv4.ipk  # ARMv7 targets
opkg install ./jennifer_<version>_arm_arm1176jzf-s_vfp.ipk      # ARMv6 (bcm2708)
opkg install ./jennifer_<version>_i386_pentium4.ipk             # 32-bit x86 (SSE2)
opkg install ./jennifer_<version>_riscv64_riscv64.ipk           # RISC-V 64
jennifer run /root/hello.j
```

### Architecture note

The `.ipk` is labelled `x86_64`, `aarch64_generic`, `arm_cortex-a7_neon-vfpv4`,
`arm_arm1176jzf-s_vfp`, `i386_pentium4` or `riscv64_riscv64`. Because the binary is a
static, CPU-*family* build (not tuned per subtarget), it runs on every aarch64
subtarget - `aarch64_cortex-a53`, `aarch64_cortex-a72`, and so on - even though
opkg's architecture check only accepts the exact label by default. On such a
target, install with:

```sh
opkg install --force-architecture ./jennifer_<version>_aarch64_generic.ipk
```

This is safe *here specifically* because the binary carries no subtarget-specific
code, within the family's floor: the armv7 build needs an ARMv7 with VFP (the
cortex-a7 / a9 / a15 targets), the armv6 build an ARMv6 with VFP
(`arm_arm1176jzf-s_vfp`, bcm27xx/bcm2708; it also runs on the ARMv7 targets),
neither an ARMv5 one such as kirkwood, the 386 build SSE2 (not `x86/legacy` or `x86/geode`). (To stamp an exact subtarget instead, pass it to `scripts/build-ipk.sh` as
the optional fourth argument, or build through the feed `Makefile` on that
target, which stamps the target's real arch.)

## Sizing - read before deploying

This is the full interpreter, not a busybox applet. Budget accordingly:

- **Storage:** ~15 MB installed (the binary; modules add little). This will
  **not** fit the internal flash of a typical 8/16 MB-flash consumer router.
  It needs a roomier target - a NAND device, an x86 box, or
  [extroot](https://openwrt.org/docs/guide-user/additional-software/extroot_configuration)
  onto USB/SD.
- **RAM:** a tree-walking interpreter plus the Go runtime wants more than a
  shell script - comfortable on 128 MB+, tight below 64 MB.

For genuinely constrained routers the right answer is `jennifer-tiny` (much
smaller, purpose-built for embedded), which is currently unbuildable upstream
(a TinyGo scheduler regression) and so not shipped here yet; when it returns it
gets its own package.

## What the package installs

- `/usr/bin/jennifer` - the interpreter.
- `/usr/share/jennifer/modules/*.j` - the `.j` module library, at the binary's
  built-in system module dir (the release builds use `PREFIX=/usr`), so bare `import "name.j";` resolves
  with no `--sysmoddir` / `JENNIFER_SYSMODDIR`.

The desktop extras the `.deb` / tarball carry (man pages, MIME definition,
vim/nvim/sublime syntax) are deliberately omitted - they only waste flash on a
router.

## Building the `.ipk`

Two paths, producing the same package:

1. **From the prebuilt binary (what CI does).** After a `CGO_ENABLED=0` build of
   `./jennifer`, run the assembler - no OpenWrt SDK needed, just `ar` / `tar`:

   ```sh
   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o jennifer ./cmd/jennifer
   scripts/build-ipk.sh <version> arm64 dist
   # -> dist/jennifer_<version>_aarch64_generic.ipk
   ```

   The release pipeline (`.github/workflows/release.yml`) runs exactly this for
   `amd64`, `arm64`, `armv6` (GOARM=6), `armv7` (GOARM=7), `386` and `riscv64` on each tagged release and attaches the results to the
   Release.

2. **Through an OpenWrt feed (this `Makefile`).** Copy `Makefile` into an
   OpenWrt buildroot/SDK feed as `package/jennifer/Makefile`, then
   `make package/jennifer/compile`. It downloads the per-arch release tarball
   and installs from it (prebuilt), so it does not depend on the SDK's Go
   version. The release pipeline also attaches a version- and hash-filled copy
   of this `Makefile` to each Release for feed maintainers.

Keep `scripts/build-ipk.sh` and this `Makefile` in step: they install the same
files to the same paths.
