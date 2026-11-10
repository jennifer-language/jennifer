# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# csv_test.j - white-box tests for csv.j. Run with:
#
#     jennifer test modules/csv_test.j
#
# The overlay splices csv.j in front of this file, so the tests reach its
# private helpers (needsQuoteQ, quoteFieldQ, sanitizeField, lineBreaks) by bare
# identifier as well as its exported surface.
use testing;

func testParseBasic() {
    def rows as list of list of string init parse("a,b,c\n1,2,3");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual(len($rows[0]), 3);
    testing.assertEqual($rows[0][0], "a");
    testing.assertEqual($rows[0][2], "c");
    testing.assertEqual($rows[1][1], "2");
}

func testParseQuotedComma() {
    def rows as list of list of string init parse("\"Smith, J\",42");
    testing.assertEqual(len($rows[0]), 2);
    testing.assertEqual($rows[0][0], "Smith, J");
    testing.assertEqual($rows[0][1], "42");
}

func testParseDoubledQuote() {
    def rows as list of list of string init parse("\"he said \"\"hi\"\"\"");
    testing.assertEqual($rows[0][0], "he said \"hi\"");
}

func testParseEmbeddedNewline() {
    def rows as list of list of string init parse("a,\"line1\nline2\"\nb,c");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][1], "line1\nline2");
    testing.assertEqual($rows[1][0], "b");
}

func testParseCRLF() {
    def rows as list of list of string init parse("a,b\r\nc,d\r\n");
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][1], "b");
    testing.assertEqual($rows[1][0], "c");
}

func testParseEdges() {
    testing.assertEqual(len(parse("")), 0); # empty input, no rows
    testing.assertEqual(len(parse("a,b\n")), 1); # trailing newline, no extra row
    def e as list of list of string init parse("\"\",x,");
    testing.assertEqual(len($e[0]), 3); # empty quoted, value, trailing empty
    testing.assertEqual($e[0][0], "");
    testing.assertEqual($e[0][1], "x");
    testing.assertEqual($e[0][2], "");
}

func testFormatQuoting() {
    def rows as list of list of string init [];
    $rows[] = ["plain", "has,comma", "has\"quote", "has\nnewline"];
    def out as string init format($rows);
    testing.assertEqual($out, "plain,\"has,comma\",\"has\"\"quote\",\"has\nnewline\"");
}

func testRoundTrip() {
    def src as string init "name,note\n\"Smith, J\",\"a \"\"quote\"\" and, comma\"\nAda,plain";
    def rows as list of list of string init parse($src);
    def back as list of list of string init parse(format($rows));
    testing.assertEqual(len($back), len($rows));
    testing.assertEqual($back[1][0], "Smith, J");
    testing.assertEqual($back[1][1], "a \"quote\" and, comma");
    testing.assertEqual($back[2][1], "plain");
}

func testTSVDelimiter() {
    def rows as list of list of string init parseWith("a\tb\tc", "\t");
    testing.assertEqual(len($rows[0]), 3);
    testing.assertEqual($rows[0][1], "b");
    # A tab inside a field forces quoting under a tab delimiter.
    def out as string init formatWith($rows, "\t");
    testing.assertEqual($out, "a\tb\tc");
}

func testToRecords() {
    def rows as list of list of string init parse("name,age\nAda,36\nGrace,45");
    def recs as list of map of string to string init toRecords($rows);
    testing.assertEqual(len($recs), 2);
    testing.assertEqual($recs[0]["name"], "Ada");
    testing.assertEqual($recs[0]["age"], "36");
    testing.assertEqual($recs[1]["name"], "Grace");
}

# A row of another width than the header, and a header naming a column twice,
# are errors: padding or dropping fields would hide broken input.
func toRecordsShortRow() {
    return toRecords([["a", "b", "c"], ["1", "2"]]);
}
func toRecordsLongRow() {
    return toRecords([["a", "b"], ["1", "2", "3"]]);
}
func toRecordsDuplicateHeader() {
    return toRecords([["a", "a"], ["1", "2"]]);
}
func testToRecordsRejectsBadShapes() {
    testing.assertThrows("toRecordsShortRow", "csv");
    testing.assertThrows("toRecordsLongRow", "csv");
    testing.assertThrows("toRecordsDuplicateHeader", "csv");
}

func testToRecordsEmpty() {
    testing.assertEqual(len(toRecords([])), 0);
}

