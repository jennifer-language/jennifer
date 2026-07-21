# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# vcard_test.j - white-box tests for vcard.j. Run with:
#
#     jennifer test modules/vcard_test.j
#
# The overlay splices vcard.j (and, through its include, ical_vcard_shared.j) in front
# of this file, so the tests reach the private helpers (encodeAdr,
# splitStructured, component, escapeText, unescapeText, fold, propName) by bare
# identifier as well as the exported surface. vcard.j already `use`s strings /
# lists, so the overlay only adds testing.
use testing;

func sample() {
    def c as Card init card("Ada Lovelace; Countess");
    $c = withName($c, "Lovelace", "Ada");
    $c = withOrg($c, "Analytical Engines, Ltd.", "Mathematician");
    $c = addEmail($c, "ada@example.com");
    $c = addEmail($c, "countess@example.org");
    $c = addPhone($c, "+44-20-7946-0000");
    $c = addAddress($c, address("12 St James's Sq", "London", "", "SW1Y 4LE", "UK"));
    $c = withUrl($c, "https://example.com/ada");
    $c = withNote($c, "First programmer.\nLoved, notes; and semis.");
    return $c;
}

# --- shared content-line codec (private, from ical_vcard_shared.j) ----------

func testEscapeShared() {
    testing.assertEqual(escapeText("a,b;c"), "a\\,b\\;c");
    testing.assertEqual(unescapeText("a\\,b\\;c"), "a,b;c");
}

func testFoldShared() {
    def long as string init "NOTE:" + strings.repeat("y", 200);
    testing.assertEqual(unfold(fold($long)), $long);
}

# --- structured values (private) --------------------------------------------

func testEncodeAdr() {
    def a as Address init address("1 High St", "Springfield", "IL", "62701", "USA");
    testing.assertEqual(encodeAdr($a), ";;1 High St;Springfield;IL;62701;USA");
}

func testSplitStructured() {
    def parts as list of string init splitStructured("Lovelace;Ada;;;");
    testing.assertEqual(len($parts), 5);
    testing.assertEqual($parts[0], "Lovelace");
    testing.assertEqual($parts[1], "Ada");
    testing.assertEqual($parts[4], "");
}

func testSplitStructuredKeepsEscapedSemicolon() {
    # An escaped `\;` is not a component boundary.
    def parts as list of string init splitStructured("O'Brien\\; Jr;Pat");
    testing.assertEqual(len($parts), 2);
    testing.assertEqual(unescapeText($parts[0]), "O'Brien; Jr");
    testing.assertEqual($parts[1], "Pat");
}

func testComponentOutOfRange() {
    def parts as list of string init splitStructured("a;b");
    testing.assertEqual(component($parts, 0), "a");
    testing.assertEqual(component($parts, 5), "");   # missing component -> ""
}

# --- encode (exported) ------------------------------------------------------

func testEncodeStructure() {
    def text as string init encode(sample());
    testing.assertTrue(strings.startsWith($text, "BEGIN:VCARD\r\n"));
    testing.assertTrue(strings.contains($text, "VERSION:4.0\r\n"));
    testing.assertTrue(strings.contains($text, "FN:Ada Lovelace\\; Countess\r\n"));
    testing.assertTrue(strings.contains($text, "N:Lovelace;Ada;;;\r\n"));
    testing.assertTrue(strings.contains($text, "ORG:Analytical Engines\\, Ltd.\r\n"));
    testing.assertTrue(strings.contains($text, "ADR:;;12 St James's Sq;London;;SW1Y 4LE;UK\r\n"));
    testing.assertTrue(strings.contains($text, "END:VCARD\r\n"));
}

func testEncodeOmitsEmptyFields() {
    def text as string init encode(card("Just A Name"));
    testing.assertTrue(strings.contains($text, "FN:Just A Name\r\n"));
    testing.assertFalse(strings.contains($text, "ORG"));
    testing.assertFalse(strings.contains($text, "TITLE"));
    testing.assertFalse(strings.contains($text, "\r\nN:"));   # the N property line (not FN)
    testing.assertFalse(strings.contains($text, "ADR"));
    testing.assertFalse(strings.contains($text, "NOTE"));
}

# --- parse + round-trip (exported) ------------------------------------------

func testRoundTrip() {
    def cards as list of Card init parse(encode(sample()));
    testing.assertEqual(len($cards), 1);
    def c as Card init $cards[0];
    testing.assertEqual($c.formattedName, "Ada Lovelace; Countess");
    testing.assertEqual($c.family, "Lovelace");
    testing.assertEqual($c.given, "Ada");
    testing.assertEqual($c.organization, "Analytical Engines, Ltd.");
    testing.assertEqual($c.title, "Mathematician");
    testing.assertEqual(len($c.emails), 2);
    testing.assertEqual($c.emails[1], "countess@example.org");
    testing.assertEqual($c.phones[0], "+44-20-7946-0000");
    testing.assertEqual(len($c.addresses), 1);
    testing.assertEqual($c.addresses[0].locality, "London");
    testing.assertEqual($c.addresses[0].postalCode, "SW1Y 4LE");
    testing.assertEqual($c.url, "https://example.com/ada");
    testing.assertEqual($c.note, "First programmer.\nLoved, notes; and semis.");
}

func testEncodeAllAndParseMany() {
    def cards as list of Card init [];
    $cards[] = card("First Person");
    $cards[] = withName(card("Second Person"), "Second", "Person");
    def text as string init encodeAll($cards);
    def back as list of Card init parse($text);
    testing.assertEqual(len($back), 2);
    testing.assertEqual($back[0].formattedName, "First Person");
    testing.assertEqual($back[1].family, "Second");
}

func testParseIgnoresParameters() {
    def src as string init "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Param User\r\nEMAIL;TYPE=work:work@x.com\r\nTEL;TYPE=cell:+15550000\r\nEND:VCARD\r\n";
    def cards as list of Card init parse($src);
    testing.assertEqual(len($cards), 1);
    testing.assertEqual($cards[0].emails[0], "work@x.com");
    testing.assertEqual($cards[0].phones[0], "+15550000");
}

func testParseOrgTakesFirstComponent() {
    def src as string init "BEGIN:VCARD\r\nFN:X\r\nORG:Acme;Engineering;Backend\r\nEND:VCARD\r\n";
    def cards as list of Card init parse($src);
    testing.assertEqual($cards[0].organization, "Acme");
}

func testParseFoldedNote() {
    def src as string init "BEGIN:VCARD\r\nFN:Folded\r\nNOTE:first part \r\n and second\r\nEND:VCARD\r\n";
    def cards as list of Card init parse($src);
    testing.assertEqual($cards[0].note, "first part and second");
}

func testParseEmptyYieldsNoCards() {
    testing.assertEqual(len(parse("")), 0);
    testing.assertEqual(len(parse("not a vcard at all")), 0);
}
