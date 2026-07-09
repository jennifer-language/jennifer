// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>
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
      "def const func struct use include import as of to init " +
      "if elseif else while for in repeat until break continue return exit " +
      "try catch throw spawn and or not",
    built_in: "len",
    type: "int float string bool bytes list map task",
    literal: "true false null",
  };

  var VARIABLE = { className: "variable", begin: /\$[A-Za-z]+/ };

  // UPPER_CASE constant names (MAX, MAX_RETRIES).
  var CONSTANT = { className: "symbol", begin: /\b[A-Z]+(_[A-Z]+)*\b/ };

  var NUMBER = {
    className: "number",
    variants: [
      { begin: /\b0[xX][0-9a-fA-F][0-9a-fA-F_]*\b/ },
      { begin: /\b0[oO][0-7][0-7_]*\b/ },
      { begin: /\b0[bB][01][01_]*\b/ },
      { begin: /\b[0-9][0-9_]*\.[0-9][0-9_]*\b/ },
      { begin: /\b[0-9][0-9_]*\b/ },
    ],
  };

  var STRING = {
    className: "string",
    variants: [
      { begin: /"/, end: /"/ },
      { begin: /'/, end: /'/ },
    ],
    contains: [{ begin: /\\[nrt\\"'0]/ }],
  };

  // Namespace prefix in a qualified call: the `io` in `io.printf(...)`.
  var NAMESPACE = { className: "built_in", begin: /\b[A-Za-z]+(?=\.[A-Za-z])/ };

  // A method name immediately before `(`.
  var FUNCTION = { className: "title", begin: /\b[A-Za-z]+(?=\s*\()/ };

  return {
    name: "Jennifer",
    aliases: ["j"],
    keywords: KEYWORDS,
    contains: [
      hljs.COMMENT("#", "$"),
      hljs.COMMENT(/\/\*/, /\*\//),
      STRING,
      NUMBER,
      VARIABLE,
      NAMESPACE,
      CONSTANT,
      FUNCTION,
    ],
  };
});
