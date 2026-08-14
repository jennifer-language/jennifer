# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Declarative validation of a `map of string to string` (a form body, a query, a
 * config) against a rule set, returning a structured error list instead of ad-hoc
 * per-field `if` checks. Rules compose as value-semantic descriptors built by the
 * `validate.required` / `isInt` / `min` / `pattern` / `email` / ... family; group
 * them per field in a `map of string to list of Rule` and call `validate.check`.
 * Pure Jennifer over `regex` + `uri` + `time` + `password` + `convert` + `lists` +
 * `strings` + `maps`, so it runs on **both binaries**.
 *
 * Values are strings (the shape `web.bodyForm` / `dotenv` / a query string
 * produce): a type rule (`isInt` / `isFloat` / `isBool`) checks the string parses;
 * `min` / `max` parse then compare; `minLen` / `maxLen` measure the string. A
 * field that is absent or blank passes every rule **except** `required` - so an
 * optional field left empty is valid. Only the fields named in the rule set are
 * checked; extra data keys are ignored.
 * @module validate
 * @example
 * import "validate.j" as validate;
 * def rules as map of string to list of validate.Rule init {
 *     "email": [validate.required(), validate.email()],
 *     "age": [validate.isInt(), validate.min(0.0), validate.max(150.0)]
 * };
 * def errs as list of validate.Failure init validate.check($form, $rules);
 * if (len($errs) > 0) { web.respond($ctx, 422, ...); }
 */

use regex;
use convert;
use lists;
use strings;
use maps;
use time;
import "./uri.j" as uri;
import "./password.j" as pw;

/**
 * One validation rule (descriptor). Built by the rule family (`required` /
 * `isInt` / `min` / ...); not usually constructed directly.
 * @field kind {string} the rule kind ("required", "isInt", "min", "pattern", ...)
 * @field num {float} the numeric threshold for `min` / `max`
 * @field intVal {int} the length threshold for `minLen` / `maxLen`
 * @field str {string} the regex source for `pattern`
 * @field choices {list of string} the allowed values for `oneOf` / the blocked values for `noneOf`
 * @field fn {func} the predicate for `custom` (a `func(value as string)` -> bool)
 * @field schema {pw.Schema} the policy for `password` (a `Schema` from the `password` module)
 * @field message {string} an override message ("" = use the rule's default)
 */
export def struct Rule {
    kind as string,
    num as float,
    intVal as int,
    str as string,
    choices as list of string,
    fn as func,
    schema as pw.Schema,
    message as string
};

/**
 * One validation failure. `rule` is a stable id (the rule kind) and `param` is
 * the rule's argument in string form (a threshold, a joined choices list) - the
 * two a caller needs to render a custom or localized message via `localize`. The
 * built-in `message` is the default (English).
 * @field field {string} the field that failed
 * @field rule {string} the rule kind that failed (the stable message id)
 * @field param {string} the rule's argument for message interpolation ("" if none)
 * @field message {string} the default (English) human-readable message
 */
export def struct Failure {
    field as string,
    rule as string,
    param as string,
    message as string
};

# alwaysTrue is the default `fn` for every non-custom rule, so a Rule literal has a
# real func value for the field; only a `custom` rule actually calls `fn`.
func alwaysTrue(v as string) {
    return true;
}

func baseRule(kind as string) {
    def z as pw.Schema;
    return Rule{kind: $kind, num: 0.0, intVal: 0, str: "", choices: [],
        fn: alwaysTrue, schema: $z, message: ""};
}

# --- rule builders -----------------------------------------------------------

/**
 * Require the field to be present and non-empty. Every other rule is skipped for
 * an absent or blank field, so pair `required` with them to also validate a value.
 * @return {Rule} the rule
 */
export func required() {
    return baseRule("required");
}

/**
 * Require the value to parse as an integer.
 * @return {Rule} the rule
 */
export func isInt() {
    return baseRule("isInt");
}

/**
 * Require the value to parse as a number (int or float).
 * @return {Rule} the rule
 */
export func isFloat() {
    return baseRule("isFloat");
}

/**
 * Require the value to be exactly "true" or "false".
 * @return {Rule} the rule
 */
export func isBool() {
    return baseRule("isBool");
}

/**
 * Require the numeric value to be at least `n`.
 * @param n {float} the minimum
 * @return {Rule} the rule
 */
export func min(n as float) {
    def r as Rule init baseRule("min");
    $r.num = $n;
    return $r;
}

/**
 * Require the numeric value to be at most `n`.
 * @param n {float} the maximum
 * @return {Rule} the rule
 */
