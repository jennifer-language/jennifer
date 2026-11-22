#!/usr/bin/env -S jennifer run
# pragma-jennifer-version: >=0.25.0 <0.27.0
# pragma-jennifer-capability: net
# lint-disable-file: L201

/**
 * Shapes and their areas. Mail info@example.org.
 *
 * @module shapes
 */
use io;
use math;
use strings as str;
use task;
use channel;
use json;
import "@jennifer/http/" as http;
include "util.j";

/* A block comment /* nests */ and "quotes" or 'raw' stay comment. */

def const MAX_ITEMS as int init 1_000;
def const MASKS as list of int init [0xFF_FF, 0o755, 0b1010, 0];
def const RATIO as float init 6.022e23 / 1.5E-3 + 2e10;

/** A segment. @field to {Point} the end */
def struct Segment {
    from as Point,
    to as Point,
    bytes as bytes
};

def enum Shape {
    Circle{radius as float},
    Square{side as float},
    Empty
};

export func area(s as Shape) {
    match ($s) {
        when Circle(c) { return math.PI * $c.radius * $c.radius; }
        when Square(q) { return $q.side * $q.side; }
        else { return 0.0; }
    }
}

func describe(seg as Segment, tags as map of string to int, f as func) {
    def name as string init "seg {$seg.to.x}..{$seg.from.y} \{literal\}\té\U0001F600";
    def raw as string init '^\d+\s*{not a slot} # not a comment $x';
    def nested as string init "{len({'a': 1, "b": 2})} keys, first {$tags["k{1}"]}";
    def n as int init len($tags) // 2 % 3;
    if ($n >= 1 and not ($n == MAX_ITEMS)) {
        $name = $name + str.repeat("-", 3) + json.map({}).kind;
    } elseif ($n != 0 or false) {
        throw Error{kind: "shape", message: "no tags"};
    } else {
        $n = $n << 2 | 1 & ~0 ^ 3 >> 1;
    }
    return $f($name);
}

def results as channel of int init channel.make(4);
def t as task of int init spawn {
    def total as int init 0;
    for (def i in 0..MAX_ITEMS) {
        if ($i > 10) { break; }
        $total = $total + $i;
    }
    repeat { $total = $total - 1; } until ($total < 0);
    while (false) { continue; }
    return $total;
};

try {
    defer io.printf("done\n");
    errdefer io.eprintf("failed\n");
    def shapes as list of Shape init [Shape.Circle{radius: 2.0}, Shape.Empty];
    for (def s in $shapes) {
        io.printf("%.2f\n", area($s));
    }
    io.printf("%d %v\n", task.wait($t), null);
} catch (e) { # lint-disable: L103
    exit 1;
}

# A value typed `null` is not an alias.
def nothing as null;
