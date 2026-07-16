# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Strict Semantic Versioning 2.0.0 (https://semver.org): parse, compare,
 * increment, and range-match version numbers - the full surface a package
 * registry or dependency resolver needs. A pure-Jennifer reference module (no
 * Go, no system library): parsing uses the canonical SemVer regex (via the
 * `regex` library); precedence comparison, sorting, and range matching are
 * hand-written Jennifer.
 *
 * Ranges follow the npm / Composer grammar: caret `^1.2.0`, tilde `~1.2`,
 * comparators `>=1.0.0 <2.0.0` (space or comma = AND), OR sets `^1 || ^2`,
 * hyphen ranges `1.2.3 - 2.3.4`, x-ranges `1.x` / `1.2.*`, and `*` for any.
 * A prerelease version satisfies a range only when a comparator in the same
 * clause pins a prerelease at the same `major.minor.patch` (the npm rule).
 * @module semver
 * @example
 * def v as semver.Version init semver.parse("1.2.3-rc.1+build.5");
 * if (semver.lt($v, semver.parse("1.2.3"))) { ... }        # true: rc < release
 * semver.satisfies("1.4.0", "^1.2.0 || >=2.5.0");          # true
 * semver.maxSatisfying(["1.0.0", "1.4.3", "2.0.0"], "^1.2.0");   # "1.4.3"
 */
use strings;
use convert;
use regex;

/**
 * A parsed SemVer version: numeric core plus optional prerelease / build tags.
 * @field major {int} the major version
 * @field minor {int} the minor version
 * @field patch {int} the patch version
 * @field prerelease {string} the prerelease tag, or "" if none
 * @field build {string} the build metadata, or "" if none
 */
export def struct Version {
    major as int,
    minor as int,
    patch as int,
    prerelease as string,
    build as string
};

# The official SemVer 2.0.0 grammar as an anchored RE2 pattern with named
# groups: three numeric core parts (no leading zeros), an optional
# dot-separated prerelease (numeric ids have no leading zero; alphanumeric ids
# are free), and an optional dot-separated build.
def const SEMVER as string init "^(?P<major>0|[1-9][0-9]*)\\.(?P<minor>0|[1-9][0-9]*)\\.(?P<patch>0|[1-9][0-9]*)(?:-(?P<prerelease>(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(?:\\.(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\\+(?P<build>[0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?$";  # lint-disable: L203

# --- small helpers (private) ---------------------------------------

func sign(n as int) {
    if ($n < 0) {
        return -1;
    }
    if ($n > 0) {
        return 1;
    }
    return 0;
}

# charCode returns the ASCII byte value of a single-character string.
func charCode(c as string) {
    def b as bytes init convert.bytesFromString($c, "utf-8");
    return $b[0];
}

# isNum reports whether s is a run of decimal digits.
func isNum(s as string) {
    return regex.matches("^[0-9]+$", $s);
}

func invalid(s as string) {
    throw Error{kind: "value", message: "invalid semver: " + $s, file: "", line: 0, col: 0};
}

# --- parse / validate / format (exported) --------------------------

/**
 * Parse a version string into a Version.
 * @param s {string} the version text (e.g. "1.2.3-rc.1+build.5")
 * @return {Version} the parsed version
 * @throws {Error} when s is not a valid SemVer 2.0.0 string
 */
export func parse(s as string) {
    def m as regex.Match init regex.find(SEMVER, $s);
    if ($m.start < 0) {
        invalid($s);
    }
    # Every named group is present in groupsNamed even when it did not
    # participate (an absent prerelease / build reads as "").
    return Version{
        major: convert.toInt($m.groupsNamed["major"]),
        minor: convert.toInt($m.groupsNamed["minor"]),
        patch: convert.toInt($m.groupsNamed["patch"]),
        prerelease: $m.groupsNamed["prerelease"],
        build: $m.groupsNamed["build"]
    };
}

/**
 * Report whether a string is a valid SemVer 2.0.0 version.
 * @param s {string} the candidate version text
 * @return {bool} true when s parses as a valid version
 */
export func isValid(s as string) {
    return regex.matches(SEMVER, $s);
}

/**
 * Render a Version back to its canonical string form.
 * @param v {Version} the version to format
 * @return {string} the "major.minor.patch[-prerelease][+build]" text
 */
export func toString(v as Version) {
    def out as string init convert.toString($v.major);
    $out = $out + "." + convert.toString($v.minor);
    $out = $out + "." + convert.toString($v.patch);
    if (len($v.prerelease) > 0) {
        $out = $out + "-" + $v.prerelease;
    }
    if (len($v.build) > 0) {
        $out = $out + "+" + $v.build;
    }
    return $out;
}

# --- comparison (exported) -----------------------------------------

# compareStr does an ASCII-lexical comparison of two identifier strings.
func compareStr(a as string, b as string) {
    def ca as list of string init strings.chars($a);
    def cb as list of string init strings.chars($b);
    def n as int init len($ca);
    if (len($cb) < $n) {
        $n = len($cb);
    }
    def i as int init 0;
    while ($i < $n) {
        def d as int init charCode($ca[$i]) - charCode($cb[$i]);
        if (not ($d == 0)) {
            return sign($d);
        }
        $i = $i + 1;
    }
    return sign(len($ca) - len($cb));
}

# compareIdent compares one prerelease identifier: numeric ids rank below
# alphanumeric ids; numeric ids compare numerically, alphanumeric lexically.
func compareIdent(a as string, b as string) {
    def an as bool init isNum($a);
    def bn as bool init isNum($b);
    if ($an and $bn) {
        return sign(convert.toInt($a) - convert.toInt($b));
    }
    if ($an and not $bn) {
        return -1;
    }
    if (not $an and $bn) {
        return 1;
    }
    return compareStr($a, $b);
}

# comparePre compares two non-empty prerelease strings field by field; a
# longer run of otherwise-equal fields ranks higher.
func comparePre(a as string, b as string) {
    def ai as list of string init strings.split($a, ".");
    def bi as list of string init strings.split($b, ".");
    def n as int init len($ai);
    if (len($bi) < $n) {
        $n = len($bi);
    }
    def i as int init 0;
    while ($i < $n) {
        def c as int init compareIdent($ai[$i], $bi[$i]);
        if (not ($c == 0)) {
            return $c;
        }
        $i = $i + 1;
    }
    return sign(len($ai) - len($bi));
}

