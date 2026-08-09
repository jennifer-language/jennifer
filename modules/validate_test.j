# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# White-box tests for modules/validate.j. Run: jennifer test modules/validate_test.j
#
# The overlay splices validate.j in front of this file, so the tests reach both
# its exported surface (required / check / ok / ...) and its private helpers
# (applyRule, parsesInt, isEmail, alwaysTrue, ...) by bare identifier.

use testing;

# A predicate for the custom rule.
func even(v as string) {
    return convert.toInt($v) % 2 == 0;
}

func firstMsg(errs as list of Failure, field as string) {
    for (def e in $errs) {
        if ($e.field == $field) {
            return $e.message;
        }
    }
    return "";
}

func paramOf(errs as list of Failure, field as string) {
    for (def e in $errs) {
        if ($e.field == $field) {
            return $e.param;
        }
    }
    return "";
}

# --- rule builders set the right shape ---

func testRuleBuilders() {
    testing.assertEqual(required().kind, "required");
    testing.assertEqual(isInt().kind, "isInt");
    testing.assertEqual(isFloat().kind, "isFloat");
    testing.assertEqual(isBool().kind, "isBool");
    testing.assertEqual(min(1.5).num, 1.5);
    testing.assertEqual(max(9.0).num, 9.0);
    testing.assertEqual(minLen(3).intVal, 3);
    testing.assertEqual(maxLen(8).intVal, 8);
    testing.assertEqual(pattern("^x$").str, "^x$");
    testing.assertEqual(email().kind, "email");
    testing.assertEqual(url().kind, "url");
    testing.assertEqual(datetime("%Y-%m-%d").str, "%Y-%m-%d");
    testing.assertEqual(len(oneOf(["a", "b"]).choices), 2);
    testing.assertEqual(len(noneOf(["a", "b"]).choices), 2);
    testing.assertEqual(password(pw.schema()).kind, "password");
    testing.assertEqual(custom(even, "must be even").message, "must be even");
    testing.assertEqual(withMessage(email(), "bad").message, "bad");
}

# --- a fully valid payload has no failures ---

func testAllValid() {
    def rules as map of string to list of Rule init {
        "email": [required(), email()],
        "age": [required(), isInt(), min(0.0), max(150.0)],
        "name": [required(), minLen(2), maxLen(10)],
        "role": [oneOf(["admin", "user"])],
        "site": [url()],
        "flag": [isBool()],
        "score": [isFloat()],
        "even": [custom(even, "must be even")]
    };
    def data as map of string to string init {"email": "a@b.com", "age": "30", "name": "Ada",
        "role": "admin", "site": "https://x.io", "flag": "true", "score": "1.5", "even": "4"};
    testing.assertTrue(ok($data, $rules));
    testing.assertEqual(len(check($data, $rules)), 0);
}

# --- each rule fails with the right message ---

func testEachRuleFails() {
    def rules as map of string to list of Rule init {
        "email": [email()],
        "site": [url()],
        "n": [isInt()],
        "f": [isFloat()],
        "b": [isBool()],
        "lo": [min(10.0)],
        "hi": [max(5.0)],
        "short": [minLen(3)],
        "long": [maxLen(2)],
        "pat": [pattern("^[0-9]+$")],
        "role": [oneOf(["admin", "user"])],
        "even": [custom(even, "must be even")]
    };
    def data as map of string to string init {"email": "nope", "site": "xx", "n": "x", "f": "x",
        "b": "maybe", "lo": "3", "hi": "9", "short": "ab", "long": "abc", "pat": "abc",
        "role": "root", "even": "3"};
    def errs as list of Failure init check($data, $rules);
    testing.assertEqual(len($errs), 12);
    testing.assertEqual(firstMsg($errs, "email"), "must be a valid email address");
    testing.assertEqual(firstMsg($errs, "site"), "must be a valid URL");
    testing.assertEqual(firstMsg($errs, "n"), "must be an integer");
    testing.assertEqual(firstMsg($errs, "f"), "must be a number");
    testing.assertEqual(firstMsg($errs, "b"), "must be true or false");
    testing.assertEqual(firstMsg($errs, "lo"), "must be at least 10.0");
    testing.assertEqual(firstMsg($errs, "hi"), "must be at most 5.0");
    testing.assertEqual(firstMsg($errs, "short"), "must be at least 3 characters");
    testing.assertEqual(firstMsg($errs, "long"), "must be at most 2 characters");
    testing.assertEqual(firstMsg($errs, "pat"), "is not in the expected format");
    testing.assertEqual(firstMsg($errs, "role"), "must be one of: admin, user");
    testing.assertEqual(firstMsg($errs, "even"), "must be even");
}

# --- required / optional-blank semantics ---

