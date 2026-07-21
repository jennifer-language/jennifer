# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * IP addresses and CIDR networks, IPv4 and IPv6. Parse an address or a
 * `address/prefix` block, test membership, and compute netmask / broadcast -
 * for allow-lists and subnet math. An `Address` holds its raw bytes (4 for IPv4,
 * 16 for IPv6, network byte order); a `Network` pairs a base address with a
 * prefix length. Pure Jennifer over `strings` + `convert` and the bitwise
 * operators; both binaries.
 * @module ipnet
 * @example
 * import "ipnet.j" as ipnet;
 * def net as ipnet.Network init ipnet.parse("192.168.1.0/24");
 * def ip as ipnet.Address init ipnet.parseAddress("192.168.1.42");
 * def inside as bool init ipnet.contains($net, $ip);   # true
 */
use strings;
use convert;

/**
 * An IP address as raw bytes.
 * @field version {int} the IP version: 4 or 6
 * @field octets {bytes} the address bytes (4 for IPv4, 16 for IPv6), network byte order
 */
export def struct Address {
    version as int,
    octets as bytes
};

/**
 * A CIDR network: a base address (host bits zeroed) plus a prefix length.
 * @field addr {Address} the network base address
 * @field prefix {int} the prefix length (0..32 for IPv4, 0..128 for IPv6)
 */
export def struct Network {
    addr as Address,
    prefix as int
};

# --- errors (private) -------------------------------------------------------

func fail(msg as string) {
    throw Error{ kind: "ipnet", message: $msg, file: "", line: 0, col: 0 };
}

# --- small parsers (private) ------------------------------------------------

# parseDecimal reads a non-negative decimal integer, rejecting empty / non-digit
# input with a clean ipnet error (avoids convert's generic message).
func parseDecimal(s as string) {
    if (len($s) == 0) {
        fail("empty number");
    }
    # Reject leading zeros: many host stacks read a leading-zero octet as octal,
    # so accepting "010" as decimal 10 lets an allow-list disagree with the
    # kernel (an SSRF / allow-list-bypass vector). Cap the length too.
    if (len($s) > 1 and strings.startsWith($s, "0")) {
        fail("leading zeros are not allowed: '" + $s + "'");
    }
    if (len($s) > 10) {
        fail("number too long: '" + $s + "'");
    }
    def v as int init 0;
    for (def ch in strings.chars($s)) {
        def d as int init strings.indexOf("0123456789", $ch);
        if ($d < 0) {
            fail("invalid decimal: '" + $s + "'");
        }
        $v = $v * 10 + $d;
    }
    return $v;
}

# parseFour parses dotted-quad IPv4 into 4 bytes.
func parseFour(s as string) {
    def parts as list of string init strings.split($s, ".");
    if (not (len($parts) == 4)) {
        fail("IPv4 address needs 4 octets: " + $s);
    }
    def out as bytes;
    for (def p in $parts) {
        def v as int init parseDecimal($p);
        if ($v < 0 or $v > 255) {
            fail("IPv4 octet out of range 0..255: " + $s);
        }
        $out[] = $v;
    }
    return $out;
}

# parseGroup parses one 1-4 digit IPv6 hex group into a 16-bit int.
func parseGroup(g as string) {
    if (len($g) == 0 or len($g) > 4) {
        fail("invalid IPv6 group: '" + $g + "'");
    }
    def v as int init 0;
    for (def ch in strings.chars($g)) {
        def d as int init strings.indexOf("0123456789abcdef", strings.lower($ch));
        if ($d < 0) {
            fail("invalid hex digit in IPv6 group: '" + $g + "'");
        }
        $v = ($v << 4) + $d;
    }
    return $v;
}

# tokensToGroups turns colon-separated IPv6 tokens into 16-bit groups, expanding
# a final embedded IPv4 (a token containing ".") into its two groups.
func tokensToGroups(tokens as list of string) {
    def groups as list of int init [];
    def n as int init len($tokens);
    def i as int init 0;
    while ($i < $n) {
        def t as string init $tokens[$i];
        if (strings.contains($t, ".")) {
            if (not ($i == $n - 1)) {
                fail("embedded IPv4 must be the last component");
            }
            def quad as bytes init parseFour($t);
            $groups[] = $quad[0] * 256 + $quad[1];
            $groups[] = $quad[2] * 256 + $quad[3];
        } else {
            $groups[] = parseGroup($t);
        }
        $i = $i + 1;
    }
    return $groups;
}

