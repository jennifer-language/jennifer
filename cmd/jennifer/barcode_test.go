// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBarcodePng renders a QR and a Code 128 symbol to PNG via the module, then
// (1) decodes each with Go's image/png to prove the hand-rolled PNG encoder
// (IHDR, zlib IDAT, CRC-32 chunks) is byte-correct and (2), when zbarimg is
// available, decodes them optically to prove the symbols actually scan.
func TestBarcodePng(t *testing.T) {
	barcodeMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "barcode.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use fs;
import %q as barcode;
def const DIR as string init %q;

def o as barcode.Options init barcode.defaults();
$o.scale = 3;
$o.height = 12;
def qr as barcode.Symbol init barcode.encode("BARCODE TEST", "qr", $o);
fs.writeBytes(DIR + "/qr.png", barcode.png($qr, $o));

def c as barcode.Symbol init barcode.encode("CODE128", "code128", $o);
fs.writeBytes(DIR + "/c128.png", barcode.png($c, $o));
`, barcodeMod, dir)
	progPath := filepath.Join(dir, "gen.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("barcode generation program failed with code %d", code)
	}

	qrPath := filepath.Join(dir, "qr.png")
	c128Path := filepath.Join(dir, "c128.png")

	// (1) The hand-rolled PNGs must decode with the standard library.
	qrImg := decodePNG(t, qrPath)
	if b := qrImg.Bounds(); b.Dx() != b.Dy() {
		t.Errorf("QR PNG is not square: %dx%d", b.Dx(), b.Dy())
	}
	// version 1 (size 21) + quiet 4 each side, scale 3 -> (21+8)*3 = 87
	if got := qrImg.Bounds().Dx(); got != 87 {
		t.Errorf("QR PNG width = %d, want 87", got)
	}
	// the finder centre module (3,3) is dark: pixel ((quiet+3)*scale) = 21
	px := (4 + 3) * 3
	if r, _, _, _ := qrImg.At(px, px).RGBA(); r > 0x8000 {
		t.Errorf("expected a dark finder-centre pixel at (%d,%d)", px, px)
	}
	decodePNG(t, c128Path) // just prove it decodes

	// (2) Optical decode when zbarimg is present (skipped otherwise).
	zbar, err := exec.LookPath("zbarimg")
	if err != nil {
		t.Log("zbarimg not found; skipping the optical-scan check")
		return
	}
	assertScans(t, zbar, qrPath, "BARCODE TEST")
	assertScans(t, zbar, c128Path, "CODE128")
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("stdlib png.Decode rejected %s: %v", filepath.Base(path), err)
	}
	return img
}

func assertScans(t *testing.T, zbar, path, want string) {
	t.Helper()
	out, _ := exec.Command(zbar, "--quiet", "-q", path).Output()
	if !strings.Contains(string(out), want) {
		t.Errorf("zbarimg(%s) = %q, want it to contain %q", filepath.Base(path), out, want)
	}
}
