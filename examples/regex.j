# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the regex library: boolean matches, first + all matches with
 * positional and named capture groups, replace with group substitution, split,
 * and escape for literal-string patterns.
 * RE2 syntax throughout.
 * @module regex
 */
use io;
use regex;
use maps;

# ---- boolean predicate ----
io.printf("matches \"hello\" in \"hello, world\" : %t\n",
    regex.matches("hello", "hello, world"));
io.printf("matches (?i)HELLO in \"hello\"       : %t\n",
    regex.matches("(?i)HELLO", "hello"));
io.printf("matches ^[A-Z]$ in \"hi\"            : %t\n",
    regex.matches("^[A-Z]$", "hi"));

# ---- first match with positional groups ----
def m as regex.Match init regex.find("(\\d+):(\\d+)", "log 12:34 hit 56:78");
io.printf("\nfirst match  : text=%s at %d..%d\n", $m.text, $m.start, $m.end);
io.printf("  group 1    : %s\n", $m.groups[0]);
io.printf("  group 2    : %s\n", $m.groups[1]);

# ---- no match sentinel ----
def none as regex.Match init regex.find("[0-9]+", "no digits here");
io.printf("no match     : start=%d text=[%s]\n", $none.start, $none.text);

# ---- findAll returns every match ----
def all as list of regex.Match init regex.findAll("\\d+", "port 8080 host 9090 admin 22");
io.printf("\nfindAll      : %d entries\n", len($all));
for (def hit in $all) {
    io.printf("  %s @ %d..%d\n", $hit.text, $hit.start, $hit.end);
}

# ---- named groups ----
def pair as regex.Match init regex.find(
    "(?P<key>[a-z]+)=(?P<val>[0-9]+)", "PORT: port=8080 host=1");
io.printf("\nnamed groups : text=%s\n", $pair.text);
io.printf("  key=%s val=%s\n",
    $pair.groupsNamed["key"], $pair.groupsNamed["val"]);
io.printf("  has(key) = %t\n", maps.has($pair.groupsNamed, "key"));

# ---- replace with positional group ----
def r as string init regex.replace("(\\d+)", "port 8080 host 22", "<$1>");
io.printf("\nreplace $1   : %s\n", $r);

# ---- replace with named group ----
def rNamed as string init regex.replace(
    "(?P<host>[\\w.]+):(?P<port>\\d+)", "cache.example.com:11211",
    "host=${host} port=${port}");
io.printf("replace name : %s\n", $rNamed);

# ---- split ----
def parts as list of string init regex.split("\\s+", "  one   two   three  four ");
io.printf("\nsplit        : %d parts\n", len($parts));
for (def p in $parts) {
    io.printf("  [%s]\n", $p);
}

# ---- escape a literal for use as pattern ----
def literal as string init "1+2=(3)";
def pat as string init regex.escape($literal);
io.printf("\nescape       : literal=%s pattern=%s\n", $literal, $pat);
io.printf("  matches ok : %t\n",
    regex.matches($pat, "the answer to 1+2=(3) is unknown"));
