# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Password generation, validation, and complexity scoring against a policy
 * schema. A `Schema` names the allowed character classes (lowercase, uppercase,
 * digits, symbols), a length range, and per-class minimum counts;
 * `generate(schema)` produces a conforming password, `validate(schema, pw)`
 * reports whether a password meets the policy (and why not), and
 * `complexity(pw)` estimates a password's strength in bits of entropy.
 *
 * Randomness comes from the `crypto` library's crypto-grade source (character
 * choice and the final shuffle), so a generated password is unpredictable and
 * safe to mint as a real credential. Pure `.j` over `crypto` / `strings` /
 * `convert`; runs on both binaries.
 * @module password
 * @example
 * import "password.j" as password;
 * use io;
 * def policy as password.Schema init password.schema();
 * def pw as string init password.generate($policy);            # 16 chars, all classes
 * io.printf("%s  valid=%t  %s\n", $pw, password.validate($policy, $pw).valid,
 *           password.complexity($pw).label);
 */
use crypto;
use strings;
use convert;

# Character-class pools. `SYMBOLS` is the default symbol set; a schema may
# override it. `AMBIGUOUS` lists easily-confused glyphs dropped when a schema
# asks to exclude them.
def const LOWER as string init "abcdefghijklmnopqrstuvwxyz";
def const UPPER as string init "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
def const DIGITS as string init "0123456789";
def const SYMBOLS as string init "!@#$%^&*()-_=+[]{}|;:,.<>?/~";
def const AMBIGUOUS as string init "0Oo1lI|";

/**
 * A password policy: allowed classes, a length range, per-class minimum
 * counts, the symbol set, and whether to drop ambiguous glyphs.
 * @field minLength {int} shortest allowed length (also the generated length floor)
 * @field maxLength {int} longest allowed length (generation picks in the range)
 * @field lower {bool} include lowercase in the generation alphabet
 * @field upper {bool} include uppercase
 * @field digits {bool} include digits
 * @field symbols {bool} include symbols (from `symbolSet`)
 * @field symbolSet {string} the symbol characters to draw from
 * @field minLower {int} minimum lowercase (generation guarantees, validation requires)
 * @field minUpper {int} minimum uppercase
 * @field minDigits {int} minimum digits
 * @field minSymbols {int} minimum symbols
 * @field excludeAmbiguous {bool} drop ambiguous glyphs (0 O o 1 l I |) from generation
 */
export def struct Schema {
    minLength as int,
    maxLength as int,
    lower as bool,
    upper as bool,
    digits as bool,
    symbols as bool,
    symbolSet as string,
    minLower as int,
    minUpper as int,
    minDigits as int,
    minSymbols as int,
    excludeAmbiguous as bool
};

/**
 * A validation result: whether the password conforms, and a list of the
 * failed rules (empty when valid).
 * @field valid {bool} true when every rule passed
 * @field reasons {list of string} human-readable failed rules
 */
export def struct Report {
    valid as bool,
    reasons as list of string
};

/**
 * An estimated-strength summary of a password.
 * @field length {int} character count
 * @field classes {int} how many of the four classes are present (0-4)
 * @field poolSize {int} the estimated alphabet size (guessing space per char)
 * @field entropy {float} estimated bits of entropy (length * log2(poolSize))
 * @field label {string} a qualitative band: very weak / weak / reasonable / strong / very strong
 */
export def struct Strength {
    length as int,
    classes as int,
    poolSize as int,
    entropy as float,
    label as string
};

func fail(msg as string) {
    throw Error{ kind: "password", message: $msg, file: "", line: 0, col: 0 };
}

# effMin is a class's effective minimum: the requested minimum when the class
# is enabled, else 0 (a disabled class contributes nothing - the enable bool is
# authoritative over a leftover minimum).
func effMin(enabled as bool, m as int) {
    if ($enabled) {
        return $m;
    }
    return 0;
}

# --- schema construction (exported) -----------------------------------------

