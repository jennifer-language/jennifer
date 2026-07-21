# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# tengine_test.j - white-box tests for tengine.j. Run with:
#
#     jennifer test modules/tengine_test.j
#
# The overlay splices tengine.j in first, so these tests reach its private
# helpers (trimMarkers, titleCase) and the exported newSet / add / render by bare
# identifier. tengine.j already `use`s json / strings / lists / convert, so the
# overlay only adds testing.
use testing;

# oneSet wraps a single template source under the name "main".
func oneSet(src as string) {
    return add(newSet(), "main", $src);
}

func testOutput() {
    testing.assertEqual(render(oneSet("{{ .a }}"), "main", json.decode("{\"a\":\"Z\"}")), "Z");
}

func testMissingKeyIsEmpty() {
    testing.assertEqual(render(oneSet("x{{ .nope }}y"), "main", json.decode("{\"a\":\"Z\"}")), "xy");
}

func testDotIsCurrentNode() {
    testing.assertEqual(render(oneSet("[{{ . }}]"), "main", json.decode("\"hi\"")), "[hi]");
}

func testIntAndFloatOutput() {
    testing.assertEqual(render(oneSet("{{ .n }}/{{ .f }}"), "main", json.decode("{\"n\":42,\"f\":1.5}")), "42/1.5");
}

func testIfTrue() {
    testing.assertEqual(render(oneSet("{{ if .show }}Y{{ else }}N{{ end }}"), "main", json.decode("{\"show\":true}")), "Y");
}

func testIfFalse() {
    testing.assertEqual(render(oneSet("{{ if .show }}Y{{ else }}N{{ end }}"), "main", json.decode("{\"show\":false}")), "N");
}

func testRange() {
    testing.assertEqual(render(oneSet("{{ range .xs }}[{{ . }}]{{ end }}"), "main", json.decode("{\"xs\":[\"a\",\"b\",\"c\"]}")), "[a][b][c]");
}

func testRangeEmptyUsesElse() {
    testing.assertEqual(render(oneSet("{{ range .xs }}[{{ . }}]{{ else }}none{{ end }}"), "main", json.decode("{\"xs\":[]}")), "none");
}

func testRangeOfObjects() {
    testing.assertEqual(render(oneSet("{{ range .ps }}<{{ .t }}>{{ end }}"), "main", json.decode("{\"ps\":[{\"t\":\"one\"},{\"t\":\"two\"}]}")), "<one><two>");
}

func testRangeOverMap() {
    testing.assertEqual(render(oneSet("{{ range .m }}{{ . }};{{ end }}"), "main", json.decode("{\"m\":{\"a\":\"1\",\"b\":\"2\"}}")), "1;2;");
}

func testWith() {
    testing.assertEqual(render(oneSet("{{ with .obj }}{{ .name }}{{ end }}"), "main", json.decode("{\"obj\":{\"name\":\"Doc\"}}")), "Doc");
}

func testPipeUpper() {
    testing.assertEqual(render(oneSet("{{ .a | upper }}"), "main", json.decode("{\"a\":\"hi\"}")), "HI");
}

func testPipeTitle() {
    testing.assertEqual(render(oneSet("{{ .t | title }}"), "main", json.decode("{\"t\":\"time travel\"}")), "Time Travel");
}

func testPipeChain() {
    testing.assertEqual(render(oneSet("{{ .t | upper | trim }}"), "main", json.decode("{\"t\":\"  hi  \"}")), "HI");
}

func testPipeHtmlEscape() {
    testing.assertEqual(render(oneSet("{{ .h | html }}"), "main", json.decode("{\"h\":\"<b>&\"}")), "&lt;b&gt;&amp;");
}

func testPipeUrlize() {
    testing.assertEqual(render(oneSet("{{ .t | urlize }}"), "main", json.decode("{\"t\":\"Time Travel\"}")), "time-travel");
}

func testNestedIfInRange() {
    testing.assertEqual(render(oneSet("{{ range .xs }}{{ if .on }}{{ .v }}{{ end }}{{ end }}"), "main",
        json.decode("{\"xs\":[{\"on\":true,\"v\":\"a\"},{\"on\":false,\"v\":\"b\"}]}")), "a");
}

func testLayoutInheritance() {
    def set as Set init add(newSet(), "base", "<h1>{{ .t }}</h1>{{ template \"content\" . }}");
    $set = add($set, "page", "{{ define \"content\" }}<p>{{ .t }}</p>{{ end }}");
    testing.assertEqual(render($set, "base", json.decode("{\"t\":\"Hi\"}")), "<h1>Hi</h1><p>Hi</p>");
}

func testBlockDefault() {
    def set as Set init add(newSet(), "base", "{{ block \"content\" . }}DEFAULT{{ end }}");
    testing.assertEqual(render($set, "base", json.decode("null")), "DEFAULT");
}

