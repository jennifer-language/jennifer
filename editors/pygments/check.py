#!/usr/bin/env python3
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
"""Check the Pygments lexer against the interpreter's own lexer.

    check.py [--jennifer BIN] FILE_OR_DIR...

For every `.j` file (directories are walked, skipping vendor/, .git/ and
node_modules/): the lexer must emit no Error token, and every token that
`jennifer tokens` reports must start a Pygments token of a matching class
(an IDENT a name, an INT a number, a keyword a keyword, ...). Prints one line
per finding and a summary; exits 1 on any finding.
"""

import argparse
import os
import re
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from jennifer_lexer import JenniferLexer  # noqa: E402
from pygments.token import (  # noqa: E402
    Comment,
    Error,
    Keyword,
    Name,
    Number,
    Operator,
    Punctuation,
    String,
)

SKIP_DIRS = {"vendor", ".git", "node_modules"}
LINE = re.compile(r'^(\d+):(\d+)\s+([A-Z_]+)(?:\s+"(.*)")?$')

PUNCT = {"LPAREN", "RPAREN", "LBRACE", "RBRACE", "LBRACKET", "RBRACKET",
         "COMMA", "SEMI", "COLON", "DOT"}
WORD_OPS = {"AND", "OR", "NOT"}
OPERATORS = {"ASSIGN", "PLUS", "MINUS", "STAR", "SLASH", "DIV", "PERCENT",
             "EQ", "NEQ", "LT", "LE", "GT", "GE", "SHL", "SHR", "BIT_AND",
             "BIT_OR", "BIT_XOR", "BIT_NOT", "DOTDOT"}


def allowed(kind):
    """Token classes a Pygments token may have where `kind` starts."""
    if kind in ("IDENT",):
        # `channel` in `channel of T` is a type keyword.
        return (Name, Keyword.Type)
    if kind == "VARREF":
        return (Name.Variable,)
    if kind in ("INT", "FLOAT"):
        return (Number,)
    if kind.startswith("STRING") and not kind.endswith("_TYPE"):
        return (String,)
    if kind.startswith("COMMENT"):
        return (Comment,)
    if kind in PUNCT:
        return (Punctuation,)
    if kind in WORD_OPS:
        return (Operator.Word,)
    if kind == "LEN":
        return (Name.Builtin,)
    if kind in OPERATORS:
        return (Operator,)
    if kind.isalpha() or kind.endswith("_TYPE"):
        return (Keyword,)
    return (Operator,)


NAME_AT = re.compile(r"[A-Za-z][A-Za-z0-9]*")


def name_position(src, pos):
    """True where a keyword-spelled word is a name (editors/README.md):
    after a `.`, before `.name` (a namespace), after `use`, or in a field
    position (`to as T`, `{to: ...}`)."""
    m = NAME_AT.match(src, pos)
    if not m:
        return False
    before = src[:pos].rstrip()
    after = src[m.end():]
    return (before.endswith(".") or re.match(r"\.[A-Za-z]", after) is not None
            or re.search(r"\buse$", before) is not None
            or re.match(r"\s*:|\s+as\b", after) is not None)


def files(paths):
    for p in paths:
        if os.path.isfile(p):
            yield p
            continue
        for d, dirs, names in os.walk(p):
            dirs[:] = sorted(x for x in dirs if x not in SKIP_DIRS)
            for n in sorted(names):
                if n.endswith(".j"):
                    yield os.path.join(d, n)


def interpreter_tokens(binary, path):
    r = subprocess.run([binary, "tokens", path], capture_output=True,
                       text=True)
    if r.returncode != 0:
        return None
    out = []
    for line in r.stdout.splitlines():
        m = LINE.match(line.strip())
        if m and m.group(3) not in ("EOF", "BLANK_LINE"):
            out.append((int(m.group(1)), int(m.group(2)), m.group(3)))
    return out


def check(binary, path):
    with open(path, encoding="utf-8") as f:
        src = f.read()
    lexer = JenniferLexer(ensurenl=False, stripnl=False)
    starts = {}
    findings = []
    for pos, tok, val in lexer.get_tokens_unprocessed(src):
        if tok in Error:
            findings.append("%s: Error token %r at offset %d" % (path, val, pos))
        starts.setdefault(pos, tok)
    toks = interpreter_tokens(binary, path)
    if toks is None:
        return findings, False
    line_off = [0]
    for line in src.split("\n"):
        line_off.append(line_off[-1] + len(line) + 1)
    for ln, col, kind in toks:
        pos = line_off[ln - 1] + col - 1
        tok = starts.get(pos)
        if tok is None:
            findings.append("%s:%d:%d: %s starts inside a Pygments token"
                            % (path, ln, col, kind))
        elif not any(tok in c for c in allowed(kind)) and not (
                tok in Name and name_position(src, pos)):
            findings.append("%s:%d:%d: %s lexed as %s"
                            % (path, ln, col, kind, tok))
    return findings, True


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--jennifer", default=os.environ.get("JENNIFER",
                                                         "jennifer"))
    ap.add_argument("paths", nargs="+")
    args = ap.parse_args()
    total = checked = skipped = 0
    for path in files(args.paths):
        findings, ok = check(args.jennifer, path)
        if ok:
            checked += 1
        else:
            skipped += 1
        for f in findings:
            print(f)
        total += len(findings)
    print("%d file(s) checked, %d not tokenisable by the interpreter, "
          "%d finding(s)" % (checked, skipped, total), file=sys.stderr)
    sys.exit(1 if total else 0)


if __name__ == "__main__":
    main()
