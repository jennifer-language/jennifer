# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * A declarative command-line argument parser, at the common surface of Python's
 * `argparse` plus the features an argparse user misses immediately: typed flags
 * (long + short), defaults, required args, `choices`, `count` / `append` actions,
 * positionals with `nargs`, subcommands, and `--version`. It is the structured
 * layer over `os.ARGS` (`os.hasFlag` / `os.flag` are primitive lookups). Pure
 * Jennifer over `strings` + `convert` + `lists` + `maps`, so it runs on **both
 * binaries**.
 *
 * Build a value-semantic `Parser` with the copy-returning builder pattern
 * (`args.parser` then `args.flag` / `args.intFlag` / `args.positional` / ...),
 * then `args.parse($p, os.ARGS)` -> a `Result`. Read values back with the typed
 * accessors (`args.asString` / `asInt` / `asFloat` / `asBool` / `asList` /
 * `args.count`). An unknown flag, a missing required arg, a bad-type value, or a
 * `choices` violation throws a catchable `Error{kind: "args"}`; `-h` / `--help`
 * (and `--version`) set the result's `done` flag with `helpText` to print, rather
 * than exiting the process, so it composes with `try` / `catch`.
 * @module args
 * @example
 * import "args.j" as args;
 * def p as args.Parser init args.parser("greet", "Say hello");
 * $p = args.flag($p, "name", "n", "world", "who to greet");
 * $p = args.countFlag($p, "verbose", "v", "verbosity (repeatable)");
 * def r as args.Result init args.parse($p, os.ARGS);
 * if ($r.done) { io.printf("%s\n", $r.helpText); exit 0; }
 * io.printf("hello %s (v=%d)\n", args.asString($r, "name"), args.count($r, "verbose"));
 */

use strings;
use convert;
use lists;
use maps;

/**
 * One argument definition (a flag or a positional). Built by the `flag` /
 * `positional` family; not usually constructed directly.
 * @field name {string} the canonical key (long flag name, or positional name)
 * @field short {string} the single-character short flag ("" for none)
 * @field kind {string} "flag" or "positional"
 * @field typ {string} the value type: "string" / "int" / "float" / "bool"
 * @field action {string} "store" (default), "count" (repeat -> int), or "append" (repeat -> list)
 * @field fallback {string} the default value in string form (when `hasDefault`)
 * @field hasDefault {bool} whether `fallback` applies when the arg is absent
 * @field required {bool} whether the arg must be supplied
 * @field nargs {string} positional arity: "" (one), "?" (0-1), "*" (0+), "+" (1+), or an integer count
 * @field choices {list of string} the allowed values ([] = any)
 * @field help {string} the per-argument help text
 */
export def struct Arg {
    name as string,
    short as string,
    kind as string,
    typ as string,
    action as string,
    fallback as string,
    hasDefault as bool,
    required as bool,
    nargs as string,
    choices as list of string,
    help as string
};

/**
 * A subcommand: a name plus its own `Parser` (its own flags and positionals).
 * @field name {string} the subcommand word (e.g. "add")
 * @field help {string} the subcommand's one-line help
 * @field parser {Parser} the parser for the subcommand's own arguments
 */
export def struct Command {
    name as string,
    help as string,
    parser as Parser
};

/**
 * A command-line specification: the program name, description, arguments, and any
 * subcommands. Value-semantic - every builder returns an updated copy.
 * @field prog {string} the program name shown in usage
 * @field help {string} the one-line description
 * @field version {string} the version string for `--version` ("" = no --version)
 * @field args {list of Arg} the declared flags and positionals, in order
 * @field commands {list of Command} the declared subcommands
 */
export def struct Parser {
    prog as string,
    help as string,
    version as string,
    args as list of Arg,
    commands as list of Command
};

