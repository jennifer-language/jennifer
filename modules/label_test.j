# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# label_test.j - white-box tests for label.j. Run with:
#
#     jennifer test modules/label_test.j
#
# The overlay splices label.j (and its included label_zpl.inc.j / label_cab.inc.j) in
# front of this file, so the tests reach the private helpers (mmToDots,
# zplEscape, cabField) by bare identifier. The networked `send` (:9100 raw port)
# is covered against an in-process fake printer in the Go suite (TestLabelSend).
use testing;

# noopts returns a zero-value BarcodeOptions (the "no options" default).
func noopts() {
    def o as BarcodeOptions;
    return $o;
}

# sampleLabel builds one label used by both dialect render tests.
func sampleLabel() {
    def l as Label init new(50.0, 30.0);
    $l = text($l, 5.0, 5.0, TextOptions{height: 4.0, points: 0, rotation: 0, bold: false}, "HELLO");
    $l = box($l, 2.0, 2.0, 46.0, 26.0, 0.5);
    $l = barcode($l, "code128", 5.0, 20.0, noopts(), "12345678");
    $l = quantity($l, 2);
    return $l;
}

func testMmToDots() {
    testing.assertEqual(mmToDots(10.0, 203), 80);
    testing.assertEqual(mmToDots(25.4, 203), 203);
    testing.assertEqual(mmToDots(25.4, 300), 300);
}

func testZplEscape() {
    testing.assertEqual(zplEscape("HELLO"), "HELLO");
    testing.assertEqual(zplEscape("a^b~c_d"), "a_5Eb_7Ec_5Fd");
}

func testRenderZpl() {
    def want as string init "^XA\n^CI28\n^PW400\n^LL240\n^FO40,40^A0N,32,32^FH^FDHELLO^FS\n^FO16,16^GB368,208,4^FS\n^FO40,160^BY2^BCN,120,Y,N,N^FH^FD12345678^FS\n^PQ2\n^XZ\n";
    testing.assertEqual(render(sampleLabel(), zpl(203)), $want);
}

func testRenderCab() {
    def want as string init "m m\nJ\nS 0,0,30.0,30.0,50.0\nT 5.0,5.0,0,3,4.0;HELLO\nG 2.0,2.0,0;R:46.0,26.0,0.5,0.5\nB 5.0,20.0,0,CODE128,15.0,0.3;12345678\nA 2\n";
    testing.assertEqual(render(sampleLabel(), cab()), $want);
}

func testRenderCabItf() {
    def l as Label init barcode(new(40.0, 20.0), "itf", 5.0, 5.0, noopts(), "1234567890123");
    def want as string init "m m\nJ\nS 0,0,20.0,20.0,40.0\nB 5.0,5.0,0,2OF5INTERLEAVED,15.0,0.3,3;01234567890123\nA 1\n";
    testing.assertEqual(render($l, cab()), $want);
}

func testItfPadsOddLength() {
    def l as Label init barcode(new(30.0, 20.0), "itf", 0.0, 0.0, noopts(), "123");
    testing.assertEqual($l.fields[0].data, "0123");
}

func testItfKeepsEvenLength() {
    def l as Label init barcode(new(30.0, 20.0), "itf", 0.0, 0.0, noopts(), "1234");
    testing.assertEqual($l.fields[0].data, "1234");
}

# A numeric symbology (EAN) rejects non-digit data at the boundary, so ZPL / CAB
# metacharacters can never reach the raw ^FD / `;` field (printer-command injection).
func testEanRejectsNonNumeric() {
    def threw as bool init false;
    def kind as string init "";
    try {
        barcode(new(40.0, 20.0), "ean13", 0.0, 0.0, noopts(), "12^XA0000");
    } catch (e) {
        $threw = true;
        $kind = $e.kind;
    }
    testing.assertTrue($threw);
    testing.assertEqual($kind, "label");
}

# A newline in an alphanumeric payload is flattened in the cab output, so it cannot
# terminate the command line and inject a following command.
func testCabFlattensNewlineInData() {
    def l as Label init barcode(new(40.0, 20.0), "code128", 5.0, 5.0, noopts(), "AB\nCD");
    def out as string init render($l, cab());
    testing.assertTrue(strings.contains($out, "AB CD"));
    testing.assertFalse(strings.contains($out, "AB\nCD"));
}

