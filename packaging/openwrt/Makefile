# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# OpenWRT feed package Makefile for Jennifer. Drop this into an OpenWRT
# buildroot / SDK feed (e.g. package/jennifer/Makefile) and build with
# `make package/jennifer/{compile,install}` to produce the same .ipk the
# release pipeline publishes.
#
# This is a PREBUILT-binary package: it downloads the per-arch static
# release tarball and installs the interpreter from it, rather than
# compiling Jennifer from source inside the SDK. Two reasons: Jennifer
# tracks a newer Go than the OpenWRT golang host usually ships, and the
# binary is a CGO-free static ELF with no libc dependency, so the exact
# host toolchain does not matter. The payload and install paths match
# scripts/build-ipk.sh (the CI assembler) - keep the two in sync.
#
# Supported target arches: x86_64 and aarch64. The static ARMv8 build runs
# on every aarch64 subtarget; on a non-default subtarget install with
# `opkg install --force-architecture` (see packaging/openwrt/README.md).
#
# @PKG_VERSION@ / @HASH_AMD64@ / @HASH_ARM64@ are substituted by the release
# pipeline (a filled copy ships as a Release asset); edit them by hand to
# pin a different tag.

include $(TOPDIR)/rules.mk

PKG_NAME:=jennifer
PKG_VERSION:=@PKG_VERSION@
PKG_RELEASE:=1

PKG_MAINTAINER:=mplx <jennifer@mplx.dev>
PKG_LICENSE:=LGPL-3.0-only

# Map the OpenWRT target arch to the Go arch in the release tarball name and
# to that tarball's SHA-256. An unsupported arch leaves GO_ARCH empty so the
# download / build fails with a clear message rather than fetching nothing.
ifeq ($(ARCH),x86_64)
  GO_ARCH:=amd64
  PKG_HASH:=@HASH_AMD64@
else ifeq ($(ARCH),aarch64)
  GO_ARCH:=arm64
  PKG_HASH:=@HASH_ARM64@
else
  GO_ARCH:=
  PKG_HASH:=skip
endif

PKG_SOURCE:=jennifer-$(PKG_VERSION)-linux-$(GO_ARCH).tar.gz
PKG_SOURCE_URL:=https://github.com/jennifer-language/jennifer/releases/download/$(PKG_VERSION)

# The release tarball extracts to jennifer-<version>-linux-<goarch>/, so point
# PKG_BUILD_DIR at that name instead of the package.mk default.
PKG_BUILD_DIR:=$(BUILD_DIR)/jennifer-$(PKG_VERSION)-linux-$(GO_ARCH)

include $(INCLUDE_DIR)/package.mk

define Package/jennifer
  SECTION:=lang
  CATEGORY:=Languages
  TITLE:=Jennifer programming language interpreter
  URL:=https://github.com/jennifer-language/jennifer
  # Static CGO-free binary: no libc / shared-library dependencies.
  DEPENDS:=
endef

define Package/jennifer/description
  Jennifer is a small, experimental, interpreted programming language;
  source files use the .j extension. This installs the standard
  full-featured interpreter and the .j module library. The binary is a
  static build that runs on OpenWRT's musl unchanged; it wants roomy
  storage/RAM (a NAND / x86 / extroot target, not small internal flash).
endef

# Prebuilt: nothing to configure or compile.
define Build/Configure
endef

define Build/Compile
	$(if $(GO_ARCH),,$(error jennifer: unsupported target arch '$(ARCH)'; prebuilt release tarballs exist only for x86_64 and aarch64))
endef

define Package/jennifer/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/jennifer $(1)/usr/bin/jennifer
	# System module directory (compile-time default), so bare
	# `import "name.j";` resolves with no --sysmoddir.
	$(INSTALL_DIR) $(1)/usr/share/jennifer/modules
	$(CP) $(PKG_BUILD_DIR)/share/jennifer/modules/*.j $(1)/usr/share/jennifer/modules/
endef

$(eval $(call BuildPackage,jennifer))