func testFromRecords() {
    def recs as list of map of string to string init [];
    def one as map of string to string init {};
    $one["name"] = "Ada";
    $one["age"] = "36";
    $recs[] = $one;
    def rows as list of list of string init fromRecords(["name", "age"], $recs);
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][0], "name"); # header row
    testing.assertEqual($rows[1][0], "Ada");
    testing.assertEqual($rows[1][1], "36");
}

# A record lacking a header key, one with a key the header does not name, and
# a header naming a column twice are errors.
func fromRecordsMissingKey() {
    return fromRecords(["name", "age"], [{"name": "Ada"}]);
}
func fromRecordsExtraKey() {
    return fromRecords(["name"], [{"name": "Ada", "age": "36"}]);
}
func fromRecordsDuplicateHeader() {
    return fromRecords(["name", "name"], [{"name": "Ada"}]);
}
func testFromRecordsRejectsBadShapes() {
    testing.assertThrows("fromRecordsMissingKey", "csv");
    testing.assertThrows("fromRecordsExtraKey", "csv");
    testing.assertThrows("fromRecordsDuplicateHeader", "csv");
}

func testRecordsRoundTrip() {
    def rows as list of list of string init parse("name,age\nAda,36\nGrace,45");
    def recs as list of map of string to string init toRecords($rows);
    def back as list of list of string init fromRecords(["name", "age"], $recs);
    testing.assertEqual(format($back), format($rows));
}

# White-box: private quoting helpers reached by bare identifier.
func testPrivateNeedsQuote() {
    def d as Dialect init dialect(",");
    testing.assertFalse(needsQuoteQ("plain", $d, false));
    testing.assertTrue(needsQuoteQ("a,b", $d, false));
    testing.assertTrue(needsQuoteQ("a\"b", $d, false));
    testing.assertTrue(needsQuoteQ("a\nb", $d, false));
    testing.assertTrue(needsQuoteQ("a\rb", $d, false));
    testing.assertFalse(needsQuoteQ("a,b", dialect("\t"), false)); # not the delimiter
    testing.assertFalse(needsQuoteQ(" a ", $d, false)); # spaces are data
    $d.trim = true;
    testing.assertTrue(needsQuoteQ(" a ", $d, false)); # trimming would strip them
    $d.comment = "#";
    testing.assertTrue(needsQuoteQ("#x", $d, true)); # would read as a comment line
    testing.assertFalse(needsQuoteQ("#x", $d, false)); # not a record's first field
}

func testPrivateQuoteField() {
    def d as Dialect init dialect(",");
    testing.assertEqual(quoteFieldQ("plain", $d, false), "plain");
    testing.assertEqual(quoteFieldQ("a,b", $d, false), "\"a,b\"");
    testing.assertEqual(quoteFieldQ("a\"b", $d, false), "\"a\"\"b\"");
}

# --- formula-injection mitigation (CWE-1236) ------------------------

# White-box: the private sanitizeField neutraliser.
func testPrivateSanitizeField() {
    testing.assertEqual(sanitizeField("=SUM(A1)"), "'=SUM(A1)");
    testing.assertEqual(sanitizeField("+1"), "'+1");
    testing.assertEqual(sanitizeField("-1"), "'-1");
    testing.assertEqual(sanitizeField("@ref"), "'@ref");
    testing.assertEqual(sanitizeField("\tx"), "'\tx"); # leading tab
    testing.assertEqual(sanitizeField("\rx"), "'\rx"); # leading CR
    testing.assertEqual(sanitizeField("plain"), "plain"); # normal untouched
    testing.assertEqual(sanitizeField(""), ""); # empty untouched
    testing.assertEqual(sanitizeField("a=b"), "a=b"); # only the FIRST char matters
}

func testFormatSafeNeutralises() {
    def rows as list of list of string init [];
    $rows[] = ["=1+2", "+cmd", "-2", "@ref", "\tinj", "normal"];
    def back as list of list of string init parse(formatSafe($rows));
    testing.assertEqual($back[0][0], "'=1+2");
    testing.assertEqual($back[0][1], "'+cmd");
    testing.assertEqual($back[0][2], "'-2");
    testing.assertEqual($back[0][3], "'@ref");
    testing.assertEqual($back[0][4], "'\tinj");
    testing.assertEqual($back[0][5], "normal"); # left alone
}

func testFormatSafeWithDelimiter() {
    def rows as list of list of string init [];
    $rows[] = ["=danger", "ok"];
    def out as string init formatSafeWith($rows, "\t");
    testing.assertEqual($out, "'=danger\tok");
}

# --- dialects -------------------------------------------------------

