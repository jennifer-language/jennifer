# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# vcard_test.j - white-box tests for vcard.j. Run with:
#
#     jennifer test modules/vcard_test.j
#
# The overlay splices vcard.j (and, through its include, ical_vcard_shared.inc.j) in front
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

# --- shared content-line codec (private, from ical_vcard_shared.inc.j) ----------

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
    testing.assertEqual(component($parts, 5), ""); # missing component -> ""
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
    testing.assertFalse(strings.contains($text, "\r\nN:")); # the N property line (not FN)
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
    testing.assertEqual($c.emails[1].value, "countess@example.org");
    testing.assertEqual($c.phones[0].value, "+44-20-7946-0000");
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

func testParseReadsTypeParameters() {
    def src as string init "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Param User\r\nEMAIL;TYPE=work:work@x.com\r\nTEL;TYPE=cell:+15550000\r\nEND:VCARD\r\n";
    def cards as list of Card init parse($src);
    testing.assertEqual(len($cards), 1);
    testing.assertEqual($cards[0].emails[0].value, "work@x.com");
    testing.assertEqual($cards[0].emails[0].type, "work");
    testing.assertEqual($cards[0].phones[0].value, "+15550000");
    testing.assertEqual($cards[0].phones[0].type, "cell");
}

# --- full N, TYPE round-trip, common fields ---

func testFullNameRoundTrip() {
    def c as Card init withFullName(
        card("Dr. Pat O'Brien Jr."),
        "O'Brien",
        "Pat",
        "Q",
        "Dr.",
        "Jr.");
    def back as Card init parse(encode($c))[0];
    testing.assertEqual($back.family, "O'Brien");
    testing.assertEqual($back.given, "Pat");
    testing.assertEqual($back.additional, "Q");
    testing.assertEqual($back.prefixes, "Dr.");
    testing.assertEqual($back.suffixes, "Jr.");
}

func testTypedContactsRoundTrip() {
    def c as Card init addPhoneTyped(
        addEmailTyped(addEmailTyped(card("T"), "w@x.com", "work"), "h@x.com", "home"),
        "+15551234",
        "cell");
    $c = addAddress($c, addressTyped("1 St", "Town", "ST", "00000", "US", "home"));
    def text as string init encode($c);
    testing.assertContains($text, "EMAIL;TYPE=work:w@x.com\r\n");
    testing.assertContains($text, "ADR;TYPE=home:;;1 St;Town;ST;00000;US\r\n");
    def back as Card init parse($text)[0];
    testing.assertEqual($back.emails[0].type, "work");
    testing.assertEqual($back.emails[1].type, "home");
    testing.assertEqual($back.phones[0].type, "cell");
    testing.assertEqual($back.addresses[0].type, "home");
}

# A TYPE value containing a `;` / `:` / `,` is quoted so it cannot inject a
# spurious parameter or truncate the value.
func testTypeParameterQuotedWhenSpecial() {
    def c as Card init card("Z");
    $c = addEmailTyped($c, "e@x.com", "work;X=1");
    def text as string init encode($c);
    testing.assertContains($text, "EMAIL;TYPE=\"work;X=1\":e@x.com\r\n");
    def back as Card init parse($text)[0];
    testing.assertEqual($back.emails[0].value, "e@x.com");
    testing.assertEqual($back.emails[0].type, "work;X=1");
}

func testCommonFieldsRoundTrip() {
    def c as Card init withPhoto(
        withBday(withNickname(card("Ada"), "The Countess"), "18151210"),
        "https://x/ada.jpg");
    $c = addCategory(addCategory($c, "vip"), "math, science");
    def text as string init encode($c);
    testing.assertContains($text, "NICKNAME:The Countess\r\n");
    testing.assertContains($text, "BDAY:18151210\r\n");
    testing.assertContains($text, "PHOTO:https://x/ada.jpg\r\n");
    # the comma inside a category value is escaped; the list separator is not
    testing.assertContains($text, "CATEGORIES:vip,math\\, science\r\n");
    def back as Card init parse($text)[0];
    testing.assertEqual($back.nickname, "The Countess");
    testing.assertEqual($back.bday, "18151210");
    testing.assertEqual($back.photo, "https://x/ada.jpg");
    testing.assertEqual(len($back.categories), 2);
    testing.assertEqual($back.categories[1], "math, science");
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
    testing.assertEqual(len(parse("\r\n\r\n")), 0);
}

# A CR/LF in a property value is stripped, so it cannot terminate the folded line
# and inject a forged property into the .vcf / .ics stream.
func testEmitLineStripsCRLF() {
    def out as string init emitLine("BDAY", "1990\r\nXEVIL:injected");
    testing.assertFalse(strings.contains($out, "\n"));
    testing.assertFalse(strings.contains($out, "\r"));
    testing.assertTrue(strings.contains($out, "1990"));
}

# A TYPE parameter holding a quote cannot inject a parameter.
func testTypeParamCannotInject() {
    def name as string init typedProp("EMAIL", "work\";PREF=1");
    testing.assertEqual(paramValue($name, "TYPE"), "work\";PREF=1");
    testing.assertEqual(paramValue($name, "PREF"), "");
}

