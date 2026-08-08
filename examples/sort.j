#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Five classic sorting algorithms over a small string data set, each stable
 * (equal elements keep their input order), each using the string ordering
 * operators. The set is sorted twice: once by raw UTF-8 byte (so accented words
 * land after all of ASCII) and once with strings.fold applied (so "Österreich"
 * folds to "Osterreich" and sorts near "O"). It also carries "elan" in both
 * Unicode normal forms (precomposed NFC and decomposed NFD): the raw pass places
 * the two identical-looking spellings far apart, while fold collapses them to
 * one "elan" key.
 *
 * Bubblesort, cocktail (shaker) sort, quicksort, a parallel mergesort (the two
 * top-level halves sort in `spawn`ed tasks), and a natural mergesort (merges the
 * runs already present in the input). All five agree - a nice cross-check.
 *
 * Run it plain for the results, or with --verbose (small data set on purpose)
 * to watch each algorithm's stages:
 *   jennifer run examples/sort.j
 *   jennifer run examples/sort.j --verbose
 * @module sort
 */

use io;
use os;
use lists;
use strings;
use convert;
use task;

def VERBOSE as bool init os.hasFlag("--verbose") or os.hasFlag("-v");

# --- shared helpers ---

# merge combines two already-sorted lists into one, stably: on a tie the element
# from the left list (a) is taken first.
func merge(a as list of string, b as list of string) {
    def out as list of string;
    def i as int init 0;
    def j as int init 0;
    while ($i < len($a) and $j < len($b)) {
        if ($b[$j] < $a[$i]) {
            $out[] = $b[$j];
            $j = $j + 1;
        } else {
            $out[] = $a[$i];
            $i = $i + 1;
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

func show(prefix as string, xs as list of string) {
    io.printf("%s%s\n", $prefix, strings.join($xs, ", "));
}

# hexOf renders the UTF-8 bytes of s as space-separated two-digit hex, so two
# strings that render alike but differ in encoding (NFC vs NFD) are told apart.
func hexOf(s as string) {
    def b as bytes init convert.bytesFromString($s, "utf-8");
    def parts as list of string;
    def i as int init 0;
    while ($i < len($b)) {
        $parts[] = io.sprintf("%d|base=16|pad=2|fill=0", $b[$i]);
        $i = $i + 1;
    }
    return strings.join($parts, " ");
}

# dumpBytes prints one line per string with its UTF-8 hex, revealing the encoding
# behind identical-looking glyphs (the two "elan" spellings differ byte-for-byte).
func dumpBytes(xs as list of string) {
    io.printf("=== dataset bytes (UTF-8) ===\n");
    for (def s in $xs) {
        io.printf("  %s  ->  %s\n", $s, hexOf($s));
    }
    io.printf("\n");
}

# --- bubblesort ---

func bubbleSort(xs as list of string) {
    if ($VERBOSE) {
        io.printf("-- bubblesort --\n");
    }
    def a as list of string init $xs;
    def n as int init len($a);
    def pass as int init 0;
    def swapped as bool init true;
    while ($swapped) {
        $swapped = false;
        def i as int init 0;
        while ($i < $n - 1 - $pass) {
            if ($a[$i + 1] < $a[$i]) {
                def t as string init $a[$i];
                $a[$i] = $a[$i + 1];
                $a[$i + 1] = $t;
                $swapped = true;
            }
            $i = $i + 1;
        }
        $pass = $pass + 1;
        if ($VERBOSE) {
            show("  pass " + convert.toString($pass) + ": ", $a);
        }
    }
    return $a;
}

# --- cocktail (shaker) sort: bubblesort that sweeps both directions ---

func shakerSort(xs as list of string) {
    if ($VERBOSE) {
        io.printf("-- shakersort --\n");
    }
    def a as list of string init $xs;
    def lo as int init 0;
    def hi as int init len($a) - 1;
    while ($lo < $hi) {
        def i as int init $lo;
        while ($i < $hi) {
            if ($a[$i + 1] < $a[$i]) {
                def t as string init $a[$i];
                $a[$i] = $a[$i + 1];
                $a[$i + 1] = $t;
            }
            $i = $i + 1;
        }
        $hi = $hi - 1;
        $i = $hi;
        while ($i > $lo) {
            if ($a[$i] < $a[$i - 1]) {
                def t as string init $a[$i];
                $a[$i] = $a[$i - 1];
                $a[$i - 1] = $t;
            }
            $i = $i - 1;
        }
        $lo = $lo + 1;
        if ($VERBOSE) {
            show("  sweep: ", $a);
        }
    }
    return $a;
}

# --- quicksort: stable three-way partition (less / equal / greater) ---

func quickSort(xs as list of string) {
    if ($VERBOSE) {
        io.printf("-- quicksort --\n");
    }
    return quickSortAt($xs, 0);
}

func quickSortAt(xs as list of string, depth as int) {
    if (len($xs) <= 1) {
        return $xs;
    }
    def pivot as string init $xs[len($xs) // 2];
    def less as list of string;
    def equal as list of string;
    def greater as list of string;
    for (def x in $xs) {
        if ($x < $pivot) {
            $less[] = $x;
        } elseif ($pivot < $x) {
            $greater[] = $x;
        } else {
            $equal[] = $x;
        }
    }
    if ($VERBOSE) {
        io.printf(
            "%spivot %s -> [%s] [%s] [%s]\n",
            strings.repeat("  ", $depth + 1),
            $pivot,
            strings.join($less, ", "),
            strings.join($equal, ", "),
            strings.join($greater, ", "));
    }
    return lists.concat(
        quickSortAt($less, $depth + 1),
        lists.concat($equal, quickSortAt($greater, $depth + 1)));
}

# --- parallel mergesort: the two top-level halves sort concurrently ---

func mergeSort(xs as list of string) {
    if ($VERBOSE) {
        io.printf("-- mergesort (parallel) --\n");
    }
    if (len($xs) <= 1) {
        return $xs;
    }
    def mid as int init len($xs) // 2;
    def left as list of string init $xs[0..$mid];
    def right as list of string init $xs[$mid..];
    if ($VERBOSE) {
        io.printf(
            "  split: [%s] | [%s], sorting halves in parallel\n",
            strings.join($left, ", "),
            strings.join($right, ", "));
    }
    def lt as task of list of string init spawn {
        return mergeSortSeq($left);
    };
    def rt as task of list of string init spawn {
        return mergeSortSeq($right);
    };
    def merged as list of string init merge(task.wait($lt), task.wait($rt));
    if ($VERBOSE) {
        show("  merged: ", $merged);
    }
    return $merged;
}

# mergeSortSeq is the ordinary (sequential) recursion each spawned half runs.
func mergeSortSeq(xs as list of string) {
    if (len($xs) <= 1) {
        return $xs;
    }
    def mid as int init len($xs) // 2;
    return merge(mergeSortSeq($xs[0..$mid]), mergeSortSeq($xs[$mid..]));
}

# --- natural mergesort: merge the ascending runs already in the input ---

func naturalMergeSort(xs as list of string) {
    if ($VERBOSE) {
        io.printf("-- natural mergesort --\n");
    }
    def runs as list of list of string init detectRuns($xs);
    if ($VERBOSE) {
        io.printf("  %d run(s): %s\n", len($runs), describeRuns($runs));
    }
    while (len($runs) > 1) {
        def next as list of list of string;
        def i as int init 0;
        while ($i < len($runs)) {
            if ($i + 1 < len($runs)) {
                $next[] = merge($runs[$i], $runs[$i + 1]);
                $i = $i + 2;
            } else {
                $next[] = $runs[$i];
                $i = $i + 1;
            }
        }
        $runs = $next;
        if ($VERBOSE) {
            io.printf("  merged to %d run(s): %s\n", len($runs), describeRuns($runs));
        }
    }
    if (len($runs) == 0) {
        def empty as list of string;
        return $empty;
    }
    return $runs[0];
}

func detectRuns(xs as list of string) {
    def runs as list of list of string;
    def cur as list of string;
    for (def x in $xs) {
        if (len($cur) == 0 or not ($x < $cur[len($cur) - 1])) {
            $cur[] = $x;
        } else {
            $runs[] = $cur;
            def fresh as list of string;
            $fresh[] = $x;
            $cur = $fresh;
        }
    }
    if (len($cur) > 0) {
        $runs[] = $cur;
    }
    return $runs;
}

func describeRuns(runs as list of list of string) {
    def parts as list of string;
    for (def r in $runs) {
        $parts[] = "[" + strings.join($r, " ") + "]";
    }
    return strings.join($parts, " ");
}

# --- driver ---

func runSuite(label as string, data as list of string) {
    io.printf("=== %s ===\n", $label);
    show("input:      ", $data);
    show("bubblesort: ", bubbleSort($data));
    show("shakersort: ", shakerSort($data));
    show("quicksort:  ", quickSort($data));
    show("mergesort:  ", mergeSort($data));
    show("natural:    ", naturalMergeSort($data));
    io.printf("\n");
}

func foldAll(xs as list of string) {
    def out as list of string;
    for (def x in $xs) {
        $out[] = strings.fold($x);
    }
    return $out;
}

# NFC vs NFD: "elan" appears twice - once precomposed (the e-acute is a
# single rune, U+00E9) and once decomposed (a plain e followed by the U+0301
# combining acute). The two spellings render alike but differ byte-for-byte:
# the precomposed form leads with a 0xC3 byte and sorts after all of ASCII,
# while the decomposed form leads with a plain "e" and sorts among the
# lowercase words - so the raw byte-wise pass places the two far apart, while
# strings.fold collapses both to "elan".
def data as list of string init [
    "Zürich",
    "Österreich",
    "apple",
    "Apple",
    "Zebra",
    "élan",
    "Ähre",
    "élan"
];

dumpBytes($data);
runSuite("byte-wise (raw)", $data);
runSuite("accent-folded (strings.fold)", foldAll($data));
