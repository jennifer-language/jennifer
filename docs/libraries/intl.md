# `intl` - internationalization (message catalogs, number formatting)

Enable with `use intl;`. Message catalogs and locale-aware translation: load a
`map of string to string` per language, pick a locale, and translate keys with
named-placeholder interpolation and an automatic fallback chain. The name is
`intl` (letters only, matching JavaScript's `Intl`), not `i18n`.

`intl` is a **system library**, not a `.j` module, because it holds **global
mutable state** (the loaded catalogs plus the current locale), which a
declarations-only module cannot. That state is per interpreter: each
interpreter (the entry program and each imported module) gets its own catalogs
and locale. The `map of string to string` you pass to `intl.load` is a one-time
ingest, copied into the library's own store.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `intl.load(lang, catalog)` | `null` | Merge a `map of string to string` into the catalog for `lang`. The **first** language loaded becomes the default. |
| `intl.setLocale(lang)` | `null` | Set the current locale (e.g. `"de"` or `"de-AT"`). It must have a catalog, itself or its language: `"de-AT"` needs `"de-AT"` or `"de"` loaded. |
| `intl.locale()` | `string` | The current locale, normalized (`""` until set). |
| `intl.tr(key)` | `string` | Translate `key` in the current locale, with fallback. |
| `intl.tr(key, params)` | `string` | Same, filling `%name%` placeholders from `params` (a `map`). |
| `intl.formatNumber(locale, value, digits)` | `string` | `value` (`int`, `float` or `decimal.Decimal`) with exactly `digits` (0-20) fraction digits, rounded half to even, written the way `locale` writes numbers. |

Every locale argument (`load`, `setLocale`, `formatNumber`) is normalized by
the rule in [Locale tags](#locale-tags), so `"de_at"`, `"DE-at"` and `"de-AT"`
name one catalog and one number format.

## Locale tags

A locale is a BCP 47 tag; `_` is read as `-`, and the tag is stored and matched
in its canonical case:

| Subtag | Form | Written | Example |
| ------ | ---- | ------- | ------- |
| language (first) | 2-8 letters | lower case | `DE` -> `de` |
| script (second, 4 letters) | 4 letters | title case | `zh_hant` -> `zh-Hant` |
| region (second or third) | 2 letters or 3 digits | upper case | `de_at` -> `de-AT`, `es-419` |
| any other | 1-8 letters or digits | lower case | `en-US-POSIX` -> `en-US-posix` |

An empty subtag (`"de-"`, `"de--AT"`), another character (`"de.AT"`) or an
empty tag is malformed and throws. The **language** of a tag is its first
subtag (`"zh-Hant-TW"` -> `"zh"`); `setLocale`, the [fallback
chain](#translating) and `formatNumber` fall back to it.

## Loading catalogs

A catalog is a `map of string to string`: a key to a message template. It can be
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

The **first** language loaded is the *default* (source) language: the last stop
in the fallback chain before the key itself, so make it your most complete
catalog.

### From `.yml` files on disk

Typically each language lives in its own file. Keep them as flat
`key: value` YAML and read each with [`fs`](fs.md) + [`yaml`](yaml.md), bridging
the decoded `yaml.Value` to the `map of string to string` `intl.load` wants:
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

def hello as string init intl.tr("greeting", {"name": "Welt"});   # "Hallo, Welt!"
def cart as string init intl.tr("cart", {"n": 3});   # "You have 3 items ..." (falls back to en)
```

The files are flat on purpose: `yaml.asString` reads a string leaf, so a nested
YAML value would need flattening into dotted keys first (`menu.file`), and a
non-string value (an unquoted number) would need `convert.toString`. The same
shape works with [`json`](json.md) (`json.keys` / `json.asString`) or
[`toml`](toml.md); [`examples/intl_toml.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/intl_toml.j) is a
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
def hello as string init intl.tr("greeting", {"name": "Welt"});
# "Servus, Welt!" if de-AT has it, else "Hallo, Welt!" from de,
# else the en default, else "greeting"
```

### Placeholders

With the two-argument form, each `%name%` in the template is replaced by
`params["name"]`. A non-string value is rendered to its display form (so a number
interpolates without a manual `convert.toString`). An unknown placeholder is left
**literal** (a missing value is visible), and `%%` is the escape for a literal
`%`:

```jennifer
def cart as string init intl.tr("cart", {"n": 3});   # "You have 3 items in your cart"
def hello as string init intl.tr("greeting", {"name": "Welt"});   # "Hallo, Welt!"
# template "raw %missing% and %%brace%%" -> "raw %missing% and %brace%"
```

The marker is `%name%`, not `{name}`, so a template reads the same whether it
lives in a cooked `"..."` or a raw `'...'` string: the language reserves `{}` for
[string interpolation](../user-guide/syntax.md) and `\` for escapes, while `intl`
owns `%`.

### Output cap (untrusted catalogs)

A catalog from an untrusted source could carry an amplification bomb: a template
that repeats a placeholder many times (`"%x%%x%%x%..."`), so a small template plus
a modest param blows up into a huge string. One interpolated translation is **capped at 1 MiB of output**, checked
incrementally so `intl.tr` **errors before** the oversized string is ever built
(a catchable error of kind `"limit"`). A real translation is a UI message
far below the cap; a document that large belongs in a templating layer.
Substitution is also single-pass (a param whose value itself
contains `%other%` is inserted verbatim, never re-expanded), so there is no
recursive-expansion blow-up either.

## Why `intl.tr`, not a global `_()`

There is no gettext-style ambient `_("key")`. `_` is not a valid Jennifer function
name (identifiers are letters-only; `_` is reserved for constant-name
separators), and a bare `tr()` builtin does not clear the bar for a language
built-in (translation is not useful to nearly every program, the way `len` is).
An ambient global also goes against the explicit, namespaced design (no
globals), so translation is a namespaced call,
`intl.tr("key")` (or `use intl as t; t.tr("key")`). Translation is *content
substitution from stateful external data*, distinct from `printf`'s job of
*presenting the value in hand*, which is why it is not a `printf` extension.

## Formatting numbers

`intl.formatNumber(locale, value, digits)` writes a number with the separators
and grouping of a locale. It takes the locale as an argument rather than the
current `setLocale` one, so a server can format each request in its own
locale without sharing state:

```jennifer
use io;
use intl;
use decimal;

def total as decimal.Decimal init decimal.from("1234567.885");
for (def loc in ["en", "de", "de-CH", "fr", "en-IN", "sv"]) {
    io.printf("%s: %s\n", $loc, intl.formatNumber($loc, $total, 2));
}
```

```text
en: 1,234,567.88
de: 1.234.567,88
de-CH: 1’234’567.88
fr: 1 234 567,88
en-IN: 12,34,567.88
sv: 1 234 567,88
```

| Aspect | Behaviour |
| ------ | --------- |
| Rounding | Half to even, as ICU and CLDR do: `.885` becomes `.88`, `.875` becomes `.88`. Round with `decimal.round` first for another mode. |
| Values | An `int` exactly, a `float` from its shortest exact decimal form (so `0.1` is `0.10`, not `0.1000000000000000055`), a `decimal.Decimal` from its own digits. Format money as `decimal.Decimal`, never as `float`. |
| Separators | The real characters CLDR uses, not look-alikes: French groups with a narrow no-break space (U+202F), Austrian, Nordic, Polish and Russian with a no-break space (U+00A0), Swiss German, Italian and English with a right single quotation mark (U+2019). Swedish, Norwegian and Finnish write a minus as U+2212. |
| Grouping | `en-IN` and `hi` group 3 then 2 (`12,34,567`). `es`, `pl` and `pt-PT` leave a four-digit number ungrouped (`1234`, but `12.345`). |
| Locales | A [normalized](#locale-tags) tag is matched exactly (`de-CH`, also `de_ch`), else by its language (`de-BE` formats as `de`). The table covers en, en-IN, en-CH, en-ZA, hi, de, de-AT, de-CH, de-LI, fr, fr-CA, fr-CH, it, it-CH, es, es-MX, es-US, pt, pt-PT, nl, da, sv, nb, no, fi, pl, cs, sk, hu, ru, uk, tr, ja, zh and ko, all with Latin digits. |
| Errors | Kind `"runtime"`: a malformed locale, a language not in the table (never a silent fallback to English), `digits` outside 0-20, or a value that is not a number. |

The text is for people to read, not to parse back: keep stored and exchanged
numbers in their plain form (`decimal.toString`, JSON numbers).

## Errors

| Kind | When |
| ---- | ---- |
| `"runtime"` | Misuse: a wrong argument count or type, a malformed locale, `setLocale` of a locale with no catalog for it or its language, `formatNumber` of a language not in its table, `digits` outside 0-20, a value that is not a number. |
| `"limit"` | `intl.tr` output past 1 MiB ([Output cap](#output-cap-untrusted-catalogs)). |

## Not here

Pluralization (CLDR per-language plural rules), an `intl.loadFile(path)`
convenience, currency and percent patterns, and date formatting. `intl.load` is
**merge-only**: there is no reset or replace, so a key removed from a reloaded
catalog lingers.

## See also

- [`json`](json.md) / [`toml`](toml.md) / [`yaml`](yaml.md) - config formats a
  catalog can be decoded from.
- [`convert`](convert.md) - `convert.toString` if you would rather stringify a
  placeholder value yourself.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Numbers and data: decimal, stats, linalg, ml, plot and dot](https://academy.jennifer-lang.dev/talks/mplx/numbers/) | locale-aware wording around numbers |