export func max(n as float) {
    def r as Rule init baseRule("max");
    $r.num = $n;
    return $r;
}

/**
 * Require the string to be at least `n` characters (runes).
 * @param n {int} the minimum length
 * @return {Rule} the rule
 */
export func minLen(n as int) {
    def r as Rule init baseRule("minLen");
    $r.intVal = $n;
    return $r;
}

/**
 * Require the string to be at most `n` characters (runes).
 * @param n {int} the maximum length
 * @return {Rule} the rule
 */
export func maxLen(n as int) {
    def r as Rule init baseRule("maxLen");
    $r.intVal = $n;
    return $r;
}

/**
 * Require the value to match an RE2 regular expression (anchored yourself with
 * `^` / `$` if you want a full match).
 * @param re {string} the regex source
 * @return {Rule} the rule
 */
export func pattern(re as string) {
    def r as Rule init baseRule("pattern");
    $r.str = $re;
    return $r;
}

/**
 * Require the value to look like an email address.
 * @return {Rule} the rule
 */
export func email() {
    return baseRule("email");
}

/**
 * Require the value to be an absolute URL (a scheme and a host).
 * @return {Rule} the rule
 */
export func url() {
    return baseRule("url");
}

/**
 * Require the value to be a valid date/time in the given `strftime` format
 * (`time`'s format codes): `"%d.%m.%Y"` for `dd.mm.yyyy`, `"%m/%d/%Y"` for
 * `mm/dd/yyyy`, `"%Y-%m-%d"` for ISO, `"%Y-%m-%d %H:%M"` for a date-time. The
 * value must both match the format *and* be a real calendar date (month 13 or day
 * 32 fail), since it is checked with `time.parse`. Pair with `withMessage` for a
 * user-facing "use DD.MM.YYYY" hint.
 * @param format {string} the strftime format the value must match
 * @return {Rule} the rule
 */
export func datetime(format as string) {
    def r as Rule init baseRule("datetime");
    $r.str = $format;
    return $r;
}

/**
 * Require the value to be one of `allowed`.
 * @param allowed {list of string} the permitted values
 * @return {Rule} the rule
 */
export func oneOf(allowed as list of string) {
    def r as Rule init baseRule("oneOf");
    $r.choices = $allowed;
    return $r;
}

/**
 * Reject the value if it is one of `blocked` (a blacklist - reserved usernames,
 * banned words). Exact, case-sensitive matching, like `oneOf`; for a
 * case-insensitive blacklist use a `pattern` with the `(?i)` flag, or normalise
 * the value first.
 * @param blocked {list of string} the forbidden values
 * @return {Rule} the rule
 */
export func noneOf(blocked as list of string) {
    def r as Rule init baseRule("noneOf");
    $r.choices = $blocked;
    return $r;
}

/**
 * Require the value to satisfy a `password` policy - length bounds and per-class
 * minimums - by delegating to the `password` module's `validate`. Build `policy`
 * with `password.schema()` and its `with*` builders. On failure the message is the
 * policy's own failed-rule reasons, joined; override with `withMessage` for a
 * single user-facing hint. Like every rule but `required`, an absent or blank
 * value is skipped - pair with `required` to also demand a value.
 * @param policy {pw.Schema} the policy to enforce (a `Schema` from the `password` module)
 * @return {Rule} the rule
 */
export func password(policy as pw.Schema) {
    def r as Rule init baseRule("password");
    $r.schema = $policy;
    return $r;
}

/**
 * A custom rule: `fn` is a `func(value as string)` returning true when valid.
 * @param fn {func} the predicate
 * @param message {string} the message when the predicate fails
 * @return {Rule} the rule
 */
export func custom(fn as func, message as string) {
    def r as Rule init baseRule("custom");
    $r.fn = $fn;
    $r.message = $message;
    return $r;
}

/**
 * Override the message of an already-built rule (fluent).
 * @param r {Rule} the rule
 * @param message {string} the message to use on failure
 * @return {Rule} the updated rule
 */
export func withMessage(r as Rule, message as string) {
    def nr as Rule init $r;
    $nr.message = $message;
    return $nr;
}

# --- checking ----------------------------------------------------------------

func msgOr(r as Rule, deflt as string) {
    if ($r.message != "") {
        return $r.message;
    }
    return $deflt;
}

# ruleParam renders a rule's argument as a string, for the Failure's `param` (so a
# localized template can interpolate it). Parameterless rules return "".
func ruleParam(r as Rule) {
    if ($r.kind == "min" or $r.kind == "max") {
        return convert.toString($r.num);
    }
    if ($r.kind == "minLen" or $r.kind == "maxLen") {
        return convert.toString($r.intVal);
    }
    if ($r.kind == "oneOf" or $r.kind == "noneOf") {
        return strings.join($r.choices, ", ");
    }
    if ($r.kind == "pattern" or $r.kind == "datetime") {
        return $r.str;
    }
    return "";
}

