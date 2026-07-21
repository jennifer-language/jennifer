# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The htmlwriter module (modules/htmlwriter.j): build an element tree and render it to escaped HTML5.
 * Run: jennifer run examples/modules/htmlwriter_demo.j
 * @module htmlwriter_demo
 */
use io;
import "../../modules/htmlwriter.j" as html;

# A list of fruit, escaped as it goes in.
def fruits as list of string init ["apples & pears", "figs", "1 < 2 plums"];

def items as list of html.Node init [];
for (def f in $fruits) {
    def kids as list of html.Node init [];
    $kids[] = html.text($f);
    $items[] = html.element("li", [], $kids);
}

def listAttrs as list of html.Attr init [];
$listAttrs[] = html.attr("class", "fruit");
def ul as html.Node init html.element("ul", $listAttrs, $items);

# A heading, the list, and a void <hr/> assembled as one fragment.
def head as list of html.Node init [];
$head[] = html.text("Shopping & co");
def page as list of html.Node init [];
$page[] = html.element("h1", [], $head);
$page[] = $ul;
$page[] = html.element("hr", [], []);

io.printf("%s\n", html.renderAll($page));