func testDialectDefaults() {
    def d as Dialect init dialect(";");
    testing.assertEqual($d.delimiter, ";");
    testing.assertEqual($d.quote, "\"");
    testing.assertEqual($d.comment, "");
    testing.assertFalse($d.trim);
    testing.assertEqual($d.width, 0);
}

func testParseDialectSemicolonCommentTrim() {
    def d as Dialect init Dialect{delimiter: ";", quote: "\"", comment: "#", trim: true, width: 0};
    def text as string init "# a header comment\n a ; b ; c \nx;y;z\n# trailing comment";
    def rows as list of list of string init parseDialect($text, $d);
    testing.assertEqual(len($rows), 2);
    testing.assertEqual($rows[0][0], "a"); # unquoted field trimmed
    testing.assertEqual($rows[0][1], "b");
    testing.assertEqual($rows[0][2], "c");
    testing.assertEqual($rows[1][0], "x");
    testing.assertEqual($rows[1][2], "z");
}

func testParseDialectQuotedKeepsWhitespace() {
    # A quoted field keeps its whitespace even under trim=true.
    def d as Dialect init Dialect{delimiter: ";", quote: "\"", comment: "", trim: true, width: 0};
    def rows as list of list of string init parseDialect("\" keep \"; drop ", $d);
    testing.assertEqual($rows[0][0], " keep ");
    testing.assertEqual($rows[0][1], "drop");
}

func testFormatDialect() {
    def rows as list of list of string init [];
    $rows[] = ["a;b", "plain"];
    def d as Dialect init dialect(";");
    # The field carrying the delimiter is quoted with the dialect quote char.
    testing.assertEqual(formatDialect($rows, $d), "\"a;b\";plain");
}

# --- streaming reader / writer round-trip ---------------------------

func testStreamCsvRoundTrip() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    def wf as fs.File init fs.open($path, "write");
    def w as Writer init writer($wf);
    writeRow($w, ["name", "note"]);
    writeRow($w, ["Smith, J", "hi"]); # embedded comma forces quoting
    writeRow($w, ["Ada", "plain"]);
    closeWriter($w);

    def rf as fs.File init fs.open($path, "read");
    def r as Reader init reader($rf);
    def rows as list of list of string init [];
    while (not readerEof($r)) {
        def row as list of string init readRow($r);
        if (len($row) > 0) {
            $rows[] = $row;
        }
    }
    closeReader($r);
    fs.remove($path);

    testing.assertEqual(len($rows), 3);
    testing.assertEqual($rows[0][0], "name");
    testing.assertEqual($rows[1][0], "Smith, J"); # quoted comma survived streaming
    testing.assertEqual($rows[1][1], "hi");
    testing.assertEqual($rows[2][1], "plain");
}

func testStreamCsvQuotedNewlineSpansLines() {
    # A quoted field with an embedded newline spans two physical lines; readRow
    # must read the whole record.
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    fs.writeString($path, "a,\"x\ny\"\nb,c\n");

    def rf as fs.File init fs.open($path, "read");
    def r as Reader init reader($rf);
    def first as list of string init readRow($r);
    def second as list of string init readRow($r);
    closeReader($r);
    fs.remove($path);

    testing.assertEqual(len($first), 2);
    testing.assertEqual($first[1], "x\ny");
    testing.assertEqual($second[0], "b");
    testing.assertEqual($second[1], "c");
}

func testLineBreaks() {
    testing.assertEqual(lineBreaks(""), 0);
    testing.assertEqual(lineBreaks("a\nb\r\nc\rd"), 3); # LF, CRLF, bare CR
}

# A delimiter or quote that cannot delimit fields is an error, not a silent
# one-field-per-line parse.
func parseBadDelim() {
    return parseWith("a;;b", ";;");
}
func formatBadDelim() {
    return formatWith([["a"]], "");
}
func dialectQuoteIsDelim() {
    def d as Dialect init dialect(",");
    $d.quote = ",";
    return parseDialect("a,b", $d);
}
func testBadDelimiterRejected() {
    testing.assertThrows("parseBadDelim", "csv");
    testing.assertThrows("formatBadDelim", "csv");
    testing.assertThrows("dialectQuoteIsDelim", "csv");
}

# The native split path and the scanner agree on quoted fields that hold the
# delimiter, doubled quotes and line breaks, and on CRLF records.
func testQuotedFieldsAcrossPaths() {
    def text as string init "a,\"b,c\",\"say \"\"hi\"\"\"\r\n\"multi\nline\",x,y";
    testing.assertEqual(parse($text), [["a", "b,c", "say \"hi\""], ["multi\nline", "x", "y"]]);
}

