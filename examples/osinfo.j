# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercise the namespaced os library in both its bare-prefix and aliased-prefix forms.
 * In real programs you pick one form per program; this example deliberately
 * uses both so the two spellings sit side-by-side, because each use activates
 * one prefix without removing the other. Golden output is pinned to
 * "linux"/"amd64" because Jennifer is Linux-only today; when Windows / macOS
 * support lands, this example's golden file will need a per-OS strategy.
 * @module osinfo
 */
use io;
use os;
use os as o;

# Bare prefix: the library name is the prefix.
io.printf("bare prefix:\n");
io.printf("  os.PLATFORM = %s\n", os.PLATFORM);
io.printf("  os.ARCH     = %s\n", os.ARCH);
io.printf("  os.DIRSEP   = %s\n", os.DIRSEP);
io.printf("  os.PATHSEP  = %s\n", os.PATHSEP);

# Aliased prefix: `o.` resolves to the same library via the alias.
io.printf("aliased prefix (`use os as o;`):\n");
io.printf("  o.PLATFORM  = %s\n", o.PLATFORM);
io.printf("  o.ARCH      = %s\n", o.ARCH);

# Alias is a rename operation: possible but should be avoided
use convert as realConvert;
use os as convert;
io.printf("aliased prefix (`use os as convert;`):\n");
io.printf("  convert.PLATFORM = %s\n", convert.PLATFORM);
io.printf("  convert.ARCH     = %s\n", convert.ARCH);

# Path functions return host-specific values (the working directory, the
# user's home, the temp dir), which vary per machine and can't be pinned in
# the golden - unlike the host constants above. So print only their type to
# keep the example portable; the values themselves are real strings.
io.printf("path functions (host-specific; type only):\n");
io.printf("  os.cwd()     -> %s\n", realConvert.typeOf(os.cwd()));
io.printf("  os.homeDir() -> %s\n", realConvert.typeOf(os.homeDir()));
io.printf("  os.tempDir() -> %s\n", realConvert.typeOf(os.tempDir()));
