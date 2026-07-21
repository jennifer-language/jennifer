# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Build and parse vCard (RFC 6350, vCard 4.0): a `Card` of contact fields
 * encoded to a `VCARD` and parsed back. The contacts counterpart to `ical` -
 * it shares the same content-line codec (TEXT escaping, 75-char line folding)
 * through the included `ical_vcard_shared.j`. Pure Jennifer over `strings` / `lists`;
 * both binaries.
 *
 * A `Card` carries a formatted name (`FN`), a structured name (`N`),
 * organisation / title, any number of emails / phones / addresses, a URL, and a
 * note. `encode` writes `FN` and `VERSION:4.0` and omits empty fields; `parse`
 * reads one or many `VCARD`s (property parameters like `;TYPE=work` are
 * ignored). Text values are escaped / unescaped and long lines folded, so
 * `parse(encode(card))` round-trips.
 * @module vcard
 * @example
 * import "vcard.j" as vcard;
 * def c as vcard.Card init vcard.card("Ada Lovelace");
 * $c = vcard.withName($c, "Lovelace", "Ada");
 * $c = vcard.addEmail($c, "ada@example.com");
 * def text as string init vcard.encode($c);
 */
use strings;
use lists;
use convert;

# The vCard / iCalendar content-line codec (TEXT escaping, 75-char folding, the
# name / value split, `emit`) is shared with ical.j via this include.
include "ical_vcard_shared.j";

/**
 * A contact card.
 * @field formattedName {string} the `FN` display name (required by vCard 4.0)
 * @field family {string} the `N` family (last) name
 * @field given {string} the `N` given (first) name
 * @field organization {string} the `ORG` organisation ("" when unset)
 * @field title {string} the `TITLE` job title ("" when unset)
 * @field emails {list of string} the `EMAIL` addresses
 * @field phones {list of string} the `TEL` phone numbers
 * @field addresses {list of Address} the `ADR` postal addresses
 * @field url {string} the `URL` ("" when unset)
 * @field note {string} the `NOTE` free-text note ("" when unset)
 */
export def struct Card {
    formattedName as string,
    family as string,
    given as string,
    organization as string,
    title as string,
    emails as list of string,
    phones as list of string,
    addresses as list of Address,
    url as string,
    note as string
};

/**
 * A postal address (`ADR`). The RFC's PO-box and extended components are not
 * modelled; the five common fields are.
 * @field street as string the street address
 * @field locality {string} the city / locality
 * @field region {string} the state / province / region
 * @field postalCode {string} the postal / ZIP code
 * @field country {string} the country name
 */
export def struct Address {
    street as string,
    locality as string,
    region as string,
    postalCode as string,
    country as string
};

# --- constructors + builders (exported) -------------------------------------

/**
 * A card with just its formatted name (`FN`). Other fields are empty until set.
 * @param formattedName {string} the display name
 * @return {Card} the card
 */
export func card(formattedName as string) {
    return Card{
        formattedName: $formattedName,
        family: "", given: "",
        organization: "", title: "",
        emails: [], phones: [], addresses: [],
        url: "", note: ""
    };
}

/**
 * A copy of the card with its structured name (`N` family / given) set.
 * @param c {Card} the card
 * @param family {string} the family (last) name
 * @param given {string} the given (first) name
 * @return {Card} a fresh card with the name set
 */
export func withName(c as Card, family as string, given as string) {
    $c.family = $family;
    $c.given = $given;
    return $c;
}

/**
 * A copy of the card with its organisation and title set.
 * @param c {Card} the card
 * @param organization {string} the organisation name
 * @param title {string} the job title
 * @return {Card} a fresh card with the org / title set
 */
export func withOrg(c as Card, organization as string, title as string) {
    $c.organization = $organization;
    $c.title = $title;
    return $c;
}

/**
 * A copy of the card with an email appended.
 * @param c {Card} the card
 * @param email {string} the email address
 * @return {Card} a fresh card with the email added
 */