# parseSix parses an IPv6 address (with optional `::` compression and embedded
# IPv4) into 16 bytes.
func parseSix(s as string) {
    def groups as list of int init [];
    def dbl as int init strings.indexOf($s, "::");
    if ($dbl >= 0) {
        def leftS as string init strings.substring($s, 0, $dbl);
        def rightS as string init strings.substring($s, $dbl + 2, len($s));
        if (strings.contains($rightS, "::")) {
            fail("multiple '::' in IPv6 address: " + $s);
        }
        # An embedded IPv4 is only the final 32 bits (RFC 4291): it may appear in
        # the tail, never in the head before '::'.
        if (strings.contains($leftS, ".")) {
            fail("embedded IPv4 must be the last component: " + $s);
        }
        def headG as list of int init [];
        if (not ($leftS == "")) {
            $headG = tokensToGroups(strings.split($leftS, ":"));
        }
        def tailG as list of int init [];
        if (not ($rightS == "")) {
            $tailG = tokensToGroups(strings.split($rightS, ":"));
        }
        def explicit as int init len($headG) + len($tailG);
        if ($explicit > 7) {
            fail("too many groups around '::' in IPv6 address: " + $s);
        }
        for (def g in $headG) {
            $groups[] = $g;
        }
        def fill as int init 8 - $explicit;
        def z as int init 0;
        while ($z < $fill) {
            $groups[] = 0;
            $z = $z + 1;
        }
        for (def g in $tailG) {
            $groups[] = $g;
        }
    } else {
        $groups = tokensToGroups(strings.split($s, ":"));
        if (not (len($groups) == 8)) {
            fail("IPv6 address needs 8 groups: " + $s);
        }
    }
    def out as bytes;
    for (def g in $groups) {
        $out[] = ($g >> 8) & 0xff;
        $out[] = $g & 0xff;
    }
    return $out;
}

# --- address parse / format (exported) --------------------------------------

/**
 * Parse a bare IP address (IPv4 dotted-quad or IPv6, with `::` compression and
 * embedded IPv4 supported).
 * @param s {string} the address text
 * @return {Address} the parsed address
 * @throws {Error} kind "ipnet" on malformed input
 */
export func parseAddress(s as string) {
    if (strings.contains($s, ":")) {
        return Address{ version: 6, octets: parseSix($s) };
    }
    if (strings.contains($s, ".")) {
        return Address{ version: 4, octets: parseFour($s) };
    }
    fail("not an IP address: " + $s);
}

# formatFour renders 4 bytes as dotted-quad.
func formatFour(octets as bytes) {
    return convert.toString($octets[0]) + "." + convert.toString($octets[1]) + "." +
        convert.toString($octets[2]) + "." + convert.toString($octets[3]);
}

# hexGroup renders a 16-bit group as lowercase hex with no leading zeros.
func hexGroup(g as int) {
    if ($g == 0) {
        return "0";
    }
    def digits as string init "0123456789abcdef";
    def out as string init "";
    def v as int init $g;
    while ($v > 0) {
        def d as int init $v & 0xf;
        $out = strings.substring($digits, $d, $d + 1) + $out;
        $v = $v >> 4;
    }
    return $out;
}

# formatSix renders 16 bytes as canonical IPv6 (RFC 5952): lowercase, no leading
# zeros, and the longest run of >= 2 zero groups compressed to `::` (leftmost on
# a tie).
func formatSix(octets as bytes) {
    def groups as list of int init [];
    def i as int init 0;
    while ($i < 16) {
        $groups[] = $octets[$i] * 256 + $octets[$i + 1];
        $i = $i + 2;
    }
    # longest zero run
    def bestStart as int init -1;
    def bestLen as int init 0;
    def curStart as int init -1;
    def curLen as int init 0;
    def j as int init 0;
    while ($j < 8) {
        if ($groups[$j] == 0) {
            if ($curStart < 0) {
                $curStart = $j;
                $curLen = 0;
            }
            $curLen = $curLen + 1;
            if ($curLen > $bestLen) {
                $bestLen = $curLen;
                $bestStart = $curStart;
            }
        } else {
            $curStart = -1;
            $curLen = 0;
        }
        $j = $j + 1;
    }
    if ($bestLen < 2) {
        # no compression: join all 8 groups
        def all as list of string init [];
        def k as int init 0;
        while ($k < 8) {
            $all[] = hexGroup($groups[$k]);
            $k = $k + 1;
        }
        return strings.join($all, ":");
    }
    def leftHex as list of string init [];
    def m as int init 0;
    while ($m < $bestStart) {
        $leftHex[] = hexGroup($groups[$m]);
        $m = $m + 1;
    }
    def rightHex as list of string init [];
    $m = $bestStart + $bestLen;
    while ($m < 8) {
        $rightHex[] = hexGroup($groups[$m]);
        $m = $m + 1;
    }
    return strings.join($leftHex, ":") + "::" + strings.join($rightHex, ":");
}

/**
 * Render an address to its canonical string (IPv4 dotted-quad, or RFC 5952
 * canonical IPv6).
 * @param addr {Address} the address
 * @return {string} the canonical text
 */
