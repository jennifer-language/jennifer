#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The args module: a declarative CLI parser (argparse-style). Build a Parser with
 * the copy-returning builders, parse an argv, then read values back with the
 * typed accessors. This demo parses a fixed argv so its output is deterministic;
 * a real tool passes os.ARGS and checks $r.done for --help / --version.
 * @module args_demo
 */
use io;
import "../../modules/args.j" as args;

# Build the spec: `greet [--name NAME] [-v...] [--lang debug|release] NAME...`
def p as args.Parser init args.parser("greet", "Greet people from the command line");
$p = args.version($p, "greet 1.0");
$p = args.flag($p, "greeting", "g", "Hello", "the greeting word");
$p = args.countFlag($p, "verbose", "v", "verbosity (repeatable)");
$p = args.boolFlag($p, "shout", "s", "upper-case the output");
$p = args.flag($p, "lang", "l", "en", "language");
$p = args.choices($p, ["en", "de", "fr"]);
$p = args.positionalList1($p, "names", "the people to greet");

# A real program would call: args.parse($p, os.ARGS)
def argv as list of string init ["greet", "-vv", "--shout", "-l", "de", "Ada", "Bo"];
def r as args.Result init args.parse($p, $argv);

# --help / --version set $r.done; a real tool prints and exits here.
if ($r.done) {
    io.printf("%s\n", $r.helpText);
    exit 0;
}

def greeting as string init args.asString($r, "greeting");
if (args.asBool($r, "shout")) {
    $greeting = "HEY";
}
io.printf("greeting=%s lang=%s verbose=%d\n",
    $greeting, args.asString($r, "lang"), args.count($r, "verbose"));
for (def name in args.asList($r, "names")) {
    io.printf("  %s, %s!\n", $greeting, $name);
}

# The auto-generated help text (what --help would print):
io.printf("\n--- args.usage($p) ---\n%s", args.usage($p));