/**
 * Compare two versions by SemVer precedence: numeric core, then a prerelease
 * ranks below its release, then prerelease fields. Build metadata is ignored.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {int} -1 if a < b, 0 if equal, 1 if a > b
 */
export func compare(a as Version, b as Version) {
    if (not ($a.major == $b.major)) {
        return sign($a.major - $b.major);
    }
    if (not ($a.minor == $b.minor)) {
        return sign($a.minor - $b.minor);
    }
    if (not ($a.patch == $b.patch)) {
        return sign($a.patch - $b.patch);
    }
    def ap as bool init len($a.prerelease) > 0;
    def bp as bool init len($b.prerelease) > 0;
    if ($ap and not $bp) {
        return -1;
    }
    if (not $ap and $bp) {
        return 1;
    }
    if (not $ap and not $bp) {
        return 0;
    }
    return comparePre($a.prerelease, $b.prerelease);
}

/**
 * Report whether a orders before b.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a < b by SemVer precedence
 */
export func lt(a as Version, b as Version) {
    return compare($a, $b) < 0;
}
/**
 * Report whether a and b have equal precedence (build metadata ignored).
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a and b compare equal
 */
export func eq(a as Version, b as Version) {
    return compare($a, $b) == 0;
}
/**
 * Report whether a orders after b.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a > b by SemVer precedence
 */
export func gt(a as Version, b as Version) {
    return compare($a, $b) > 0;
}
/**
 * Report whether a orders at or before b.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a <= b
 */
export func lte(a as Version, b as Version) {
    return compare($a, $b) <= 0;
}
/**
 * Report whether a orders at or after b.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a >= b
 */
export func gte(a as Version, b as Version) {
    return compare($a, $b) >= 0;
}
/**
 * Report whether a and b differ in precedence.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {bool} true when a and b are not equal
 */
export func neq(a as Version, b as Version) {
    return not (compare($a, $b) == 0);
}

# --- classification + increment (exported) -------------------------

/**
 * Report whether the version carries a prerelease tag.
 * @param v {Version} the version to classify
 * @return {bool} true when a prerelease tag is present
 */
export func isPrerelease(v as Version) {
    return len($v.prerelease) > 0;
}

/**
 * Report whether the version is stable: a released (major >= 1) version with no
 * prerelease tag. A 0.y.z version is unstable by SemVer convention.
 * @param v {Version} the version to classify
 * @return {bool} true when major >= 1 and there is no prerelease tag
 */
export func isStable(v as Version) {
    return $v.major >= 1 and len($v.prerelease) == 0;
}

/**
 * The kind of change from a to b: "major", "minor", "patch", "prerelease" (only
 * the prerelease tag differs), or "" when the two are equal. The highest-order
 * difference wins.
 * @param a {Version} the left version
 * @param b {Version} the right version
 * @return {string} the release-type difference
 */
export func diff(a as Version, b as Version) {
    if (not ($a.major == $b.major)) {
        return "major";
    }
    if (not ($a.minor == $b.minor)) {
        return "minor";
    }
    if (not ($a.patch == $b.patch)) {
        return "patch";
    }
    if (not ($a.prerelease == $b.prerelease)) {
        return "prerelease";
    }
    return "";
}

/**
 * Bump the major version, resetting minor / patch and clearing the tags.
 * @param v {Version} the starting version
 * @return {Version} a new version with major + 1 and minor = patch = 0
 */
export func incMajor(v as Version) {
    return Version{major: $v.major + 1, minor: 0, patch: 0, prerelease: "", build: ""};
}
/**
 * Bump the minor version, resetting patch and clearing the tags.
 * @param v {Version} the starting version
 * @return {Version} a new version with minor + 1 and patch = 0
 */
export func incMinor(v as Version) {
    return Version{major: $v.major, minor: $v.minor + 1, patch: 0, prerelease: "", build: ""};
}
/**
 * Bump the patch version, clearing the tags.
 * @param v {Version} the starting version
 * @return {Version} a new version with patch + 1
 */
export func incPatch(v as Version) {
    def next as int init $v.patch + 1;
    return Version{major: $v.major, minor: $v.minor, patch: $next, prerelease: "", build: ""};
}

# --- sort (exported) -----------------------------------------------


/**
 * Return a new list ordered ascending by SemVer precedence. lists.sort is
 * scalar-only, so this is a merge sort over compare() - O(n log n), where an
 * insertion sort was O(n^2).
 * @param vs {list of Version} the versions to order
 * @return {list of Version} a new list sorted ascending
 */
export func sort(vs as list of Version) {
    return mergeSort($vs);
}

# mergeSort orders a list of Version ascending by compare(), stable and
# O(n log n).
func mergeSort(vs as list of Version) {
    def n as int init len($vs);
    if ($n <= 1) {
        return $vs;
    }
    def mid as int init $n // 2;
    def left as list of Version init [];
    def right as list of Version init [];
    def i as int init 0;
    while ($i < $mid) {
        $left[] = $vs[$i];
        $i = $i + 1;
    }
    while ($i < $n) {
        $right[] = $vs[$i];
        $i = $i + 1;
    }
    return mergeVersions(mergeSort($left), mergeSort($right));
}

func mergeVersions(a as list of Version, b as list of Version) {
    def out as list of Version init [];
    def i as int init 0;
    def j as int init 0;
    while ($i < len($a) and $j < len($b)) {
        if (compare($a[$i], $b[$j]) <= 0) {
            $out[] = $a[$i];
            $i = $i + 1;
        } else {
            $out[] = $b[$j];
            $j = $j + 1;
        }
    }
    while ($i < len($a)) {
        $out[] = $a[$i];
        $i = $i + 1;
    }
    while ($j < len($b)) {
        $out[] = $b[$j];
        $j = $j + 1;
    }
    return $out;
}

/**
 * Return a new list ordered descending by SemVer precedence (highest first).
 * @param vs {list of Version} the versions to order
 * @return {list of Version} a new list sorted descending
 */
export func rsort(vs as list of Version) {
    def asc as list of Version init sort($vs);
    def out as list of Version init [];
    def i as int init len($asc) - 1;
    while ($i >= 0) {
        $out[] = $asc[$i];
        $i = $i - 1;
    }
    return $out;
}

# --- ranges (private machinery) ------------------------------------

# A partial numeric core plus how many components the text specified (1-3).
# Missing / wildcard components read as 0; count drives the caret / tilde /
# x-range upper-bound rules.
def struct Core {
    major as int,
    minor as int,
    patch as int,
    count as int
};

