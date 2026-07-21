# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The semver module (modules/semver.j): parse, compare, sort, and range-match
 * Semantic Versioning strings - the surface a package registry needs.
 * @module semver_demo
 */
use io;
import "../../modules/semver.j" as semver;

def v as semver.Version init semver.parse("1.4.2-rc.1+build.9");
io.printf("parsed %d.%d.%d pre=%s build=%s\n", $v.major, $v.minor, $v.patch, $v.prerelease, $v.build);
io.printf("1.4.2-rc.1 < 1.4.2 : %t\n", semver.lt($v, semver.parse("1.4.2")));
io.printf("next minor of 1.4.2 : %s\n", semver.toString(semver.incMinor(semver.parse("1.4.2"))));

def vs as list of semver.Version init [];
$vs[] = semver.parse("2.0.0");
$vs[] = semver.parse("1.0.0-alpha");
$vs[] = semver.parse("1.0.0");
$vs[] = semver.parse("1.10.0");
$vs[] = semver.parse("1.2.0");
io.printf("sorted:");
for (def s in semver.sort($vs)) {
    io.printf(" %s", semver.toString($s));
}
io.printf("\n");

# Range matching - the package-registry surface.
io.printf("--- ranges ---\n");
io.printf("1.4.0 satisfies ^1.2.0            : %t\n", semver.satisfies("1.4.0", "^1.2.0"));
io.printf("2.0.0 satisfies ^1.2.0            : %t\n", semver.satisfies("2.0.0", "^1.2.0"));
io.printf("1.9.0 satisfies >=1.2.0 <2.0.0    : %t\n", semver.satisfies("1.9.0", ">=1.2.0 <2.0.0"));
io.printf("3.4.0 satisfies ^1.0.0 || ^3.0.0  : %t\n", semver.satisfies("3.4.0", "^1.0.0 || ^3.0.0"));

# Resolve the best available version a registry would install for "^1.2.0".
def tags as list of string init ["1.0.0", "1.2.0", "1.4.3", "1.5.0-rc.1", "2.0.0"];
io.printf("maxSatisfying(tags, ^1.2.0)       : %s\n", semver.maxSatisfying($tags, "^1.2.0"));
io.printf("diff(1.4.3, 2.0.0)                : %s\n", semver.diff(semver.parse("1.4.3"), semver.parse("2.0.0")));