export func addEmail(c as Card, email as string) {
    $c.emails = lists.push($c.emails, $email);
    return $c;
}

/**
 * A copy of the card with a phone number appended.
 * @param c {Card} the card
 * @param phone {string} the phone number
 * @return {Card} a fresh card with the phone added
 */
export func addPhone(c as Card, phone as string) {
    $c.phones = lists.push($c.phones, $phone);
    return $c;
}

/**
 * A postal address.
 * @param street {string} the street address
 * @param locality {string} the city / locality
 * @param region {string} the state / province / region
 * @param postalCode {string} the postal / ZIP code
 * @param country {string} the country name
 * @return {Address} the address
 */
export func address(street as string, locality as string, region as string, postalCode as string, country as string) {
    return Address{ street: $street, locality: $locality, region: $region, postalCode: $postalCode, country: $country };
}

/**
 * A copy of the card with a postal address appended.
 * @param c {Card} the card
 * @param a {Address} the address
 * @return {Card} a fresh card with the address added
 */
export func addAddress(c as Card, a as Address) {
    $c.addresses = lists.push($c.addresses, $a);
    return $c;
}

/**
 * A copy of the card with its URL set.
 * @param c {Card} the card
 * @param url {string} the URL
 * @return {Card} a fresh card with the URL set
 */
export func withUrl(c as Card, url as string) {
    $c.url = $url;
    return $c;
}

/**
 * A copy of the card with its note set.
 * @param c {Card} the card
 * @param note {string} the note text
 * @return {Card} a fresh card with the note set
 */
export func withNote(c as Card, note as string) {
    $c.note = $note;
    return $c;
}

# --- structured values (private) --------------------------------------------

# valueColon returns the index of the value-separating colon, scanning the
# name/parameter section while tracking double-quote state so a colon inside a
# quoted parameter value (`ADR;LABEL="HQ: entrance":...`) is not mistaken for
# the separator. Returns -1 when there is no unquoted colon.
func valueColon(line as string) {
    def cs as list of string init strings.chars($line);
    def inQuote as bool init false;
    def i as int init 0;
    while ($i < len($cs)) {
        def ch as string init $cs[$i];
        if ($ch == "\"") {
            $inQuote = not $inQuote;
        } elseif ($ch == ":" and not $inQuote) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# encodeAdr renders an Address as the 7-component ADR value
# (POBox;Ext;Street;Locality;Region;Postal;Country), leaving PO box and extended
# empty and escaping each modelled component.
func encodeAdr(a as Address) {
    return ";;" + escapeText($a.street) + ";" + escapeText($a.locality) + ";" +
        escapeText($a.region) + ";" + escapeText($a.postalCode) + ";" + escapeText($a.country);
}

# splitStructured splits a structured value on unescaped `;`, keeping any `\;`
# pair intact for a later per-component unescapeText.
func splitStructured(value as string) {
    def parts as list of string init [];
    def cur as string init "";
    def chars as list of string init strings.chars($value);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        if ($c == "\\" and $i + 1 < $n) {
            $cur = $cur + $c + $chars[$i + 1];
            $i = $i + 2;
            continue;
        }
        if ($c == ";") {
            $parts[] = $cur;
            $cur = "";
            $i = $i + 1;
            continue;
        }
        $cur = $cur + $c;
        $i = $i + 1;
    }
    $parts[] = $cur;
    return $parts;
}

# component returns the unescaped structured component at index i, or "" when the
# value has too few components.
func component(parts as list of string, i as int) {
    if ($i < len($parts)) {
        return unescapeText($parts[$i]);
    }
    return "";
}

# --- encode (exported) ------------------------------------------------------

# encodeLines renders one VCARD (BEGIN..END) as a list of folded content lines.
func encodeLines(c as Card) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VCARD";
    $lines[] = "VERSION:4.0";
    $lines[] = emitLine("FN", escapeText($c.formattedName));
    if (not ($c.family == "") or not ($c.given == "")) {
        $lines[] = emitLine("N", escapeText($c.family) + ";" + escapeText($c.given) + ";;;");
    }
    if (not ($c.organization == "")) {
        $lines[] = emitLine("ORG", escapeText($c.organization));
    }
    if (not ($c.title == "")) {
        $lines[] = emitLine("TITLE", escapeText($c.title));
    }
    for (def e in $c.emails) {
        $lines[] = emitLine("EMAIL", escapeText($e));
    }
    for (def p in $c.phones) {
        $lines[] = emitLine("TEL", escapeText($p));
    }
    for (def a in $c.addresses) {
        $lines[] = emitLine("ADR", encodeAdr($a));
    }
    if (not ($c.url == "")) {
        $lines[] = emitLine("URL", escapeText($c.url));
    }
    if (not ($c.note == "")) {
        $lines[] = emitLine("NOTE", escapeText($c.note));
    }
    $lines[] = "END:VCARD";
    return $lines;
}