# rest returns s with its first n characters dropped.
func rest(s as string, n as int) {
    return strings.substring($s, $n, len($s));
}

# relVersion builds a plain release Version (no prerelease / build).
func relVersion(major as int, minor as int, patch as int) {
    return Version{major: $major, minor: $minor, patch: $patch, prerelease: "", build: ""};
}

# isWild reports whether an operand means "any": empty, *, x, X, or "any".
func isWild(s as string) {
    def t as string init strings.trim($s);
    return $t == "" or $t == "*" or $t == "x" or $t == "X" or $t == "any";
}

# parseCore reads the numeric core of a version / range operand, stopping at the
# first non-numeric component (a wildcard or the prerelease / build suffix).
# count is how many numeric components were consumed (0-3).
func parseCore(s as string) {
    def core as Core init Core{major: 0, minor: 0, patch: 0, count: 0};
    def base as string init $s;
    def dash as int init strings.indexOf($base, "-");
    if ($dash >= 0) {
        $base = strings.substring($base, 0, $dash);
    }
    def plus as int init strings.indexOf($base, "+");
    if ($plus >= 0) {
        $base = strings.substring($base, 0, $plus);
    }
    def idx as int init 0;
    for (def part in strings.split($base, ".")) {
        if ($idx >= 3 or not isNum($part)) {
            break;
        }
        def n as int init convert.toInt($part);
        if ($idx == 0) {
            $core.major = $n;
        } elseif ($idx == 1) {
            $core.minor = $n;
        } else {
            $core.patch = $n;
        }
        $core.count = $idx + 1;
        $idx = $idx + 1;
    }
    return $core;
}

func coreLower(core as Core) {
    return relVersion($core.major, $core.minor, $core.patch);
}

# caretUpper is the exclusive upper bound of a caret range: it bumps the
# left-most non-zero component, widening a partial "^0" per npm semantics.
# ^1.2.3 -> <2.0.0, ^0.2.3 -> <0.3.0, ^0.0.3 -> <0.0.4, ^0.0 -> <0.1.0, ^0 -> <1.0.0.
func caretUpper(core as Core) {
    if ($core.major > 0) {
        return relVersion($core.major + 1, 0, 0);
    }
    if ($core.minor > 0) {
        return relVersion(0, $core.minor + 1, 0);
    }
    if ($core.count >= 3) {
        return relVersion(0, 0, $core.patch + 1);
    }
    if ($core.count == 2) {
        return relVersion(0, 1, 0);
    }
    return relVersion(1, 0, 0);
}

# tildeUpper is the exclusive upper bound of a tilde range and of a bare partial
# / x-range: a specified minor allows patch moves, else major moves. ~1.2.3 /
# ~1.2 / 1.2 / 1.2.x -> <1.3.0; ~1 / 1 / 1.x -> <2.0.0.
func tildeUpper(core as Core) {
    if ($core.count >= 2) {
        return relVersion($core.major, $core.minor + 1, 0);
    }
    return relVersion($core.major + 1, 0, 0);
}

func boundedMatch(v as Version, lo as Version, hi as Version) {
    return compare($v, $lo) >= 0 and compare($v, $hi) < 0;
}

# operandOf strips a leading comparator operator (^ ~ >= <= > < =) off a token.
func operandOf(c as string) {
    def s as string init strings.trim($c);
    if (strings.startsWith($s, ">=") or strings.startsWith($s, "<=")) {
        return rest($s, 2);
    }
    if (strings.startsWith($s, "^") or strings.startsWith($s, "~") or
        strings.startsWith($s, ">") or strings.startsWith($s, "<") or strings.startsWith($s, "=")) {
        return rest($s, 1);
    }
    return $s;
}

# cmpOp evaluates a comparator (>= <= > < =) against v, expanding a partial
# operand the npm way (>1.2 means >=1.3.0, <=1.2 means <1.3.0, etc.).
func cmpOp(v as Version, operand as string, op as string) {
    def o as string init strings.trim($operand);
    if (isWild($o)) {
        return true;
    }
    if (isValid($o)) {
        def c as int init compare($v, parse($o));
        if ($op == ">=") {
            return $c >= 0;
        }
        if ($op == "<=") {
            return $c <= 0;
        }
        if ($op == ">") {
            return $c > 0;
        }
        if ($op == "<") {
            return $c < 0;
        }
        return $c == 0;
    }
    def core as Core init parseCore($o);
    if ($core.count == 0) {
        return true;
    }
    def lo as Version init coreLower($core);
    def hi as Version init tildeUpper($core);
    if ($op == ">=") {
        return compare($v, $lo) >= 0;
    }
    if ($op == ">") {
        return compare($v, $hi) >= 0;
    }
    if ($op == "<=") {
        return compare($v, $hi) < 0;
    }
    if ($op == "<") {
        return compare($v, $lo) < 0;
    }
    return boundedMatch($v, $lo, $hi);
}

# eqOrRange evaluates a bare / `=` operand: an exact match for a full version
# (prerelease honoured), else the partial / x-range it denotes.
func eqOrRange(v as Version, operand as string) {
    def o as string init strings.trim($operand);
    if (isWild($o)) {
        return true;
    }
    if (isValid($o)) {
        return compare($v, parse($o)) == 0;
    }
    def core as Core init parseCore($o);
    if ($core.count == 0) {
        return true;
    }
    if ($core.count >= 3) {
        return compare($v, coreLower($core)) == 0;
    }
    return boundedMatch($v, coreLower($core), tildeUpper($core));
}

# rawComparator evaluates one comparator numerically (no prerelease gate).
func rawComparator(v as Version, c as string) {
    def s as string init strings.trim($c);
    if (isWild($s)) {
        return true;
    }
    if (strings.startsWith($s, "^")) {
        def operand as string init rest($s, 1);
        def core as Core init parseCore($operand);
        # A prerelease operand (`^1.2.3-rc.1`) keeps its prerelease as the
        # inclusive lower bound; stripping it to the release (coreLower) would
        # exclude the very prerelease the caret pins.
        if (isValid($operand)) {
            return boundedMatch($v, parse($operand), caretUpper($core));
        }
        return boundedMatch($v, coreLower($core), caretUpper($core));
    }
    if (strings.startsWith($s, "~")) {
        def operand as string init rest($s, 1);
        def core as Core init parseCore($operand);
        if (isValid($operand)) {
            return boundedMatch($v, parse($operand), tildeUpper($core));
        }
        return boundedMatch($v, coreLower($core), tildeUpper($core));
    }
    if (strings.startsWith($s, ">=")) {
        return cmpOp($v, rest($s, 2), ">=");
    }
    if (strings.startsWith($s, "<=")) {
        return cmpOp($v, rest($s, 2), "<=");
    }
    if (strings.startsWith($s, ">")) {
        return cmpOp($v, rest($s, 1), ">");
    }
    if (strings.startsWith($s, "<")) {
        return cmpOp($v, rest($s, 1), "<");
    }
    if (strings.startsWith($s, "=")) {
        return eqOrRange($v, rest($s, 1));
    }
    return eqOrRange($v, $s);
}

