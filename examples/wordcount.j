# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Small word-frequency analyzer that exercises core
 * features in a realistic shape: split a sentence into a list of words,
 * build a frequency map, find the top word, and render an ASCII bar
 * chart. Then a nested example aggregates per-reviewer totals across a
 * list of map-of-string-to-int reviews. Used as a golden integration
 * test by cmd/jennifer/examples_test.go.
 * @module wordcount
 */

use io;
use strings;
use convert;
use maps;

# --- Split the input into a list of words ---
def sentence as string init "the quick brown fox jumps over the lazy dog the quick fox";
def words as list of string init strings.split($sentence, " ");

io.printf("=== input ===\n");
io.printf("sentence: %s\n", $sentence);
io.printf("words:    %d\n", len($words));

# --- Build a frequency map ---
#
# `maps.has` is the test-for-presence companion to indexed-read; without
# it, a `$counts[$w]` read on a missing key would error.
def counts as map of string to int init {};
for (def w in $words) {
    if (maps.has($counts, $w)) {
        $counts[$w] = $counts[$w] + 1;
    } else {
        $counts[$w] = 1;
    }
}

io.printf("unique:   %d\n", len($counts));

# --- Print counts in insertion order ---
io.printf("\n=== counts (insertion order) ===\n");
for (def w in $counts) {
    io.printf("  %s = %d\n", $w, $counts[$w]);
}

# --- Find the maximum count ---
def topWord as string init "";
def topCount as int init 0;
for (def w in $counts) {
    if ($counts[$w] > $topCount) {
        $topCount = $counts[$w];
        $topWord = $w;
    }
}
io.printf("\nmost frequent: %s (%d)\n", $topWord, $topCount);

# --- Render as an ASCII bar chart ---
io.printf("\n=== histogram ===\n");
for (def w in $counts) {
    io.printf("  %s : %s (%d)\n", $w, strings.repeat("#", $counts[$w]), $counts[$w]);
}

# --- Nested: list of map-of-string-to-int, aggregate per key ---
#
# Each entry is one rater's scores; we sum across all raters per
# person. Demonstrates iterating a list whose element is itself a map.
io.printf("\n=== reviews ===\n");
def reviews as list of map of string to int init [
    {"alice": 5, "bob": 4},
    {"alice": 3, "bob": 5, "carol": 4},
    {"alice": 4, "carol": 5}
];

def totals as map of string to int init {};
def raters as map of string to int init {};
for (def review in $reviews) {
    for (def name in $review) {
        if (maps.has($totals, $name)) {
            $totals[$name] = $totals[$name] + $review[$name];
            $raters[$name] = $raters[$name] + 1;
        } else {
            $totals[$name] = $review[$name];
            $raters[$name] = 1;
        }
    }
}

for (def name in $totals) {
    io.printf("  %s: total=%d (across %d reviewer", $name, $totals[$name], $raters[$name]);
    if ($raters[$name] == 1) {
        io.printf(")\n");
    } else {
        io.printf("s)\n");
    }
}

# --- 2D grid: list of list of int ---
#
# Build a 3x3 identity matrix via index writes, then render it. Shows
# nested index writes (`$grid[$i][$i]`) and that the outer iteration
# variable lives in its own scope each pass through the C-style for.
io.printf("\n=== identity matrix 3x3 ===\n");
def grid as list of list of int init [
    [0, 0, 0],
    [0, 0, 0],
    [0, 0, 0]
];
for (def i as int init 0; $i < 3; $i = $i + 1) {
    $grid[$i][$i] = 1;
}
for (def i as int init 0; $i < 3; $i = $i + 1) {
    def row as string init "";
    for (def j as int init 0; $j < 3; $j = $j + 1) {
        $row = $row + " " + convert.toString($grid[$i][$j]);
    }
    io.printf("%s\n", $row);
}

# --- Value semantics demonstration ---
#
# Defensively copy a list before mutating, prove the original is
# untouched. Trivial because Jennifer does this automatically on
# assignment - the point is to make it visible.
io.printf("\n=== value semantics ===\n");
def original as list of int init [1, 2, 3];
def working as list of int init $original;
$working[0] = 99;
io.printf("original[0] = %d (unchanged)\n", $original[0]);
io.printf("working[0]  = %d (mutated)\n", $working[0]);

io.printf("\n=== done ===\n");