func testBlockOverride() {
    def set as Set init add(newSet(), "base", "{{ block \"content\" . }}DEFAULT{{ end }}");
    $set = add($set, "page", "{{ define \"content\" }}OVER{{ end }}");
    testing.assertEqual(render($set, "base", json.decode("null")), "OVER");
}

func testComment() {
    testing.assertEqual(render(oneSet("a{{/* hidden */}}b"), "main", json.decode("null")), "ab");
}

func testTrimMarkers() {
    def src as string init "<ul>\n  {{- range .xs }}\n  <li>{{ . }}</li>\n  {{- end }}\n</ul>";
    testing.assertEqual(render(oneSet($src), "main", json.decode("{\"xs\":[\"a\",\"b\"]}")),
        "<ul>\n  <li>a</li>\n  <li>b</li>\n</ul>");
}

func testTrimMarkerHelper() {
    # {{- x -}} collapses whitespace on both sides
    testing.assertEqual(trimMarkers("a\n  {{- .x -}}  \nb"), "a{{ .x }}b");
    testing.assertEqual(titleCase("hello world"), "Hello World");
}

# --- CMS features: $ root, conditionals, variables, more pipes -------------

func testRootAccessInRange() {
    testing.assertEqual(render(oneSet("{{ range .xs }}{{ $.site }}:{{ . }} {{ end }}"), "main",
        json.decode("{\"site\":\"S\",\"xs\":[\"a\",\"b\"]}")), "S:a S:b ");
}

func testEqNe() {
    testing.assertEqual(render(oneSet("{{ if eq .role \"admin\" }}Y{{ else }}N{{ end }}"), "main", json.decode("{\"role\":\"admin\"}")), "Y");
    testing.assertEqual(render(oneSet("{{ if ne .role \"admin\" }}Y{{ else }}N{{ end }}"), "main", json.decode("{\"role\":\"user\"}")), "Y");
}

func testCompareNumbers() {
    testing.assertEqual(render(oneSet("{{ if gt .n 3 }}big{{ else }}small{{ end }}"), "main", json.decode("{\"n\":5}")), "big");
    testing.assertEqual(render(oneSet("{{ if le .n 5 }}ok{{ end }}"), "main", json.decode("{\"n\":5}")), "ok");
}

func testAndOrNot() {
    testing.assertEqual(render(oneSet("{{ if and .a (not .b) }}Y{{ end }}"), "main", json.decode("{\"a\":true,\"b\":false}")), "Y");
    testing.assertEqual(render(oneSet("{{ if or (eq .s \"a\") (eq .s \"b\") }}hit{{ end }}"), "main", json.decode("{\"s\":\"b\"}")), "hit");
    testing.assertEqual(render(oneSet("{{ if or (eq .s \"a\") (eq .s \"b\") }}hit{{ else }}miss{{ end }}"), "main", json.decode("{\"s\":\"c\"}")), "miss");
}

func testElseIf() {
    def tpl as string init "{{ if eq .x 1 }}one{{ else if eq .x 2 }}two{{ else if eq .x 3 }}three{{ else }}other{{ end }}";
    testing.assertEqual(render(oneSet($tpl), "main", json.decode("{\"x\":1}")), "one");
    testing.assertEqual(render(oneSet($tpl), "main", json.decode("{\"x\":2}")), "two");
    testing.assertEqual(render(oneSet($tpl), "main", json.decode("{\"x\":3}")), "three");
    testing.assertEqual(render(oneSet($tpl), "main", json.decode("{\"x\":9}")), "other");
}

func testRangeIndex() {
    testing.assertEqual(render(oneSet("{{ range $i, $e := .xs }}{{ $i }}:{{ $e }} {{ end }}"), "main",
        json.decode("{\"xs\":[\"x\",\"y\"]}")), "0:x 1:y ");
}

func testAssignVariable() {
    testing.assertEqual(render(oneSet("{{ $t := .title }}{{ range .xs }}{{ $t }}/{{ . }} {{ end }}"), "main",
        json.decode("{\"title\":\"T\",\"xs\":[\"a\",\"b\"]}")), "T/a T/b ");
}

func testPipeDefault() {
    testing.assertEqual(render(oneSet("{{ .missing | default \"fallback\" }}"), "main", json.decode("{\"x\":1}")), "fallback");
    testing.assertEqual(render(oneSet("{{ .name | default \"fallback\" }}"), "main", json.decode("{\"name\":\"Ada\"}")), "Ada");
}

func testPipeTruncate() {
    testing.assertEqual(render(oneSet("{{ .s | truncate 5 }}"), "main", json.decode("{\"s\":\"hello world\"}")), "hello...");
    testing.assertEqual(render(oneSet("{{ .s | truncate 20 }}"), "main", json.decode("{\"s\":\"short\"}")), "short");
}

func testPipeJoin() {
    testing.assertEqual(render(oneSet("{{ .tags | join \", \" }}"), "main", json.decode("{\"tags\":[\"go\",\"cms\"]}")), "go, cms");
}