/**
 * A strong default policy: exactly 16 characters, all four classes, at least
 * one of each, the default symbol set, ambiguous glyphs allowed.
 * @return {Schema} the default policy
 */
export func schema() {
    return Schema{
        minLength: 16, maxLength: 16,
        lower: true, upper: true, digits: true, symbols: true,
        symbolSet: SYMBOLS,
        minLower: 1, minUpper: 1, minDigits: 1, minSymbols: 1,
        excludeAmbiguous: false
    };
}

/**
 * Copy a schema with a new length range (set `lo == hi` for a fixed length).
 * @param s {Schema} the base schema
 * @param lo {int} minimum length
 * @param hi {int} maximum length
 * @return {Schema} a fresh schema
 */
export func withLength(s as Schema, lo as int, hi as int) {
    def out as Schema init $s;
    $out.minLength = $lo;
    $out.maxLength = $hi;
    return $out;
}

/**
 * Copy a schema with new class enables (which classes appear in generation).
 * @param s {Schema} the base schema
 * @param lo {bool} lowercase
 * @param up {bool} uppercase
 * @param dig {bool} digits
 * @param sym {bool} symbols
 * @return {Schema} a fresh schema
 */
export func withClasses(s as Schema, lo as bool, up as bool, dig as bool, sym as bool) {
    def out as Schema init $s;
    $out.lower = $lo;
    $out.upper = $up;
    $out.digits = $dig;
    $out.symbols = $sym;
    return $out;
}

/**
 * Copy a schema with new per-class minimum counts.
 * @param s {Schema} the base schema
 * @param lo {int} minimum lowercase
 * @param up {int} minimum uppercase
 * @param dig {int} minimum digits
 * @param sym {int} minimum symbols
 * @return {Schema} a fresh schema
 */
export func withMinimums(s as Schema, lo as int, up as int, dig as int, sym as int) {
    def out as Schema init $s;
    $out.minLower = $lo;
    $out.minUpper = $up;
    $out.minDigits = $dig;
    $out.minSymbols = $sym;
    return $out;
}

/**
 * Copy a schema with a replacement symbol set.
 * @param s {Schema} the base schema
 * @param setChars {string} the symbol characters to draw from
 * @return {Schema} a fresh schema
 */
export func withSymbolSet(s as Schema, setChars as string) {
    def out as Schema init $s;
    $out.symbolSet = $setChars;
    return $out;
}

/**
 * Copy a schema that excludes ambiguous glyphs (0 O o 1 l I |) from generation.
 * @param s {Schema} the base schema
 * @return {Schema} a fresh schema
 */
export func withoutAmbiguous(s as Schema) {
    def out as Schema init $s;
    $out.excludeAmbiguous = true;
    return $out;
}

# --- character pools (private) ----------------------------------------------

# filterOut returns `s` with every character that appears in `remove` deleted.
func filterOut(s as string, remove as string) {
    def out as string init "";
    def cs as list of string init strings.chars($s);
    for (def ch in $cs) {
        if (not strings.contains($remove, $ch)) {
            $out = $out + $ch;
        }
    }
    return $out;
}

# classPool returns the (ambiguity-filtered) character pool for one class.
func classPool(s as Schema, which as string) {
    def base as string init "";
    if ($which == "lower") { $base = LOWER; }
    if ($which == "upper") { $base = UPPER; }
    if ($which == "digits") { $base = DIGITS; }
    if ($which == "symbols") { $base = $s.symbolSet; }
    if ($s.excludeAmbiguous) {
        $base = filterOut($base, AMBIGUOUS);
    }
    return $base;
}

# alphabet returns the union of the enabled classes' pools.
func alphabet(s as Schema) {
    def a as string init "";
    if ($s.lower) { $a = $a + classPool($s, "lower"); }
    if ($s.upper) { $a = $a + classPool($s, "upper"); }
    if ($s.digits) { $a = $a + classPool($s, "digits"); }
    if ($s.symbols) { $a = $a + classPool($s, "symbols"); }
    return $a;
}