# --- strict parsing: VcardError ---------------------------------------------

func crlf(s as string) {
    return strings.replace($s, "\n", "\r\n");
}

# parseFault returns "kind@line" of the error parse throws, or "ok".
func parseFault(text as string) {
    try {
        parse($text);
    } catch (e) {
        if ($e.kind == "vcard") {
            def ve as VcardError init $e;
            return "vcard@{$ve.line}";
        }
        return $e.kind;
    }
    return "ok";
}

func testParseRejectsMalformedCards() {
    testing.assertEqual(parseFault("not a vcard at all"), "vcard@1");
    testing.assertEqual(parseFault(crlf("NOTE:x\nBEGIN:VCARD\nFN:A\nEND:VCARD\n")), "vcard@1");
    # no END:VCARD: reported at its BEGIN
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nVERSION:4.0\nFN:Ada\n")), "vcard@1");
    testing.assertEqual(
        parseFault(crlf("BEGIN:VCARD\nFN:A\nBEGIN:VCARD\nFN:B\nEND:VCARD\nEND:VCARD\n")),
        "vcard@3");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nFN:A\nGARBAGE\nEND:VCARD\n")), "vcard@3");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nFN:A\nEND:VCALENDAR\n")), "vcard@3");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nVERSION:5.0\nFN:A\nEND:VCARD\n")), "vcard@2");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nFN:A\nEND:VCARD\njunk\n")), "vcard@4");
}

func testParseRequiresFnExceptIn21() {
    testing.assertEqual(
        parseFault(crlf("BEGIN:VCARD\nVERSION:4.0\nN:L;A;;;\nEND:VCARD\n")),
        "vcard@1");
    testing.assertEqual(
        parseFault(crlf("BEGIN:VCARD\nVERSION:3.0\nN:L;A;;;\nEND:VCARD\n")),
        "vcard@1");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nN:L;A;;;\nEND:VCARD\n")), "vcard@1");
    def c as Card init parse(crlf("BEGIN:VCARD\nVERSION:2.1\nN:L;A;;;\nEND:VCARD\n"))[0];
    testing.assertEqual($c.family, "L");
}

func testParseErrorIsVcardError() {
    def caught as bool init false;
    try {
        parse(crlf("BEGIN:VCARD\nFN:A\nX\nEND:VCARD\n"));
    } catch (e) {
        $caught = true;
        def ve as VcardError init $e;
        testing.assertEqual($ve.kind, "vcard");
        testing.assertEqual($ve.line, 3);
        testing.assertTrue(strings.startsWith($ve.message, "line 3: "));
    }
    testing.assertTrue($caught);
}

# A property group prefix (`item1.EMAIL`, as Apple Contacts writes) is stripped.
func testParseStripsGroupPrefix() {
    def c as Card init parse(
        crlf("BEGIN:VCARD\nVERSION:3.0\nFN:A\nitem1.EMAIL;type=INTERNET:x@y\n" +
            "item1.X-ABLabel:home\nitem2.TEL:+1\nEND:VCARD\n"))[0];
    testing.assertEqual(len($c.emails), 1);
    testing.assertEqual($c.emails[0].value, "x@y");
    testing.assertEqual($c.emails[0].type, "INTERNET");
    testing.assertEqual($c.phones[0].value, "+1");
    testing.assertEqual(parseFault(crlf("BEGIN:VCARD\nFN:A\n.EMAIL:x\nEND:VCARD\n")), "vcard@3");
}

# vCard 2.1 quoted-printable values decode, soft line breaks included, in their
# CHARSET.
func testParseQuotedPrintable21() {
    def src as string init crlf(
        "BEGIN:VCARD\nVERSION:2.1\nFN;ENCODING=QUOTED-PRINTABLE;CHARSET=UTF-8:Ad=C3=A9\n" +
            "NOTE;QUOTED-PRINTABLE:line one=0D=0Aline =\ntwo\n" +
            "ADR;HOME;ENCODING=QUOTED-PRINTABLE;CHARSET=ISO-8859-1:;;Stra=DFe 1;K=F6ln;;;\n" +
            "TEL;WORK:+1\nEND:VCARD\n");
    def c as Card init parse($src)[0];
    testing.assertEqual($c.formattedName, "Adé");
    testing.assertEqual($c.note, "line one\r\nline two");
    testing.assertEqual($c.addresses[0].street, "Straße 1");
    testing.assertEqual($c.addresses[0].locality, "Köln");
    testing.assertEqual($c.phones[0].value, "+1");
    testing.assertEqual(
        parseFault(crlf("BEGIN:VCARD\nVERSION:2.1\nFN;QUOTED-PRINTABLE:a=ZZ\nEND:VCARD\n")),
        "vcard@3");
    testing.assertEqual(
        parseFault(crlf("BEGIN:VCARD\nVERSION:2.1\nFN;QUOTED-PRINTABLE:a=C3\nEND:VCARD\n")),
        "vcard@3");
    testing.assertEqual(
        parseFault(
            crlf("BEGIN:VCARD\nVERSION:2.1\nFN;QUOTED-PRINTABLE;CHARSET=X-NONE:a\nEND:VCARD\n")),
        "vcard@3");
}
