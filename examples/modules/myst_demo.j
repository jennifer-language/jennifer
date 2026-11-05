# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The myst module (modules/myst.j): read a MyST document - front matter, a
 * target and a reference, directives in both fence styles, roles, dollar math
 * and an embedded rST block - render it to HTML and styled terminal text,
 * report its problems, and lay it out as a PDF with the doctree module.
 * @module myst_demo
 */
use io;
use fs;
use os;
use path;
import "../../modules/myst.j" as myst;
import "../../modules/doctree.j" as doctree;

# MyST goes in raw '...' strings: a cooked "..." string would read {note} as
# interpolation.
def src as string init '---
title: Release notes
---
(changes)=
# What changed

The parser is *faster*; the summary is in {ref}`changes`.

:::{note}
Upgrading needs no code changes.
:::

```{list-table} Speed-ups
:header-rows: 1

* - Area
  - Change
* - parser
  - 2x
```

Water is H{sub}`2`O, and $e = mc^2$.

$$
\int_0^1 x\,dx = \tfrac{1}{2}
$$ (eq-int)

See {eq}`eq-int`.

```{eval-rst}
.. tip:: This block is reStructuredText.
```
';

io.printf("=== front matter ===\n%s\n\n", myst.frontMatter($src));
io.printf("=== HTML ===\n%s\n\n", myst.toHtml($src));
io.printf("=== ANSI ===\n%s\n\n", myst.toAnsi($src));

# Lenient parsing: problems come back from check.
def broken as string init 'See {ref}`nowhere` and {bogus}`x`.

```{frobnicate}
y
```
';
for (def d in myst.check($broken)) {
    io.printf("%s, line %d: %s\n", $d.severity, $d.line, $d.message);
}

def doc as doctree.Node init myst.parse($src);
def pdfBytes as bytes init doctree.renderPdf($doc, doctree.pdfDefaults());
def pdfPath as string init path.join(os.tempDir(), "myst_demo.pdf");
fs.writeBytes($pdfPath, $pdfBytes);
io.printf("\n=== PDF ===\nwrote %d bytes to %s\n", len($pdfBytes), $pdfPath);
