// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
//
// highlight.js language definition for Jennifer (.j).
// Written to the common highlight.js API (className + contains + hljs.COMMENT)
// so it works on both v10 (mdBook bundles 10.1.1) and v11. Works as a CommonJS
// module (module.exports = factory) and self-registers when a global `hljs` is
// present (e.g. appended to a highlight.js bundle). See editors/README.md.

(function (factory) {
  if (typeof hljs !== "undefined" && hljs.registerLanguage) {
    hljs.registerLanguage("jennifer", factory);
  }
  if (typeof module !== "undefined" && module.exports) {
    module.exports = factory;
  }
})(function (hljs) {
  var KEYWORDS = {
    keyword:
      "export def const func struct enum use include import as of to init " +
      "if elseif else while for in repeat until match when break continue return exit " +
      "try catch throw defer errdefer spawn and or not",
    built_in: "len",
    type: "int float string bool bytes list map task",
    literal: "true false null",
  };

  var VARIABLE = { className: "variable", begin: /\$[A-Za-z][A-Za-z0-9]*/ };

  // UPPER_CASE constant names (MAX, MAX_RETRIES).
  var CONSTANT = {
    className: "symbol",
    begin: /\b[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*\b/,
  };

  // CamelCase struct, enum and variant names (Point, Shape.Circle, Error).
  var TYPE_NAME = { className: "type", begin: /\b[A-Z][A-Za-z0-9]*\b/ };

  var NUMBER = {
    className: "number",
    variants: [
      { begin: /\b0[xX][0-9a-fA-F](_?[0-9a-fA-F])*\b/ },
      { begin: /\b0[oO][0-7](_?[0-7])*\b/ },
      { begin: /\b0[bB][01](_?[01])*\b/ },
      { begin: /\b[0-9](_?[0-9])*(\.[0-9](_?[0-9])*([eE][+-]?[0-9]+)?|[eE][+-]?[0-9]+)\b/ },
      { begin: /\b[0-9](_?[0-9])*\b/ },
    ],
  };

  // Single-quoted is raw: backslashes are literal and there is no interpolation.
  var STRING_RAW = {
    className: "string",
    begin: /'/,
    end: /'/,
  };

  // The name after a `.` is never a keyword: `strings.repeat(...)`,
  // `json.map()`, `$e.to`, `$s.when`.
  var MEMBER = {
    variants: [
      { className: "title", begin: /(?<=[^.]\.)[A-Za-z][A-Za-z0-9_]*(?=\s*\()/ },
      { className: "symbol", begin: /(?<=[^.]\.)[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*\b/ },
      { className: "type", begin: /(?<=[^.]\.)[A-Z][A-Za-z0-9]*/ },
      { begin: /(?<=[^.]\.)[a-z][A-Za-z0-9]*/, relevance: 0 },
    ],
  };

  // Namespace prefix in a qualified call: the `io` in `io.printf(...)`, the
  // `task` in `task.wait(...)`.
  var NAMESPACE = {
    className: "built_in",
    begin: /\b[a-z][A-Za-z0-9]*(?=\.[A-Za-z])/,
  };

  // The library after `use` is a namespace, even when it is also a type name
  // (`use task;`), and so is an alias (`use strings as str;`,
  // `import "x.j" as x;`).
  var USE = {
    beginKeywords: "use",
    end: /;/,
    keywords: "use as",
    contains: [{ className: "built_in", begin: /\b(?!as\b)[a-z][A-Za-z0-9]*/ }],
  };
  var ALIAS = {
    className: "built_in",
    begin: /(?<=\bas\s+)(?!(?:int|float|string|bool|bytes|list|map|task|func)\b)[a-z][A-Za-z0-9]*(?=\s*;)/,
  };

  // A field or binding name is never a keyword: `to as Point`,
  // `Written{bytes: 0}`.
  var FIELD = {
    begin: /\b[a-z][A-Za-z0-9]*(?=\s*:|\s+as\b)/,
    relevance: 0,
  };

  // A function name immediately before `(`; a keyword before `(` (`if (`,
  // `len(`) stays a keyword.
  var FUNCTION = {
    className: "title",
    begin: new RegExp("\\b(?!(?:" + [KEYWORDS.keyword, KEYWORDS.built_in, KEYWORDS.type, KEYWORDS.literal]
      .join(" ").split(" ").join("|") + ")\\b)[a-z][A-Za-z0-9]*(?=\\s*\\()"),
  };

  // `channel` is a type keyword only in `channel of T`.
  var CHANNEL_TYPE = { className: "type", begin: /\bchannel(?=\s+of\b)/ };

  // Block comments nest; a `/** */` docblock highlights its `@tags`.
  var BLOCK_COMMENT = hljs.COMMENT(/\/\*/, /\*\//, { contains: ["self"] });
  var DOC_COMMENT = hljs.COMMENT(/\/\*\*(?!\/)/, /\*\//, {
    contains: [{ className: "doctag", begin: /(?<=\s)@[a-z]+\b/ }, BLOCK_COMMENT],
  });

  // `# pragma-jennifer-version: ...` / `# lint-disable: ...` directives.
  var DIRECTIVE = {
    className: "meta",
    begin: /#\s*(pragma-jennifer-[a-z]+|lint-disable(-file)?):/,
    end: /$/,
  };

  // Double-quoted is cooked: escape sequences are highlighted and an unescaped
  // `{expr}` is an interpolation slot (rendered as an embedded `subst`). `\{` /
  // `\}` are literal-brace escapes (matched before the slot rule so they do not
  // open a slot). A slot holds one expression, which may contain strings of
  // either kind and a `{...}` map literal.
  var SLOT_CONTENT = [];
  var BRACES = { begin: /\{/, end: /\}/, keywords: KEYWORDS, contains: SLOT_CONTENT };
  var SUBST = { className: "subst", begin: /\{/, end: /\}/, keywords: KEYWORDS, contains: SLOT_CONTENT };
  var STRING_COOKED = {
    className: "string",
    begin: /"/,
    end: /"/,
    contains: [
      { begin: /\\([nrt\\"'0{}]|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})/ },
      SUBST,
    ],
  };
  SLOT_CONTENT.push(STRING_COOKED, STRING_RAW, NUMBER, MEMBER, ALIAS, NAMESPACE, FIELD,
    VARIABLE, CHANNEL_TYPE, CONSTANT, TYPE_NAME, FUNCTION, BRACES);

  // REPL transcript prompts (`>>> ` input, `... ` continuation) at line
  // start, so a pasted `jennifer repl` session highlights: the prompt shows
  // as meta and the rest of the line is highlighted as Jennifer. Real source
  // never begins a line with these, so this is inert in ordinary code.
  var PROMPT = { className: "meta", begin: /^(>>>|\.\.\.)\s/ };

  return {
    name: "Jennifer",
    aliases: ["j"],
    keywords: KEYWORDS,
    contains: [
      PROMPT,
      { className: "meta", begin: /^#!.*jennifer.*$/, relevance: 10 },
      DIRECTIVE,
      hljs.COMMENT("#", "$"),
      DOC_COMMENT,
      BLOCK_COMMENT,
      STRING_COOKED,
      STRING_RAW,
      NUMBER,
      MEMBER,
      USE,
      ALIAS,
      NAMESPACE,
      FIELD,
      VARIABLE,
      CHANNEL_TYPE,
      CONSTANT,
      TYPE_NAME,
      FUNCTION,
    ],
  };
});
