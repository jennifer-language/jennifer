#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The validate module: declarative validation of a string map (a form body, a
 * query, a config) against a rule set, returning a structured failure list.
 * Rules compose per field; check returns every failure. This demo validates a
 * good, a bad, and a reserved-name "signup form" (the last shows the username
 * blacklist and the password policy firing), then re-renders the bad form's
 * failures in German via localize (a rule-id -> template map). Deterministic.
 * @module validate_demo
 */
use io;
import "../../modules/validate.j" as validate;
import "../../modules/password.j" as password;

# A custom rule: usernames must be lower-case-ish (letters, digits, underscore).
func isHandle(v as string) {
    return validate.ok({"h": $v}, {"h": [validate.pattern("^[a-z0-9_]+$")]});
}

# The password policy validate.password enforces: 8-64 chars, at least one
# lower / upper / digit (symbols optional). Built with the password module's own
# schema builders, then handed to validate.password($policy) as a rule.
def policy as password.Schema init
    password.withMinimums(password.withLength(password.schema(), 8, 64), 1, 1, 1, 0);

# The rule set: each field maps to the rules it must satisfy. `noneOf` blacklists
# reserved usernames; `password` delegates to the policy above.
def rules as map of string to list of validate.Rule init {
    "username": [validate.required(), validate.minLen(3), validate.maxLen(20), validate.noneOf(["admin", "root", "administrator"]), validate.custom(isHandle, "must be lower-case letters, digits, or _")],
    "email": [validate.required(), validate.email()],
    "age": [validate.isInt(), validate.min(13.0), validate.max(120.0)],
    "role": [validate.oneOf(["reader", "author", "admin"])],
    "website": [validate.url()],
    "password": [validate.required(), validate.password($policy)]
};

func report(label as string, form as map of string to string) {
    def errs as list of validate.Failure init validate.check($form, $rules);
    if (validate.ok($form, $rules)) {
        io.printf("%s: valid\n", $label);
        return;
    }
    io.printf("%s: %d problem(s)\n", $label, len($errs));
    for (def m in validate.messages($errs)) {
        io.printf("  - %s\n", $m);
    }
}

# A good signup (website omitted - optional, so it passes).
def good as map of string to string init {"username": "ada_lovelace", "email": "ada@example.com", "age": "36", "role": "author", "password": "Str0ngPass"};
report("good", $good);

# A bad signup: short username with a capital, malformed email, under-age, bad
# role, non-URL website, and a too-short password missing an uppercase and a digit.
def bad as map of string to string init {"username": "Ad", "email": "not-an-email", "age": "9", "role": "root", "website": "example.com", "password": "weak"};
report("bad", $bad);

# A reserved-name attempt: "admin" passes minLen and the handle pattern, but the
# noneOf blacklist rejects it - the one rule that fires here.
def reserved as map of string to string init {"username": "admin", "email": "a@b.com", "age": "30", "role": "reader", "password": "Str0ngPass"};
report("reserved", $reserved);

# --- localisation: the same failures rendered in German via localize ---------
# `templates` maps a rule id to a message; %param% interpolates the rule's
# argument (the minLen / min threshold, the oneOf choices). The custom "handle"
# rule has no German entry, so it keeps its default English message - a partial
# catalog only overrides the rules it names. `password` localizes as one message
# (its per-reason detail stays in the default English text), so give it a generic
# German line. For full i18n, build this map from `intl.tr` per rule id.
#
# The markers are %name%-style (brace-free), so they never collide with the
# language's brace string interpolation and read the same in cooked or raw strings.
def de as map of string to string init {
    "required": "ist erforderlich",
    "minLen": "muss mindestens %param% Zeichen lang sein",
    "email": "muss eine gültige E-Mail-Adresse sein",
    "min": "muss mindestens %param% sein",
    "oneOf": "muss eines von %param% sein",
    "noneOf": "darf nicht %param% sein",
    "url": "muss eine gültige URL sein",
    "password": "erfüllt die Passwortrichtlinie nicht"
};
def badErrs as list of validate.Failure init validate.check($bad, $rules);
io.printf("\nbad (Deutsch):\n");
for (def m in validate.messages(validate.localize($badErrs, $de))) {
    io.printf("  - %s\n", $m);
}
