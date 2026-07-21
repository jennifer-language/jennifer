# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# label_test.j - white-box tests for label.j. Run with:
#
#     jennifer test modules/label_test.j
#
# The overlay splices label.j (and its included label_zpl.j / label_cab.j) in
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
    $l = text($l, 5.0, 5.0, TextOptions{ height: 4.0, points: 0, rotation: 0, bold: false }, "HELLO");
    $l = box($l, 2.0, 2.0, 46.0, 26.0, 0.5);
    $l = barcode($l, 5.0, 20.0, "code128", noopts(), "12345678");
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
    def want as string init "^XA\n^FO40,40^A0N,32,32^FH^FDHELLO^FS\n^FO16,16^GB368,208,4^FS\n^FO40,160^BY2^BCN,120,Y,N,N^FH^FD12345678^FS\n^PQ2\n^XZ\n";
    testing.assertEqual(render(sampleLabel(), zpl(203)), $want);
}

func testRenderCab() {
    def want as string init "m m\nJ\nS 0,0,30.0,30.0,50.0\nT 5.0,5.0,0,3,4.0;HELLO\nG 2.0,2.0,0;R:46.0,26.0,0.5,0.5\nB 5.0,20.0,0,CODE128,15.0,0.3;12345678\nA 2\n";
    testing.assertEqual(render(sampleLabel(), cab()), $want);
}

func testRenderCabItf() {
    def l as Label init barcode(new(40.0, 20.0), 5.0, 5.0, "itf", noopts(), "1234567890123");
    def want as string init "m m\nJ\nS 0,0,20.0,20.0,40.0\nB 5.0,5.0,0,2OF5INTERLEAVED,15.0,0.3,3;01234567890123\nA 1\n";
    testing.assertEqual(render($l, cab()), $want);
}

func testItfPadsOddLength() {
    def l as Label init barcode(new(30.0, 20.0), 0.0, 0.0, "itf", noopts(), "123");
    testing.assertEqual($l.fields[0].data, "0123");
}

func testItfKeepsEvenLength() {
    def l as Label init barcode(new(30.0, 20.0), 0.0, 0.0, "itf", noopts(), "1234");
    testing.assertEqual($l.fields[0].data, "1234");
}

