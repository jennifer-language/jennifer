# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# osinfo.j - exercise the M8 namespaced `os` library in both its
# bare-prefix and aliased-prefix forms. In real programs you pick
# one form per program (the style guide and user-guide/imports.md
# document why). This example deliberately uses both so the two
# spellings sit side-by-side; the runtime accepts the combination
# because each `use` activates one prefix without removing the
# other.
#
# Golden output is pinned to "linux" because Jennifer is Linux-only
# today. When Windows / macOS support lands, this example's golden
# file (examples/expected/osinfo.txt) will need a per-OS strategy -
# until then, this example is the deliberate home for printing the
# actual host-OS values; showcase.j only convert.typeOf()-s them so it stays
# portable.

use io;
use os;
use os as o;

# Bare prefix: the library name is the prefix.
io.printf("bare prefix:\n");
io.printf("  os.platform()       = %s\n", os.platform());
io.printf("  os.JENNIFER_OS      = %s\n", os.JENNIFER_OS);

# Aliased prefix: `o.` resolves to the same library via the alias.
io.printf("aliased prefix (`use os as o;`):\n");
io.printf("  o.platform()        = %s\n", o.platform());
io.printf("  o.JENNIFER_OS       = %s\n", o.JENNIFER_OS);

# Alias is a rename operation: possible but should be avoided
use convert as realConvert;
use os as convert;
io.printf("aliased prefix (`use os as convert;`):\n");
io.printf("  convert.platform()        = %s\n", convert.platform());
io.printf("  convert.JENNIFER_OS       = %s\n", convert.JENNIFER_OS);
