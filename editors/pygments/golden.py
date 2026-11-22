#!/usr/bin/env python3
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
"""Compare the lexer's tokens for example.j with example.tokens.

    golden.py           exit 1 and show the first differences on a mismatch
    golden.py --update  rewrite example.tokens from the current lexer

Each line of example.tokens is one token: the repr of its text, padded, then
its type without the `Token.` prefix.
"""

import difflib
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

from jennifer_lexer import JenniferLexer  # noqa: E402


def render(src):
    lines = []
    for tok, val in JenniferLexer().get_tokens(src):
        lines.append("%-32s %s\n" % (repr(val), str(tok)[len("Token."):]))
    return "".join(lines)


def main():
    with open(os.path.join(HERE, "example.j"), encoding="utf-8") as f:
        got = render(f.read())
    path = os.path.join(HERE, "example.tokens")
    if "--update" in sys.argv[1:]:
        with open(path, "w", encoding="utf-8") as f:
            f.write(got)
        print("wrote %s" % path)
        return
    with open(path, encoding="utf-8") as f:
        want = f.read()
    if got == want:
        print("example.tokens matches")
        return
    diff = difflib.unified_diff(want.splitlines(True), got.splitlines(True),
                                "example.tokens", "lexer")
    sys.stdout.writelines(list(diff)[:60])
    sys.exit(1)


if __name__ == "__main__":
    main()