func testRequiredAbsentAndBlank() {
    def rules as map of string to list of Rule init {"x": [required(), minLen(2)]};
    # absent -> required fires
    testing.assertEqual(len(check({}, $rules)), 1);
    # present but blank -> required fires (and minLen is skipped for blank)
    def blank as map of string to string init {"x": ""};
    def eb as list of Failure init check($blank, $rules);
    testing.assertEqual(len($eb), 1);
    testing.assertEqual($eb[0].rule, "required");
}

func testOptionalBlankPasses() {
    # no `required`: an absent or blank field passes every rule
    def rules as map of string to list of Rule init {"x": [isInt(), min(0.0), email()]};
    testing.assertTrue(ok({}, $rules));
    def blank as map of string to string init {"x": ""};
    testing.assertTrue(ok($blank, $rules));
}

# --- min/max on a non-numeric value reports "must be a number" ---

func testMinMaxNonNumeric() {
    def rules as map of string to list of Rule init {"a": [min(0.0)], "b": [max(0.0)]};
    def data as map of string to string init {"a": "x", "b": "y"};
    def errs as list of Failure init check($data, $rules);
    testing.assertEqual(len($errs), 2);
    testing.assertEqual(firstMsg($errs, "a"), "must be a number");
    testing.assertEqual(firstMsg($errs, "b"), "must be a number");
}

func testMinMaxBoundariesPass() {
    def rules as map of string to list of Rule init {"a": [min(5.0), max(5.0)]};
    testing.assertTrue(ok({"a": "5"}, $rules));       # equal passes both
    testing.assertFalse(ok({"a": "4"}, $rules));      # below min
    testing.assertFalse(ok({"a": "6"}, $rules));      # above max
}

# --- custom pass + a per-rule message override ---

func testCustomAndOverride() {
    def rules as map of string to list of Rule init {"n": [custom(even, "must be even")]};
    testing.assertTrue(ok({"n": "8"}, $rules));
    testing.assertFalse(ok({"n": "7"}, $rules));
    # withMessage overrides the default on any rule
    def r2 as map of string to list of Rule init {"e": [withMessage(email(), "enter an email")]};
    testing.assertEqual(firstMsg(check({"e": "nope"}, $r2), "e"), "enter an email");
}

# --- messages / byField renderers ---

func testMessagesAndByField() {
    def rules as map of string to list of Rule init {"x": [required(), minLen(3)]};
    def errs as list of Failure init check({"x": ""}, $rules);
    def m as list of string init messages($errs);
    testing.assertEqual($m[0], "x: is required");

    # byField groups; a field with two failures collects both
    def two as map of string to list of Rule init {"p": [minLen(5), pattern("^[0-9]+$")]};
    def be as list of Failure init check({"p": "ab"}, $two);
    def bf as map of string to list of string init byField($be);
    testing.assertEqual(len($bf["p"]), 2);
}

# --- date rule (format + calendar validity via time.parse) ---

func testDatetimeRule() {
    def rules as map of string to list of Rule init {"dob": [datetime("%d.%m.%Y")]};
    testing.assertTrue(ok({"dob": "25.12.2026"}, $rules));   # valid
    testing.assertFalse(ok({"dob": "2026-12-25"}, $rules));  # wrong format
    testing.assertFalse(ok({"dob": "25.13.2026"}, $rules));  # month 13 (calendar-invalid)
    testing.assertEqual(firstMsg(check({"dob": "nope"}, $rules), "dob"), "must be a valid date/time");
    # ISO + a friendly override
    testing.assertTrue(ok({"d": "2026-01-31"}, {"d": [datetime("%Y-%m-%d")]}));
}

# --- a custom predicate that throws is caught, not leaked ---

func testCustomPredicateThrowIsCaught() {
    # `even` throws convert.toInt on a non-numeric value; check must treat that as
    # invalid (return a failure) rather than letting the exception escape.
    def rules as map of string to list of Rule init {"n": [custom(even, "must be even")]};
    def errs as list of Failure init check({"n": "abc"}, $rules);
    testing.assertEqual(len($errs), 1);
    testing.assertEqual($errs[0].message, "must be even");
}

# --- Failure.param + localize (i18n / custom messages) ---

func testParamAndLocalize() {
    def rules as map of string to list of Rule init {
        "a": [min(5.0)], "b": [minLen(3)], "c": [oneOf(["x", "y"])],
        "d": [datetime("%Y")], "e": [email()]
    };
    def data as map of string to string init {"a": "1", "b": "x", "c": "q", "d": "nope", "e": "bad"};
    def errs as list of Failure init check($data, $rules);
    # param carries the rule's argument; parameterless rules (email) -> ""
    testing.assertEqual(paramOf($errs, "a"), "5.0");
    testing.assertEqual(paramOf($errs, "b"), "3");
    testing.assertEqual(paramOf($errs, "c"), "x, y");
    testing.assertEqual(paramOf($errs, "d"), "%Y");
    testing.assertEqual(paramOf($errs, "e"), "");
    # localize: %param% / %field% substitution; unlisted rules keep the default.
    # The markers are brace-free, so they never collide with string interpolation.
    def tmpl as map of string to string init {"min": "at least %param%!", "oneOf": "%field%: pick %param%"};
    def loc as list of Failure init localize($errs, $tmpl);
    testing.assertEqual(firstMsg($loc, "a"), "at least 5.0!");
    testing.assertEqual(firstMsg($loc, "c"), "c: pick x, y");
    testing.assertEqual(firstMsg($loc, "e"), "must be a valid email address");
}