func testCabExtraBarcodes() {
    def a as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code39", noopts(), "ABC123");
    testing.assertEqual(render($a, cab()),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,CODE39,15.0,0.3,3;ABC123\nA 1\n");
    def b as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", noopts(), "(00)300653005555555552");
    testing.assertEqual(render($b, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128,15.0,0.3;(00)300653005555555552\nA 1\n");
    def c as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "datamatrix", noopts(), "HELLO");
    testing.assertEqual(render($c, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,DATAMATRIX,1.0;HELLO\nA 1\n");
}

func testZplExtraBarcodes() {
    def a as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code39", noopts(), "ABC123");
    testing.assertEqual(render($a, zpl(203)),
        "^XA\n^FO40,40^BY2^B3N,N,120,Y,N^FH^FDABC123^FS\n^PQ1\n^XZ\n");
    def b as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", noopts(), "(00)300653005555555552");
    testing.assertEqual(render($b, zpl(203)),
        "^XA\n^FO40,40^BY2^BCN,120,Y,N,N,D^FH^FD(00)300653005555555552^FS\n^PQ1\n^XZ\n");
    def c as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "datamatrix", noopts(), "HELLO");
    testing.assertEqual(render($c, zpl(203)),
        "^XA\n^FO40,40^BXN,8,200^FH^FDHELLO^FS\n^PQ1\n^XZ\n");
}

func testImageBothDialects() {
    def l as Label init image(new(40.0, 40.0), 10.0, 10.0, "LOGO");
    testing.assertEqual(render($l, cab()),
        "m m\nJ\nS 0,0,40.0,40.0,40.0\nI 10.0,10.0,0,1,1,a;LOGO\nA 1\n");
    testing.assertEqual(render($l, zpl(203)),
        "^XA\n^FO80,80^XGLOGO,1,1^FS\n^PQ1\n^XZ\n");
}

func testBarcodeOptionsCab() {
    # hideText -> lowercase symbology name (suppress the human-readable line).
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code128", $a, "12345");
    testing.assertEqual(render($la, cab()),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,code128,15.0,0.3;12345\nA 1\n");
    # checkDigit -> +MOD10.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", $b, "(00)30065300555555555");
    testing.assertEqual(render($lb, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128+MOD10,15.0,0.3;(00)30065300555555555\nA 1\n");
    # errorLevel + explicit height override.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    $c.height = 2.0;
    def lc as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "qr", $c, "hello");
    testing.assertEqual(render($lc, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,QRCODE+ELH,2.0;hello\nA 1\n");
}

func testBarcodeOptionsZpl() {
    # hideText -> interpretation line N.
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code128", $a, "12345");
    testing.assertEqual(render($la, zpl(203)),
        "^XA\n^FO40,40^BY2^BCN,120,N,N,N^FH^FD12345^FS\n^PQ1\n^XZ\n");
    # checkDigit -> ITF native check digit Y.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(40.0, 20.0), 5.0, 5.0, "itf", $b, "123456");
    testing.assertEqual(render($lb, zpl(203)),
        "^XA\n^FO40,40^BY2^B2N,120,Y,N,Y^FD123456^FS\n^PQ1\n^XZ\n");
    # QR error level -> ^FD prefix.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    def lc as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "qr", $c, "hello");
    testing.assertEqual(render($lc, zpl(203)),
        "^XA\n^FO40,40^BQN,2,8^FH^FDHA,hello^FS\n^PQ1\n^XZ\n");
}

func badItf() {
    barcode(new(10.0, 10.0), 0.0, 0.0, "itf", noopts(), "12a4");
}

func badBarcodeType() {
    barcode(new(10.0, 10.0), 0.0, 0.0, "pdf417", noopts(), "x");
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
    def l as Label init barcode(new(20.0, 15.0), 3.7, 2.0, "ean8", $o, "93125192");
    testing.assertEqual(render($l, cab()),
        "m m\nJ\nS 0,0,15.0,15.0,20.0\nB 3.7,2.0,0,ean8,6.0,0.2;93125192\nA 1\n");
}

func testEanEightZpl() {
    def o as BarcodeOptions;
    $o.height = 6.0;
    def l as Label init barcode(new(20.0, 15.0), 3.7, 2.0, "ean8", $o, "93125192");
    testing.assertEqual(render($l, zpl(203)),
        "^XA\n^FO30,16^BY2^B8N,48,Y,N^FD93125192^FS\n^PQ1\n^XZ\n");
}

func testTextRotationBoldPointsCab() {
    def o as TextOptions;
    $o.rotation = 90;
    $o.bold = true;
    $o.points = 8;
    def l as Label init text(new(20.0, 15.0), 2.2, 4.0, $o, "06");
    testing.assertEqual(render($l, cab()),
        "m m\nJ\nS 0,0,15.0,15.0,20.0\nT 2.2,4.0,90,5,pt8;06\nA 1\n");
}

func testTextRotationZpl() {
    def o as TextOptions;
    $o.rotation = 90;
    $o.points = 8;
    def l as Label init text(new(20.0, 15.0), 2.2, 4.0, $o, "06");
    # 8 pt at 203 dpi = round(8 * 203 / 72) = 23 dots.
    testing.assertEqual(render($l, zpl(203)),
        "^XA\n^FO18,32^A0B,23,23^FH^FD06^FS\n^PQ1\n^XZ\n");
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
    testing.assertEqual(render($l, cabWith($s)),
        "m m\nJ Barcode1\nH 100,5,T,R0\nO R\nS l1;0.0,0.0,12.0,15.0,17.0,20.0,4\nT 2.2,10.0,0,5,pt8;U260322\nA 1\n");
}

func testItfRatioOption() {
    # an explicit ratio + narrow width overrides the ITF defaults (ne 0.3, ratio 3).
    def o as BarcodeOptions;
    $o.hideText = true;
    $o.height = 6.0;
    $o.moduleWidth = 0.2;
    $o.ratio = 2.0;
    def l as Label init barcode(new(20.0, 15.0), 2.0, 3.1, "itf", $o, "8864441113");
    testing.assertEqual(render($l, cab()),
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
    testing.assertEqual(render($l, cabWith($s)),
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
    testing.assertEqual(render($l, cab()),
        "m m\nJ\nS 0,0,30.0,30.0,50.0\nT 5.0,5.0,0,3,4.0;HI\nA 1\n");
}

# plainMm returns a plain millimetre-height TextOptions.
func plainMm(h as float) {
    def o as TextOptions;
    $o.height = $h;
    return $o;
}