# pushRandom appends `n` characters drawn uniformly at random from `pool`.
func pushRandom(acc as list of string, pool as string, n as int) {
    if (len($pool) == 0) {
        return $acc;
    }
    def i as int init 0;
    while ($i < $n) {
        def idx as int init crypto.randInt(0, len($pool) - 1);
        $acc[] = strings.substring($pool, $idx, $idx + 1);
        $i = $i + 1;
    }
    return $acc;
}

# shuffleChars returns a crypto-grade Fisher-Yates shuffle of `xs`. Used
# instead of `lists.shuffle` (which draws from math's non-crypto RNG) so the
# arrangement of a generated password is as unpredictable as its characters.
func shuffleChars(xs as list of string) {
    def i as int init len($xs) - 1;
    while ($i > 0) {
        def j as int init crypto.randInt(0, $i);
        def tmp as string init $xs[$i];
        $xs[$i] = $xs[$j];
        $xs[$j] = $tmp;
        $i = $i - 1;
    }
    return $xs;
}

# countIn counts how many characters of `pw` appear in `pool`.
func countIn(pool as string, pw as string) {
    def n as int init 0;
    def cs as list of string init strings.chars($pw);
    for (def ch in $cs) {
        if (strings.contains($pool, $ch)) {
            $n = $n + 1;
        }
    }
    return $n;
}

# --- generation (exported) --------------------------------------------------

/**
 * Generate a password conforming to the schema: a length picked in
 * `[minLength, maxLength]`, at least the per-class minimums, the remainder
 * drawn from the enabled alphabet, then shuffled.
 * @param s {Schema} the policy
 * @return {string} a fresh password
 * @throws {Error} kind "password" if the schema is infeasible (no classes, an
 *   empty required pool, or minimums that exceed the length)
 */
export func generate(s as Schema) {
    if ($s.minLength > $s.maxLength) {
        fail("minLength exceeds maxLength");
    }
    if (len(alphabet($s)) == 0) {
        fail("schema enables no character classes");
    }
    def minLo as int init effMin($s.lower, $s.minLower);
    def minUp as int init effMin($s.upper, $s.minUpper);
    def minDig as int init effMin($s.digits, $s.minDigits);
    def minSym as int init effMin($s.symbols, $s.minSymbols);
    if ($minSym > 0 and len(classPool($s, "symbols")) == 0) {
        fail("minimum symbols required but the symbol pool is empty");
    }
    def required as int init $minLo + $minUp + $minDig + $minSym;
    # Reject an infeasible schema up front (deterministically), then draw the
    # target length from the feasible window [max(minLength, required),
    # maxLength]. Drawing from minLength and throwing when the random target
    # happened to fall below `required` made the same schema succeed or throw
    # depending on the draw.
    if ($required > $s.maxLength) {
        fail("class minimums (" + convert.toString($required) + ") exceed maxLength (" + convert.toString($s.maxLength) + ")");
    }
    def minTarget as int init $s.minLength;
    if ($required > $minTarget) {
        $minTarget = $required;
    }
    def target as int init $minTarget;
    if ($s.maxLength > $minTarget) {
        $target = crypto.randInt($minTarget, $s.maxLength);
    }
    def chars as list of string;
    $chars = pushRandom($chars, classPool($s, "lower"), $minLo);
    $chars = pushRandom($chars, classPool($s, "upper"), $minUp);
    $chars = pushRandom($chars, classPool($s, "digits"), $minDig);
    $chars = pushRandom($chars, classPool($s, "symbols"), $minSym);
    $chars = pushRandom($chars, alphabet($s), $target - $required);
    $chars = shuffleChars($chars);
    return strings.join($chars, "");
}

# --- validation (exported) --------------------------------------------------

/**
 * Validate a password against a schema. Checks the length bounds and each
 * per-class minimum; it does not reject characters outside the alphabet (a
 * policy sets minimums, not a whitelist).
 * @param s {Schema} the policy
 * @param pw {string} the password to check
 * @return {Report} `valid` plus the list of failed-rule `reasons`
 */
