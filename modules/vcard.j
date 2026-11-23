# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.27.0

/**
 * Build and parse vCard (RFC 6350, vCard 4.0): a `Card` of contact fields
 * encoded to a `VCARD` and parsed back. The contacts counterpart to `ical` -
 * it shares the same content-line codec (TEXT escaping, 75-char line folding)
 * through the included `ical_vcard_shared.inc.j`. Pure Jennifer over `strings` / `lists`;
 * both binaries.
 *
 * A `Card` carries a formatted name (`FN`), a full structured name (`N`:
 * family / given / additional / prefixes / suffixes), a nickname,
 * organisation / title, any number of emails / phones / addresses (each with an
 * optional `TYPE` like `work` / `home`), a birthday, a photo URI, categories,
 * a URL, and a note. `encode` writes `FN` and `VERSION:4.0` and omits empty
 * fields; `parse` reads one or many vCard 2.1 / 3.0 / 4.0 `VCARD`s, including
 * the `TYPE` parameter on emails / phones / addresses, and throws a
 * `VcardError` for malformed input. Text values are escaped / unescaped and
 * long lines folded, so `parse(encode(card))` round-trips.
 * @module vcard
 * @example
 * import "vcard.j" as vcard;
 * def c as vcard.Card init vcard.card("Ada Lovelace");
 * $c = vcard.withName($c, "Lovelace", "Ada");
 * $c = vcard.addEmail($c, "ada@example.com");
 * def text as string init vcard.encode($c);
 */
use convert;
use strings;
use encoding;
use lists;

# The vCard / iCalendar content-line codec (TEXT escaping, 75-char folding, the
# name / value split, `emit`) is shared with ical.j via this include.
include "ical_vcard_shared.inc.j";

/**
 * Malformed vCard text. Every error of kind `"vcard"` is one, so its fields are
 * safe to read once the kind matches.
 * @field kind {string} always `"vcard"`
 * @field message {string} what is wrong, prefixed `line N: `
 * @field line {int} the 1-based physical line of `parse`'s input the fault is on
 *     (a folded line counts from its first physical line)
 */
export def struct VcardError {
    kind as string,
    message as string,
    line as int
};

/**
 * A value with an optional `TYPE` parameter (e.g. an email or phone marked
 * `work` / `home`). `type` is "" when unspecified.
 * @field value {string} the property value (the email / phone number)
 * @field type {string} the `TYPE` parameter ("work" / "home" / "cell" / ...; "" if none)
 */
export def struct Typed {
    value as string,
    type as string
};

/**
 * A contact card.
 * @field formattedName {string} the `FN` display name (required by vCard 4.0)
 * @field family {string} the `N` family (last) name
 * @field given {string} the `N` given (first) name
 * @field additional {string} the `N` additional (middle) name(s)
 * @field prefixes {string} the `N` honorific prefixes (e.g. "Dr.")
 * @field suffixes {string} the `N` honorific suffixes (e.g. "Jr.", "PhD")
 * @field nickname {string} the `NICKNAME` ("" when unset)
 * @field organization {string} the `ORG` organisation ("" when unset)
 * @field title {string} the `TITLE` job title ("" when unset)
 * @field emails {list of Typed} the `EMAIL` addresses (each with an optional TYPE)
 * @field phones {list of Typed} the `TEL` phone numbers (each with an optional TYPE)
 * @field addresses {list of Address} the `ADR` postal addresses
 * @field url {string} the `URL` ("" when unset)
 * @field bday {string} the `BDAY` birthday ("" when unset; a `YYYYMMDD` or partial date)
 * @field photo {string} the `PHOTO` URI ("" when unset)
 * @field categories {list of string} the `CATEGORIES` tags
 * @field note {string} the `NOTE` free-text note ("" when unset)
 */
export def struct Card {
    formattedName as string,
    family as string,
    given as string,
    additional as string,
    prefixes as string,
    suffixes as string,
    nickname as string,
    organization as string,
    title as string,
    emails as list of Typed,
    phones as list of Typed,
    addresses as list of Address,
    url as string,
    bday as string,
    photo as string,
    categories as list of string,
    note as string
};

