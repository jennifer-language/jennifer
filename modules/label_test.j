# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
    $l = text($l, 5.0, 5.0, TextOptions{ height: 4.0 }, "HELLO");
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
    def want as string init "^XA\n^FO40,40^A0N,32,32^FH^FDHELLO^FS\n^FO16,16^GB368,208,4^FS\n^FO40,160^BY2^BCN,120,Y,N,N^FD12345678^FS\n^PQ2\n^XZ\n";
    testing.assertEqual(render(sampleLabel(), Device{ dialect: "zpl", dpi: 203 }), $want);
}

func testRenderCab() {
    def want as string init "m m\nJ\nS 0,0,30.0,30.0,50.0\nT 5.0,5.0,0,3,4.0;HELLO\nG 2.0,2.0,0;R:46.0,26.0,0.5,0.5\nB 5.0,20.0,0,CODE128,15.0,0.3;12345678\nA 2\n";
    testing.assertEqual(render(sampleLabel(), Device{ dialect: "cab", dpi: 0 }), $want);
}

func testRenderCabItf() {
    def l as Label init barcode(new(40.0, 20.0), 5.0, 5.0, "itf", noopts(), "1234567890123");
    def want as string init "m m\nJ\nS 0,0,20.0,20.0,40.0\nB 5.0,5.0,0,2OF5INTERLEAVED,15.0,0.3,3;01234567890123\nA 1\n";
    testing.assertEqual(render($l, Device{ dialect: "cab", dpi: 0 }), $want);
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
    testing.assertEqual(render($a, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,CODE39,15.0,0.3,3;ABC123\nA 1\n");
    def b as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", noopts(), "(00)300653005555555552");
    testing.assertEqual(render($b, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128,15.0,0.3;(00)300653005555555552\nA 1\n");
    def c as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "datamatrix", noopts(), "HELLO");
    testing.assertEqual(render($c, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,DATAMATRIX,1.0;HELLO\nA 1\n");
}

func testZplExtraBarcodes() {
    def a as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code39", noopts(), "ABC123");
    testing.assertEqual(render($a, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BY2^B3N,N,120,Y,N^FDABC123^FS\n^PQ1\n^XZ\n");
    def b as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", noopts(), "(00)300653005555555552");
    testing.assertEqual(render($b, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BY2^BCN,120,Y,N,N,D^FD(00)300653005555555552^FS\n^PQ1\n^XZ\n");
    def c as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "datamatrix", noopts(), "HELLO");
    testing.assertEqual(render($c, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BXN,8,200^FDHELLO^FS\n^PQ1\n^XZ\n");
}

func testImageBothDialects() {
    def l as Label init image(new(40.0, 40.0), 10.0, 10.0, "LOGO");
    testing.assertEqual(render($l, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,40.0,40.0,40.0\nI 10.0,10.0,0,1,1,a;LOGO\nA 1\n");
    testing.assertEqual(render($l, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO80,80^XGLOGO,1,1^FS\n^PQ1\n^XZ\n");
}

func testBarcodeOptionsCab() {
    # hideText -> lowercase symbology name (suppress the human-readable line).
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code128", $a, "12345");
    testing.assertEqual(render($la, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,20.0,20.0,50.0\nB 5.0,5.0,0,code128,15.0,0.3;12345\nA 1\n");
    # checkDigit -> +MOD10.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(80.0, 30.0), 5.0, 5.0, "gs1-128", $b, "(00)30065300555555555");
    testing.assertEqual(render($lb, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,30.0,30.0,80.0\nB 5.0,5.0,0,EAN128+MOD10,15.0,0.3;(00)30065300555555555\nA 1\n");
    # errorLevel + explicit height override.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    $c.height = 2.0;
    def lc as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "qr", $c, "hello");
    testing.assertEqual(render($lc, Device{ dialect: "cab", dpi: 0 }),
        "m m\nJ\nS 0,0,30.0,30.0,30.0\nB 5.0,5.0,0,QRCODE+ELH,2.0;hello\nA 1\n");
}

func testBarcodeOptionsZpl() {
    # hideText -> interpretation line N.
    def a as BarcodeOptions;
    $a.hideText = true;
    def la as Label init barcode(new(50.0, 20.0), 5.0, 5.0, "code128", $a, "12345");
    testing.assertEqual(render($la, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BY2^BCN,120,N,N,N^FD12345^FS\n^PQ1\n^XZ\n");
    # checkDigit -> ITF native check digit Y.
    def b as BarcodeOptions;
    $b.checkDigit = "mod10";
    def lb as Label init barcode(new(40.0, 20.0), 5.0, 5.0, "itf", $b, "123456");
    testing.assertEqual(render($lb, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BY2^B2N,120,Y,N,Y^FD123456^FS\n^PQ1\n^XZ\n");
    # QR error level -> ^FD prefix.
    def c as BarcodeOptions;
    $c.errorLevel = "H";
    def lc as Label init barcode(new(30.0, 30.0), 5.0, 5.0, "qr", $c, "hello");
    testing.assertEqual(render($lc, Device{ dialect: "zpl", dpi: 203 }),
        "^XA\n^FO40,40^BQN,2,5^FDHA,hello^FS\n^PQ1\n^XZ\n");
}

func badItf() {
    barcode(new(10.0, 10.0), 0.0, 0.0, "itf", noopts(), "12a4");
}

func badBarcodeType() {
    barcode(new(10.0, 10.0), 0.0, 0.0, "pdf417", noopts(), "x");
}

func badDialect() {
    render(new(10.0, 10.0), Device{ dialect: "xyz", dpi: 203 });
}

func testInvalidInputsThrow() {
    testing.assertThrows("badItf", "label");
    testing.assertThrows("badBarcodeType", "label");
    testing.assertThrows("badDialect", "label");
}