export func validate(s as Schema, pw as string) {
    def reasons as list of string;
    def n as int init len($pw);
    if ($n < $s.minLength) {
        $reasons[] = "too short (minimum " + convert.toString($s.minLength) + ")";
    }
    if ($n > $s.maxLength) {
        $reasons[] = "too long (maximum " + convert.toString($s.maxLength) + ")";
    }
    def lo as int init countIn(LOWER, $pw);
    def up as int init countIn(UPPER, $pw);
    def dig as int init countIn(DIGITS, $pw);
    def sym as int init $n - $lo - $up - $dig;
    if ($lo < effMin($s.lower, $s.minLower)) {
        $reasons[] = "needs at least " + convert.toString($s.minLower) + " lowercase";
    }
    if ($up < effMin($s.upper, $s.minUpper)) {
        $reasons[] = "needs at least " + convert.toString($s.minUpper) + " uppercase";
    }
    if ($dig < effMin($s.digits, $s.minDigits)) {
        $reasons[] = "needs at least " + convert.toString($s.minDigits) + " digits";
    }
    if ($sym < effMin($s.symbols, $s.minSymbols)) {
        $reasons[] = "needs at least " + convert.toString($s.minSymbols) + " symbols";
    }
    return Report{ valid: (len($reasons) == 0), reasons: $reasons };
}

# --- complexity (exported) --------------------------------------------------

# binaryLog computes log base 2 of x (x > 0) by the bit-by-bit method: the
# integer part by halving into [1, 2), then 30 fractional bits. Exact for
# powers of two. (math has no log, so it lives here.)
func binaryLog(x as float) {
    def n as int init 0;
    def v as float init $x;
    while ($v >= 2.0) {
        $v = $v / 2.0;
        $n = $n + 1;
    }
    while ($v < 1.0) {
        $v = $v * 2.0;
        $n = $n - 1;
    }
    def result as float init convert.toFloat($n);
    def weight as float init 0.5;
    def i as int init 0;
    while ($i < 30) {
        $v = $v * $v;
        if ($v >= 2.0) {
            $result = $result + $weight;
            $v = $v / 2.0;
        }
        $weight = $weight / 2.0;
        $i = $i + 1;
    }
    return $result;
}

# labelFor maps entropy bits to a qualitative strength band.
func labelFor(bits as float) {
    if ($bits < 28.0) { return "very weak"; }
    if ($bits < 36.0) { return "weak"; }
    if ($bits < 60.0) { return "reasonable"; }
    if ($bits < 128.0) { return "strong"; }
    return "very strong";
}

/**
 * Estimate a password's strength. The alphabet size is the sum of the class
 * sizes present (lower/upper 26 each, digits 10, symbols the default-set
 * size), and entropy is `length * log2(alphabet)` bits.
 * @param pw {string} the password
 * @return {Strength} length, class count, pool size, entropy bits, and a label
 */
export func complexity(pw as string) {
    def n as int init len($pw);
    def lo as int init countIn(LOWER, $pw);
    def up as int init countIn(UPPER, $pw);
    def dig as int init countIn(DIGITS, $pw);
    def sym as int init $n - $lo - $up - $dig;
    def classes as int init 0;
    def pool as int init 0;
    if ($lo > 0) { $classes = $classes + 1; $pool = $pool + 26; }
    if ($up > 0) { $classes = $classes + 1; $pool = $pool + 26; }
    if ($dig > 0) { $classes = $classes + 1; $pool = $pool + 10; }
    if ($sym > 0) { $classes = $classes + 1; $pool = $pool + len(SYMBOLS); }
    def bits as float init 0.0;
    if ($pool >= 2 and $n > 0) {
        $bits = convert.toFloat($n) * binaryLog(convert.toFloat($pool));
    }
    return Strength{ length: $n, classes: $classes, poolSize: $pool, entropy: $bits, label: labelFor($bits) };
}
