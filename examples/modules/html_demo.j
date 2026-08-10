# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The html module (modules/html.j): build an element tree and render it to escaped
 * HTML5, then parse HTML back into the same tree and query it with selectors.
 * Run: jennifer run examples/modules/html_demo.j
 * @module html_demo
 */
use io;
import "../../modules/html.j" as html;

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

# --- parse existing HTML back into the same node tree, and query it ----------
io.printf("\n--- parse + query ---\n");
def doc as html.Node init html.parse("<article><h2>Fruit</h2><ul><li>apples</li><li>figs</li></ul></article>");
io.printf("heading: %s\n", html.get($doc, "article/h2").children[0].text);
for (def li in html.findAll($doc, "article/ul/li")) {
    io.printf("  item: %s\n", $li.children[0].text);
}
# a parsed subtree re-renders through the same render() - build and parse share one model.
io.printf("re-rendered ul: %s\n", html.render(html.get($doc, "article/ul")));