# --- strict quoting (RFC 4180) --------------------------------------

# messageOf returns the csv error parse raises on text, or "" when it parses.
func messageOf(text as string) {
    try {
        parse($text);
    } catch (e) {
        return $e.message;
    }
    return "";
}

func testBareQuoteInUnquotedFieldIsAnError() {
    testing.assertEqual(
        messageOf("id,v,w\n1,5\" disk,x\n2,y,z\n"),
        "csv: a quote inside an unquoted field at line 2, column 4");
    testing.assertEqual(
        messageOf("q\"r,s"),
        "csv: a quote inside an unquoted field at line 1, column 2");
}

func testTextAfterClosingQuoteIsAnError() {
    testing.assertEqual(
        messageOf("\"ab\"c,d"),
        "csv: text after a closing quote at line 1, column 5");
    testing.assertEqual(
        messageOf("a,\"b\" "),
        "csv: text after a closing quote at line 1, column 6");
}

func testUnclosedQuoteIsAnError() {
    testing.assertEqual(
        messageOf("a,b\n1,\"x\ny"),
        "csv: the quoted field opened at line 2, column 3 is not closed");
}

func testErrorPositionsCountLinesInsideQuotes() {
    # The quoted field spans lines 1-2; the bad quote is on line 3.
    testing.assertEqual(
        messageOf("\"a\nb\",c\nd,e\"f"),
        "csv: a quote inside an unquoted field at line 3, column 4");
    # Bare CR records go through the scanner and count lines the same way.
    testing.assertEqual(
        messageOf("a,b\rc,d\"x"),
        "csv: a quote inside an unquoted field at line 2, column 4");
}

func testTrimAllowsSpacesAroundQuotedFields() {
    def d as Dialect init dialect(",");
    $d.trim = true;
    testing.assertEqual(parseDialect("a, \"b, c\" ,d", $d), [["a", "b, c", "d"]]);
}

func spacesAroundQuotesUntrimmed() {
    return parse("a, \"b\"");
}
func testSpacesBeforeQuoteWithoutTrimIsAnError() {
    testing.assertThrows("spacesAroundQuotesUntrimmed", "csv");
}

# --- record widths --------------------------------------------------

func testRaggedRecordsAreAnError() {
    testing.assertEqual(messageOf("a,b\nc"), "csv: record 2 has 1 fields, expected 2");
    testing.assertEqual(messageOf("a,b\nc,d,e"), "csv: record 2 has 3 fields, expected 2");
}

func testDialectWidth() {
    def d as Dialect init dialect(",");
    $d.width = -1;
    testing.assertEqual(parseDialect("a,b\nc", $d), [["a", "b"], ["c"]]);
    $d.width = 3;
    testing.assertEqual(parseDialect("a,b,c", $d), [["a", "b", "c"]]);
}

func dialectWidthMismatch() {
    def d as Dialect init dialect(",");
    $d.width = 3;
    return parseDialect("a,b", $d);
}
func dialectWidthInvalid() {
    def d as Dialect init dialect(",");
    $d.width = -2;
    return parseDialect("a", $d);
}
func testDialectWidthErrors() {
    testing.assertThrows("dialectWidthMismatch", "csv");
    testing.assertThrows("dialectWidthInvalid", "csv");
}

# A blank line is a record with one empty field (RFC 4180: a record is one or
# more fields, a field may be empty).
func testBlankLineIsOneEmptyField() {
    testing.assertEqual(parse("a\n\nb"), [["a"], [""], ["b"]]);
    testing.assertEqual(messageOf("a,b\n\nc,d"), "csv: record 2 has 1 fields, expected 2");
}

# --- formatting -----------------------------------------------------

func testFormatUsesCRLF() {
    testing.assertEqual(format([["a", "b"], ["c", "d"]]), "a,b\r\nc,d");
    testing.assertEqual(formatDialect([["a", "b"], ["c", "d"]], dialect(";")), "a;b\r\nc;d");
}

func testFormatLoneEmptyFieldRoundTrips() {
    def rows as list of list of string init [["a"], [""], ["b"]];
    testing.assertEqual(format($rows), "a\r\n\"\"\r\nb");
    testing.assertEqual(parse(format($rows)), $rows);
    testing.assertEqual(parse(format([[""]])), [[""]]);
}

func formatEmptyRow() {
    return format([["a"], []]);
}
func formatRaggedRows() {
    return format([["a", "b"], ["c"]]);
}
func testFormatRejectsUnwritableRows() {
    testing.assertThrows("formatEmptyRow", "csv");
    testing.assertThrows("formatRaggedRows", "csv");
}

