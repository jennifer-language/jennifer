# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Loads `intl` message catalogs from TOML files on disk. Writes an en.toml and a
 * de.toml into a unique scratch directory (fs.makeTempDir), reads each back with
 * `fs`, decodes it with `toml`, and bridges the decoded `toml.Value` to the
 * `map of string to string` that `intl.load` wants (`toml.keys` walks the keys,
 * `toml.asString` pulls each value). The scratch dir is removed on the way out,
 * and every output is deterministic, so it doubles as a golden test.
 * @module intl
 */

use io;
use fs;
use toml;
use intl;

/**
 * Read a flat "key = value" TOML catalog into a map of string to string.
 * @param path {string} the path to a TOML catalog file
 * @return {map of string to string} the key -> template pairs to hand intl.load
 */
func loadCatalog(path as string) {
    def doc as toml.Value init toml.decode(fs.readString($path));
    def out as map of string to string init {};
    for (def key in toml.keys($doc)) {
        $out[$key] = toml.asString($doc, "/" + $key);
    }
    return $out;
}

# A fresh unique scratch directory, then the two catalog files (the "assets" a
# real app ships).
def dir as string init fs.makeTempDir("", "intl-");
fs.writeString($dir + "/en.toml",
    "greeting = \"Hello, {name}!\"\n"
    + "cart = \"You have {n} items in your cart\"\n"
    + "bye = \"Goodbye\"\n");
fs.writeString($dir + "/de.toml",
    "greeting = \"Hallo, {name}!\"\n"
    + "bye = \"Auf Wiedersehen\"\n");

# Load them: the first language loaded (en) is the default / fallback.
intl.load("en", loadCatalog($dir + "/en.toml"));
intl.load("de", loadCatalog($dir + "/de.toml"));

intl.setLocale("de");
io.printf("locale: %s\n", intl.locale());
io.printf("%s\n", intl.tr("greeting", {"name": "Welt"}));
io.printf("%s\n", intl.tr("bye"));
io.printf("%s\n", intl.tr("cart", {"n": 3}));   # not in de -> falls back to en

# Clean up the scratch directory.
fs.removeAll($dir);
io.printf("cleanup: exists = %t\n", fs.exists($dir));
