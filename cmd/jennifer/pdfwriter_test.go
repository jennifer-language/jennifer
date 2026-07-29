// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPdfwriterProducesValidPDF renders a PDF through the module and checks it:
// always with byte-level structural assertions, and - when the tools are on the
// PATH - with the real validators qpdf (--check) and pdftotext.
func TestPdfwriterProducesValidPDF(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "out.pdf")
	mod, err := filepath.Abs(filepath.Join("..", "..", "modules", "pdfwriter.j"))
	if err != nil {
		t.Fatal(err)
	}
	fontPath, err := filepath.Abs(filepath.Join("..", "..", "modules", "testdata", "font_fixture.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	testdata := func(name string) string {
		p, aerr := filepath.Abs(filepath.Join("..", "..", "modules", "testdata", name))
		if aerr != nil {
			t.Fatal(aerr)
		}
		return p
	}
	rgbPng, rgbaPng, jpg := testdata("img_rgb.png"), testdata("img_rgba.png"), testdata("img.jpg")
	prog := fmt.Sprintf(`import %q as pdf;
use fs;
def const PATH as string init %q;

def one as pdf.Page init pdf.page(612, 792);
$one = pdf.text($one, 72, 720, "Helvetica", 24, "Hello Jennifer PDF");
$one = pdf.text($one, 72, 690, "Times-Roman", 12, "escaped (parens) and \\slash\\");
$one = pdf.color($one, 30, 90, 200);
$one = pdf.rect($one, 72, 600, 200, 60, true);
$one = pdf.line($one, 72, 560, 540, 560);

# three embedded raster images: opaque RGB PNG, RGBA PNG (soft mask), RGB JPEG
def rgb as pdf.Image init pdf.loadImage("Rgb", fs.readBytes(%q));
def rgba as pdf.Image init pdf.loadImage("Rgba", fs.readBytes(%q));
def jpg as pdf.Image init pdf.loadImage("Jpg", fs.readBytes(%q));
$one = pdf.drawImage($one, $rgb, 72, 440, 96, 72);
$one = pdf.drawImage($one, $rgba, 200, 440, 96, 72);
$one = pdf.drawImage($one, $jpg, 328, 440, 96, 72);

# a wrapped, justified text block (text layout)
def const BLURB as string init "Jennifer flows wrapped and justified paragraphs into a column using the standard fourteen font metrics for accurate width measurement.";
$one = pdf.textBlock($one, 72, 400, 460, "Helvetica", 11, 15, BLURB, "justify");

def two as pdf.Page init pdf.page(595, 842);
$two = pdf.text($two, 50, 800, "Courier", 10, "second page");

# an embedded TrueType font drawing selectable Unicode text
def lf as pdf.LoadedFont init pdf.loadFont("Body", fs.readBytes(%q));
$two = pdf.textUnicode($two, 50, 760, $lf, 20, "ABC");

def doc as pdf.Document init pdf.document();
$doc = pdf.info($doc, "Title", "Jennifer Report");
$doc = pdf.info($doc, "Author", "Ada Lovelace");
$doc = pdf.addFont($doc, $lf);
$doc = pdf.addImage($doc, $rgb);
$doc = pdf.addImage($doc, $rgba);
$doc = pdf.addImage($doc, $jpg);
$doc = pdf.addPage($doc, $one);
$doc = pdf.addPage($doc, $two);
fs.writeBytes(PATH, pdf.render($doc));
`, mod, pdfPath, rgbPng, rgbaPng, jpg, fontPath)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("pdf generation program failed: code %d", code)
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	// Structural checks that never need an external tool.
	for _, want := range [][]byte{
		[]byte("%PDF-1.7\n"),
		[]byte("/Type /Catalog"),
		[]byte("/Type /Pages"),
		[]byte("/Filter /FlateDecode"),
		[]byte("/Producer (Jennifer pdfwriter)"),
		[]byte("/Title (Jennifer Report)"),
		[]byte("/Info "),
		[]byte("/Subtype /Type0"),
		[]byte("/CIDFontType2"),
		[]byte("/FontFile2"),
		[]byte("/Encoding /Identity-H"),
		[]byte("/ToUnicode"),
		[]byte("/Subtype /Image"),
		[]byte("/Filter /DCTDecode"),
		[]byte("/SMask "),
		[]byte("/Predictor 15"),
		[]byte("/XObject << /Rgb "),
		[]byte("\nxref\n"),
		[]byte("\ntrailer\n"),
		[]byte("\nstartxref\n"),
		[]byte("%%EOF"),
	} {
		if !bytes.Contains(data, want) {
			t.Errorf("PDF missing %q", want)
		}
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.7")) {
		t.Errorf("PDF does not start with the header")
	}

	// qpdf --check: a rigorous structural / stream validator.
	if qpdf, lerr := exec.LookPath("qpdf"); lerr == nil {
		out, _ := exec.Command(qpdf, "--check", pdfPath).CombinedOutput()
		if !strings.Contains(string(out), "No syntax or stream encoding errors found") {
			t.Errorf("qpdf --check did not pass cleanly:\n%s", out)
		}
	} else {
		t.Log("qpdf not on PATH; skipping structural validation")
	}

	// pdftotext: confirm the text (and escaping) survived into the PDF.
	if ptt, lerr := exec.LookPath("pdftotext"); lerr == nil {
		out, _ := exec.Command(ptt, pdfPath, "-").Output()
		text := string(out)
		for _, want := range []string{"Hello Jennifer PDF", "escaped (parens) and \\slash\\", "second page", "ABC", "measurement"} {
			if !strings.Contains(text, want) {
				t.Errorf("pdftotext missing %q; got:\n%s", want, text)
			}
		}
	} else {
		t.Log("pdftotext not on PATH; skipping text extraction check")
	}

	// pdfimages: confirm the three embedded images (and the RGBA soft mask)
	// decode. The listing has one row per image plus a "smask" row for the
	// RGBA image, and a "jpeg" encoding for the DCTDecode image.
	if pim, lerr := exec.LookPath("pdfimages"); lerr == nil {
		out, _ := exec.Command(pim, "-list", pdfPath).Output()
		listing := string(out)
		if n := strings.Count(listing, " image "); n < 3 {
			t.Errorf("pdfimages found %d image rows, want >= 3; got:\n%s", n, listing)
		}
		for _, want := range []string{"jpeg", "smask"} {
			if !strings.Contains(listing, want) {
				t.Errorf("pdfimages listing missing %q; got:\n%s", want, listing)
			}
		}
	} else {
		t.Log("pdfimages not on PATH; skipping image check")
	}

	// pdfinfo: confirm the Info-dictionary metadata is readable.
	if pi, lerr := exec.LookPath("pdfinfo"); lerr == nil {
		out, _ := exec.Command(pi, pdfPath).Output()
		info := string(out)
		for _, want := range []string{"Jennifer Report", "Ada Lovelace", "Jennifer pdfwriter"} {
			if !strings.Contains(info, want) {
				t.Errorf("pdfinfo missing metadata %q; got:\n%s", want, info)
			}
		}
	} else {
		t.Log("pdfinfo not on PATH; skipping metadata check")
	}
}