func testPipeLen() {
    testing.assertEqual(render(oneSet("{{ .xs | len }}"), "main", json.decode("{\"xs\":[1,2,3]}")), "3");
    testing.assertEqual(render(oneSet("{{ .s | len }}"), "main", json.decode("{\"s\":\"abcd\"}")), "4");
}

func testPrintfPipe() {
    testing.assertEqual(render(oneSet("{{ .n | printf \"%02d\" }}"), "main", json.decode("{\"n\":7}")), "07");
    testing.assertEqual(render(oneSet("[{{ .s | printf \"%-6s\" }}]"), "main", json.decode("{\"s\":\"hi\"}")), "[hi    ]");
}

func testPrintfFunc() {
    testing.assertEqual(render(oneSet("{{ printf \"%s: %d\" .k .n }}"), "main", json.decode("{\"k\":\"posts\",\"n\":5}")), "posts: 5");
    testing.assertEqual(render(oneSet("{{ printf \"$%.2f\" .price }}"), "main", json.decode("{\"price\":3.5}")), "$3.50");
    testing.assertEqual(render(oneSet("[{{ printf \"%6s\" .s }}]"), "main", json.decode("{\"s\":\"hi\"}")), "[    hi]");
    testing.assertEqual(render(oneSet("{{ printf \"%t/%v\" .b .n }}"), "main", json.decode("{\"b\":true,\"n\":9}")), "true/9");
}

func testPrintfLiteralPercent() {
    testing.assertEqual(render(oneSet("{{ printf \"100%%\" }}"), "main", json.decode("null")), "100%");
}

func testSprintfHelper() {
    testing.assertEqual(sprintfValues("%03d", [numVal(5)]), "005");
    testing.assertEqual(sprintfValues("%.1f", [strVal("2")]), "2.0");
}

func testNumberAndBoolLiterals() {
    testing.assertEqual(render(oneSet("{{ if eq .flag true }}on{{ end }}"), "main", json.decode("{\"flag\":true}")), "on");
    testing.assertEqual(render(oneSet("{{ if lt .n 10 }}under{{ end }}"), "main", json.decode("{\"n\":7}")), "under");
}

# --- negative: unknown pipe throws Error{kind:"tengine"} --------------------

func throwsUnknownPipe() {
    render(oneSet("{{ .a | bogus }}"), "main", json.decode("{\"a\":\"x\"}"));
}

func throwsMissingTemplate() {
    render(oneSet("{{ template \"nope\" . }}"), "main", json.decode("null"));
}

func testErrorsThrow() {
    testing.assertThrows("throwsUnknownPipe", "tengine");
    testing.assertThrows("throwsMissingTemplate", "tengine");
}

# --- recursion guard ---------------------------------------------------------
# A template that includes itself (directly or through a partner) must throw a
# catchable tengine error instead of recursing until the process dies.

func throwsSelfInclude() {
    render(add(newSet(), "m", "{{ template \"m\" . }}"), "m", json.decode("null"));
}

func throwsMutualInclude() {
    render(add(add(newSet(), "a", "{{ template \"b\" . }}"), "b", "{{ template \"a\" . }}"), "a", json.decode("null"));
}

func testRecursionGuardThrows() {
    testing.assertThrows("throwsSelfInclude", "tengine");
    testing.assertThrows("throwsMutualInclude", "tengine");
}

func testDeepButFiniteNestingRenders() {
    # 40 nested ifs stay well inside the guard.
    def src as string init "";
    def n as int init 40;
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        $src = $src + "{{ if .x }}";
    }
    $src = $src + "ok";
    for (def k as int init 0; $k < $n; $k = $k + 1) {
        $src = $src + "{{ end }}";
    }
    testing.assertEqual(render(oneSet($src), "main", json.decode("{\"x\":true}")), "ok");
}

# --- quote-aware action / pipeline scanning ---------------------------------
# A `}}` inside a quoted string must not terminate the action early.
func testActionCloseInsideQuotes() {
    testing.assertEqual(
        render(oneSet("{{ if eq .a \"}}\" }}Y{{ else }}N{{ end }}"), "main", json.decode("{\"a\":\"}}\"}")),
        "Y");
}

# A `|` inside a quoted string is not a pipeline separator.
func testPipeSplitIgnoresQuotedBar() {
    testing.assertEqual(render(oneSet("{{ \"x|y\" }}"), "main", json.decode("null")), "x|y");
}

# A printf format string may legitimately contain `|`.
func testPrintfFormatWithBar() {
    testing.assertEqual(
        render(oneSet("{{ printf \"%s|%s\" .a .b }}"), "main", json.decode("{\"a\":\"L\",\"b\":\"R\"}")),
        "L|R");
}

# A `}}` inside a comment body does not end the comment; the terminator is `*/}}`.
func testCommentWithBracesInBody() {
    testing.assertEqual(render(oneSet("a{{/* note }} here */}}b"), "main", json.decode("null")), "ab");
}
