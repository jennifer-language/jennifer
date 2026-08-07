# `semver` - Semantic Versioning 2.0.0

Import with `import "semver.j" as semver;`. Parses, compares, sorts,
increments, and **range-matches** strict [SemVer 2.0.0](https://semver.org)
version strings - the full surface a package registry or dependency resolver
needs. Pure Jennifer - parsing uses the canonical SemVer regex (`regex`); the
precedence comparison, sort, and range matching are hand-written - so it runs on
either binary.

```jennifer
use io;
import "semver.j" as semver;

def v as semver.Version init semver.parse("1.4.2-rc.1+build.9");
io.printf("%d.%d.%d pre=%s\n", $v.major, $v.minor, $v.patch, $v.prerelease);
io.printf("rc < release: %t\n", semver.lt($v, semver.parse("1.4.2"))); # true
io.printf("next minor: %s\n", semver.toString(semver.incMinor($v)));   # 1.5.0
```

Runnable: [`examples/modules/semver_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/semver_demo.j).

## The `Version` struct

```jennifer
export def struct Version {
    major as int,
    minor as int,
    patch as int,
    prerelease as string,   # "" when absent (the text after `-`, no dash)
    build as string         # "" when absent (the text after `+`, no plus)
};
```

A value-semantic struct you hold and pass around; `semver` stores nothing.
Build it with `semver.parse` or a literal
(`semver.Version{major: 1, minor: 0, patch: 0, prerelease: "", build: ""}`).

## Surface

| Call                       | Returns          | Notes                                                                 |
| -------------------------- | ---------------- | --------------------------------------------------------------------- |
| `semver.parse(s)`          | `Version`        | Parse a strict version string; `throw`s (`kind: "value"`) on invalid. |
| `semver.isValid(s)`        | `bool`           | Whether `s` is a strict SemVer string (no throw).                     |
| `semver.coerce(s)`         | `string`         | Extract a version from loose text (a `v`-tag, a partial); `""` if none. |
| `semver.clean(s)`          | `string`         | Strict-normalise (trim, drop `v` / `=`); `""` if not a full version.  |
| `semver.toString(v)`       | `string`         | Canonical form; round-trips `parse`.                                  |
| `semver.compare(a, b)`     | `int`            | `-1` / `0` / `1` by SemVer precedence. Build metadata ignored.        |
| `semver.lt(a, b)`          | `bool`           | `compare(a, b) < 0`.                                                   |
| `semver.eq(a, b)`          | `bool`           | `compare(a, b) == 0` (so `1.0.0+a` equals `1.0.0+b`).                  |
| `semver.gt(a, b)`          | `bool`           | `compare(a, b) > 0`.                                                   |
| `semver.gte(a, b)`         | `bool`           | `compare(a, b) >= 0`.                                                  |
| `semver.lte(a, b)`         | `bool`           | `compare(a, b) <= 0`.                                                  |
| `semver.neq(a, b)`         | `bool`           | `compare(a, b) != 0`.                                                  |
| `semver.diff(a, b)`        | `string`         | The change kind: `"major"` / `"minor"` / `"patch"` / `"prerelease"` / `""`. |
| `semver.isStable(v)`       | `bool`           | `major >= 1` and no prerelease. `0.y.z` is unstable by convention.    |
| `semver.isPrerelease(v)`   | `bool`           | Whether a prerelease tag is present.                                  |
| `semver.incMajor(v)`       | `Version`        | `major+1`; resets minor / patch and clears prerelease + build.        |
| `semver.incMinor(v)`       | `Version`        | `minor+1`; resets patch and clears prerelease + build.                |
| `semver.incPatch(v)`       | `Version`        | `patch+1`; clears prerelease + build.                                 |
| `semver.sort(vs)`          | `list of Version`| A new list ordered ascending by precedence.                           |
| `semver.rsort(vs)`         | `list of Version`| A new list ordered descending (highest first).                        |
| `semver.satisfies(ver, range)` | `bool`       | Whether the version string matches the range. See [Ranges](#ranges-and-constraints). |
| `semver.maxSatisfying(vers, range)` | `string`| Highest version string in `vers` matching `range`, or `""`.           |
| `semver.minSatisfying(vers, range)` | `string`| Lowest version string in `vers` matching `range`, or `""`.            |
| `semver.validRange(range)` | `bool`           | Whether a range expression is well-formed.                            |
| `semver.minVersion(range)` | `string`         | The lowest version that could satisfy `range` (its floor), or `""`.   |
| `semver.intersects(a, b)`  | `bool`           | Whether two ranges share any satisfying version.                      |
| `semver.subset(inner, outer)` | `bool`        | Whether every version in `inner` is also allowed by `outer`.          |
| `semver.gtr(v, range)`     | `bool`           | Whether `v` is above the whole range.                                 |
| `semver.ltr(v, range)`     | `bool`           | Whether `v` is below the whole range.                                 |
| `semver.outside(v, range)` | `bool`           | `gtr(v, range) or ltr(v, range)`.                                     |
| `semver.simplifyRange(vers, range)` | `string`| The shortest range matching the same subset of `vers`.                |

## Strict, not a loose parser

`semver.parse` accepts exactly `MAJOR.MINOR.PATCH` with an optional
`-prerelease` and `+build`, per the spec. It **rejects** everything the
grammar disallows:

```jennifer
semver.isValid("1.2.3");        # true
semver.isValid("1.2.3-rc.1");   # true
semver.isValid("1.0.0+build");  # true
semver.isValid("1.2.3.4");      # false - four segments have no defined order
semver.isValid("1.2");          # false - too few
semver.isValid("01.0.0");       # false - leading zero in a numeric part
semver.isValid("1.0.0-01");     # false - leading zero in a numeric prerelease id
semver.isValid("1.0.0-");       # false - empty prerelease
```

A looser N-segment form (`1.2.3.4`) has no defined ordering (`1.2.3` vs
`1.2.3.0`?), which would quietly break sorting, so it is invalid rather
than best-effort. Jennifer's own `meta.VERSION` is valid strict SemVer, so
`semver.parse(meta.VERSION)` works out of the box.

Use `isValid` to test without a throw; `parse` for the value (wrap it in
`try` / `catch` to handle untrusted input).

## Precedence

`compare` implements SemVer clause 11 exactly:

1. Compare `major`, then `minor`, then `patch` numerically.
2. A version **with** a prerelease ranks **below** the same version
   without one: `1.0.0-alpha < 1.0.0`.
3. Two prereleases compare field by field (split on `.`): numeric
   identifiers compare numerically and rank below alphanumeric ones, which
   compare in ASCII order; a longer run of otherwise-equal fields ranks
   higher.
4. **Build metadata is ignored** - `1.0.0+a` and `1.0.0+b` are equal.

```jennifer
# 1.0.0-alpha < 1.0.0-alpha.1 < 1.0.0-alpha.beta < 1.0.0-beta
#   < 1.0.0-beta.2 < 1.0.0-beta.11 < 1.0.0-rc.1 < 1.0.0
```

Note `beta.2 < beta.11` (numeric, not lexical) - the classic loose-parser
trap the field-by-field rule avoids.

## Sorting a list

`lists.sort` is scalar-only, so `semver.sort` runs its own pass over
`compare` and returns a new ascending list (the input is untouched -
value semantics):

```jennifer
def vs as list of semver.Version init [];
$vs[] = semver.parse("2.0.0");
$vs[] = semver.parse("1.0.0-alpha");
$vs[] = semver.parse("1.0.0");
$vs[] = semver.parse("1.10.0");
$vs[] = semver.parse("1.2.0");

for (def s in semver.sort($vs)) {
    io.printf(" %s", semver.toString($s));
}
# sorted: 1.0.0-alpha 1.0.0 1.2.0 1.10.0 2.0.0
```

## Ranges and constraints

`semver.satisfies(version, range)` matches a concrete version string against a
range expression, following the npm / Composer grammar:

| Form | Example | Means |
| ---- | ------- | ----- |
| exact | `1.2.3` / `=1.2.3` | that version exactly |
| caret | `^1.2.0` | `>=1.2.0 <2.0.0` (up to the next non-zero left component) |
| tilde | `~1.2` | `>=1.2.0 <1.3.0` |
| comparators | `>=1.0.0` `<2.0.0` `>1.2` `<=3` | numeric bounds (a partial operand expands, npm-style) |
| AND | `>=1.2.0 <2.0.0` (space or `,`) | all comparators in the clause hold |
| OR | `^1.0.0 \|\| ^3.0.0` | any clause holds |
| hyphen | `1.2.3 - 2.3.4` | `>=1.2.3 <=2.3.4` (a partial upper bumps to `<`) |
| x-range | `1.x` / `1.2.*` | `>=1.0.0 <2.0.0` / `>=1.2.0 <1.3.0` |
| any | `*` / `""` / `"any"` | any released version |

```jennifer
semver.satisfies("1.4.0", "^1.2.0");                    # true
semver.satisfies("2.0.0", "^1.2.0");                    # false
semver.satisfies("1.9.0", ">=1.2.0 <2.0.0");            # true
semver.satisfies("3.4.0", "^1.0.0 || ^3.0.0");          # true
semver.satisfies("2.0.0", "1.2.3 - 2.3.4");             # true
```

**Prereleases are excluded** from ranges by default: a version like `2.0.0-rc.1`
satisfies a range only when a comparator in the *same clause* pins a prerelease
at the same `major.minor.patch` (the npm rule), e.g. `>=1.2.3-rc.1 <1.3.0`
admits `1.2.3-rc.2` but not `1.4.0-rc.1`. An invalid version string never
satisfies anything.

### Selecting from a set

For a registry resolving "the best available version", `maxSatisfying` /
`minSatisfying` pick the highest / lowest candidate that matches, skipping any
non-SemVer entries and returning `""` when none match:

```jennifer
def tags as list of string init ["1.0.0", "1.2.0", "1.4.3", "2.0.0"];
semver.maxSatisfying($tags, "^1.2.0");   # "1.4.3"
semver.minSatisfying($tags, "^1.2.0");   # "1.2.0"
```

`semver.validRange(range)` reports whether a range expression is well-formed,
without evaluating it. `semver.minVersion(range)` returns the lowest version
that could satisfy a range (its floor), with no candidate list:
`minVersion("^1.2.0")` is `"1.2.0"`, `minVersion(">1.2.3")` is `"1.2.4"`.

### Ingesting loose versions

Real registries take messy tags. `semver.coerce(s)` extracts a version from a
`v`-prefix, a partial, or surrounding noise (`coerce("v1.2.3")` -> `"1.2.3"`,
`coerce("1.2")` -> `"1.2.0"`, `coerce("latest")` -> `""`), while
`semver.clean(s)` strictly normalises a near-clean string (trim, drop a leading
`v` / `=`) and returns `""` unless it is already a full version.

### Range algebra

For a dependency **solver** (conflict detection, deduplication), the range-vs-range
operators reason over interval sets - no candidate list needed:

| Call | Question |
| ---- | -------- |
| `semver.intersects(a, b)` | do ranges `a` and `b` share any version? `^1.2.0` ∩ `>=1.5.0` = true; `^1.2.0` ∩ `^2.0.0` = false |
| `semver.subset(inner, outer)` | is every version in `inner` also in `outer`? `subset("^1.5.0", "^1.0.0")` = true |
| `semver.gtr(v, range)` | is `v` above the whole range? |
| `semver.ltr(v, range)` | is `v` below the whole range? |
| `semver.outside(v, range)` | above or below (not in an interior gap) |

The algebra is **prerelease-precise**: intervals carry full-version bounds with
inclusivity flags plus the `major.minor.patch` tuples at which each clause
admits prereleases, so `intersects(">=1.2.3-rc.1 <1.2.3", ">=1.5.0-rc.1 <1.5.0")`
is `false` (prereleases at different tuples never meet) while
`intersects(">=1.2.3-rc.1 <1.2.3", ">=1.2.3-rc.2 <1.2.3")` is `true`.

### Simplifying against a version set

`semver.simplifyRange(versions, range)` returns the shortest range that matches
the *same subset of the given versions* - runs of consecutive matches collapse
to `>=lo <=hi` clauses joined by `||`, `*` when all match, `<0.0.0-0` when none
do, and the original is kept when it is already at least as short:

```jennifer
def vers as list of string init ["1.0.0", "1.1.0", "1.2.0", "1.3.0", "2.0.0"];
semver.simplifyRange($vers, ">=1.0.0 <=1.0.0 || >=1.1.0 <=1.3.0");   # ">=1.0.0 <=1.3.0"
```

## See also

- [regex.md](../libraries/regex.md) - `parse` matches against the
  canonical SemVer pattern with named groups.
- [meta.md](../libraries/meta.md) - `meta.VERSION`, itself valid SemVer.
- [modules/index.md](index.md) - the module catalog and import rules.
