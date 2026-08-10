# `intl` - internationalization (message catalogs)

Enable with `use intl;`. Message catalogs and locale-aware translation: load a
`map of string to string` per language, pick a locale, and translate keys with
named-placeholder interpolation and an automatic fallback chain. The name is
`intl` (letters only, matching JavaScript's `Intl`) because a Jennifer library
namespace cannot contain a digit - so no `i18n`.

`intl` is a **system library**, not a `.j` module, for two independent reasons:
it holds **global mutable state** (the loaded catalogs plus the current locale),
which a declarations-only module cannot; and it keeps each catalog in a Go
`map[string]string` for **O(1)** lookup, where a Jennifer `map` (a linear-scan
`[]MapEntry`) would be O(n) on every translation. The `map of string to string`
you pass to `intl.load` is the one-time ingest; the per-lookup scan is what the
library avoids.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `intl.load(lang, catalog)` | `null` | Merge a `map of string to string` into the catalog for `lang`. The **first** language loaded becomes the default. |
| `intl.setLocale(lang)` | `null` | Set the current locale (e.g. `"de"` or `"de-AT"`). |
| `intl.locale()` | `string` | The current locale (`""` until set). |
| `intl.tr(key)` | `string` | Translate `key` in the current locale, with fallback. |
| `intl.tr(key, params)` | `string` | Same, filling `%name%` placeholders from `params` (a `map`). |

## Loading catalogs

A catalog is a `map of string to string` - a key to a message template. It can be
a literal, or built from a config file decoded with [`json`](json.md),
[`toml`](toml.md), or [`yaml`](yaml.md). Load one per language; loading the same
language again **merges** (later keys win):

```jennifer
use intl;

intl.load("en", {
    "greeting": "Hello, %name%!",
    "cart": "You have %n% items in your cart"
});
intl.load("de", { "greeting": "Hallo, %name%!" });
```

The **first** language loaded is the *default* (source) language - the last stop
in the fallback chain before the key itself, so make it your most complete
catalog.

### From `.yml` files on disk

In practice each language lives in its own file. Keep them as flat
`key: value` YAML and read each with [`fs`](fs.md) + [`yaml`](yaml.md), bridging
the decoded `yaml.Value` to the `map of string to string` `intl.load` wants -
`yaml.keys` walks the top-level keys, `yaml.asString` pulls each value:

```yaml
# locales/en.yml
greeting: "Hello, %name%!"
cart: "You have %n% items in your cart"
bye: "Goodbye"
```

```yaml
# locales/de.yml
greeting: "Hallo, %name%!"
bye: "Auf Wiedersehen"
```

```jennifer
use fs;
use yaml;
use intl;

# Read a flat "key: value" YAML catalog into a map of string to string.
func loadCatalog(path as string) {
    def doc as yaml.Value init yaml.decode(fs.readString($path));
    def out as map of string to string init {};
    for (def key in yaml.keys($doc)) {
        $out[$key] = yaml.asString($doc, "/" + $key);
    }
    return $out;
}

intl.load("en", loadCatalog("locales/en.yml"));   # the default language
intl.load("de", loadCatalog("locales/de.yml"));
intl.setLocale("de");

intl.tr("greeting", {"name": "Welt"})   # "Hallo, Welt!"
intl.tr("cart", {"n": 3})               # "You have 3 items ..." (falls back to en)
```

The files are flat on purpose: `yaml.asString` reads a string leaf, so a nested
YAML value would need flattening into dotted keys first (`menu.file`), and a
non-string value (an unquoted number) would need `convert.toString`. The same
shape works with [`json`](json.md) (`json.keys` / `json.asString`) or
[`toml`](toml.md) if you prefer those formats - `examples/intl_toml.j` is a
runnable version that writes `en.toml` / `de.toml`, reads them back, and loads
them with `toml.keys` / `toml.asString`:

```jennifer
func loadCatalog(path as string) {
    def doc as toml.Value init toml.decode(fs.readString($path));
    def out as map of string to string init {};
    for (def key in toml.keys($doc)) {
        $out[$key] = toml.asString($doc, "/" + $key);
    }
    return $out;
}
intl.load("en", loadCatalog("locales/en.toml"));
```

## Translating

`intl.tr(key)` resolves the key through a fallback chain, so a missing
translation degrades gracefully and is always **visible** rather than blank:

1. the **current locale** exactly (`"de-AT"`);
2. its **base language** (`"de-AT"` -> `"de"`, region stripped);
3. the **default** (first-loaded) language;
4. the **key itself**.

```jennifer
intl.setLocale("de-AT");
intl.tr("greeting", {"name": "Welt"})   # "Servus, Welt!" if de-AT has it,
                                        # else "Hallo, Welt!" from de,
                                        # else the en default, else "greeting"
```

### Placeholders

With the two-argument form, each `%name%` in the template is replaced by
`params["name"]`. A non-string value is rendered to its display form (so a number
interpolates without a manual `convert.toString`). An unknown placeholder is left
**literal** (a missing value is visible), and `%%` is the escape for a literal
`%`:

```jennifer
intl.tr("cart", {"n": 3})               # "You have 3 items in your cart"
intl.tr("greeting", {"name": "Welt"})   # "Hallo, Welt!"
# template "raw %missing% and %%brace%%" -> "raw %missing% and %brace%"
```

The marker is `%name%`, not `{name}`, so a template reads the same whether it
lives in a cooked `"..."` or a raw `'...'` string: the language reserves `{}` for
[string interpolation](../user-guide/syntax.md) and `\` for escapes, while `intl`
owns `%`. The two never collide.

### Output cap (untrusted catalogs)

A catalog from an untrusted source could carry an amplification bomb - a template
that repeats a placeholder many times (`"%x%%x%%x%..."`), so a small template plus
a modest param blows up into a huge string. To keep that from driving the process
toward OOM, one interpolated translation is **capped at 1 MiB of output**, checked
incrementally so `intl.tr` **errors before** the oversized string is ever built
(the error is catchable with `try` / `catch`). A genuine translation is a UI
message far below the cap; a document that large is a templating layer's job, not
a catalog entry. Substitution is also single-pass - a param whose value itself
contains `%other%` is inserted verbatim, never re-expanded - so there is no
recursive-expansion blow-up either.

## Why `intl.tr`, not a global `_()`

There is no gettext-style ambient `_("key")`. `_` is not a valid Jennifer method
name (identifiers are letters-only; `_` is reserved for constant-name
separators), and a bare `tr()` builtin does not clear the bar for a language
built-in (translation is not useful to nearly every program, the way `len` is).
An ambient global is also exactly what the **explicit** and **namespaced, no
globals** stances rule out - so translation is a namespaced call,
`intl.tr("key")` (or `use intl as t; t.tr("key")`). Translation is *content
substitution from stateful external data*, distinct from `printf`'s job of
*presenting the value in hand*, which is why it is not a `printf` extension.

## Not yet here

Pluralization (CLDR per-language plural rules), an `intl.loadFile(path)`
convenience, and locale-aware **value** formatting (number / date grouping - a
separate, open `printf` question) are follow-ons. `intl.load` is also
**merge-only** today - there is no reset or replace, so a key removed from a
reloaded catalog lingers; a `reset` / replace-semantics verb is a follow-on for
the hot-reload use case.

## See also

- [`json`](json.md) / [`toml`](toml.md) / [`yaml`](yaml.md) - config formats a
  catalog can be decoded from.
- [`convert`](convert.md) - `convert.toString` if you would rather stringify a
  placeholder value yourself.
