# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

# --- file-handle streaming reader / writer round-trip ---------------
# jsonl.j already `use`s fs, so the overlay reaches fs.* directly.

func testStreamValueRoundTrip() {
    def path as string init fs.makeTempFile("", "jsonl-", ".jsonl");
    def wf as fs.File init fs.open($path, "write");
    def w as Writer init writer($wf);
    writeValue($w, rec("{\"id\":1,\"name\":\"ada\"}"));
    writeValue($w, rec("[10,20,30]"));
    writeValue($w, rec("42"));
    closeWriter($w);

    def rf as fs.File init fs.open($path, "read");
    def r as Reader init reader($rf);
    def got as list of json.Value init [];
    while (not fs.eof($rf)) {
        $got[] = readValue($r);
    }
    closeReader($r);
    fs.remove($path);

    testing.assertEqual(len($got), 3);
    testing.assertEqual(json.asInt(json.get($got[0], "/id")), 1);
    testing.assertEqual(json.asString(json.get($got[0], "/name")), "ada");
    testing.assertEqual(json.length($got[1]), 2 + 1);
    testing.assertEqual(json.asInt($got[2]), 42);
}

# --- path-based file convenience functions (writeFile / readFile / appendFile) ---

func testWriteReadFileRoundTrip() {
    def path as string init fs.makeTempFile("", "jsonl-rt-", ".jsonl");
    writeFile($path, rows());
    def back as list of json.Value init readFile($path);
    fs.remove($path);
    testing.assertEqual(len($back), 3);
    testing.assertEqual(json.asInt(json.get($back[0], "/id")), 1);
    testing.assertEqual(json.asInt($back[2]), 42);
}

func testAppendFileGrowsTheFile() {
    def path as string init fs.makeTempFile("", "jsonl-app-", ".jsonl");
    def first as list of json.Value init [];
    $first[] = rec("{\"a\":1}");
    writeFile($path, $first);
    def second as list of json.Value init [];
    $second[] = rec("{\"a\":2}");
    appendFile($path, $second);
    def back as list of json.Value init readFile($path);
    fs.remove($path);
    testing.assertEqual(len($back), 2);
    testing.assertEqual(json.asInt(json.get($back[0], "/a")), 1);
    testing.assertEqual(json.asInt(json.get($back[1], "/a")), 2);
}

# --- path-based streaming reader (openReader / hasMore / readRecord / closeReader) ---

func testPathReaderStreaming() {
    def path as string init fs.makeTempFile("", "jsonl-rd-", ".jsonl");
    writeFile($path, rows());
    def r as Reader init openReader($path);
    testing.assertTrue(hasMore($r));   # non-empty file
    def count as int init 0;
    def next as Record init readRecord($r);
    while (not $next.done) {
        $count = $count + 1;
        $next = readRecord($r);
    }
    closeReader($r);
    fs.remove($path);
    testing.assertEqual($count, 3);
}
