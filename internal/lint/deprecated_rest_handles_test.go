// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedRestHandles: L301 reports the deprecated forms of the handle
// batch (M29.48, M29.49): the pin-only gpio calls and gpio.chip, spi.configure,
// and httpd's listenTLS / listenTLSWith / serveFileEtag / serveDirEtag; the
// current forms are left alone.
func TestDeprecatedRestHandles(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"gpio pin-only", "use gpio;\ngpio.setup(17, gpio.OUT);\ngpio.write(17, 1);\ndef v as int init gpio.read(17);\ngpio.release(17);", 4,
			"is deprecated and removed in 0.29.0; open the chip with `gpio.open(path)` and pass the `gpio.Chip` first"},
		{"gpio.chip", "use gpio;\ngpio.chip(\"/dev/gpiochip1\");", 1,
			"`gpio.chip` is deprecated and removed in 0.29.0; use `gpio.open(path)`"},
		{"gpio aliased", "use gpio as g;\ng.setup(17, g.OUT);", 1, "the pin-only `g.setup`"},
		{"gpio chip handle", "use gpio;\ndef c as gpio.Chip init gpio.open(\"/dev/gpiochip0\");\ngpio.setup($c, 17, gpio.OUT);\ngpio.write($c, 17, 1);\ndef v as int init gpio.read($c, 17);\ngpio.release($c, 17);\ngpio.close($c);", 0, ""},
		{"spi.configure", "use spi;\ndef d as spi.Device init spi.open(\"/dev/spidev0.0\");\nspi.configure($d, 0, 1000000);", 1,
			"`spi.configure` is deprecated and removed in 0.29.0; use `spi.openWith(path, spi.Options{mode: ..., speedHz: ..., bitsPerWord: ...})`"},
		{"spi.openWith", "use spi;\ndef d as spi.Device init spi.openWith(\"/dev/spidev0.0\", spi.Options{speedHz: 1000000});", 0, ""},
		{"httpd.listenTLS", "use httpd;\ndef pem as bytes;\ndef s as httpd.Server init httpd.listenTLS(\":8443\", $pem, $pem);", 1,
			"`httpd.listenTLS` is deprecated and removed in 0.29.0; use `httpd.listenWith(addr, httpd.Options{certPem: ..., keyPem: ...})`"},
		{"httpd.serveFileEtag", "use httpd;\nfunc h(r as httpd.Request) {\n    httpd.serveFileEtag($r, \"index.html\");\n}", 1,
			"use `httpd.serveFile` on a server opened with `httpd.Options{etag: true}`"},
		{"httpd.listenWith TLS", "use httpd;\ndef pem as bytes;\ndef s as httpd.Server init httpd.listenWith(\":8443\", httpd.Options{certPem: $pem, keyPem: $pem, etag: true});", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Fatalf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			for _, d := range diags {
				if d.ID == "L301" && !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}