# preAtTuple reports whether a comparator pins a prerelease at v's exact
# major.minor.patch - the npm gate that lets a prerelease satisfy a range.
func preAtTuple(comparator as string, v as Version) {
    def o as string init strings.trim(operandOf($comparator));
    if (not isValid($o)) {
        return false;
    }
    def t as Version init parse($o);
    return len($t.prerelease) > 0 and $t.major == $v.major and $t.minor == $v.minor and $t.patch == $v.patch;
}

# isBareOperator reports whether a token is a comparator operator with no
# operand attached.
func isBareOperator(t as string) {
    return $t == ">=" or $t == "<=" or $t == ">" or $t == "<" or $t == "=" or $t == "^" or $t == "~";
}

# tokenize splits a clause into comparators on whitespace or commas, then
# rejoins a bare operator with the operand that follows it: `>= 1.2.3` tokenizes
# to `>=` + `1.2.3` but means `>=1.2.3`, not the wildcard operator plus an exact
# `1.2.3`.
func tokenize(clause as string) {
    def spaced as string init strings.replace($clause, ",", " ");
    def raw as list of string init [];
    for (def tok in strings.split($spaced, " ")) {
        def t as string init strings.trim($tok);
        if (not ($t == "")) {
            $raw[] = $t;
        }
    }
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def t as string init $raw[$i];
        if (isBareOperator($t) and $i + 1 < len($raw)) {
            $out[] = $t + $raw[$i + 1];
            $i = $i + 2;
        } else {
            $out[] = $t;
            $i = $i + 1;
        }
    }
    return $out;
}

# matchesHyphen evaluates a "A - B" range: >=lower(A) and <=B (or <upper(B)
# when B is partial), with the same prerelease gate as a clause.
func matchesHyphen(v as Version, clause as string) {
    def idx as int init strings.indexOf($clause, " - ");
    def aStr as string init strings.trim(strings.substring($clause, 0, $idx));
    def bStr as string init strings.trim(strings.substring($clause, $idx + 3, len($clause)));
    def lo as Version init coreLower(parseCore($aStr));
    if (compare($v, $lo) < 0) {
        return false;
    }
    if (isValid($bStr)) {
        if (compare($v, parse($bStr)) > 0) {
            return false;
        }
    } else {
        if (compare($v, tildeUpper(parseCore($bStr))) >= 0) {
            return false;
        }
    }
    if (isPrerelease($v)) {
        return preAtTuple($aStr, $v) or preAtTuple($bStr, $v);
    }
    return true;
}

# matchesClause evaluates one AND-clause (space / comma separated comparators),
# applying the prerelease gate across the whole clause.
func matchesClause(v as Version, clause as string) {
    def cl as string init strings.trim($clause);
    if (strings.contains($cl, " - ")) {
        return matchesHyphen($v, $cl);
    }
    def comps as list of string init tokenize($cl);
    if (len($comps) == 0) {
        return not isPrerelease($v);
    }
    for (def c in $comps) {
        if (not rawComparator($v, $c)) {
            return false;
        }
    }
    if (isPrerelease($v)) {
        def ok as bool init false;
        for (def c in $comps) {
            if (preAtTuple($c, $v)) {
                $ok = true;
            }
        }
        if (not $ok) {
            return false;
        }
    }
    return true;
}

# --- ranges (exported) ---------------------------------------------

/**
 * Report whether a concrete version satisfies a range. The range grammar is the
 * npm / Composer set: caret `^1.2.0`, tilde `~1.2`, comparators
 * `>=1.0.0 <2.0.0` (space or comma = AND), OR sets `^1 || ^2`, hyphen ranges
 * `1.2.3 - 2.3.4`, x-ranges `1.x` / `1.2.*`, and `*` / `""` / `"any"` for any
 * release. A prerelease version matches only when a comparator in the same
 * clause pins a prerelease at the same major.minor.patch. An invalid version
 * never satisfies anything.
 * @param version {string} the concrete version to test (e.g. "1.4.0")
 * @param range {string} the range expression
 * @return {bool} true when version satisfies range
 */
export func satisfies(version as string, range as string) {
    if (not isValid($version)) {
        return false;
    }
    def v as Version init parse($version);
    def r as string init strings.trim($range);
    if ($r == "" or $r == "*" or $r == "any") {
        return not isPrerelease($v);
    }
    for (def clause in strings.split($r, "||")) {
        if (matchesClause($v, $clause)) {
            return true;
        }
    }
    return false;
}

# partialValid reports whether a partial / operand is structurally well-formed
# (each component a digit run or a wildcard; at most three; tags allowed).
func partialValid(p as string) {
    def t as string init strings.trim($p);
    if (isWild($t)) {
        return true;
    }
    def base as string init $t;
    def dash as int init strings.indexOf($base, "-");
    if ($dash >= 0) {
        $base = strings.substring($base, 0, $dash);
    }
    def plus as int init strings.indexOf($base, "+");
    if ($plus >= 0) {
        $base = strings.substring($base, 0, $plus);
    }
    def parts as list of string init strings.split($base, ".");
    if (len($parts) == 0 or len($parts) > 3) {
        return false;
    }
    for (def part in $parts) {
        if (not (isNum($part) or $part == "*" or $part == "x" or $part == "X")) {
            return false;
        }
    }
    return true;
}

func comparatorValid(c as string) {
    def s as string init strings.trim($c);
    if (isWild($s)) {
        return true;
    }
    return partialValid(operandOf($s));
}

/**
 * Report whether a range expression is well-formed (parseable). Does not
 * evaluate it against any version.
 * @param range {string} the range expression
 * @return {bool} true when the range is valid
 */