/**
 * The outcome of a parse. Read it with the typed accessors rather than poking the
 * maps directly. When `done` is true the parser handled `-h`/`--help`/`--version`:
 * print `helpText` and stop.
 * @field command {string} the chosen subcommand name ("" if none)
 * @field values {map of string to string} store-action values (string form)
 * @field lists {map of string to list of string} append-action and variadic-positional values
 * @field counts {map of string to int} count-action tallies
 * @field present {map of string to bool} which args were supplied on the command line
 * @field helpText {string} the text to print when `done` is set (`--help` / `--version`)
 * @field done {bool} true when `--help` / `--version` was handled (caller should stop)
 */
export def struct Result {
    command as string,
    values as map of string to string,
    lists as map of string to list of string,
    counts as map of string to int,
    present as map of string to bool,
    helpText as string,
    done as bool
};

func fail(msg as string) {
    throw Error{kind: "args", message: "args: " + $msg, file: "", line: 0, col: 0};
}

# --- builders (value-semantic; each returns an updated Parser) ---------------

/**
 * Start a parser with a program name and one-line description.
 * @param prog {string} the program name (shown in usage)
 * @param help {string} the one-line description
 * @return {Parser} an empty parser
 */
export func parser(prog as string, help as string) {
    return Parser{prog: $prog, help: $help, version: "", args: [], commands: []};
}

func addArg(p as Parser, a as Arg) {
    def np as Parser init $p;
    $np.args = lists.push($np.args, $a);
    return $np;
}

/**
 * Add a string-valued optional flag (`--long` / `-short`), with a default.
 * @param p {Parser} the parser
 * @param long {string} the long name (used without the leading "--")
 * @param short {string} the single-char short name ("" for none)
 * @param deflt {string} the default when the flag is absent
 * @param help {string} the flag's help text
 * @return {Parser} the updated parser
 */
