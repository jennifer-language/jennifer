# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# jsonl_test.j - white-box tests for jsonl.j. Run with:
#
#     jennifer test modules/jsonl_test.j
#
# These exercise the in-memory encode / decode surface (the file + streaming
# helpers are driven against a temp file in the Go suite, cmd/jennifer/jsonl_test.go).
# jsonl.j already `use`s json / strings / fs, so the overlay only adds testing.
use testing;

func rec(s as string) {
    return json.decode($s);
}

func rows() {
    def r as list of json.Value init [];
    $r[] = rec("{\"id\":1,\"name\":\"ada\"}");
    $r[] = rec("[10,20,30]");
    $r[] = rec("42");
    return $r;
}

func testEncodeBasic() {
    testing.assertEqual(encode(rows()), "{\"id\":1,\"name\":\"ada\"}\n[10,20,30]\n42\n");
}

func testEncodeEmpty() {
    def empty as list of json.Value init [];
    testing.assertEqual(encode($empty), "");
}

func testDecodeBasic() {
    def got as list of json.Value init decode("{\"a\":1}\n[2,3]\n\"x\"");
    testing.assertEqual(len($got), 3);
    testing.assertEqual(json.asInt(json.get($got[0], "/a")), 1);
    testing.assertEqual(json.length($got[1]), 2);
    testing.assertEqual(json.asString($got[2]), "x");
}

func testDecodeSkipsBlankLines() {
    def got as list of json.Value init decode("{\"a\":1}\n\n   \n\t\n{\"b\":2}\n");
    testing.assertEqual(len($got), 2);
    testing.assertEqual(json.asInt(json.get($got[1], "/b")), 2);
}

func testDecodeCRLF() {
    def got as list of json.Value init decode("{\"a\":1}\r\n{\"b\":2}\r\n");
    testing.assertEqual(len($got), 2);
    testing.assertEqual(json.asInt(json.get($got[0], "/a")), 1);
}

func testDecodeEmpty() {
    testing.assertEqual(len(decode("")), 0);
    testing.assertEqual(len(decode("\n\n  \n")), 0);
}

func testMixedTopLevelTypes() {
    def got as list of json.Value init decode("{\"o\":1}\n[1,2]\n7\n\"s\"\ntrue\nnull");
    testing.assertEqual(len($got), 6);
    testing.assertEqual(json.typeOf($got[0]), "map");
    testing.assertEqual(json.typeOf($got[1]), "list");
    testing.assertEqual(json.typeOf($got[2]), "int");
    testing.assertEqual(json.typeOf($got[3]), "string");
    testing.assertEqual(json.typeOf($got[4]), "bool");
    testing.assertEqual(json.typeOf($got[5]), "null");
}

func testRoundTrip() {
    def src as list of json.Value init rows();
    def back as list of json.Value init decode(encode($src));
    testing.assertEqual(len($back), len($src));
    def i as int init 0;
    while ($i < len($src)) {
        # Compare canonical compact forms.
        testing.assertEqual(json.encode($back[$i]), json.encode($src[$i]));
        $i = $i + 1;
    }
}

func testDecodeToleratesWhitespaceAroundValue() {
    def got as list of json.Value init decode("   {\"a\":1}   \n  42  ");
    testing.assertEqual(len($got), 2);
    testing.assertEqual(json.asInt(json.get($got[0], "/a")), 1);
    testing.assertEqual(json.asInt($got[1]), 42);
}
