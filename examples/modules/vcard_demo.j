#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The vcard module (modules/vcard.j): build two contact cards, encode them to
 * RFC 6350 vCard 4.0 text (escaped, folded, CRLF), then parse them back.
 * Run: jennifer run examples/modules/vcard_demo.j
 * @module vcard_demo
 */
use io;
import "../../modules/vcard.j" as vcard;

def ada as vcard.Card init vcard.card("Ada Lovelace");
$ada = vcard.withName($ada, "Lovelace", "Ada");
$ada = vcard.withOrg($ada, "Analytical Engines", "Mathematician");
$ada = vcard.addEmail($ada, "ada@example.com");
$ada = vcard.addPhone($ada, "+44-20-7946-0000");
$ada = vcard.addAddress($ada, vcard.address("12 St James's Sq", "London", "", "SW1Y 4LE", "UK"));
$ada = vcard.withNote($ada, "First programmer; note the comma, and semicolon.");

def grace as vcard.Card init vcard.card("Grace Hopper");
$grace = vcard.withName($grace, "Hopper", "Grace");
$grace = vcard.addEmail($grace, "grace@navy.mil");

def text as string init vcard.encodeAll([$ada, $grace]);
io.printf("=== encoded vCard ===\n%s\n", $text);

def cards as list of vcard.Card init vcard.parse($text);
io.printf("=== parsed %d cards ===\n", len($cards));
for (def c in $cards) {
    io.printf("- %s <%s>\n", $c.formattedName, $c.emails[0]);
    if (len($c.addresses) > 0) {
        io.printf("    %s, %s %s\n", $c.addresses[0].street, $c.addresses[0].locality, $c.addresses[0].country);
    }
}
