# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A workload for the allocation profiler, run with jennifer profile --allocs.
 * It exercises the value-semantics copy paths that --allocs tracks: eager
 * copies (a def / assignment / parameter binding that deep-copies a compound
 * value) and spawn-frame deep copies. It also aliases-then-mutates the way that
 * would force a copy-on-write detachment under a lazier strategy, but the
 * interpreter copies eagerly at every store and keeps the append/index hot loop
 * unshared, so COW detachments stay at or near zero. Drop --allocs for the
 * statement/time profile instead.
 * @module profile
 */
use io;
use lists;
use task;

# Eager copy at parameter binding: each call deep-copies its list argument.
func sink(zs as list of int) {
    return lists.first($zs);
}

# Eager copies: a def-init and a parameter binding of a compound value,
# repeated in a loop so the copy site stands out in the profile.
func eagerCopies() {
    def xs as list of int init lists.range(1, 51);
    def acc as int init 0;
    def i as int init 0;
    while ($i < 200) {
        def ys as list of int init $xs;
        $acc = $acc + sink($ys);
        $i = $i + 1;
    }
    return $acc;
}

# Spawn-frame deep copies: each spawn snapshots (deep-copies) the scope,
# so the captured `data` list is copied once per launch.
func spawnCopies() {
    def data as list of int init lists.range(1, 101);
    def handles as list of task of int;
    def i as int init 0;
    while ($i < 8) {
        $handles[] = spawn { return lists.first($data); };
        $i = $i + 1;
    }
    def sum as int init 0;
    for (def t in $handles) {
        $sum = $sum + task.wait($t);
    }
    return $sum;
}

# Alias-then-mutate: `def alias init $base` reads and stores $base, then a
# write lands on the alias. Under lazy copy-on-write the write would detach
# a shared backing; here the def-init already copied eagerly, so the
# mutation target is private and no detachment fires.
func aliasMutate() {
    def base as list of int init [1, 2, 3];
    def i as int init 0;
    while ($i < 50) {
        def alias as list of int init $base;
        $alias[0] = $i;
        $i = $i + 1;
    }
    return $base[0];
}

io.printf("eager=%d spawn=%d base=%d\n",
    eagerCopies(), spawnCopies(), aliasMutate());