func testFormatDialectRoundTripsTrimAndComments() {
    def d as Dialect init dialect(",");
    $d.trim = true;
    $d.comment = "#";
    def rows as list of list of string init [[" padded ", "#not a comment"], ["#x", "y"]];
    testing.assertEqual(parseDialect(formatDialect($rows, $d), $d), $rows);
    $d.width = -1;
    testing.assertEqual(formatDialect([["a", "b"], ["c"]], $d), "a,b\r\nc");
}

# --- streaming reader -----------------------------------------------

# streamed writes text to a temporary file and reads every record back with
# readRow.
func streamed(text as string) {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    defer fs.remove($path);
    fs.writeString($path, $text);
    def r as Reader init reader(fs.open($path, "read"));
    defer closeReader($r);
    def rows as list of list of string init [];
    while (not readerEof($r)) {
        $rows[] = readRow($r);
    }
    return $rows;
}

func testReadRowMatchesParse() {
    def inputs as list of string init [
        "a,b\rc,d\re,f\n",
        "id,text\r\n1,\"line1\r\nline2\"\r\n2,x\r\n",
        "a\n\nb\n",
        "a,b\nc,d",
        "a,\"x\ry\",\"\"\"q\"\"\"\r\n",
        "\"\"\r\n\"\"",
        "é,ü\nö,\"ä\nß\"\n"
    ];
    for (def text in $inputs) {
        testing.assertEqual(streamed($text), parse($text));
    }
}

func testReadRowEmptyFile() {
    testing.assertEqual(streamed(""), []);
}

# Records that cross the reader's chunk boundary, including a CRLF split
# between two chunks (the CR is the first chunk's last character).
func testReadRowAcrossChunks() {
    def long as string init strings.repeat("x", READ_CHUNK - 3);
    def text as string init $long + ",a\r\nb,c\r\n";
    testing.assertEqual(streamed($text), [[$long, "a"], ["b", "c"]]);
    def field as string init strings.repeat("y\r\n", READ_CHUNK);
    def quoted as string init "k,\"" + $field + "\"\nm,n\n";
    testing.assertEqual(streamed($quoted), [["k", $field], ["m", "n"]]);
}

# A malformed record is consumed before its error, so reading goes on with the
# next record; a stray quote does not swallow the rest of the file.
func testReadRowErrorsAndContinues() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    defer fs.remove($path);
    fs.writeString($path, "id,v\n1,5\" disk\n2,y\n3\n4,z\n");
    def r as Reader init reader(fs.open($path, "read"));
    defer closeReader($r);
    testing.assertEqual(readRow($r), ["id", "v"]);
    def messages as list of string init [];
    def rows as list of list of string init [];
    while (not readerEof($r)) {
        try {
            $rows[] = readRow($r);
        } catch (e) {
            $messages[] = $e.message;
        }
    }
    testing.assertEqual($rows, [["2", "y"], ["4", "z"]]);
    testing.assertEqual(
        $messages,
        [
            "csv: a quote inside an unquoted field at line 2, column 4",
            "csv: record 4 has 1 fields, expected 2"
        ]);
}

# --- streaming writer -----------------------------------------------

func testWriteRowCRLFAndLoneEmptyField() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    defer fs.remove($path);
    def w as Writer init writer(fs.open($path, "write"));
    writeRow($w, ["a", "b,c"]);
    writeRow($w, ["", ""]);
    closeWriter($w);
    testing.assertEqual(fs.readString($path), "a,\"b,c\"\r\n,\r\n");
    def w1 as Writer init writer(fs.open($path, "write"));
    writeRow($w1, [""]);
    closeWriter($w1);
    testing.assertEqual(fs.readString($path), "\"\"\r\n");
    testing.assertEqual(streamed(fs.readString($path)), [[""]]);
}

func writeRaggedRow() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    defer fs.remove($path);
    def w as Writer init writer(fs.open($path, "write"));
    defer closeWriter($w);
    writeRow($w, ["a", "b"]);
    writeRow($w, ["c"]);
}
func writeEmptyRow() {
    def path as string init fs.makeTempFile("", "csv-", ".csv");
    defer fs.remove($path);
    def w as Writer init writer(fs.open($path, "write"));
    defer closeWriter($w);
    writeRow($w, []);
}
func testWriteRowRejectsUnwritableRows() {
    testing.assertThrows("writeRaggedRow", "csv");
    testing.assertThrows("writeEmptyRow", "csv");
}