/**
 * A postal address (`ADR`). The RFC's PO-box and extended components are not
 * modelled; the five common fields are, plus an optional `TYPE`.
 * @field street {string} the street address
 * @field locality {string} the city / locality
 * @field region {string} the state / province / region
 * @field postalCode {string} the postal / ZIP code
 * @field country {string} the country name
 * @field type {string} the `TYPE` parameter ("work" / "home" / ...; "" if none)
 */
export def struct Address {
    street as string,
    locality as string,
    region as string,
    postalCode as string,
    country as string,
    type as string
};

# --- constructors + builders (exported) -------------------------------------

/**
 * A card with just its formatted name (`FN`). Other fields are empty until set.
 * @param formattedName {string} the display name
 * @return {Card} the card
 */
export func card(formattedName as string) {
    def emails as list of Typed init [];
    def phones as list of Typed init [];
    def addrs as list of Address init [];
    def cats as list of string init [];
    return Card{
        formattedName: $formattedName,
        family: "",
        given: "",
        additional: "",
        prefixes: "",
        suffixes: "",
        nickname: "",
        organization: "",
        title: "",
        emails: $emails,
        phones: $phones,
        addresses: $addrs,
        url: "",
        bday: "",
        photo: "",
        categories: $cats,
        note: ""
    };
}

/**
 * A copy of the card with its structured name (`N` family / given) set. The
 * additional (middle) name and honorific prefixes / suffixes are left empty; use
 * `withFullName` to set all five components.
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
 * A copy of the card with its full structured name (`N`) set: family, given,
 * additional (middle) name(s), and honorific prefixes / suffixes.
 * @param c {Card} the card
 * @param family {string} the family (last) name
 * @param given {string} the given (first) name
 * @param additional {string} the additional (middle) name(s)
 * @param prefixes {string} the honorific prefixes (e.g. "Dr.")
 * @param suffixes {string} the honorific suffixes (e.g. "Jr.", "PhD")
 * @return {Card} a fresh card with the full name set
 */
export func withFullName(
    c as Card,
    family as string,
    given as string,
    additional as string,
    prefixes as string,
    suffixes as string) {
    $c.family = $family;
    $c.given = $given;
    $c.additional = $additional;
    $c.prefixes = $prefixes;
    $c.suffixes = $suffixes;
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
    return addEmailTyped($c, $email, "");
}

/**
 * A copy of the card with a `TYPE`d email appended (e.g. `type` "work" / "home").
 * @param c {Card} the card
 * @param email {string} the email address
 * @param type {string} the `TYPE` (e.g. "work" / "home"; "" for none)
 * @return {Card} a fresh card with the email added
 */
export func addEmailTyped(c as Card, email as string, type as string) {
    $c.emails[] = Typed{value: $email, type: $type};
    return $c;
}

/**
 * A copy of the card with a phone number appended.
 * @param c {Card} the card
 * @param phone {string} the phone number
 * @return {Card} a fresh card with the phone added
 */
export func addPhone(c as Card, phone as string) {
    return addPhoneTyped($c, $phone, "");
}

/**
 * A copy of the card with a `TYPE`d phone number appended (e.g. `type` "cell").
 * @param c {Card} the card
 * @param phone {string} the phone number
 * @param type {string} the `TYPE` (e.g. "work" / "home" / "cell"; "" for none)
 * @return {Card} a fresh card with the phone added
 */