export func flag(p as Parser, long as string, short as string, deflt as string, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "string", action: "store",
        fallback: $deflt, hasDefault: true, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add an int-valued optional flag.
 * @param p {Parser} the parser
 * @param long {string} the long name
 * @param short {string} the short name ("" for none)
 * @param deflt {int} the default value
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func intFlag(p as Parser, long as string, short as string, deflt as int, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "int", action: "store",
        fallback: convert.toString($deflt), hasDefault: true, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add a float-valued optional flag.
 * @param p {Parser} the parser
 * @param long {string} the long name
 * @param short {string} the short name ("" for none)
 * @param deflt {float} the default value
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func floatFlag(p as Parser, long as string, short as string, deflt as float, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "float", action: "store",
        fallback: convert.toString($deflt), hasDefault: true, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add a boolean flag (presence sets it true; the argparse `store_true` action).
 * @param p {Parser} the parser
 * @param long {string} the long name
 * @param short {string} the short name ("" for none)
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func boolFlag(p as Parser, long as string, short as string, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "bool", action: "store",
        fallback: "false", hasDefault: true, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add a repeatable counting flag: each occurrence increments a tally (`-vvv` ->
 * 3), the argparse `count` action. Read with `args.count`.
 * @param p {Parser} the parser
 * @param long {string} the long name
 * @param short {string} the short name ("" for none)
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func countFlag(p as Parser, long as string, short as string, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "int", action: "count",
        fallback: "0", hasDefault: true, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add a repeatable value flag: each occurrence appends to a list (the argparse
 * `append` action). Read with `args.asList`.
 * @param p {Parser} the parser
 * @param long {string} the long name
 * @param short {string} the short name ("" for none)
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func listFlag(p as Parser, long as string, short as string, help as string) {
    return addArg($p, Arg{name: $long, short: $short, kind: "flag", typ: "string", action: "append",
        fallback: "", hasDefault: false, required: false, nargs: "", choices: [], help: $help});
}

/**
 * Add a single required positional argument.
 * @param p {Parser} the parser
 * @param name {string} the positional's name (its result key)
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func positional(p as Parser, name as string, help as string) {
    return addArg($p, Arg{name: $name, short: "", kind: "positional", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: true, nargs: "", choices: [], help: $help});
}

/**
 * Add an optional positional (`nargs "?"`, 0 or 1) with a default.
 * @param p {Parser} the parser
 * @param name {string} the positional's name
 * @param deflt {string} the default when absent
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func positionalOpt(p as Parser, name as string, deflt as string, help as string) {
    return addArg($p, Arg{name: $name, short: "", kind: "positional", typ: "string", action: "store",
        fallback: $deflt, hasDefault: true, required: false, nargs: "?", choices: [], help: $help});
}

/**
 * Add a variadic positional collecting zero or more values into a list
 * (`nargs "*"`). Read with `args.asList`.
 * @param p {Parser} the parser
 * @param name {string} the positional's name
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func positionalList(p as Parser, name as string, help as string) {
    return addArg($p, Arg{name: $name, short: "", kind: "positional", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: false, nargs: "*", choices: [], help: $help});
}

/**
 * Add a variadic positional requiring one or more values (`nargs "+"`). Read with
 * `args.asList`.
 * @param p {Parser} the parser
 * @param name {string} the positional's name
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func positionalList1(p as Parser, name as string, help as string) {
    return addArg($p, Arg{name: $name, short: "", kind: "positional", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: true, nargs: "+", choices: [], help: $help});
}

/**
 * Add a positional taking exactly `n` values into a list (`nargs N`).
 * @param p {Parser} the parser
 * @param name {string} the positional's name
 * @param n {int} the exact number of values
 * @param help {string} the help text
 * @return {Parser} the updated parser
 */
export func positionalN(p as Parser, name as string, n as int, help as string) {
    return addArg($p, Arg{name: $name, short: "", kind: "positional", typ: "string", action: "store",
        fallback: "", hasDefault: false, required: true, nargs: convert.toString($n), choices: [], help: $help});
}

/**
 * Mark the most-recently-added argument as required.
 * @param p {Parser} the parser
 * @return {Parser} the updated parser
 * @throws {Error} kind "args" if no argument has been added yet
 */
export func required(p as Parser) {
    if (len($p.args) == 0) {
        fail("required() called before any argument was added");
    }
    def np as Parser init $p;
    def last as int init len($np.args) - 1;
    $np.args[$last].required = true;
    return $np;
}

/**
 * Constrain the most-recently-added argument to a set of allowed values.
 * @param p {Parser} the parser
 * @param allowed {list of string} the permitted values
 * @return {Parser} the updated parser
 * @throws {Error} kind "args" if no argument has been added yet
 */
export func choices(p as Parser, allowed as list of string) {
    if (len($p.args) == 0) {
        fail("choices() called before any argument was added");
    }
    def np as Parser init $p;
    def last as int init len($np.args) - 1;
    $np.args[$last].choices = $allowed;
    return $np;
}

/**
 * Add a subcommand with its own parser (argparse subparsers).
 * @param p {Parser} the parent parser
 * @param name {string} the subcommand word
 * @param help {string} the subcommand's one-line help
 * @param sub {Parser} the subcommand's own parser
 * @return {Parser} the updated parser
 */
export func command(p as Parser, name as string, help as string, sub as Parser) {
    def np as Parser init $p;
    $np.commands = lists.push($np.commands, Command{name: $name, help: $help, parser: $sub});
    return $np;
}

/**
 * Enable a `--version` action printing `ver`.
 * @param p {Parser} the parser
 * @param ver {string} the version string
 * @return {Parser} the updated parser
 */
export func version(p as Parser, ver as string) {
    def np as Parser init $p;
    $np.version = $ver;
    return $np;
}

# --- lookups (pure) ----------------------------------------------------------

func findLong(p as Parser, name as string) {
    def i as int init 0;
    while ($i < len($p.args)) {
        if ($p.args[$i].kind == "flag" and $p.args[$i].name == $name) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

func findShort(p as Parser, ch as string) {
    def i as int init 0;
    while ($i < len($p.args)) {
        if ($p.args[$i].kind == "flag" and $p.args[$i].short == $ch) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

func findCommand(p as Parser, name as string) {
    def i as int init 0;
    while ($i < len($p.commands)) {
        if ($p.commands[$i].name == $name) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

func isIntStr(s as string) {
    if (len($s) == 0) {
        return false;
    }
    def i as int init 0;
    while ($i < len($s)) {
        def c as string init strings.substring($s, $i, $i + 1);
        if ($c < "0" or $c > "9") {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

# validateValue checks a raw string against an arg's type and choices, throwing an
# args error on a mismatch; it returns the value unchanged on success.
func validateValue(a as Arg, v as string) {
    if ($a.typ == "int") {
        def okInt as bool init true;
        try {
            convert.toInt($v);
        } catch (e) {
            $okInt = false;
        }
        if (not $okInt) {
            fail("argument " + argLabel($a) + ": '" + $v + "' is not an integer");
        }
    } elseif ($a.typ == "float") {
        def okF as bool init true;
        try {
            convert.toFloat($v);
        } catch (e) {
            $okF = false;
        }
        if (not $okF) {
            fail("argument " + argLabel($a) + ": '" + $v + "' is not a number");
        }
    }
    if (len($a.choices) > 0 and not lists.contains($a.choices, $v)) {
        fail("argument " + argLabel($a) + ": '" + $v + "' is not one of " + strings.join($a.choices, ", "));
    }
    return $v;
}

func argLabel(a as Arg) {
    if ($a.kind == "positional") {
        return $a.name;
    }
    return "--" + $a.name;
}

# --- result mutators (pure; take a Result, return an updated one) ------------

func setValue(r as Result, name as string, v as string) {
    def nr as Result init $r;
    $nr.values[$name] = $v;
    $nr.present[$name] = true;
    return $nr;
}

func bumpCount(r as Result, name as string) {
    def nr as Result init $r;
    def c as int init 0;
    if (maps.has($nr.counts, $name)) {
        $c = $nr.counts[$name];
    }
    $nr.counts[$name] = $c + 1;
    $nr.present[$name] = true;
    return $nr;
}

# appendList grows a Result list by one. Because the Result is value-semantic (it
# is copied as it threads through the parse), a *repeated* append flag is O(n^2)
# in its own occurrence count - fine for a real CLI (a handful of `-D` / `-I`
# flags), a slow tail only on a pathologically long generated invocation. CLI
# args are trusted invocation, not untrusted wire data, so this is a performance
# note, not a limit that needs a guard.
func appendList(r as Result, name as string, v as string) {
    def nr as Result init $r;
    def cur as list of string init [];
    if (maps.has($nr.lists, $name)) {
        $cur = $nr.lists[$name];
    }
    $nr.lists[$name] = lists.push($cur, $v);
    $nr.present[$name] = true;
    return $nr;
}

# --- parsing -----------------------------------------------------------------

/**
 * Parse `argv` (the full `os.ARGS`, whose first element is the program name and is
 * skipped) against the parser.
 * @param p {Parser} the specification
 * @param argv {list of string} the argument vector (pass `os.ARGS`)
 * @return {Result} the parsed values; check `.done` for --help / --version
 * @throws {Error} kind "args" on an unknown flag, missing required arg, bad type, or bad choice
 */
export func parse(p as Parser, argv as list of string) {
    def toks as list of string init [];
    if (len($argv) > 1) {
        $toks = $argv[1..];
    }
    def r as Result init Result{command: "", values: {}, lists: {}, counts: {},
        present: {}, helpText: "", done: false};
    return run($p, $toks, $r);
}

func isLongFlag(t as string) {
    return strings.startsWith($t, "--") and len($t) > 2;
}

func isShortFlag(t as string) {
    if (not strings.startsWith($t, "-") or len($t) < 2 or strings.startsWith($t, "--")) {
        return false;
    }
    # A leading "-" followed by a digit is a negative number (a positional), not a
    # short flag cluster.
    def c as string init strings.substring($t, 1, 2);
    return $c < "0" or $c > "9";
}

# Step is a handler's outcome: the updated Result, the next token index, and
# whether the caller should stop immediately (a -h / --help / --version hit).
def struct Step {
    r as Result,
    next as int,
    stop as bool
};

# handleLong consumes one --flag / --flag=value token at index i.
func handleLong(p as Parser, toks as list of string, i as int, r as Result) {
    def nr as Result init $r;
    def body as string init strings.substring($toks[$i], 2, len($toks[$i]));
    def name as string init $body;
    def inlineVal as string init "";
    def hasInline as bool init false;
    def eq as int init strings.indexOf($body, "=");
    if ($eq >= 0) {
        $name = strings.substring($body, 0, $eq);
        $inlineVal = strings.substring($body, $eq + 1, len($body));
        $hasInline = true;
    }
    if ($name == "help") {
        $nr.helpText = usage($p);
        $nr.done = true;
        return Step{r: $nr, next: $i, stop: true};
    }
    if ($name == "version" and $p.version != "") {
        $nr.helpText = $p.version;
        $nr.done = true;
        return Step{r: $nr, next: $i, stop: true};
    }
    def ai as int init findLong($p, $name);
    if ($ai < 0) {
        fail("unknown flag --" + $name);
    }
    def a as Arg init $p.args[$ai];
    def next as int init $i + 1;
    if ($a.action == "count") {
        $nr = bumpCount($nr, $a.name);
    } elseif ($a.typ == "bool") {
        $nr = setValue($nr, $a.name, "true");
    } else {
        def val as string init "";
        if ($hasInline) {
            $val = $inlineVal;
        } else {
            if ($i + 1 >= len($toks)) {
                fail("flag --" + $name + " needs a value");
            }
            $val = $toks[$i + 1];
            $next = $i + 2;
        }
        $val = validateValue($a, $val);
        if ($a.action == "append") {
            $nr = appendList($nr, $a.name, $val);
        } else {
            $nr = setValue($nr, $a.name, $val);
        }
    }
    return Step{r: $nr, next: $next, stop: false};
}

# handleShort consumes one short-flag cluster (-x, -xvalue, -abc, -x value).
func handleShort(p as Parser, toks as list of string, i as int, r as Result) {
    def nr as Result init $r;
    def cluster as string init strings.substring($toks[$i], 1, len($toks[$i]));
    def j as int init 0;
    def took as bool init false;
    def extra as bool init false;
    while ($j < len($cluster) and not $took) {
        def ch as string init strings.substring($cluster, $j, $j + 1);
        if ($ch == "h") {
            $nr.helpText = usage($p);
            $nr.done = true;
            return Step{r: $nr, next: $i, stop: true};
        }
        def ai as int init findShort($p, $ch);
        if ($ai < 0) {
            fail("unknown flag -" + $ch);
        }
        def a as Arg init $p.args[$ai];
        if ($a.action == "count") {
            $nr = bumpCount($nr, $a.name);
            $j = $j + 1;
        } elseif ($a.typ == "bool") {
            $nr = setValue($nr, $a.name, "true");
            $j = $j + 1;
        } else {
            def val as string init "";
            if ($j + 1 < len($cluster)) {
                $val = strings.substring($cluster, $j + 1, len($cluster));
            } else {
                if ($i + 1 >= len($toks)) {
                    fail("flag -" + $ch + " needs a value");
                }
                $val = $toks[$i + 1];
                $extra = true;
            }
            $val = validateValue($a, $val);
            if ($a.action == "append") {
                $nr = appendList($nr, $a.name, $val);
            } else {
                $nr = setValue($nr, $a.name, $val);
            }
            $took = true;
        }
    }
    def next as int init $i + 1;
    if ($extra) {
        $next = $i + 2;
    }
    return Step{r: $nr, next: $next, stop: false};
}

# run walks the token list, filling the Result. It returns early with done=true
# for -h / --help / --version, recurses into a subcommand's parser, and finalises
# positionals / defaults / required-checks at the end.
func run(p as Parser, toks as list of string, r as Result) {
    def posQueue as list of string init [];
    def i as int init 0;
    def endFlags as bool init false;
    def nr as Result init $r;

    while ($i < len($toks)) {
        def t as string init $toks[$i];

        if (not $endFlags and $t == "--") {
            $endFlags = true;
            $i = $i + 1;
        } elseif (not $endFlags and isLongFlag($t)) {
            def sl as Step init handleLong($p, $toks, $i, $nr);
            $nr = $sl.r;
            $i = $sl.next;
            if ($sl.stop) {
                return $nr;
            }
        } elseif (not $endFlags and isShortFlag($t)) {
            def ss as Step init handleShort($p, $toks, $i, $nr);
            $nr = $ss.r;
            $i = $ss.next;
            if ($ss.stop) {
                return $nr;
            }
        } else {
            # a positional token, or a subcommand word
            if (len($p.commands) > 0) {
                def ci as int init findCommand($p, $t);
                if ($ci < 0) {
                    fail("unknown command '" + $t + "'");
                }
                def subToks as list of string init [];
                if ($i + 1 < len($toks)) {
                    $subToks = $toks[$i + 1..];
                }
                def subR as Result init run($p.commands[$ci].parser, $subToks, $nr);
                $subR.command = $t;
                return $subR;
            }
            $posQueue[] = $t;
            $i = $i + 1;
        }
    }

    return finalize($p, $posQueue, $nr);
}

# finalize assigns collected positionals per their nargs, then applies defaults
# and enforces required flags.
func finalize(p as Parser, posQueue as list of string, r as Result) {
    def nr as Result init $r;
    def qi as int init 0;

    def k as int init 0;
    while ($k < len($p.args)) {
        def a as Arg init $p.args[$k];
        if ($a.kind == "positional") {
            def remaining as int init len($posQueue) - $qi;
            if ($a.nargs == "*" or $a.nargs == "+") {
                if ($a.nargs == "+" and $remaining < 1) {
                    fail("positional " + $a.name + " needs at least one value");
                }
                def col as list of string init [];
                while ($qi < len($posQueue)) {
                    $col[] = validateValue($a, $posQueue[$qi]);
                    $qi = $qi + 1;
                }
                $nr.lists[$a.name] = $col;
                $nr.present[$a.name] = $remaining > 0;
            } elseif ($a.nargs == "?") {
                if ($remaining >= 1) {
                    $nr = setValue($nr, $a.name, validateValue($a, $posQueue[$qi]));
                    $qi = $qi + 1;
                } elseif ($a.hasDefault) {
                    $nr.values[$a.name] = $a.fallback;
                }
            } elseif (isIntStr($a.nargs)) {
                def n as int init convert.toInt($a.nargs);
                if ($remaining < $n) {
                    fail("positional " + $a.name + " needs " + convert.toString($n) + " values");
                }
                def coln as list of string init [];
                def c as int init 0;
                while ($c < $n) {
                    $coln[] = validateValue($a, $posQueue[$qi]);
                    $qi = $qi + 1;
                    $c = $c + 1;
                }
                $nr.lists[$a.name] = $coln;
                $nr.present[$a.name] = true;
            } else {
                if ($remaining < 1) {
                    fail("missing positional argument: " + $a.name);
                }
                $nr = setValue($nr, $a.name, validateValue($a, $posQueue[$qi]));
                $qi = $qi + 1;
            }
        }
        $k = $k + 1;
    }

    if ($qi < len($posQueue)) {
        fail("unexpected extra argument: '" + $posQueue[$qi] + "'");
    }

    # flags: apply defaults / enforce required for those not supplied
    def k2 as int init 0;
    while ($k2 < len($p.args)) {
        def a as Arg init $p.args[$k2];
        if ($a.kind == "flag") {
            def seen as bool init maps.has($nr.present, $a.name) and $nr.present[$a.name];
            if (not $seen) {
                if ($a.action == "count") {
                    if (not maps.has($nr.counts, $a.name)) {
                        $nr.counts[$a.name] = 0;
                    }
                } elseif ($a.action == "append") {
                    if (not maps.has($nr.lists, $a.name)) {
                        $nr.lists[$a.name] = [];
                    }
                } elseif ($a.required) {
                    fail("missing required flag: --" + $a.name);
                } elseif ($a.hasDefault) {
                    $nr.values[$a.name] = $a.fallback;
                }
            }
        }
        $k2 = $k2 + 1;
    }
    return $nr;
}

# --- accessors ---------------------------------------------------------------

/**
 * The string value of an argument (its provided value or default; "" if unset).
 * @param r {Result} the parse result
 * @param name {string} the argument name
 * @return {string} the value
 */
export func asString(r as Result, name as string) {
    if (maps.has($r.values, $name)) {
        return $r.values[$name];
    }
    return "";
}

/**
 * The int value of an argument (parsed from its stored string).
 * @param r {Result} the parse result
 * @param name {string} the argument name
 * @return {int} the value (0 if unset)
 */
export func asInt(r as Result, name as string) {
    def s as string init asString($r, $name);
    if ($s == "") {
        return 0;
    }
    return convert.toInt($s);
}

/**
 * The float value of an argument.
 * @param r {Result} the parse result
 * @param name {string} the argument name
 * @return {float} the value (0.0 if unset)
 */
export func asFloat(r as Result, name as string) {
    def s as string init asString($r, $name);
    if ($s == "") {
        return 0.0;
    }
    return convert.toFloat($s);
}

/**
 * The bool value of a flag (true when the stored value is "true").
 * @param r {Result} the parse result
 * @param name {string} the flag name
 * @return {bool} the value
 */
export func asBool(r as Result, name as string) {
    return asString($r, $name) == "true";
}

/**
 * The list value of an append flag or variadic positional.
 * @param r {Result} the parse result
 * @param name {string} the argument name
 * @return {list of string} the collected values ([] if none)
 */
export func asList(r as Result, name as string) {
    if (maps.has($r.lists, $name)) {
        return $r.lists[$name];
    }
    return [];
}

/**
 * The tally of a count flag.
 * @param r {Result} the parse result
 * @param name {string} the flag name
 * @return {int} the number of occurrences (0 if none)
 */
export func count(r as Result, name as string) {
    if (maps.has($r.counts, $name)) {
        return $r.counts[$name];
    }
    return 0;
}

/**
 * Whether an argument was actually supplied on the command line (as opposed to
 * taking its default).
 * @param r {Result} the parse result
 * @param name {string} the argument name
 * @return {bool} true if supplied
 */
export func has(r as Result, name as string) {
    return maps.has($r.present, $name) and $r.present[$name];
}

# --- subcommand dispatch -----------------------------------------------------

/**
 * Dispatch the parsed subcommand to its handler. `handlers` maps a subcommand
 * name to a `func` value `func(r as Result)`; the handler for `r.command` is
 * called with the whole `Result` (so it reads its own args with `args.asString`
 * / `asInt` / ...), and its return value is passed back - so a handler may
 * return an exit code. A `Result` with `done` set (a handled `--help` /
 * `--version`) dispatches nothing and returns `null`; a caller usually checks
 * `r.done` and prints `r.helpText` before calling this. A missing handler for
 * the selected subcommand, or a selection of none, is a catchable error.
 *
 * The handlers are ordinary `func` values, not names - a func value called here
 * runs in the entry program's own context, so it resolves its own imports.
 * @param r {Result} the parse result
 * @param handlers {map of string to func} subcommand name -> its handler
 * @return the handler's return value (`null` when `r.done`)
 * @throws {Error} kind "args" when no subcommand was selected or none matches
 */
export func dispatch(r as Result, handlers as map of string to func) {
    if ($r.done) {
        return;
    }
    if ($r.command == "") {
        fail("dispatch: no subcommand was selected (check r.command, or the parser declares no commands)");
    }
    if (not maps.has($handlers, $r.command)) {
        fail("dispatch: no handler for subcommand \"" + $r.command + "\"");
    }
    def fn as func init $handlers[$r.command];
    return $fn($r);
}

# --- usage generation --------------------------------------------------------

/**
 * The generated usage / help text for a parser (what `--help` prints).
 * @param p {Parser} the parser
 * @return {string} the multi-line help text
 */
export func usage(p as Parser) {
    def out as string init "Usage: " + $p.prog;
    if (len($p.args) > 0) {
        $out = $out + " [options]";
    }
    def pi as int init 0;
    while ($pi < len($p.args)) {
        def a as Arg init $p.args[$pi];
        if ($a.kind == "positional") {
            $out = $out + " " + positionalUsage($a);
        }
        $pi = $pi + 1;
    }
    if (len($p.commands) > 0) {
        $out = $out + " <command>";
    }
    $out = $out + "\n";
    if ($p.help != "") {
        $out = $out + "\n" + $p.help + "\n";
    }

    def opts as string init "";
    def poss as string init "";
    def ci as int init 0;
    while ($ci < len($p.args)) {
        def a as Arg init $p.args[$ci];
        if ($a.kind == "flag") {
            $opts = $opts + "  " + flagUsage($a) + describeTail($a) + "\n";
        } else {
            $poss = $poss + "  " + $a.name + "    " + $a.help + describeChoices($a) + "\n";
        }
        $ci = $ci + 1;
    }
    if ($poss != "") {
        $out = $out + "\nPositional arguments:\n" + $poss;
    }
    $out = $out + "\nOptions:\n";
    if ($p.version != "") {
        $out = $out + "  --version    show version and exit\n";
    }
    $out = $out + "  -h, --help    show this help and exit\n" + $opts;

    if (len($p.commands) > 0) {
        $out = $out + "\nCommands:\n";
        def cj as int init 0;
        while ($cj < len($p.commands)) {
            $out = $out + "  " + $p.commands[$cj].name + "    " + $p.commands[$cj].help + "\n";
            $cj = $cj + 1;
        }
    }
    return $out;
}

func positionalUsage(a as Arg) {
    if ($a.nargs == "*" or $a.nargs == "?") {
        return "[" + $a.name + "]";
    }
    if ($a.nargs == "+") {
        return $a.name + " [" + $a.name + " ...]";
    }
    return $a.name;
}

func flagUsage(a as Arg) {
    def s as string init "";
    if ($a.short != "") {
        $s = "-" + $a.short + ", ";
    }
    $s = $s + "--" + $a.name;
    if ($a.action != "count" and $a.typ != "bool") {
        $s = $s + " " + strings.upper($a.name);
    }
    return $s;
}

func describeTail(a as Arg) {
    def s as string init "    " + $a.help;
    $s = $s + describeChoices($a);
    if ($a.required) {
        $s = $s + " (required)";
    } elseif ($a.hasDefault and $a.fallback != "" and $a.typ != "bool" and $a.action != "count") {
        $s = $s + " (default: " + $a.fallback + ")";
    }
    return $s;
}

func describeChoices(a as Arg) {
    if (len($a.choices) > 0) {
        return " (one of: " + strings.join($a.choices, ", ") + ")";
    }
    return "";
}
