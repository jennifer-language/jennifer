#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Build one label and render it in both printer dialects.
 * The build and render stages are pure and run on both binaries; this prints the ZPL and cab JScript command streams to stdout. To actually print, pipe a rendered stream to a printer's :9100 raw port with label.send(host, 9100, rendered) - that needs the default `jennifer` binary (it uses `net`) and a printer.
 * @module label_demo
 */
use io;
import "../../modules/label.j" as label;

# A device-independent 100x50 mm shipping label, described once in millimetres.
def l as label.Label init label.new(100.0, 50.0);
$l = label.box($l, 1.0, 1.0, 98.0, 48.0, 0.4);
def title as label.TextOptions; $title.height = 6.0; $title.bold = true;
$l = label.text($l, 5.0, 5.0, $title, "ACME LOGISTICS");
def small as label.TextOptions; $small.height = 3.0;
$l = label.text($l, 5.0, 14.0, $small, "Order 4711");
# A point-sized caption turned on its side (rotation is portable across dialects).
def side as label.TextOptions; $side.points = 7; $side.rotation = 90;
$l = label.text($l, 96.0, 20.0, $side, "PICK");
# Most barcodes need no options; a zero-value BarcodeOptions is "no options".
def noopts as label.BarcodeOptions;
$l = label.barcode($l, 5.0, 22.0, "code128", $noopts, "4711000512");
# EAN-8 (7 data digits + check) with its own text line suppressed and a tight
# narrow-element width.
def ean as label.BarcodeOptions;
$ean.hideText = true;
$ean.height = 6.0;
$ean.moduleWidth = 0.25;
$l = label.barcode($l, 5.0, 38.0, "ean8", $ean, "93125192");
# A GS1-128 logistics code (SSCC) and a 2D DataMatrix with a chosen error level.
$l = label.barcode($l, 55.0, 22.0, "gs1-128", $noopts, "(00)340123450000000012");
def dmopts as label.BarcodeOptions;
$dmopts.errorLevel = "M";
$l = label.barcode($l, 80.0, 33.0, "datamatrix", $dmopts, "https://mplx.eu/4711");
# A company logo already stored on the printer (cab: images/ folder).
$l = label.image($l, 82.0, 5.0, "LOGO");
$l = label.quantity($l, 3);

io.printf("=== ZPL (Zebra, 203 dpi) ===\n");
io.printf("%s\n", label.render($l, label.zpl(203)));

# cab JScript with printer setup that has no ZPL equivalent (heat/speed,
# orientation, and a named label sensor) carried in a cab-only CabSetup.
def setup as label.CabSetup;
$setup.jobName = "Shipping";
$setup.heat = 100;
$setup.speed = 5;
$setup.mode = "T,R0";
$setup.orientation = "R";
$setup.sensor = "l1";
io.printf("=== cab JScript ===\n");
io.printf("%s", label.render($l, label.cabWith($setup)));

# To print, emit the rendered stream to the printer's raw port:
#   label.send("192.168.1.50", 9100, label.render($l, label.cab()));
