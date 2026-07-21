# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `intl` library: load message catalogs for several languages,
 * switch locale, and translate keys with named-placeholder interpolation and a
 * locale -> base-language -> default-language -> key fallback chain. Every
 * output is deterministic, so it doubles as a golden test.
 * @module intl
 */

use io;
use intl;

# The first language loaded is the default (source) language.
intl.load("en", {
    "greeting": "Hello, {name}!",
    "cart": "You have {n} items in your cart",
    "bye": "Goodbye"
});
intl.load("de", { "greeting": "Hallo, {name}!", "bye": "Auf Wiedersehen" });
intl.load("de-AT", { "greeting": "Servus, {name}!" });

# No locale set yet: everything resolves against the default language.
io.printf("%s\n", intl.tr("greeting", {"name": "World"}));

intl.setLocale("de");
io.printf("locale is %s\n", intl.locale());
io.printf("%s\n", intl.tr("greeting", {"name": "Welt"}));
io.printf("%s\n", intl.tr("cart", {"n": 3}));      # -> default 'en'

# de-AT overrides only 'greeting'; the rest fall back de -> en.
intl.setLocale("de-AT");
io.printf("%s\n", intl.tr("greeting", {"name": "Welt"}));
io.printf("%s\n", intl.tr("bye"));                 # -> base 'de'

# A missing key is echoed back, so a translation gap is visible.
io.printf("%s\n", intl.tr("checkout.button"));
