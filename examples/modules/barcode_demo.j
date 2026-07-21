#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The barcode module (modules/barcode.j): generate scannable codes as images.
 * Prints a QR code as terminal art, writes a QR PNG and SVG, and writes a
 * Code 128 barcode PNG - all decodable by any scanner. Pure `.j`; runs on both
 * binaries. Output files land in the current directory.
 * Run: jennifer run examples/modules/barcode_demo.j [text]
 * @module barcode_demo
 */
use io;
use os;
use fs;
import "../../modules/barcode.j" as barcode;

def text as string init "https://github.com/jennifer-language/jennifer/";
if (len(os.ARGS) > 1) { $text = os.ARGS[1]; }

def o as barcode.Options init barcode.defaults();
$o.scale = 6;

# QR as terminal art (Unicode half-blocks).
def qr as barcode.Symbol init barcode.encode($text, "qr", $o);
io.printf("QR for: %s   (version %d, %dx%d modules)\n\n", $text, ($qr.size - 17) // 4, $qr.size, $qr.size);
io.printf("%s\n", barcode.terminal($qr));

# QR to PNG and SVG.
fs.writeBytes("qr.png", barcode.png($qr, $o));
fs.writeString("qr.svg", barcode.svg($qr, $o));

# A Code 128 barcode to PNG.
def code as barcode.Symbol init barcode.encode("JENNIFER-2026", "code128", $o);
fs.writeBytes("code128.png", barcode.png($code, $o));

io.printf("wrote qr.png, qr.svg, code128.png\n");