export func validRange(range as string) {
    def r as string init strings.trim($range);
    if ($r == "" or $r == "*" or $r == "any") {
        return true;
    }
    for (def clause in strings.split($r, "||")) {
        def cl as string init strings.trim($clause);
        if (not ($cl == "")) {
            if (strings.contains($cl, " - ")) {
                def idx as int init strings.indexOf($cl, " - ");
                def a as string init strings.trim(strings.substring($cl, 0, $idx));
                def b as string init strings.trim(strings.substring($cl, $idx + 3, len($cl)));
                if (not partialValid($a) or not partialValid($b)) {
                    return false;
                }
            } else {
                def comps as list of string init tokenize($cl);
                if (len($comps) == 0) {
                    return false;
                }
                for (def c in $comps) {
                    if (not comparatorValid($c)) {
                        return false;
                    }
                }
            }
        }
    }
    return true;
}

/**
 * Pick the highest version from a list that satisfies the range. Versions that
 * are not valid SemVer are skipped. Returns "" when none match.
 * @param versions {list of string} the candidate versions
 * @param range {string} the range expression
 * @return {string} the highest satisfying version, or "" if none match
 */
export func maxSatisfying(versions as list of string, range as string) {
    def chosen as string init "";
    def have as bool init false;
    def bestVer as Version;
    for (def ver in $versions) {
        if (satisfies($ver, $range)) {
            def parsed as Version init parse($ver);
            if (not $have or compare($parsed, $bestVer) > 0) {
                $bestVer = $parsed;
                $chosen = $ver;
                $have = true;
            }
        }
    }
    return $chosen;
}

/**
 * Pick the lowest version from a list that satisfies the range. Versions that
 * are not valid SemVer are skipped. Returns "" when none match.
 * @param versions {list of string} the candidate versions
 * @param range {string} the range expression
 * @return {string} the lowest satisfying version, or "" if none match
 */
export func minSatisfying(versions as list of string, range as string) {
    def chosen as string init "";
    def have as bool init false;
    def lowVer as Version;
    for (def ver in $versions) {
        if (satisfies($ver, $range)) {
            def parsed as Version init parse($ver);
            if (not $have or compare($parsed, $lowVer) < 0) {
                $lowVer = $parsed;
                $chosen = $ver;
                $have = true;
            }
        }
    }
    return $chosen;
}

# --- lenient input (exported) --------------------------------------

/**
 * Extract a version from a loose string - a git tag, a partial, or text with a
 * version-like run - and return canonical `major.minor.patch` (missing parts are
 * 0). Handles a leading `v` and surrounding noise. Returns "" when no numeric
 * core is found. `coerce("v1.2.3")` -> "1.2.3", `coerce("1.2")` -> "1.2.0".
 * @param s {string} the loose text
 * @return {string} a canonical version, or "" if none was found
 */
export func coerce(s as string) {
    def m as regex.Match init regex.find("(?P<maj>[0-9]+)(?:\\.(?P<min>[0-9]+))?(?:\\.(?P<pat>[0-9]+))?", $s);
    if ($m.start < 0) {
        return "";
    }
    def maj as string init convert.toString(convert.toInt($m.groupsNamed["maj"]));
    def minV as string init "0";
    if (not ($m.groupsNamed["min"] == "")) {
        $minV = convert.toString(convert.toInt($m.groupsNamed["min"]));
    }
    def patV as string init "0";
    if (not ($m.groupsNamed["pat"] == "")) {
        $patV = convert.toString(convert.toInt($m.groupsNamed["pat"]));
    }
    return $maj + "." + $minV + "." + $patV;
}

/**
 * Normalise a version string: trim whitespace and a leading `=` / `v`, then
 * return the canonical form if it is a valid full version, else "". Strict
 * (unlike `coerce`): `clean("v1.2.3")` -> "1.2.3", but `clean("1.2")` -> "".
 * @param s {string} the version text
 * @return {string} the canonical version, or "" if not valid
 */
export func clean(s as string) {
    def t as string init strings.trim($s);
    if (strings.startsWith($t, "=")) {
        $t = strings.trim(rest($t, 1));
    }
    if (strings.startsWith($t, "v") or strings.startsWith($t, "V")) {
        $t = rest($t, 1);
    }
    if (isValid($t)) {
        return toString(parse($t));
    }
    return "";
}

# --- range algebra (private): prerelease-precise intervals ----------
# Each OR-clause reduces to an interval with full-version bounds and inclusivity
# flags [lo(loIncl), hi(hiIncl)], plus `pins`: the major.minor.patch tuples at
# which the clause admits prereleases (npm's rule - a prerelease is in range only
# when a comparator pins one at its tuple). Full-version bounds + pins make the
# algebra prerelease-precise, not just release-space.
def const RANGE_INF as int init 1000000000000;

def struct Interval {
    lo as Version,
    loIncl as bool,
    hi as Version,
    hiIncl as bool,
    pins as list of string
};

func zeroVersion() {
    return relVersion(0, 0, 0);
}
func infVersion() {
    return relVersion(RANGE_INF, 0, 0);
}
func isInf(v as Version) {
    return compare($v, infVersion()) >= 0;
}
func releaseCore(v as Version) {
    return relVersion($v.major, $v.minor, $v.patch);
}

# tupleStr is the "major.minor.patch" key used to pin a prerelease tuple.
func tupleStr(v as Version) {
    return convert.toString($v.major) + "." + convert.toString($v.minor) + "." + convert.toString($v.patch);
}
func pinnedAt(pins as list of string, t as string) {
    for (def p in $pins) {
        if ($p == $t) {
            return true;
        }
    }
    return false;
}
# pinList is [tuple] when a full operand carries a prerelease, else [].
func pinList(op as string) {
    def out as list of string init [];
    if (isValid($op)) {
        def v as Version init parse($op);
        if (len($v.prerelease) > 0) {
            $out[] = tupleStr($v);
        }
    }
    return $out;
}
# preMin is the smallest prerelease at a tuple ("X.Y.Z-0").
func preMin(t as string) {
    def v as Version init parse($t);
    return Version{major: $v.major, minor: $v.minor, patch: $v.patch, prerelease: "0", build: ""};
}
func fullInterval() {
    def none as list of string init [];
    return Interval{lo: zeroVersion(), loIncl: true, hi: infVersion(), hiIncl: true, pins: $none};
}