/**
 * Render a single card to vCard 4.0 text (a `VCARD`, CRLF-terminated). Empty
 * optional fields are omitted.
 * @param c {Card} the card to encode
 * @return {string} the vCard text
 */
export func encode(c as Card) {
    return strings.join(encodeLines($c), "\r\n") + "\r\n";
}

/**
 * Render many cards to one vCard text (concatenated `VCARD`s).
 * @param cards {list of Card} the cards to encode
 * @return {string} the vCard text
 */
export func encodeAll(cards as list of Card) {
    def lines as list of string init [];
    for (def c in $cards) {
        for (def ln in encodeLines($c)) {
            $lines[] = $ln;
        }
    }
    return strings.join($lines, "\r\n") + "\r\n";
}

# --- parse (exported) -------------------------------------------------------

/**
 * Parse vCard text into a list of `Card`s (one entry per `VCARD`). Unfolds
 * folded lines, ignores property parameters (the `;KEY=VALUE` after a name),
 * reads the structured `N` / `ADR` values, and unescapes text values.
 * @param text {string} the vCard text
 * @return {list of Card} the parsed cards (empty when the text has none)
 */
export func parse(text as string) {
    def cards as list of Card init [];
    def inCard as bool init false;
    def cur as Card init card("");
    for (def line in splitLines(unfold($text))) {
        if ($line == "") {
            continue;
        }
        def colon as int init valueColon($line);
        if ($colon < 0) {
            continue;
        }
        def name as string init propName(strings.substring($line, 0, $colon));
        def value as string init strings.substring($line, $colon + 1, len($line));
        if ($name == "BEGIN" and strings.upper($value) == "VCARD") {
            $inCard = true;
            $cur = card("");
            continue;
        }
        if ($name == "END" and strings.upper($value) == "VCARD") {
            if ($inCard) {
                $cards[] = $cur;
            }
            $inCard = false;
            continue;
        }
        if (not $inCard) {
            continue;
        }
        if ($name == "FN") {
            $cur.formattedName = unescapeText($value);
        } elseif ($name == "N") {
            def parts as list of string init splitStructured($value);
            $cur.family = component($parts, 0);
            $cur.given = component($parts, 1);
        } elseif ($name == "ORG") {
            # ORG is itself structured (org;unit;...); take the first component.
            $cur.organization = component(splitStructured($value), 0);
        } elseif ($name == "TITLE") {
            $cur.title = unescapeText($value);
        } elseif ($name == "EMAIL") {
            $cur.emails = lists.push($cur.emails, unescapeText($value));
        } elseif ($name == "TEL") {
            $cur.phones = lists.push($cur.phones, unescapeText($value));
        } elseif ($name == "ADR") {
            def parts as list of string init splitStructured($value);
            def a as Address init address(component($parts, 2), component($parts, 3),
                component($parts, 4), component($parts, 5), component($parts, 6));
            $cur.addresses = lists.push($cur.addresses, $a);
        } elseif ($name == "URL") {
            $cur.url = unescapeText($value);
        } elseif ($name == "NOTE") {
            $cur.note = unescapeText($value);
        }
    }
    return $cards;
}