export func toString(addr as Address) {
    if ($addr.version == 4) {
        return formatFour($addr.octets);
    }
    return formatSix($addr.octets);
}

# --- CIDR (exported) --------------------------------------------------------

# applyMask zeros the host bits of octets beyond the given prefix.
func applyMask(octets as bytes, prefix as int) {
    def out as bytes;
    def n as int init len($octets);
    def i as int init 0;
    while ($i < $n) {
        def bitsLeft as int init $prefix - $i * 8;
        def b as int init $octets[$i];
        if ($bitsLeft <= 0) {
            $b = 0;
        } elseif ($bitsLeft < 8) {
            def mask as int init (0xff << (8 - $bitsLeft)) & 0xff;
            $b = $b & $mask;
        }
        $out[] = $b;
        $i = $i + 1;
    }
    return $out;
}

/**
 * Parse a CIDR block `address/prefix` into a `Network` with host bits zeroed.
 * @param cidr {string} the CIDR text (e.g. "10.0.0.0/8" or "2001:db8::/32")
 * @return {Network} the network
 * @throws {Error} kind "ipnet" on malformed input or an out-of-range prefix
 */
export func parse(cidr as string) {
    def slash as int init strings.indexOf($cidr, "/");
    if ($slash < 0) {
        fail("CIDR needs a '/prefix': " + $cidr);
    }
    def addr as Address init parseAddress(strings.substring($cidr, 0, $slash));
    def prefix as int init parseDecimal(strings.substring($cidr, $slash + 1, len($cidr)));
    def maxp as int init 32;
    if ($addr.version == 6) {
        $maxp = 128;
    }
    if ($prefix > $maxp) {
        fail("prefix out of range 0.." + convert.toString($maxp) + ": " + $cidr);
    }
    return Network{ addr: Address{ version: $addr.version, octets: applyMask($addr.octets, $prefix) }, prefix: $prefix };
}

/**
 * Render a network as `address/prefix`.
 * @param net {Network} the network
 * @return {string} the CIDR text
 */
export func networkString(net as Network) {
    return toString($net.addr) + "/" + convert.toString($net.prefix);
}

# bytesEqual compares two byte slices.
func bytesEqual(a as bytes, b as bytes) {
    if (not (len($a) == len($b))) {
        return false;
    }
    def i as int init 0;
    while ($i < len($a)) {
        if (not ($a[$i] == $b[$i])) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

/**
 * Whether an address falls within a network (same version and matching prefix
 * bits). A version mismatch is simply false.
 * @param net {Network} the network
 * @param addr {Address} the address to test
 * @return {bool} true if the address is in the network
 */
export func contains(net as Network, addr as Address) {
    if (not ($addr.version == $net.addr.version)) {
        return false;
    }
    return bytesEqual(applyMask($addr.octets, $net.prefix), $net.addr.octets);
}

/**
 * Whether two addresses are equal (same version and bytes).
 * @param a {Address} the first address
 * @param b {Address} the second address
 * @return {bool} true if equal
 */
export func equal(a as Address, b as Address) {
    if (not ($a.version == $b.version)) {
        return false;
    }
    return bytesEqual($a.octets, $b.octets);
}

/**
 * The IP version of an address (4 or 6).
 * @param addr {Address} the address
 * @return {int} 4 or 6
 */
export func version(addr as Address) {
    return $addr.version;
}

/**
 * The netmask of a network as an address (e.g. 255.255.255.0 for a /24).
 * @param net {Network} the network
 * @return {Address} the netmask address
 */
export func netmask(net as Network) {
    def out as bytes;
    def n as int init len($net.addr.octets);
    def i as int init 0;
    while ($i < $n) {
        def bitsLeft as int init $net.prefix - $i * 8;
        def b as int init 0;
        if ($bitsLeft >= 8) {
            $b = 0xff;
        } elseif ($bitsLeft > 0) {
            $b = (0xff << (8 - $bitsLeft)) & 0xff;
        }
        $out[] = $b;
        $i = $i + 1;
    }
    return Address{ version: $net.addr.version, octets: $out };
}

/**
 * The broadcast (last) address of a network - every host bit set. For IPv4 this
 * is the broadcast address; for IPv6 it is the last address in the block.
 * @param net {Network} the network
 * @return {Address} the last address in the network
 */
export func broadcast(net as Network) {
    def out as bytes;
    def n as int init len($net.addr.octets);
    def i as int init 0;
    while ($i < $n) {
        def bitsLeft as int init $net.prefix - $i * 8;
        def b as int init $net.addr.octets[$i];
        if ($bitsLeft <= 0) {
            $b = 0xff;
        } elseif ($bitsLeft < 8) {
            def hostmask as int init (0xff >> $bitsLeft) & 0xff;
            $b = $b | $hostmask;
        }
        $out[] = $b;
        $i = $i + 1;
    }
    return Address{ version: $net.addr.version, octets: $out };
}