# boundGe / boundGt / boundLe / boundLt / boundEq turn one comparator into a
# one-sided (or point) interval, honouring prerelease operands and inclusivity.
func boundGe(op as string) {
    def o as string init strings.trim($op);
    def none as list of string init [];
    if (isValid($o)) {
        return Interval{lo: parse($o), loIncl: true, hi: infVersion(), hiIncl: true, pins: pinList($o)};
    }
    return Interval{lo: coreLower(parseCore($o)), loIncl: true, hi: infVersion(), hiIncl: true, pins: $none};
}
func boundGt(op as string) {
    def o as string init strings.trim($op);
    def none as list of string init [];
    if (isValid($o)) {
        return Interval{lo: parse($o), loIncl: false, hi: infVersion(), hiIncl: true, pins: pinList($o)};
    }
    return Interval{lo: tildeUpper(parseCore($o)), loIncl: true, hi: infVersion(), hiIncl: true, pins: $none};
}
func boundLe(op as string) {
    def o as string init strings.trim($op);
    def none as list of string init [];
    if (isValid($o)) {
        return Interval{lo: zeroVersion(), loIncl: true, hi: parse($o), hiIncl: true, pins: pinList($o)};
    }
    return Interval{lo: zeroVersion(), loIncl: true, hi: tildeUpper(parseCore($o)), hiIncl: false, pins: $none};
}
func boundLt(op as string) {
    def o as string init strings.trim($op);
    def none as list of string init [];
    if (isValid($o)) {
        return Interval{lo: zeroVersion(), loIncl: true, hi: parse($o), hiIncl: false, pins: pinList($o)};
    }
    return Interval{lo: zeroVersion(), loIncl: true, hi: coreLower(parseCore($o)), hiIncl: false, pins: $none};
}
func boundEq(op as string) {
    def o as string init strings.trim($op);
    if (isWild($o)) {
        return fullInterval();
    }
    def none as list of string init [];
    if (isValid($o)) {
        def v as Version init parse($o);
        return Interval{lo: $v, loIncl: true, hi: $v, hiIncl: true, pins: pinList($o)};
    }
    def core as Core init parseCore($o);
    if ($core.count >= 3) {
        return Interval{lo: coreLower($core), loIncl: true, hi: coreLower($core), hiIncl: true, pins: $none};
    }
    return Interval{lo: coreLower($core), loIncl: true, hi: tildeUpper($core), hiIncl: false, pins: $none};
}

func comparatorInterval(c as string) {
    def s as string init strings.trim($c);
    def none as list of string init [];
    if (isWild($s)) {
        return fullInterval();
    }
    if (strings.startsWith($s, "^")) {
        def operand as string init rest($s, 1);
        def core as Core init parseCore($operand);
        # A prerelease operand keeps its prerelease as the interval's lower
        # bound (and pins it), so minVersion / the interval algebra return the
        # exact prerelease the caret allows rather than the bare release.
        if (isValid($operand)) {
            return Interval{lo: parse($operand), loIncl: true, hi: caretUpper($core), hiIncl: false, pins: pinList($operand)};
        }
        return Interval{lo: coreLower($core), loIncl: true, hi: caretUpper($core), hiIncl: false, pins: $none};
    }
    if (strings.startsWith($s, "~")) {
        def operand as string init rest($s, 1);
        def core as Core init parseCore($operand);
        if (isValid($operand)) {
            return Interval{lo: parse($operand), loIncl: true, hi: tildeUpper($core), hiIncl: false, pins: pinList($operand)};
        }
        return Interval{lo: coreLower($core), loIncl: true, hi: tildeUpper($core), hiIncl: false, pins: $none};
    }
    if (strings.startsWith($s, ">=")) {
        return boundGe(rest($s, 2));
    }
    if (strings.startsWith($s, "<=")) {
        return boundLe(rest($s, 2));
    }
    if (strings.startsWith($s, ">")) {
        return boundGt(rest($s, 1));
    }
    if (strings.startsWith($s, "<")) {
        return boundLt(rest($s, 1));
    }
    if (strings.startsWith($s, "=")) {
        return boundEq(rest($s, 1));
    }
    return boundEq($s);
}

# intersectIntervals ANDs two intervals (tighter lo, tighter hi, unioned pins).
func intersectIntervals(a as Interval, b as Interval) {
    def lo as Version init $a.lo;
    def loIncl as bool init $a.loIncl;
    def cl as int init compare($b.lo, $a.lo);
    if ($cl > 0) {
        $lo = $b.lo;
        $loIncl = $b.loIncl;
    } elseif ($cl == 0) {
        $loIncl = $a.loIncl and $b.loIncl;
    }
    def hi as Version init $a.hi;
    def hiIncl as bool init $a.hiIncl;
    def ch as int init compare($b.hi, $a.hi);
    if ($ch < 0) {
        $hi = $b.hi;
        $hiIncl = $b.hiIncl;
    } elseif ($ch == 0) {
        $hiIncl = $a.hiIncl and $b.hiIncl;
    }
    def pins as list of string init [];
    for (def p in $a.pins) {
        $pins[] = $p;
    }
    for (def p in $b.pins) {
        $pins[] = $p;
    }
    return Interval{lo: $lo, loIncl: $loIncl, hi: $hi, hiIncl: $hiIncl, pins: $pins};
}

func intervalEmpty(iv as Interval) {
    def c as int init compare($iv.lo, $iv.hi);
    if ($c > 0) {
        return true;
    }
    if ($c == 0) {
        return not ($iv.loIncl and $iv.hiIncl);
    }
    return false;
}

# intervalHasRelease reports whether a release version lies in the interval.
func intervalHasRelease(iv as Interval) {
    if (intervalEmpty($iv)) {
        return false;
    }
    def cand as Version init $iv.lo;
    if (len($iv.lo.prerelease) > 0) {
        $cand = releaseCore($iv.lo);
    } elseif (not $iv.loIncl) {
        $cand = incPatch($iv.lo);
    }
    def hc as int init compare($cand, $iv.hi);
    return $hc < 0 or ($iv.hiIncl and $hc == 0);
}

