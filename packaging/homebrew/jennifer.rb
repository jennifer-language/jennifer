# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Homebrew formula for the Jennifer language interpreter (best-effort,
# UNSUPPORTED macOS build - Linux is the only supported platform). It builds the
# standard-Go `jennifer` from source, so it works on Intel and Apple Silicon
# with no code-signing / Gatekeeper friction: Homebrew does not quarantine what
# it builds, and the Go linker ad-hoc-signs the arm64 binary it produces.
#
# This file is the canonical template. The `url` and `sha256` are placeholders;
# packaging/homebrew/publish.sh renders the real values for a release tag and
# pushes the result to the tap (see packaging/homebrew/README.md).
class Jennifer < Formula
  desc "Experimental interpreted programming language"
  homepage "https://jennifer-lang.dev"
  url "https://github.com/jennifer-language/jennifer/archive/refs/tags/0.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "LGPL-3.0-only"
  head "https://github.com/jennifer-language/jennifer.git", branch: "main"

  depends_on "go" => :build

  def install
    # Bake the version and the module search path at link time. Homebrew builds
    # with standard Go, which honours -ldflags -X (TinyGo would not); the GitHub
    # source tarball omits the gitignored version_gen.go, so no init() overrides
    # these. opt_prefix keeps the module path stable across version bumps.
    modules = "#{opt_prefix}/share/jennifer/modules"
    ldflags = %W[
      -s -w
      -X jennifer-lang.dev/jennifer/internal/version.Version=#{version}
      -X jennifer-lang.dev/jennifer/internal/module.compileDefaultSysmoddir=#{modules}
    ].join(" ")
    system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/"jennifer", "./cmd/jennifer"

    # The importable system modules (minus the white-box *_test.j overlays), so a
    # bare `import "name.j";` resolves against the baked path with no env var.
    (share/"jennifer/modules").install Dir["modules/*.j"].grep_v(/_test\.j$/)

    doc.install "README.md", "JENNIFER.md"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/jennifer version")

    (testpath/"hello.j").write('use io; io.printf("hi %d\n", 42);')
    assert_match "hi 42", shell_output("#{bin}/jennifer run #{testpath}/hello.j")

    # The module path is compiled in, so a bare import resolves with no setup.
    program = 'import "semver.j" as v; use io; io.printf("%t\n", v.isValid("1.2.3"));'
    assert_match "true", pipe_output("#{bin}/jennifer run -", program)
  end
end