# --- noneOf (blacklist) ---

func testNoneOf() {
    def rules as map of string to list of Rule init {"u": [noneOf(["admin", "root", "Administrator"])]};
    testing.assertTrue(ok({"u": "bob"}, $rules));            # not blocked
    testing.assertFalse(ok({"u": "root"}, $rules));          # blocked
    testing.assertTrue(ok({"u": "Root"}, $rules));           # case-sensitive: "Root" != "root"
    testing.assertEqual(firstMsg(check({"u": "admin"}, $rules), "u"),
        "must not be one of: admin, root, Administrator");
    testing.assertEqual(paramOf(check({"u": "admin"}, $rules), "u"), "admin, root, Administrator");
    testing.assertTrue(ok({}, $rules));                      # absent -> only required fires
}

# --- password (passthrough to the password module's policy engine) ---

func testPasswordRule() {
    def policy as pw.Schema init pw.withMinimums(pw.withLength(pw.schema(), 8, 64), 1, 1, 1, 0);
    def rules as map of string to list of Rule init {"pw": [required(), password($policy)]};
    testing.assertTrue(ok({"pw": "Str0ngPass"}, $rules));    # 10 chars, upper+lower+digit
    # too short + missing classes -> one failure carrying the policy's joined reasons
    def errs as list of Failure init check({"pw": "weak"}, $rules);
    testing.assertEqual(len($errs), 1);
    testing.assertEqual($errs[0].rule, "password");
    testing.assertTrue(strings.contains($errs[0].message, "too short"));
    # withMessage collapses the policy reasons to a single hint
    def r2 as map of string to list of Rule init {"pw": [withMessage(password($policy), "8+ chars, mixed case + a digit")]};
    testing.assertEqual(firstMsg(check({"pw": "weak"}, $r2), "pw"), "8+ chars, mixed case + a digit");
    # absent/blank is skipped (pair with required to demand a value)
    testing.assertTrue(ok({}, {"pw": [password($policy)]}));
    testing.assertTrue(ok({"pw": ""}, {"pw": [password($policy)]}));
}

# --- localize escaping (%%) + single-pass (no re-scan) ---

func testLocalizeEscaping() {
    def rules as map of string to list of Rule init {"x": [min(5.0)]};
    def errs as list of Failure init check({"x": "1"}, $rules);
    # %% -> literal %, in a cooked string (the string engine never touches %)
    testing.assertEqual(firstMsg(localize($errs, {"min": "at least %param% (100%% sure)"}), "x"),
        "at least 5.0 (100% sure)");
    # identical result from a raw-string template - %% is the module's escape, not the engine's
    testing.assertEqual(firstMsg(localize($errs, {"min": 'at least %param% (100%% sure)'}), "x"),
        "at least 5.0 (100% sure)");
    # single pass: a param value that itself contains "%field%" is NOT re-scanned
    def rr as map of string to list of Rule init {"fld": [oneOf(["%field%"])]};
    def er as list of Failure init check({"fld": "nope"}, $rr);
    testing.assertEqual(firstMsg(localize($er, {"oneOf": "pick %param%"}), "fld"), "pick %field%");
    # applyTemplate / matchAt directly
    testing.assertEqual(applyTemplate("%field%: %param%", "P", "F"), "F: P");
    testing.assertEqual(applyTemplate("100%% %bogus%", "P", "F"), "100% %bogus%");
    testing.assertTrue(matchAt(strings.chars("%param%"), 0, "%param%"));
    testing.assertFalse(matchAt(strings.chars("%para"), 0, "%param%"));
}

# --- private helpers directly ---

func testHelpers() {
    testing.assertTrue(alwaysTrue("anything"));
    testing.assertTrue(parsesInt("42"));
    testing.assertFalse(parsesInt("4.2"));
    testing.assertTrue(parsesFloat("4.2"));
    testing.assertFalse(parsesFloat("x"));
    testing.assertTrue(isEmail("a@b.co"));
    testing.assertFalse(isEmail("a@b"));
    testing.assertTrue(isUrl("http://h/p"));
    testing.assertFalse(isUrl("relative/path"));
    testing.assertEqual(msgOr(required(), "def"), "def");
    testing.assertEqual(msgOr(withMessage(required(), "over"), "def"), "over");
}
