# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
"""A Pygments lexer for Jennifer (https://jennifer-lang.dev/).

The token classes follow the other definitions in jennifer/editors (see
editors/README.md, "Contributing another editor"): `$` variables,
`UPPER_CASE` constants, `CamelCase` types, `ns.name` calls with the prefix as
a namespace, nested `/* */` comments, `/** */` docblocks with `@tags`, raw
`'...'` and cooked `"..."` strings with `{expr}` slots.
"""

import re

from pygments.lexer import RegexLexer, bygroups, include, words
from pygments.token import (
    Comment,
    Error,
    Keyword,
    Name,
    Number,
    Operator,
    Punctuation,
    String,
    Whitespace,
)

__all__ = ["JenniferLexer"]

TYPES = ("int", "float", "string", "bool", "bytes", "list", "map", "task")


class JenniferLexer(RegexLexer):
    """Lexer for Jennifer source (`.j` files and `#!... jennifer` scripts)."""

    name = "Jennifer"
    url = "https://jennifer-lang.dev/"
    aliases = ["jennifer"]
    filenames = ["*.j"]
    mimetypes = ["text/x-jennifer"]
    # Pygments' Objective-J lexer claims `*.j` too. Without source to
    # analyse, the higher priority wins the file name; with source,
    # analyse_text decides.
    priority = 0.1

    tokens = {
        "root": [
            (r"\A#!.*\n", Comment.Hashbang),
            (r"\s+", Whitespace),
            (r"#\s*pragma-jennifer-[a-z]+:[^\n]*", Comment.Preproc),
            (r"#\s*lint-disable(-file)?:[^\n]*", Comment.Special),
            (r"#[^\n]*", Comment.Single),
            (r"/\*\*(?!/)", Comment.Multiline, "docblock"),
            (r"/\*", Comment.Multiline, "comment"),
            (r'"', String.Double, "cooked"),
            (r"'", String.Single, "raw"),
            (r"0[xX][0-9a-fA-F](_?[0-9a-fA-F])*", Number.Hex),
            (r"0[oO][0-7](_?[0-7])*", Number.Oct),
            (r"0[bB][01](_?[01])*", Number.Bin),
            (
                r"[0-9](_?[0-9])*"
                r"(\.[0-9](_?[0-9])*([eE][+\-]?[0-9]+)?|[eE][+\-]?[0-9]+)",
                Number.Float,
            ),
            (r"[0-9](_?[0-9])*", Number.Integer),
            (r"\.\.", Operator),
            # After a `.`, any spelling is a name, keywords included.
            (
                r"(\.)([A-Za-z][A-Za-z0-9_]*)(?=\s*\()",
                bygroups(Punctuation, Name.Function),
            ),
            (
                r"(\.)([A-Z][A-Z0-9]*(?:_[A-Z][A-Z0-9]*)*)\b",
                bygroups(Punctuation, Name.Constant),
            ),
            (r"(\.)([A-Z][A-Za-z0-9]*)", bygroups(Punctuation, Name.Class)),
            (r"(\.)([a-z][A-Za-z0-9]*)", bygroups(Punctuation, Name.Property)),
            (r"\$[A-Za-z][A-Za-z0-9]*", Name.Variable),
            (
                r"\b(use)(\s+)([a-z][A-Za-z0-9]*)",
                bygroups(Keyword.Namespace, Whitespace, Name.Namespace),
            ),
            (
                r"\b(as)(\s+)(?!(?:%s|func|null|true|false)\b)"
                r"([a-z][A-Za-z0-9]*)(?=\s*;)" % "|".join(TYPES),
                bygroups(Keyword.Declaration, Whitespace, Name.Namespace),
            ),
            (r"\b[a-z][A-Za-z0-9]*(?=\.[A-Za-z])", Name.Namespace),
            # A field position: `to as Point`, `Written{bytes: 0}`.
            (r"\b[a-z][A-Za-z0-9]*(?=\s*:|\s+as\b)", Name),
            (words(("use", "include", "import"), prefix=r"\b", suffix=r"\b"),
             Keyword.Namespace),
            (
                words(
                    ("export", "def", "const", "func", "struct", "enum", "as",
                     "of", "to", "init"),
                    prefix=r"\b",
                    suffix=r"\b",
                ),
                Keyword.Declaration,
            ),
            (
                words(
                    ("if", "elseif", "else", "while", "for", "in", "repeat",
                     "until", "match", "when", "break", "continue", "return",
                     "exit", "try", "catch", "throw", "defer", "errdefer",
                     "spawn"),
                    prefix=r"\b",
                    suffix=r"\b",
                ),
                Keyword,
            ),
            (words(("and", "or", "not"), prefix=r"\b", suffix=r"\b"),
             Operator.Word),
            (words(("true", "false", "null"), prefix=r"\b", suffix=r"\b"),
             Keyword.Constant),
            (words(TYPES, prefix=r"\b", suffix=r"\b"), Keyword.Type),
            (r"\bchannel(?=\s+of\b)", Keyword.Type),
            (r"\blen\b", Name.Builtin),
            (r"\b[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*\b", Name.Constant),
            (r"\b[A-Z][A-Za-z0-9]*\b", Name.Class),
            (r"[a-z][A-Za-z0-9]*(?=\s*\()", Name.Function),
            (r"[A-Za-z][A-Za-z0-9_]*", Name),
            (r"//|<<|>>|<=|>=|==|!=|[\-+*/%&|^~<>=]", Operator),
            (r"[{}()\[\];,.:]", Punctuation),
        ],
        "comment": [
            (r"/\*", Comment.Multiline, "#push"),
            (r"\*/", Comment.Multiline, "#pop"),
            (r"[^*/]+", Comment.Multiline),
            (r"[*/]", Comment.Multiline),
        ],
        "docblock": [
            (r"(?<=\s)@[a-z]+\b", Comment.Special),
            (r"/\*", Comment.Multiline, "comment"),
            (r"\*/", Comment.Multiline, "#pop"),
            (r"[^*/@]+", Comment.Multiline),
            (r"[*/@]", Comment.Multiline),
        ],
        "cooked": [
            (r'"', String.Double, "#pop"),
            (r"""\\([nrt\\"'0{}]|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})""",
             String.Escape),
            (r"\\.?", Error),
            (r"\{", String.Interpol, "interpolation"),
            (r"\}", Error),
            (r'[^"\\{}]+', String.Double),
        ],
        "raw": [
            (r"'", String.Single, "#pop"),
            (r"[^']+", String.Single),
        ],
        # A `{expr}` slot. A map literal inside it opens `braces`, so the
        # slot ends at its own `}`.
        "interpolation": [
            (r"\}", String.Interpol, "#pop"),
            (r"\{", Punctuation, "braces"),
            include("root"),
        ],
        "braces": [
            (r"\}", Punctuation, "#pop"),
            (r"\{", Punctuation, "#push"),
            include("root"),
        ],
    }

    _SHEBANG = re.compile(r"\A#!.*\bjennifer\b")
    _PRAGMA = re.compile(r"^#\s*pragma-jennifer-version:", re.M)
    _USE = re.compile(
        r"^\s*use\s+[a-z][A-Za-z0-9]*(\s+as\s+[a-z][A-Za-z0-9]*)?\s*;", re.M
    )
    _DEF = re.compile(
        r"^\s*(export\s+)?def\s+(const\s+)?[A-Za-z][A-Za-z0-9_]*\s+as\s+"
        r"(%s|func)\b" % "|".join(TYPES),
        re.M,
    )

    def analyse_text(text):
        if JenniferLexer._SHEBANG.search(text):
            return 1.0
        if JenniferLexer._PRAGMA.search(text):
            return 1.0
        if JenniferLexer._USE.search(text):
            return 0.5
        if JenniferLexer._DEF.search(text):
            return 0.3
        return 0.0