func parsesInt(v as string) {
    def okv as bool init true;
    try {
        convert.toInt($v);
    } catch (e) {
        $okv = false;
    }
    return $okv;
}

func parsesFloat(v as string) {
    def okv as bool init true;
    try {
        convert.toFloat($v);
    } catch (e) {
        $okv = false;
    }
    return $okv;
}

func isEmail(v as string) {
    return regex.matches('^[^@\s]+@[^@\s]+\.[^@\s]+$', $v);
}

func isUrl(v as string) {
    def u as uri.Uri init uri.parse($v);
    return $u.scheme != "" and $u.host != "";
}

# applyRule runs one rule against a field's value, returning an error message or
# "" when the rule passes. `required` is the only rule that fires on an absent or
# blank field; every other rule treats absent-or-blank as "nothing to validate".
func applyRule(r as Rule, value as string, present as bool) {
    if ($r.kind == "required") {
        if (not $present or $value == "") {
            return msgOr($r, "is required");
        }
        return "";
    }
    if (not $present or $value == "") {
        return "";
    }
    if ($r.kind == "isInt") {
        if (not parsesInt($value)) {
            return msgOr($r, "must be an integer");
        }
    } elseif ($r.kind == "isFloat") {
        if (not parsesFloat($value)) {
            return msgOr($r, "must be a number");
        }
    } elseif ($r.kind == "isBool") {
        if ($value != "true" and $value != "false") {
            return msgOr($r, "must be true or false");
        }
    } elseif ($r.kind == "min") {
        if (not parsesFloat($value)) {
            return msgOr($r, "must be a number");
        }
        if (convert.toFloat($value) < $r.num) {
            return msgOr($r, "must be at least " + convert.toString($r.num));
        }
    } elseif ($r.kind == "max") {
        if (not parsesFloat($value)) {
            return msgOr($r, "must be a number");
        }
        if (convert.toFloat($value) > $r.num) {
            return msgOr($r, "must be at most " + convert.toString($r.num));
        }
    } elseif ($r.kind == "minLen") {
        if (len($value) < $r.intVal) {
            return msgOr($r, "must be at least " + convert.toString($r.intVal) + " characters");
        }
    } elseif ($r.kind == "maxLen") {
        if (len($value) > $r.intVal) {
            return msgOr($r, "must be at most " + convert.toString($r.intVal) + " characters");
        }
    } elseif ($r.kind == "pattern") {
        if (not regex.matches($r.str, $value)) {
            return msgOr($r, "is not in the expected format");
        }
    } elseif ($r.kind == "email") {
        if (not isEmail($value)) {
            return msgOr($r, "must be a valid email address");
        }
    } elseif ($r.kind == "url") {
        if (not isUrl($value)) {
            return msgOr($r, "must be a valid URL");
        }
    } elseif ($r.kind == "oneOf") {
        if (not lists.contains($r.choices, $value)) {
            return msgOr($r, "must be one of: " + strings.join($r.choices, ", "));
        }
    } elseif ($r.kind == "noneOf") {
        if (lists.contains($r.choices, $value)) {
            return msgOr($r, "must not be one of: " + strings.join($r.choices, ", "));
        }
    } elseif ($r.kind == "password") {
        def rep as pw.Report init pw.validate($r.schema, $value);
        if (not $rep.valid) {
            return msgOr($r, strings.join($rep.reasons, "; "));
        }
    } elseif ($r.kind == "datetime") {
        def okd as bool init true;
        try {
            time.parse($value, $r.str);
        } catch (e) {
            $okd = false;
        }
        if (not $okd) {
            return msgOr($r, "must be a valid date/time");
        }
    } elseif ($r.kind == "custom") {
        # A predicate is caller-supplied; if it throws (e.g. a naive
        # convert.toInt on a non-numeric value) treat the value as invalid rather
        # than letting the exception escape check().
        def valid as bool init false;
        try {
            $valid = $r.fn($value);
        } catch (e) {
            $valid = false;
        }
        if (not $valid) {
            return msgOr($r, "is invalid");
        }
    }
    return "";
}

/**
 * Validate `data` against a rule set, returning every failure. Empty means valid.
 * @param data {map of string to string} the field values
 * @param rules {map of string to list of Rule} rules per field
 * @return {list of Failure} the failures (empty when valid)
 */