export func addPhoneTyped(c as Card, phone as string, type as string) {
    $c.phones[] = Typed{value: $phone, type: $type};
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
export func address(
    street as string,
    locality as string,
    region as string,
    postalCode as string,
    country as string) {
    return addressTyped($street, $locality, $region, $postalCode, $country, "");
}

/**
 * A postal address with a `TYPE` (e.g. "work" / "home").
 * @param street {string} the street address
 * @param locality {string} the city / locality
 * @param region {string} the state / province / region
 * @param postalCode {string} the postal / ZIP code
 * @param country {string} the country name
 * @param type {string} the `TYPE` ("work" / "home"; "" for none)
 * @return {Address} the address
 */
export func addressTyped(
    street as string,
    locality as string,
    region as string,
    postalCode as string,
    country as string,
    type as string) {
    return Address{
        street: $street,
        locality: $locality,
        region: $region,
        postalCode: $postalCode,
        country: $country,
        type: $type
    };
}

/**
 * A copy of the card with a postal address appended.
 * @param c {Card} the card
 * @param a {Address} the address
 * @return {Card} a fresh card with the address added
 */
export func addAddress(c as Card, a as Address) {
    $c.addresses[] = $a;
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

/**
 * A copy of the card with its nickname (`NICKNAME`) set.
 * @param c {Card} the card
 * @param nickname {string} the nickname
 * @return {Card} a fresh card with the nickname set
 */
export func withNickname(c as Card, nickname as string) {
    $c.nickname = $nickname;
    return $c;
}

/**
 * A copy of the card with its birthday (`BDAY`) set. The value is stored
 * verbatim (a `YYYYMMDD` date, or a partial date like `--0315`).
 * @param c {Card} the card
 * @param bday {string} the birthday value
 * @return {Card} a fresh card with the birthday set
 */
export func withBday(c as Card, bday as string) {
    $c.bday = $bday;
    return $c;
}

/**
 * A copy of the card with its photo (`PHOTO`) URI set.
 * @param c {Card} the card
 * @param photo {string} the photo URI
 * @return {Card} a fresh card with the photo set
 */
export func withPhoto(c as Card, photo as string) {
    $c.photo = $photo;
    return $c;
}

/**
 * A copy of the card with a category (`CATEGORIES` tag) appended.
 * @param c {Card} the card
 * @param category {string} the category tag
 * @return {Card} a fresh card with the category added
 */
export func addCategory(c as Card, category as string) {
    $c.categories[] = $category;
    return $c;
}

# --- structured values (private) --------------------------------------------

# valueColon returns the index of the value-separating colon, scanning the
# name/parameter section while tracking double-quote state so a colon inside a
# quoted parameter value (`ADR;LABEL="HQ: entrance":...`) is not mistaken for
# the separator. Returns -1 when there is no unquoted colon.
func valueColon(line as string) {
    # Fast path: no quote before the first colon (the usual line).
    def first as int init strings.indexOf($line, ":");
    if ($first < 0) {
        return -1;
    }
    if (not strings.contains($line[..$first], "\"")) {
        return $first;
    }
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
    def cur as list of string init [];
    def chars as list of string init strings.chars($value);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        if ($c == "\\" and $i + 1 < $n) {
            $cur[] = $c;
            $cur[] = $chars[$i + 1];
            $i = $i + 2;
            continue;
        }
        if ($c == ";") {
            $parts[] = strings.join($cur, "");
            $cur = [];
            $i = $i + 1;
            continue;
        }
        $cur[] = $c;
        $i = $i + 1;
    }
    $parts[] = strings.join($cur, "");
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

# splitCategories splits a CATEGORIES value on unescaped `,` and unescapes each
# tag (the comma-list counterpart to splitStructured's `;`).
func splitCategories(value as string) {
    def out as list of string init [];
    def cur as list of string init [];
    def chars as list of string init strings.chars($value);
    def n as int init len($chars);
    def i as int init 0;
    while ($i < $n) {
        def c as string init $chars[$i];
        if ($c == "\\" and $i + 1 < $n) {
            $cur[] = $c;
            $cur[] = $chars[$i + 1];
            $i = $i + 2;
            continue;
        }
        if ($c == ",") {
            $out[] = unescapeText(strings.join($cur, ""));
            $cur = [];
            $i = $i + 1;
            continue;
        }
        $cur[] = $c;
        $i = $i + 1;
    }
    $out[] = unescapeText(strings.join($cur, ""));
    return $out;
}

# --- encode (exported) ------------------------------------------------------

# typedProp appends a `;TYPE=<type>` parameter to a property name when the type
# is set, leaving the bare name otherwise.
func typedProp(name as string, type as string) {
    if ($type == "") {
        return $name;
    }
    return $name + ";TYPE=" + quoteParam($type);
}

# hasName reports whether any of the five structured-name components is set.
func hasName(c as Card) {
    return not ($c.family == "") or not ($c.given == "") or not ($c.additional == "") or
        not ($c.prefixes == "") or not ($c.suffixes == "");
}

# encodeCategories joins the escaped category tags with the (unescaped) `,`
# CATEGORIES separator.
func encodeCategories(cats as list of string) {
    def parts as list of string init [];
    for (def cat in $cats) {
        $parts[] = escapeText($cat);
    }
    return strings.join($parts, ",");
}

# encodeLines renders one VCARD (BEGIN..END) as a list of folded content lines.
func encodeLines(c as Card) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VCARD";
    $lines[] = "VERSION:4.0";
    $lines[] = emitLine("FN", escapeText($c.formattedName));
    if (hasName($c)) {
        $lines[] = emitLine(
            "N",
            escapeText($c.family) + ";" + escapeText($c.given) + ";" +
                escapeText($c.additional) + ";" + escapeText($c.prefixes) + ";" +
                escapeText($c.suffixes));
    }
    if (not ($c.nickname == "")) {
        $lines[] = emitLine("NICKNAME", escapeText($c.nickname));
    }
    if (not ($c.organization == "")) {
        $lines[] = emitLine("ORG", escapeText($c.organization));
    }
    if (not ($c.title == "")) {
        $lines[] = emitLine("TITLE", escapeText($c.title));
    }
    for (def e in $c.emails) {
        $lines[] = emitLine(typedProp("EMAIL", $e.type), escapeText($e.value));
    }
    for (def p in $c.phones) {
        $lines[] = emitLine(typedProp("TEL", $p.type), escapeText($p.value));
    }
    for (def a in $c.addresses) {
        $lines[] = emitLine(typedProp("ADR", $a.type), encodeAdr($a));
    }
    if (not ($c.bday == "")) {
        $lines[] = emitLine("BDAY", $c.bday);
    }
    if (not ($c.url == "")) {
        $lines[] = emitLine("URL", escapeText($c.url));
    }
    if (not ($c.photo == "")) {
        $lines[] = emitLine("PHOTO", $c.photo);
    }
    if (len($c.categories) > 0) {
        $lines[] = emitLine("CATEGORIES", encodeCategories($c.categories));
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

# vcardError builds the VcardError for a fault at a physical line, prefixing
# the message with the line number.
func vcardError(line as int, message as string) {
    return VcardError{kind: "vcard", message: "line {$line}: {$message}", line: $line};
}

# stripGroup drops a property group prefix (`item1.EMAIL` -> `EMAIL`, RFC 6350
# 3.3); the group must be a non-empty run of letters, digits and `-`.
func stripGroup(name as string, line as int) {
    def dot as int init strings.indexOf($name, ".");
    if ($dot < 0) {
        return $name;
    }
    def group as string init $name[..$dot];
    def rest as string init $name[$dot + 1..];
    if ($group == "" or $rest == "" or strings.contains($rest, ".")) {
        throw vcardError($line, "\"{$name}\" is not a property name");
    }
    for (def c in strings.chars($group)) {
        if (not (($c >= "A" and $c <= "Z") or ($c >= "0" and $c <= "9") or $c == "-")) {
            throw vcardError($line, "\"{$group}\" is not a property group");
        }
    }
    return $rest;
}

# isQuotedPrintable reports whether a property is quoted-printable encoded: the
# `ENCODING=QUOTED-PRINTABLE` parameter, or vCard 2.1's bare `QUOTED-PRINTABLE`.
func isQuotedPrintable(nameSection as string) {
    if (not strings.contains(strings.upper($nameSection), "QUOTED-PRINTABLE")) {
        return false;
    }
    if (strings.upper(paramValue($nameSection, "ENCODING")) == "QUOTED-PRINTABLE") {
        return true;
    }
    def parts as list of string init strings.split($nameSection, ";");
    for (def i in 1..len($parts)) {
        if (strings.upper($parts[$i]) == "QUOTED-PRINTABLE") {
            return true;
        }
    }
    return false;
}

# hexDigit reports whether a one-character string is a hexadecimal digit.
func hexDigit(c as string) {
    return ($c >= "0" and $c <= "9") or ($c >= "A" and $c <= "F") or ($c >= "a" and $c <= "f");
}

# qpEscEnd is where a quoted-printable escape starting at `i` ends: three
# characters on, or the value's end.
func qpEscEnd(i as int, n as int) {
    if ($i + 3 < $n) {
        return $i + 3;
    }
    return $n;
}

# decodeQuotedPrintable decodes a vCard 2.1 quoted-printable value (soft line
# breaks already joined) to bytes, then to text in its CHARSET (UTF-8 when
# unset). A `=` not followed by two hex digits, an unknown charset, or bytes
# invalid in it are a VcardError.
func decodeQuotedPrintable(nameSection as string, value as string, line as int) {
    def hex as list of string init [];
    def cs as list of string init strings.chars($value);
    def n as int init len($cs);
    def i as int init 0;
    while ($i < $n) {
        if ($cs[$i] == "=") {
            if ($i + 2 >= $n or not hexDigit($cs[$i + 1]) or not hexDigit($cs[$i + 2])) {
                def esc as string init strings.join($cs[$i..qpEscEnd($i, $n)], "");
                throw vcardError($line, "quoted-printable: \"{$esc}\" is not an =XX escape");
            }
            $hex[] = $cs[$i + 1] + $cs[$i + 2];
            $i = $i + 3;
            continue;
        }
        $hex[] = encoding.toText("hex", encoding.encode("utf-8", $cs[$i]));
        $i = $i + 1;
    }
    def raw as bytes init encoding.fromText("hex", strings.join($hex, ""));
    def charset as string init strings.lower(paramValue($nameSection, "CHARSET"));
    if ($charset == "us-ascii") {
        $charset = "ascii";
    }
    try {
        if ($charset == "" or $charset == "utf-8") {
            return encoding.decode("utf-8", $raw);
        }
        return encoding.decode($charset, $raw);
    } catch (e) {
        # "encoding": bytes the charset does not decode; "runtime": a CHARSET
        # name encoding does not know. Both are a fault of the card.
        if (not ($e.kind == "encoding" or $e.kind == "runtime")) {
            throw $e;
        }
        def shown as string init $charset;
        if ($shown == "") {
            $shown = "utf-8";
        }
        throw vcardError($line, "quoted-printable value is not valid {$shown}");
    }
}

# joinSoftBreaks joins a quoted-printable line ending in a `=` soft line break
# with the physical lines that continue it (vCard 2.1 writes those unindented).
func joinSoftBreaks(lines as list of ContentLine) {
    def out as list of ContentLine init [];
    def i as int init 0;
    while ($i < len($lines)) {
        def cl as ContentLine init $lines[$i];
        $i = $i + 1;
        if (not strings.contains(strings.upper($cl.text), "QUOTED-PRINTABLE")) {
            $out[] = $cl;
            continue;
        }
        def colon as int init valueColon($cl.text);
        if ($colon < 0 or not isQuotedPrintable($cl.text[..$colon])) {
            $out[] = $cl;
            continue;
        }
        def parts as list of string init [];
        def cur as string init $cl.text;
        while (strings.endsWith($cur, "=") and $i < len($lines)) {
            $parts[] = $cur[..len($cur) - 1];
            $cur = $lines[$i].text;
            $i = $i + 1;
        }
        $parts[] = $cur;
        $out[] = ContentLine{text: strings.join($parts, ""), line: $cl.line};
    }
    return $out;
}

# applyProperty stores one property of a card being parsed into it; a
# property the Card does not model is ignored.
func applyProperty(c as Card, name as string, nameSection as string, value as string) {
    match ($name) {
        when "FN" { $c.formattedName = unescapeText($value); }
        when "N" {
            def parts as list of string init splitStructured($value);
            $c.family = component($parts, 0);
            $c.given = component($parts, 1);
            $c.additional = component($parts, 2);
            $c.prefixes = component($parts, 3);
            $c.suffixes = component($parts, 4);
        }
        when "NICKNAME" { $c.nickname = unescapeText($value); }
        when "ORG" {
            # ORG is itself structured (org;unit;...); take the first component.
            $c.organization = component(splitStructured($value), 0);
        }
        when "TITLE" { $c.title = unescapeText($value); }
        when "EMAIL" {
            $c.emails[] = Typed{
                value: unescapeText($value),
                type: paramValue($nameSection, "TYPE")
            };
        }
        when "TEL" {
            $c.phones[] = Typed{
                value: unescapeText($value),
                type: paramValue($nameSection, "TYPE")
            };
        }
        when "ADR" {
            def parts as list of string init splitStructured($value);
            def a as Address init addressTyped(
                component($parts, 2),
                component($parts, 3),
                component($parts, 4),
                component($parts, 5),
                component($parts, 6),
                paramValue($nameSection, "TYPE"));
            $c.addresses[] = $a;
        }
        when "BDAY" { $c.bday = $value; }
        when "URL" { $c.url = unescapeText($value); }
        when "PHOTO" { $c.photo = $value; }
        when "CATEGORIES" { $c.categories = splitCategories($value); }
        when "NOTE" { $c.note = unescapeText($value); }
    }
    return $c;
}

/**
 * Parse vCard text into a list of `Card`s (one entry per `VCARD`). Accepts
 * vCard 2.1, 3.0 and 4.0: unfolds folded lines, strips property group prefixes
 * (`item1.EMAIL` reads as `EMAIL`), decodes 2.1 quoted-printable values (with
 * their `CHARSET`), reads the `TYPE` parameter on `EMAIL` / `TEL` / `ADR` into
 * `Typed.type` (other `;KEY=VALUE` parameters are ignored), reads the
 * structured `N` / `ADR` values, and unescapes text values.
 * @param text {string} the vCard text
 * @return {list of Card} the parsed cards (empty when the text is blank)
 * @throws {VcardError} kind "vcard" for malformed input: text outside a
 *     `BEGIN:VCARD` ... `END:VCARD`, a card without `END:VCARD`, a nested
 *     `BEGIN`, a line without a `:`, an unknown `VERSION`, a 3.0 / 4.0 card
 *     without `FN`, or a bad quoted-printable value
 */
export func parse(text as string) {
    def cards as list of Card init [];
    def inCard as bool init false;
    def beginLine as int init 0;
    def hasFn as bool init false;
    def version as string init "";
    def cur as Card init card("");
    for (def cl in joinSoftBreaks(contentLines($text))) {
        def line as string init $cl.text;
        def ln as int init $cl.line;
        if ($line == "") {
            continue;
        }
        def colon as int init valueColon($line);
        if ($colon <= 0) {
            throw vcardError($ln, "content line has no property name and ':' separator");
        }
        def nameSection as string init $line[..$colon];
        def name as string init stripGroup(propName($nameSection), $ln);
        def value as string init $line[$colon + 1..];
        if (isQuotedPrintable($nameSection)) {
            $value = decodeQuotedPrintable($nameSection, $value, $ln);
        }
        if (not $inCard) {
            if (not ($name == "BEGIN" and strings.upper($value) == "VCARD")) {
                throw vcardError($ln, "expected BEGIN:VCARD");
            }
            $inCard = true;
            $beginLine = $ln;
            $hasFn = false;
            $version = "";
            $cur = card("");
            continue;
        }
        if ($name == "BEGIN") {
            throw vcardError($ln, "BEGIN:{$value} inside the card begun at line {$beginLine}");
        }
        if ($name == "END") {
            if (not (strings.upper($value) == "VCARD")) {
                throw vcardError($ln, "END:{$value} does not close BEGIN:VCARD");
            }
            if (not $hasFn and not ($version == "2.1")) {
                throw vcardError($beginLine, "card has no FN (required in vCard 3.0 and 4.0)");
            }
            $cards[] = $cur;
            $inCard = false;
            continue;
        }
        if ($name == "VERSION") {
            if (not lists.contains(["2.1", "3.0", "4.0"], $value)) {
                throw vcardError($ln, "VERSION {$value} is not 2.1, 3.0 or 4.0");
            }
            $version = $value;
        }
        if ($name == "FN") {
            $hasFn = true;
        }
        $cur = applyProperty($cur, $name, $nameSection, $value);
    }
    if ($inCard) {
        throw vcardError($beginLine, "BEGIN:VCARD has no matching END:VCARD");
    }
    return $cards;
}
