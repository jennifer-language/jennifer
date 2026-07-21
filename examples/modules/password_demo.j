#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The password module (modules/password.j): generate, validate, and score
 * passwords against a policy schema. Runs on both binaries (pure `.j`).
 * NOTE: randomness is `math`'s non-crypto RNG - fine for a demo, not for
 * high-value secrets (see the module docs).
 * Run: jennifer run examples/modules/password_demo.j
 * @module password_demo
 */
use io;
use convert;
import "../../modules/password.j" as password;

# 1. Generate from the strong default policy (16 chars, all four classes).
def policy as password.Schema init password.schema();
io.printf("default policy passwords:\n");
def i as int init 0;
while ($i < 3) {
    def pw as string init password.generate($policy);
    def s as password.Strength init password.complexity($pw);
    io.printf("  %s   %d bits (%s)\n", $pw, convert.toInt($s.entropy), $s.label);
    $i = $i + 1;
}

# 2. A custom policy: 20-24 chars, no symbols, exclude ambiguous glyphs.
def readable as password.Schema init password.withoutAmbiguous(
    password.withClasses(password.withLength($policy, 20, 24), true, true, true, false));
io.printf("\nreadable (no symbols, no ambiguous glyphs):\n  %s\n", password.generate($readable));

# 3. Validate user-supplied passwords against the default policy.
io.printf("\nvalidation against the default policy:\n");
def samples as list of string init ["abc", "password", "Sup3rSecret!", "Xq7#mZ2!vBn9"];
for (def candidate in $samples) {
    def r as password.Report init password.validate($policy, $candidate);
    if ($r.valid) {
        io.printf("  OK    %s\n", $candidate);
    } else {
        io.printf("  FAIL  %s  ->  %s\n", $candidate, $r.reasons[0]);
    }
}

# 4. Complexity of arbitrary passwords.
io.printf("\ncomplexity scoring:\n");
for (def candidate in $samples) {
    def s as password.Strength init password.complexity($candidate);
    io.printf("  %s|pad=14 classes=%d  pool=%d  %d bits  %s\n",
        $candidate, $s.classes, $s.poolSize, convert.toInt($s.entropy), $s.label);
}
