# `validate` - declarative data validation

Import with `import "validate.j" as validate;`. Validate a `map of string to
string` (a form body, a query string, a config) against a rule set and get a
structured failure list back, instead of ad-hoc per-field `if` checks scattered
through a handler. Rules compose as value-semantic descriptors built by the
`validate.required` / `isInt` / `min` / `email` / ... family. Pure Jennifer over
`regex` + `uri` + `time` + `password` + `convert` + `lists` + `strings` + `maps`,
so it runs on **both binaries**.

```jennifer
import "validate.j" as validate;

def rules as map of string to list of validate.Rule init {
    "email": [validate.required(), validate.email()],
    "age":   [validate.isInt(), validate.min(0.0), validate.max(150.0)],
    "role":  [validate.oneOf(["reader", "author", "admin"])]
};

def errs as list of validate.Failure init validate.check($form, $rules);
if (len($errs) > 0) {
    for (def m in validate.messages($errs)) { io.printf("%s\n", $m); }
}
```

Runnable: [`examples/modules/validate_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/validate_demo.j)

## The model

A rule set is a `map of string to list of Rule`: each field maps to the rules it
must satisfy, in order. `validate.check(data, rules)` returns a `list of Failure`
(`{field, rule, message}`) - empty means valid; `validate.ok(data, rules)` is the
bool short-circuit.

Values are **strings** - the shape `web.bodyForm`, a query string, or `dotenv`
produce. A type rule (`isInt` / `isFloat` / `isBool`) checks the string *parses*;
`min` / `max` parse then compare; `minLen` / `maxLen` measure the string (in
runes).

Two conventions to know:

- **Only fields named in the rule set are checked.** Extra keys in `data` are
  ignored.
- **An absent or blank field passes every rule except `required`.** So an
  optional field left empty is valid; pair `required` with the other rules when a
  value must be present *and* well-formed.

## Rules

| Builder                       | Passes when the value...                             |
| ----------------------------- | ---------------------------------------------------- |
| `validate.required()`         | is present and non-empty                             |
| `validate.isInt()`            | parses as an integer                                 |
| `validate.isFloat()`          | parses as a number                                   |
| `validate.isBool()`           | is exactly `"true"` or `"false"`                     |
| `validate.min(n)`             | is numeric and `>= n` (a `float`)                    |
| `validate.max(n)`             | is numeric and `<= n`                                |
| `validate.minLen(n)`          | is at least `n` characters (an `int`)                |
| `validate.maxLen(n)`          | is at most `n` characters                            |
| `validate.pattern(re)`        | matches the RE2 source `re` (anchor with `^`/`$`)    |
| `validate.email()`            | looks like an email address                          |
| `validate.url()`              | is an absolute URL (a scheme and a host, via `uri`)  |
| `validate.datetime(format)`       | is a real date/time in the `strftime` `format`   |
| `validate.oneOf(allowed)`     | is one of `allowed` (a whitelist)                    |
| `validate.noneOf(blocked)`    | is **not** one of `blocked` (a blacklist)            |
| `validate.password(policy)`   | meets a `password.Schema` policy (length + classes)  |
| `validate.custom(fn, msg)`    | the predicate `fn(value)` returns true (else `msg`)  |
| `validate.withMessage(r, m)`  | (modifier) replaces rule `r`'s message with `m`      |

`noneOf` matches exactly and case-sensitively, like `oneOf`; for a
case-insensitive blacklist use a `pattern` with the `(?i)` flag
(`validate.pattern("(?i)^(admin|root)$")`) or normalise the value first.

`custom` takes a first-class **`func`** value - a top-level `func(v as string)`
returning a bool:

```jennifer
func isHandle(v as string) { return validate.ok({"h": $v}, {"h": [validate.pattern("^[a-z0-9_]+$")]}); }
# ...
"username": [validate.required(), validate.custom(isHandle, "must be a valid handle")]
```

The predicate is **exception-safe**: if it throws (say a naive `convert.toInt` on
a non-numeric value), the field is treated as invalid rather than the exception
escaping `check`.

`validate.datetime(format)` uses the `time` library's `strftime` codes, and checks
both the format and calendar validity (month 13 / day 32 fail):

| `format`        | Accepts        |
| --------------- | -------------- |
| `"%d.%m.%Y"`    | `25.12.2026`   |
| `"%m/%d/%Y"`    | `12/25/2026`   |
| `"%Y-%m-%d"`    | `2026-12-25` (ISO) |
| `"%Y-%m-%d %H:%M"` | `2026-12-25 14:30` |

The default message is generic ("must be a valid date"); pair it with
`withMessage` for a user-facing hint: `validate.withMessage(validate.datetime("%d.%m.%Y"), "use DD.MM.YYYY")`.

`validate.password(policy)` delegates to the [`password`](password.md) module's
`validate`: build a `password.Schema` with `password.schema()` and its `with*`
builders (length range, per-class minimums), and the rule enforces it. The failure
message is the policy's own failed-rule reasons joined with `"; "` (e.g. `too short
(minimum 8); needs at least 1 uppercase`); collapse them to one hint with
`withMessage`:

```jennifer
import "password.j" as password;
import "validate.j" as validate;

# at least 8 chars, at least one lower / upper / digit (symbols optional)
def policy as password.Schema init
    password.withMinimums(password.withLength(password.schema(), 8, 64), 1, 1, 1, 0);

def rules as map of string to list of validate.Rule init {
    "password": [validate.required(), validate.password($policy)]
};
```

Because Jennifer has no closures, the policy travels in the `Rule` itself (a
`schema` field), so `validate` imports `password` - it stays pure and runs on both
binaries (the rule only reads a policy, it never generates).

## Reading failures

| Call                        | Returns                             | Use                                     |
| --------------------------- | ----------------------------------- | --------------------------------------- |
| `validate.check(data, r)`   | `list of Failure`                   | every failure (empty = valid)           |
| `validate.ok(data, r)`      | `bool`                              | a quick valid / invalid gate            |
| `validate.messages(errs)`   | `list of string`                    | `"field: message"` lines for a log/flash |
| `validate.byField(errs)`    | `map of string to list of string`   | field -> its messages, to re-render a form |
| `validate.localize(errs, t)`| `list of Failure`                   | re-message with your own templates (i18n) |

A `Failure` is `{field, rule, param, message}`. `rule` is the failed rule's kind
(e.g. `"required"`, `"min"`) - a **stable id** for machine-readable responses or
your own message table - and `param` is the rule's argument in string form (a
threshold, a joined choices list), the piece a localized message interpolates.

## Non-English (or custom) messages

The built-in `message` is English. Because `Failure` carries a stable `rule` id
and its `param`, you can re-render every message however you like with
`validate.localize(errs, templates)`: `templates` maps a rule id to a template
where `%param%` and `%field%` are substituted. A rule **not** in the map keeps its
default message, so a partial map only overrides what it names, and the result is
a `list of Failure` (feed it to `messages` / `byField`):

```jennifer
def de as map of string to string init {
    "required": "ist erforderlich",
    "email":    "muss eine gültige E-Mail-Adresse sein",
    "min":      "muss mindestens %param% sein",
    "minLen":   "muss mindestens %param% Zeichen lang sein",
    "oneOf":    "muss eines von %param% sein"
};
def msgs as list of string init validate.messages(validate.localize($errs, $de));
```

The markers are `%name%`-style (brace-free) on purpose: they never collide with
the language's `{expr}` string interpolation, so a template reads the same in a
cooked or raw string. Substitution is a single left-to-right pass (a substituted
value is never re-scanned), and a literal `%` is written `%%` - valid in cooked or
raw alike, since the string engine never touches `%` (it owns `\` and `{}`, the
module owns `%`).

For full internationalization, build the `templates` map from
[`intl`](../libraries/intl.md) - one `intl.tr("validate.min")` per rule id, so the
active locale picks the language - then hand it to `localize`. The rule ids
(`required`, `isInt`, `isFloat`, `isBool`, `min`, `max`, `minLen`, `maxLen`,
`pattern`, `email`, `url`, `datetime`, `oneOf`, `noneOf`, `password`, `custom`) are
the catalog keys. (A `password` rule's default message is the `password` module's
own English reasons; a `"password"` template replaces the whole message.)

## Pairs with

- **`web`** - validate `web.bodyForm($ctx)` and respond `422` with
  `validate.byField` when invalid.
- **`rest`** - guard a request body before sending or after receiving.
- **`dotenv`** - validate a loaded config map.

## Scope

Values are strings, so validation is over the string form (what forms / queries /
config give you). A nested `json.Value` is not validated directly - pull the
fields you need into a `map of string to string` first. There is no cross-field
rule (e.g. "password == confirm") in v1; express that with a `custom` rule over
the whole map, or a follow-up check.

## See also

- [regex.md](../libraries/regex.md) / [uri.md](uri.md) / [time.md](../libraries/time.md)
  / [password.md](password.md) - the `pattern` / `email` / `url` / `datetime` /
  `password` backends.
- [intl.md](../libraries/intl.md) - the translation catalog to feed `localize`
  for non-English messages.
- [modules/index.md](index.md) - the module catalog and import rules.