func clauseToInterval(clause as string) {
    def cl as string init strings.trim($clause);
    if (strings.contains($cl, " - ")) {
        def idx as int init strings.indexOf($cl, " - ");
        def aStr as string init strings.trim(strings.substring($cl, 0, $idx));
        def bStr as string init strings.trim(strings.substring($cl, $idx + 3, len($cl)));
        def hi as Version init tildeUpper(parseCore($bStr));
        def hiIncl as bool init false;
        if (isValid($bStr)) {
            $hi = parse($bStr);
            $hiIncl = true;
        }
        def pins as list of string init [];
        for (def p in pinList($aStr)) {
            $pins[] = $p;
        }
        for (def p in pinList($bStr)) {
            $pins[] = $p;
        }
        return Interval{lo: coreLower(parseCore($aStr)), loIncl: true, hi: $hi, hiIncl: $hiIncl, pins: $pins};
    }
    def iv as Interval init fullInterval();
    for (def c in tokenize($cl)) {
        $iv = intersectIntervals($iv, comparatorInterval($c));
    }
    return $iv;
}

func rangeToIntervals(range as string) {
    def out as list of Interval init [];
    def r as string init strings.trim($range);
    if ($r == "" or $r == "*" or $r == "any") {
        $out[] = fullInterval();
        return $out;
    }
    for (def clause in strings.split($r, "||")) {
        def cl as string init strings.trim($clause);
        if (not ($cl == "")) {
            def iv as Interval init clauseToInterval($cl);
            if (not intervalEmpty($iv)) {
                $out[] = $iv;
            }
        }
    }
    return $out;
}

# --- interval cover (private, for subset) --------------------------

func loCovers(m as Interval, iv as Interval) {
    def c as int init compare($m.lo, $iv.lo);
    if ($c < 0) {
        return true;
    }
    if ($c == 0) {
        return $m.loIncl or not $iv.loIncl;
    }
    return false;
}
func hiCovers(m as Interval, iv as Interval) {
    def c as int init compare($m.hi, $iv.hi);
    if ($c > 0) {
        return true;
    }
    if ($c == 0) {
        return $m.hiIncl or not $iv.hiIncl;
    }
    return false;
}
func insertByLo(sorted as list of Interval, iv as Interval) {
    def out as list of Interval init [];
    def placed as bool init false;
    for (def w in $sorted) {
        if (not $placed and compare($iv.lo, $w.lo) < 0) {
            $out[] = $iv;
            $placed = true;
        }
        $out[] = $w;
    }
    if (not $placed) {
        $out[] = $iv;
    }
    return $out;
}
# mergeCover folds intervals into a sorted, disjoint cover (touching bounds merge).
func mergeCover(ivs as list of Interval) {
    def sorted as list of Interval init [];
    for (def iv in $ivs) {
        $sorted = insertByLo($sorted, $iv);
    }
    def merged as list of Interval init [];
    def have as bool init false;
    def cur as Interval;
    def none as list of string init [];
    for (def iv in $sorted) {
        if (not $have) {
            $cur = $iv;
            $have = true;
        } else {
            def c as int init compare($iv.lo, $cur.hi);
            def touch as bool init $c < 0;
            if ($c == 0 and ($cur.hiIncl or $iv.loIncl)) {
                $touch = true;
            }
            if ($touch) {
                def hc as int init compare($iv.hi, $cur.hi);
                if ($hc > 0 or ($hc == 0 and $iv.hiIncl)) {
                    $cur = Interval{lo: $cur.lo, loIncl: $cur.loIncl, hi: $iv.hi, hiIncl: $iv.hiIncl, pins: $none};
                }
            } else {
                $merged[] = $cur;
                $cur = $iv;
            }
        }
    }
    if ($have) {
        $merged[] = $cur;
    }
    return $merged;
}
func coveredByCover(target as Interval, cover as list of Interval) {
    for (def m in $cover) {
        if (loCovers($m, $target) and hiCovers($m, $target)) {
            return true;
        }
    }
    return false;
}

# --- range algebra (exported) --------------------------------------

/**
 * The lowest version that could satisfy a range (its floor), or "" when the
 * range is empty or invalid. Prerelease-precise: `minVersion(">=1.2.3-rc.1")` is
 * "1.2.3-rc.1", `minVersion("^1.2.0")` is "1.2.0", `minVersion(">1.2.3")` is
 * "1.2.4".
 * @param range {string} the range expression
 * @return {string} the lowest satisfying version, or ""
 */
export func minVersion(range as string) {
    if (not validRange($range)) {
        return "";
    }
    def ivs as list of Interval init rangeToIntervals($range);
    def have as bool init false;
    def floor as Version;
    for (def iv in $ivs) {
        def cand as Version init $iv.lo;
        if (not $iv.loIncl) {
            if (len($iv.lo.prerelease) > 0) {
                $cand = releaseCore($iv.lo);
            } else {
                $cand = incPatch($iv.lo);
            }
        } elseif (len($iv.lo.prerelease) > 0 and not pinnedAt($iv.pins, tupleStr($iv.lo))) {
            $cand = releaseCore($iv.lo);
        }
        if (not $have or compare($cand, $floor) < 0) {
            $floor = $cand;
            $have = true;
        }
    }
    if (not $have) {
        return "";
    }
    return toString($floor);
}

/**
 * Report whether two ranges share at least one satisfying version, prereleases
 * included (a prerelease overlap needs both ranges to pin the same
 * major.minor.patch). `^1.2.0` intersects `>=1.5.0` (true) but not `^2.0.0`.
 * Invalid ranges never intersect.
 * @param rangeA {string} the first range
 * @param rangeB {string} the second range
 * @return {bool} true when the ranges overlap
 */
export func intersects(rangeA as string, rangeB as string) {
    if (not validRange($rangeA) or not validRange($rangeB)) {
        return false;
    }
    def ia as list of Interval init rangeToIntervals($rangeA);
    def ib as list of Interval init rangeToIntervals($rangeB);
    for (def a in $ia) {
        for (def b in $ib) {
            def iv as Interval init intersectIntervals($a, $b);
            if (not intervalEmpty($iv)) {
                if (intervalHasRelease($iv)) {
                    return true;
                }
                def t as string init tupleStr($iv.lo);
                if (pinnedAt($a.pins, $t) and pinnedAt($b.pins, $t)) {
                    return true;
                }
            }
        }
    }
    return false;
}

/**
 * Report whether every version allowed by `inner` is also allowed by `outer`
 * (inner is the tighter / implied constraint), prereleases included.
 * `subset("^1.5.0", "^1.0.0")` is true; `subset("^1.0.0", "^1.5.0")` is false.
 * @param inner {string} the candidate subset range
 * @param outer {string} the superset range
 * @return {bool} true when inner is a subset of outer
 */