export func check(data as map of string to string, rules as map of string to list of Rule) {
    def errs as list of Failure init [];
    for (def field in $rules) {
        def present as bool init maps.has($data, $field);
        def value as string init "";
        if ($present) {
            $value = $data[$field];
        }
        for (def r in $rules[$field]) {
            def msg as string init applyRule($r, $value, $present);
            if ($msg != "") {
                $errs[] = Failure{field: $field, rule: $r.kind, param: ruleParam($r), message: $msg};
            }
        }
    }
    return $errs;
}

/**
 * Whether `data` satisfies every rule (a short-circuit over `check`).
 * @param data {map of string to string} the field values
 * @param rules {map of string to list of Rule} rules per field
 * @return {bool} true when valid
 */
export func ok(data as map of string to string, rules as map of string to list of Rule) {
    return len(check($data, $rules)) == 0;
}

/**
 * Render an error list as "field: message" strings (for logging or a flash).
 * @param errs {list of Failure} the failures
 * @return {list of string} one line per error
 */
export func messages(errs as list of Failure) {
    def out as list of string init [];
    for (def e in $errs) {
        $out[] = $e.field + ": " + $e.message;
    }
    return $out;
}

/**
 * Group error messages by field (for re-rendering a form with per-field errors).
 * @param errs {list of Failure} the failures
 * @return {map of string to list of string} field -> its messages
 */
export func byField(errs as list of Failure) {
    def out as map of string to list of string init {};
    for (def e in $errs) {
        def cur as list of string init [];
        if (maps.has($out, $e.field)) {
            $cur = $out[$e.field];
        }
        $cur[] = $e.message;
        $out[$e.field] = $cur;
    }
    return $out;
}

# matchAt reports whether the fixed `marker` string appears at char index `i` of
# `cs` (a rune list), without running past the end.
func matchAt(cs as list of string, i as int, marker as string) {
    def ms as list of string init strings.chars($marker);
    def m as int init len($ms);
    if ($i + $m > len($cs)) {
        return false;
    }
    def k as int init 0;
    while ($k < $m) {
        if ($cs[$i + $k] != $ms[$k]) {
            return false;
        }
        $k = $k + 1;
    }
    return true;
}

# applyTemplate substitutes %param% / %field% in a single left-to-right pass, so a
# substituted value is never re-scanned (a param value that itself contains
# "%field%" stays literal). "%%" is an escaped literal "%"; a lone "%" that starts
# no marker is emitted as-is.
func applyTemplate(tmpl as string, param as string, field as string) {
    def out as list of string init [];
    def cs as list of string init strings.chars($tmpl);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        if ($cs[$i] != "%") {
            $out[] = $cs[$i];
            $i = $i + 1;
        } elseif (matchAt($cs, $i, "%param%")) {
            $out[] = $param;
            $i = $i + 7;
        } elseif (matchAt($cs, $i, "%field%")) {
            $out[] = $field;
            $i = $i + 7;
        } elseif ($i + 1 < $n and $cs[$i + 1] == "%") {
            $out[] = "%";
            $i = $i + 2;
        } else {
            $out[] = "%";
            $i = $i + 1;
        }
    }
    return strings.join($out, "");
}

/**
 * Re-render failures with caller-supplied per-rule message templates - for
 * non-English messages, or a house style. `templates` maps a rule id (the
 * `Failure.rule`, e.g. `"min"` / `"email"`) to a template string; `%param%` is
 * replaced with the failure's `param` (a threshold, joined choices) and `%field%`
 * with its field name. A rule **not** in `templates` keeps its default `message`,
 * so a partial map overrides only the rules it names. The templates can come from
 * anywhere - a literal map, a config, or `intl.tr` per rule id. Returns the
 * failures with `message` replaced, so it composes with `messages` / `byField`.
 * The markers are `%name%`-style (brace-free) on purpose: they never collide with
 * the language's `{expr}` string interpolation, so a template reads the same in a
 * cooked or raw string. Substitution is a single pass (a substituted value is
 * never re-scanned), and a literal `%` is written `%%`.
 * @param failures {list of Failure} the failures from `check`
 * @param templates {map of string to string} rule id -> message template
 * @return {list of Failure} the failures with localized messages
 */
export func localize(failures as list of Failure, templates as map of string to string) {
    def out as list of Failure init [];
    for (def f in $failures) {
        def nf as Failure init $f;
        if (maps.has($templates, $f.rule)) {
            $nf.message = applyTemplate($templates[$f.rule], $f.param, $f.field);
        }
        $out[] = $nf;
    }
    return $out;
}