func testCabExtraBarcodes() {
    def a as Label init barcode(new(50.0, 20.0), "code39", 5.0, 5.0, noopts(), "ABC123");
    testing.assertEqual(
        render($a, cab()),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,CODE39,15.0,0.3,3;ABC123\nA 1\n");
    def b as Label init barcode(
        new(80.0, 30.0),
        "gs1-128",
        5.0,
        5.0,
        noopts(),
        "(00)300653005555555552");
    testing.assertEqual(
        render($b, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128,15.0,0.3;(00)300653005555555552\nA 1\n");
    def c as Label init barcode(new(30.0, 30.0), "datamatrix", 5.0, 5.0, noopts(), "HELLO");
    testing.assertEqual(
        render($c, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,DATAMATRIX,1.0;HELLO\nA 1\n");
}

func testZplExtraBarcodes() {
    def a as Label init barcode(new(50.0, 20.0), "code39", 5.0, 5.0, noopts(), "ABC123");
    testing.assertEqual(
        render($a, zpl(203)),
        "^XA\n^CI28\n^PW400\n^LL160\n^FO40,40^BY2^B3N,N,120,Y,N^FH^FDABC123^FS\n^PQ1\n^XZ\n");
    def b as Label init barcode(
        new(80.0, 30.0),
        "gs1-128",
        5.0,
        5.0,
        noopts(),
        "(00)300653005555555552");
    testing.assertEqual(
        render($b, zpl(203)),
        "^XA\n^CI28\n^PW639\n^LL240\n^FO40,40^BY2^BCN,120,Y,N,N,D^FH^FD(00)300653005555555552^FS\n^PQ1\n^XZ\n");
    def c as Label init barcode(new(30.0, 30.0), "datamatrix", 5.0, 5.0, noopts(), "HELLO");
    testing.assertEqual(
        render($c, zpl(203)),
        "^XA\n^CI28\n^PW240\n^LL240\n^FO40,40^BXN,8,200^FH^FDHELLO^FS\n^PQ1\n^XZ\n");
}

func testImageBothDialects() {
    def l as Label init image(new(40.0, 40.0), 10.0, 10.0, "LOGO");
    testing.assertEqual(
        render($l, cab()),
        "m m\nJ\nS 0,0,40.0,40.0,40.0\nI 10.0,10.0,0,1,1,a;LOGO\nA 1\n");
    testing.assertEqual(
        render($l, zpl(203)),
        "^XA\n^CI28\n^PW320\n^LL320\n^FO80,80^XGLOGO,1,1^FS\n^PQ1\n^XZ\n");
}

func testBarcodeOptionsCab() {
    # hideText -> lowercase symbology name (suppress the human-readable line).
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), "code128", 5.0, 5.0, $a, "12345");
    testing.assertEqual(
        render($la, cab()),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,code128,15.0,0.3;12345\nA 1\n");
    # checkDigit -> +MOD10.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(80.0, 30.0), "gs1-128", 5.0, 5.0, $b, "(00)30065300555555555");
    testing.assertEqual(
        render($lb, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128+MOD10,15.0,0.3;(00)30065300555555555\nA 1\n");
    # errorLevel + explicit height override.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    $c.height = 2.0;
    def lc as Label init barcode(new(30.0, 30.0), "qr", 5.0, 5.0, $c, "hello");
    testing.assertEqual(
        render($lc, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,QRCODE+ELH,2.0;hello\nA 1\n");
}

func testBarcodeOptionsZpl() {
    # hideText -> interpretation line N.
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), "code128", 5.0, 5.0, $a, "12345");
    testing.assertEqual(
        render($la, zpl(203)),
        "^XA\n^CI28\n^PW400\n^LL160\n^FO40,40^BY2^BCN,120,N,N,N^FH^FD12345^FS\n^PQ1\n^XZ\n");
    # checkDigit -> ITF native check digit Y.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(40.0, 20.0), "itf", 5.0, 5.0, $b, "123456");
    testing.assertEqual(
        render($lb, zpl(203)),
        "^XA\n^CI28\n^PW320\n^LL160\n^FO40,40^BY2^B2N,120,Y,N,Y^FD123456^FS\n^PQ1\n^XZ\n");
    # QR error level -> ^FD prefix.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    def lc as Label init barcode(new(30.0, 30.0), "qr", 5.0, 5.0, $c, "hello");
    testing.assertEqual(
        render($lc, zpl(203)),
        "^XA\n^CI28\n^PW240\n^LL240\n^FO40,40^BQN,2,8^FH^FDHA,hello^FS\n^PQ1\n^XZ\n");
}

func badItf() {
    barcode(new(10.0, 10.0), "itf", 0.0, 0.0, noopts(), "12a4");
}

func badBarcodeType() {
    barcode(new(10.0, 10.0), "pdf417", 0.0, 0.0, noopts(), "x");
}

func badDialect() {
    def dev as Device;
    $dev.dialect = "xyz";
    $dev.dpi = 203;
    render(new(10.0, 10.0), $dev);
}

func testInvalidInputsThrow() {
    testing.assertThrows("badItf", "label");
    testing.assertThrows("badBarcodeType", "label");
    testing.assertThrows("badDialect", "label");
}

func testEanEightCab() {
    # hideText -> lowercase; explicit height and narrow-element width.
    def o as BarcodeOptions;
    $o.hideText = true;
    $o.height = 6.0;
    $o.moduleWidth = 0.2;
    def l as Label init barcode(new(20.0, 15.0), "ean8", 3.7, 2.0, $o, "93125192");
    testing.assertEqual(
        render($l, cab()),
        "m m\nJ\nS 0,0,15.0,15.0,20.0\nB 3.7,2.0,0,ean8,6.0,0.2;93125192\nA 1\n");
}

func testEanEightZpl() {
    def o as BarcodeOptions;
    $o.height = 6.0;
    def l as Label init barcode(new(20.0, 15.0), "ean8", 3.7, 2.0, $o, "93125192");
    testing.assertEqual(
        render($l, zpl(203)),
        "^XA\n^CI28\n^PW160\n^LL120\n^FO30,16^BY2^B8N,48,Y,N^FD93125192^FS\n^PQ1\n^XZ\n");
}

func testTextRotationBoldPointsCab() {
    def o as TextOptions;
    $o.rotation = 90;
    $o.bold = true;
    $o.points = 8;
    def l as Label init text(new(20.0, 15.0), 2.2, 4.0, $o, "06");
    testing.assertEqual(
        render($l, cab()),
        "m m\nJ\nS 0,0,15.0,15.0,20.0\nT 2.2,4.0,90,5,pt8;06\nA 1\n");
}

func testTextRotationZpl() {
    def o as TextOptions;
    $o.rotation = 90;
    $o.points = 8;
    def l as Label init text(new(20.0, 15.0), 2.2, 4.0, $o, "06");
    # 8 pt at 203 dpi = round(8 * 203 / 72) = 23 dots.
    testing.assertEqual(
        render($l, zpl(203)),
        "^XA\n^CI28\n^PW160\n^LL120\n^FO18,32^A0B,23,23^FH^FD06^FS\n^PQ1\n^XZ\n");
}

# ZPL sets the print width (^PW) and label length (^LL) from the label size in
# dots at the target dpi: 100 x 50 mm at 300 dpi is 1181 x 591 dots.
func testZplLabelSize() {
    def l as Label init new(100.0, 50.0);
    def out as string init render($l, zpl(300));
    testing.assertEqual($out, "^XA\n^CI28\n^PW1181\n^LL591\n^PQ1\n^XZ\n");
}

# ZPL ignores bold: ^A0 is a bold face already, so the output is unchanged.
func testZplIgnoresBold() {
    def o as TextOptions;
    $o.height = 3.0;
    def plain as Label init text(new(20.0, 15.0), 2.0, 2.0, $o, "X");
    $o.bold = true;
    def bold as Label init text(new(20.0, 15.0), 2.0, 2.0, $o, "X");
    testing.assertEqual(render($bold, zpl(203)), render($plain, zpl(203)));
}

func testCabSetupPreamble() {
    # A 4-up die (columns > 1 -> the column pitch + count are appended).
    def s as CabSetup;
    $s.jobName = "Barcode1";
    $s.heat = 100;
    $s.speed = 5;
    $s.mode = "T,R0";
    $s.orientation = "R";
    $s.sensor = "l1";
    $s.height = 12.0;
    $s.pitch = 15.0;
    $s.width = 17.0;
    $s.columnPitch = 20.0;
    $s.columns = 4;
    def l as Label init text(new(20.0, 15.0), 2.2, 10.0, plainPt(8), "U260322");
    testing.assertEqual(
        render($l, cabWith($s)),
        "m m\nJ Barcode1\nH 100,5,T,R0\nO R\nS l1;0.0,0.0,12.0,15.0,17.0,20.0,4\nT 2.2,10.0,0,5,pt8;U260322\nA 1\n");
}

func testItfRatioOption() {
    # an explicit ratio + narrow width overrides the ITF defaults (ne 0.3, ratio 3).
    def o as BarcodeOptions;
    $o.hideText = true;
    $o.height = 6.0;
    $o.moduleWidth = 0.2;
    $o.ratio = 2.0;
    def l as Label init barcode(new(20.0, 15.0), "itf", 2.0, 3.1, $o, "8864441113");
    testing.assertEqual(
        render($l, cab()),
        "m m\nJ\nS 0,0,15.0,15.0,20.0\nB 2.0,3.1,0,2of5interleaved,6.0,0.2,2.0;8864441113\nA 1\n");
}

func testCabSizeSingleUp() {
    # width set but columns <= 1 -> a single-up geometry, no column pitch/count.
    def s as CabSetup;
    $s.sensor = "l1";
    $s.height = 12.0;
    $s.pitch = 15.0;
    $s.width = 17.0;
    def l as Label init text(new(20.0, 15.0), 2.0, 2.0, plainMm(3.0), "X");
    testing.assertEqual(
        render($l, cabWith($s)),
        "m m\nJ\nS l1;0.0,0.0,12.0,15.0,17.0\nT 2.0,2.0,0,3,3.0;X\nA 1\n");
}

# plainPt returns a bold, point-sized, unrotated TextOptions (used by the cab
# preamble test).
func plainPt(pt as int) {
    def o as TextOptions;
    $o.points = $pt;
    $o.bold = true;
    return $o;
}

func testDefaultCabUnchanged() {
    # A zero-value CabSetup still emits the bare J / derived S (no H / O).
    def l as Label init text(new(50.0, 30.0), 5.0, 5.0, plainMm(4.0), "HI");
    testing.assertEqual(
        render($l, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,50.0\nT 5.0,5.0,0,3,4.0;HI\nA 1\n");
}

# plainMm returns a plain millimetre-height TextOptions.
func plainMm(h as float) {
    def o as TextOptions;
    $o.height = $h;
    return $o;
}

# An image name cannot add ^XG parameters: the separator is stripped.
func testZplImageNameStripsSeparator() {
    def l as Label init image(new(50.0, 30.0), 1.0, 1.0, "R:LOGO.GRF,9,9");
    def out as string init render($l, zpl(203));
    testing.assertContains($out, "^XGR:LOGO.GRF99,1,1^FS");
}

# ZPL output selects UTF-8 (^CI28) right after ^XA, so the hex-escaped UTF-8
# bytes of non-ASCII text print as the characters they encode.
func testZplSelectsUtf8() {
    def l as Label init text(new(20.0, 10.0), 1.0, 1.0, plainMm(3.0), "Grüße");
    testing.assertEqual(
        render($l, zpl(203)),
        "^XA\n^CI28\n^PW160\n^LL80\n^FO8,8^A0N,24,24^FH^FDGr_C3_BC_C3_9Fe^FS\n^PQ1\n^XZ\n");
}

# Builders that must throw kind "label" (see testInvalidBuildInputsThrow).
func badNegativeSize() {
    new(-50.0, 30.0);
}

func badZeroSize() {
    new(50.0, 0.0);
}

func badEanLength() {
    barcode(new(40.0, 20.0), "ean13", 0.0, 0.0, noopts(), "12345");
}

func badEanCheck() {
    barcode(new(40.0, 20.0), "ean13", 0.0, 0.0, noopts(), "4006381333932");
}

func badEan8Length() {
    barcode(new(40.0, 20.0), "ean8", 0.0, 0.0, noopts(), "123456789");
}

func badCode39Star() {
    barcode(new(40.0, 20.0), "code39", 0.0, 0.0, noopts(), "AB*CD");
}

func badCode39Lower() {
    barcode(new(40.0, 20.0), "code39", 0.0, 0.0, noopts(), "abc");
}

func badEmptyData() {
    barcode(new(40.0, 20.0), "code128", 0.0, 0.0, noopts(), "");
}

func badQrLevel() {
    def o as BarcodeOptions;
    $o.errorLevel = "Z";
    barcode(new(40.0, 20.0), "qr", 0.0, 0.0, $o, "hello");
}

func badLinearLevel() {
    def o as BarcodeOptions;
    $o.errorLevel = "H";
    barcode(new(40.0, 20.0), "code128", 0.0, 0.0, $o, "hello");
}

func badCheckDigit() {
    def o as BarcodeOptions;
    $o.checkDigit = "mod7";
    barcode(new(40.0, 20.0), "code128", 0.0, 0.0, $o, "hello");
}

func badBarcodeHeight() {
    def o as BarcodeOptions;
    $o.height = -1.0;
    barcode(new(40.0, 20.0), "code128", 0.0, 0.0, $o, "hello");
}

func badRotation() {
    def o as TextOptions;
    $o.height = 3.0;
    $o.rotation = 45;
    text(new(40.0, 20.0), 0.0, 0.0, $o, "x");
}

func badTextSize() {
    def o as TextOptions;
    $o.points = -8;
    text(new(40.0, 20.0), 0.0, 0.0, $o, "x");
}

func badTextNoSize() {
    def o as TextOptions;
    text(new(40.0, 20.0), 0.0, 0.0, $o, "x");
}

func badOrigin() {
    box(new(40.0, 20.0), -1.0, 0.0, 5.0, 5.0, 0.5);
}

func badBoxSize() {
    box(new(40.0, 20.0), 1.0, 1.0, 5.0, -5.0, 0.5);
}

func badQuantity() {
    quantity(new(40.0, 20.0), -3);
}

func badQuantityHuge() {
    quantity(new(40.0, 20.0), 100000000);
}

func badQuantityStruct() {
    def l as Label init new(40.0, 20.0);
    $l.quantity = 0;
    render($l, cab());
}

# Invalid sizes, data and options are refused at the builder (kind "label")
# instead of reaching the printer as an invalid or silently altered command.
func testInvalidBuildInputsThrow() {
    for (def name in [
        "badNegativeSize",
        "badZeroSize",
        "badEanLength",
        "badEanCheck",
        "badEan8Length",
        "badCode39Star",
        "badCode39Lower",
        "badEmptyData",
        "badQrLevel",
        "badLinearLevel",
        "badCheckDigit",
        "badBarcodeHeight",
        "badRotation",
        "badTextSize",
        "badTextNoSize",
        "badOrigin",
        "badBoxSize",
        "badQuantity",
        "badQuantityHuge",
        "badQuantityStruct"
    ]) {
        testing.assertThrows($name, "label");
    }
}

# EAN data with the check digit omitted, or with a correct one, is accepted.
func testEanLengthsAccepted() {
    def l as Label init barcode(new(40.0, 20.0), "ean13", 0.0, 0.0, noopts(), "400638133393");
    $l = barcode($l, "ean13", 0.0, 0.0, noopts(), "4006381333931");
    $l = barcode($l, "ean8", 0.0, 0.0, noopts(), "9638507");
    $l = barcode($l, "ean8", 0.0, 0.0, noopts(), "96385074");
    testing.assertEqual(len($l.fields), 4);
}
