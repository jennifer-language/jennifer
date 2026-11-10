# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The images library: build a picture, edit it, encode it and read it back.
 * Pixels stay on the Go side; every call returns a new images.Image.
 * @module images
 */

use images;
use io;

# A 320x200 sky-blue canvas with a half-transparent red box on it.
def sky as images.Image init images.create(320, 200, images.hex("#87ceeb"));
def boxed as images.Image init images.fill($sky, 40, 30, 120, 80, images.rgba(255, 0, 0, 128));
io.printf("canvas %v\n", $boxed);
io.printf("inside the box %v\n", images.pixel($boxed, 50, 40));
io.printf("outside it     %v\n", images.pixel($boxed, 300, 190));

# Encode to PNG and read the header back without decoding the pixels.
def png as bytes init images.encode("png", $boxed);
io.printf("info %v\n", images.info($png));
io.printf("PNG round-trips exactly: %t\n", images.decode($png) == $boxed);

# A thumbnail: fit inside 100x100, keeping the aspect ratio.
def thumb as images.Image init images.fit($boxed, 100, 100);
io.printf("thumbnail %dx%d\n", images.width($thumb), images.height($thumb));

# Turn it a quarter clockwise and stamp it into a corner of the canvas.
def turned as images.Image init images.rotate($thumb, 90);
def stamped as images.Image init images.draw($boxed, $turned, 320 - images.width($turned), 0);
io.printf("stamped corner %v\n", images.pixel($stamped, 319, 0));

# One-bit output for a label printer: grayscale, then Floyd-Steinberg.
def mono as images.Image init images.dither(images.grayscale($boxed));
def raw as bytes init images.pixels($mono);
def black as int init 0;
for (def i in 0..len($raw) // 4) {
    if ($raw[$i * 4] == 0) {
        $black = $black + 1;
    }
}
io.printf("dithered: %d of %d pixels black\n", $black, len($raw) // 4);

# A decode bomb is refused from the header alone.
try {
    images.decode($png, 1000);
} catch (e) {
    io.printf("%s: %s\n", $e.kind, $e.message);
}