export func subset(inner as string, outer as string) {
    if (not validRange($inner) or not validRange($outer)) {
        return false;
    }
    def innerIvs as list of Interval init rangeToIntervals($inner);
    if (len($innerIvs) == 0) {
        return true;
    }
    def outerIvs as list of Interval init rangeToIntervals($outer);
    def cover as list of Interval init mergeCover($outerIvs);
    def none as list of string init [];
    for (def s in $innerIvs) {
        if (not coveredByCover($s, $cover)) {
            return false;
        }
        for (def t in $s.pins) {
            def tcover as list of Interval init [];
            for (def o in $outerIvs) {
                if (pinnedAt($o.pins, $t)) {
                    $tcover[] = $o;
                }
            }
            def preSpan as Interval init Interval{lo: preMin($t), loIncl: true, hi: releaseCore(parse($t)), hiIncl: false, pins: $none};
            def sPre as Interval init intersectIntervals($s, $preSpan);
            if (not intervalEmpty($sPre)) {
                if (not coveredByCover($sPre, mergeCover($tcover))) {
                    return false;
                }
            }
        }
    }
    return true;
}

/**
 * Report whether a version is greater than every version a range allows (beyond
 * its upper extreme). False for an unbounded range or a version in an interior
 * gap.
 * @param version {string} the version to test
 * @param range {string} the range expression
 * @return {bool} true when version is above the whole range
 */
export func gtr(version as string, range as string) {
    if (not isValid($version) or not validRange($range)) {
        return false;
    }
    def ivs as list of Interval init rangeToIntervals($range);
    if (len($ivs) == 0) {
        return false;
    }
    def maxHi as Version init zeroVersion();
    def maxIncl as bool init false;
    for (def iv in $ivs) {
        def c as int init compare($iv.hi, $maxHi);
        if ($c > 0) {
            $maxHi = $iv.hi;
            $maxIncl = $iv.hiIncl;
        } elseif ($c == 0) {
            $maxIncl = $maxIncl or $iv.hiIncl;
        }
    }
    if (isInf($maxHi)) {
        return false;
    }
    def c as int init compare(parse($version), $maxHi);
    return $c > 0 or ($c == 0 and not $maxIncl);
}

/**
 * Report whether a version is less than every version a range allows (below its
 * lower extreme).
 * @param version {string} the version to test
 * @param range {string} the range expression
 * @return {bool} true when version is below the whole range
 */
export func ltr(version as string, range as string) {
    if (not isValid($version) or not validRange($range)) {
        return false;
    }
    def ivs as list of Interval init rangeToIntervals($range);
    if (len($ivs) == 0) {
        return false;
    }
    def minLo as Version init infVersion();
    def minIncl as bool init false;
    for (def iv in $ivs) {
        def c as int init compare($iv.lo, $minLo);
        if ($c < 0) {
            $minLo = $iv.lo;
            $minIncl = $iv.loIncl;
        } elseif ($c == 0) {
            $minIncl = $minIncl or $iv.loIncl;
        }
    }
    def c as int init compare(parse($version), $minLo);
    return $c < 0 or ($c == 0 and not $minIncl);
}

/**
 * Report whether a version is beyond a range's extremes (above it or below it).
 * A version in an interior gap of a multi-clause range is not "outside".
 * @param version {string} the version to test
 * @param range {string} the range expression
 * @return {bool} true when version is above or below the whole range
 */
export func outside(version as string, range as string) {
    return gtr($version, $range) or ltr($version, $range);
}

/**
 * Simplify a range against a known list of versions: return the shortest range
 * that matches exactly the same subset of `versions`. Runs of consecutive
 * matching versions collapse to `>=lo <=hi` clauses joined by `||`; the original
 * range is kept when it is already at least as short. "*" when every listed
 * version matches, `<0.0.0-0` (matches nothing) when none do.
 * @param versions {list of string} the known versions
 * @param range {string} the range to simplify
 * @return {string} the simplified range
 */
export func simplifyRange(versions as list of string, range as string) {
    def all as list of Version init [];
    for (def v in $versions) {
        if (isValid($v)) {
            $all[] = parse($v);
        }
    }
    def sorted as list of Version init sort($all);
    def clauses as list of string init [];
    def matched as int init 0;
    def anyPre as bool init false;
    def i as int init 0;
    def n as int init len($sorted);
    while ($i < $n) {
        if (satisfies(toString($sorted[$i]), $range)) {
            def startS as string init toString($sorted[$i]);
            def endS as string init $startS;
            $matched = $matched + 1;
            if (isPrerelease($sorted[$i])) {
                $anyPre = true;
            }
            while ($i + 1 < $n and satisfies(toString($sorted[$i + 1]), $range)) {
                $i = $i + 1;
                $endS = toString($sorted[$i]);
                $matched = $matched + 1;
                if (isPrerelease($sorted[$i])) {
                    $anyPre = true;
                }
            }
            if ($startS == $endS) {
                $clauses[] = $startS;
            } else {
                $clauses[] = ">=" + $startS + " <=" + $endS;
            }
        }
        $i = $i + 1;
    }
    if ($matched == 0) {
        return "<0.0.0-0";
    }
    # Pick a candidate simplification, but never take the "*" shortcut when a
    # prerelease matched: "*" excludes prereleases, so it would drop them.
    def candidate as string init "";
    if ($matched == $n and len($clauses) == 1 and not $anyPre) {
        $candidate = "*";
    } else {
        def joined as string init strings.join($clauses, " || ");
        def orig as string init strings.trim($range);
        if (len($orig) <= len($joined)) {
            $candidate = $orig;
        } else {
            $candidate = $joined;
        }
    }
    # Verify the candidate selects exactly the versions the original range did;
    # otherwise fall back to an explicit OR of the matched versions (which
    # always reproduces the set exactly, prereleases included).
    if (sameMatchSet($sorted, $candidate, $range)) {
        return $candidate;
    }
    def pins as list of string init [];
    for (def v in $sorted) {
        if (satisfies(toString($v), $range)) {
            $pins[] = toString($v);
        }
    }
    return strings.join($pins, " || ");
}

# sameMatchSet reports whether `candidate` and `orig` accept exactly the same
# subset of `sorted`.
func sameMatchSet(sorted as list of Version, candidate as string, orig as string) {
    for (def v in $sorted) {
        def s as string init toString($v);
        if (not (satisfies($s, $candidate) == satisfies($s, $orig))) {
            return false;
        }
    }
    return true;
}
