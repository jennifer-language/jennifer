# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * IP addresses and CIDR networks, IPv4 and IPv6. Parse an address or a
 * `address/prefix` block, test membership, and compute netmask / broadcast -
 * for allow-lists and subnet math. An `Address` holds its raw bytes (4 for IPv4,
 * 16 for IPv6, network byte order); a `Network` pairs a base address with a
 * prefix length. Pure Jennifer over `strings` + `convert` and the bitwise
 * operators; both binaries. `parseAddress` folds an IPv4-mapped IPv6 literal
 * (`::ffff:a.b.c.d`) down to a v4 `Address` so it can't slip past a v4
 * allow-list (see `unmap`).
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
    throw Error{kind: "ipnet", message: $msg, file: "", line: 0, col: 0};
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

# parse4 parses dotted-quad IPv4 into 4 bytes.
func parse4(s as string) {
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
            def quad as bytes init parse4($t);
            $groups[] = $quad[0] * 256 + $quad[1];
            $groups[] = $quad[2] * 256 + $quad[3];
        } else {
            $groups[] = parseGroup($t);
        }
        $i = $i + 1;
    }
    return $groups;
}

# parse6 parses an IPv6 address (with optional `::` compression and embedded
# IPv4) into 16 bytes.
func parse6(s as string) {
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

# parseRaw parses an address to its literal version WITHOUT folding a v4-mapped
# v6 form down to v4 - so a caller (like `parse`) that needs the literal's own
# version and bit-width can decide the fold itself. `parseAddress` layers `unmap`
# on top.
func parseRaw(s as string) {
    if (strings.contains($s, ":")) {
        return Address{version: 6, octets: parse6($s)};
    }
    if (strings.contains($s, ".")) {
        return Address{version: 4, octets: parse4($s)};
    }
    fail("not an IP address: " + $s);
}

/**
 * Parse a bare IP address (IPv4 dotted-quad or IPv6, with `::` compression and
 * embedded IPv4 supported).
 * @param s {string} the address text
 * @return {Address} the parsed address
 * @throws {Error} kind "ipnet" on malformed input
 */
export func parseAddress(s as string) {
    return unmap(parseRaw($s));
}

/**
 * Fold an IPv4-mapped IPv6 address (`::ffff:a.b.c.d`, the `::ffff:0:0/96`
 * range) down to the plain IPv4 `Address` it represents; any other address is
 * returned unchanged. Without this, `::ffff:127.0.0.1` stays a version-6
 * `Address` and silently misses a version-4 network in a `contains` allow-list
 * / deny-list check - a bypass of exactly the kind the leading-zero and
 * embedded-IPv4 guards already close (OM-010). `parseAddress` applies it, so a
 * v4-mapped literal parses straight to a v4 `Address`. The deprecated
 * IPv4-compatible form (`::a.b.c.d`) is intentionally left alone: it is
 * ambiguous with low addresses like `::1` and unmapping it would be wrong.
 * @param addr {Address} the address to normalize
 * @return {Address} a v4 Address if `addr` was v4-mapped, else `addr` unchanged
 */
export func unmap(addr as Address) {
    if (not ($addr.version == 6)) {
        return $addr;
    }
    if (not (len($addr.octets) == 16)) {
        return $addr;
    }
    def i as int init 0;
    while ($i < 10) {
        if (not ($addr.octets[$i] == 0)) {
            return $addr;
        }
        $i = $i + 1;
    }
    if (not ($addr.octets[10] == 0xff and $addr.octets[11] == 0xff)) {
        return $addr;
    }
    def out as bytes;
    $out[] = $addr.octets[12];
    $out[] = $addr.octets[13];
    $out[] = $addr.octets[14];
    $out[] = $addr.octets[15];
    return Address{version: 4, octets: $out};
}

# format4 renders 4 bytes as dotted-quad.
func format4(octets as bytes) {
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

# format6 renders 16 bytes as canonical IPv6 (RFC 5952): lowercase, no leading
# zeros, and the longest run of >= 2 zero groups compressed to `::` (leftmost on
# a tie).
func format6(octets as bytes) {
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
        return format4($addr.octets);
    }
    return format6($addr.octets);
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
 * The prefix range is taken from the **literal**, so an IPv6 v4-mapped block
 * (`::ffff:0:0/96`) is accepted as a /96 and then folded to its v4 equivalent
 * (`0.0.0.0/0`, i.e. the prefix is translated down by the 96-bit v4-mapped
 * offset) - mirroring how `parseAddress` folds a v4-mapped address. A block with
 * a prefix shorter than 96 stays a genuine v6 network (it spans beyond the
 * v4-mapped range).
 * @param cidr {string} the CIDR text (e.g. "10.0.0.0/8" or "2001:db8::/32")
 * @return {Network} the network
 * @throws {Error} kind "ipnet" on malformed input or an out-of-range prefix
 */
export func parse(cidr as string) {
    def slash as int init strings.indexOf($cidr, "/");
    if ($slash < 0) {
        fail("CIDR needs a '/prefix': " + $cidr);
    }
    # Parse to the literal's own version (no v4-mapped fold yet) so the prefix is
    # validated against the literal's bit-width, not the post-fold one.
    def raw as Address init parseRaw(strings.substring($cidr, 0, $slash));
    def prefix as int init parseDecimal(strings.substring($cidr, $slash + 1, len($cidr)));
    def maxp as int init 32;
    if ($raw.version == 6) {
        $maxp = 128;
    }
    if ($prefix > $maxp) {
        fail("prefix out of range 0.." + convert.toString($maxp) + ": " + $cidr);
    }
    def masked as Address init Address{version: $raw.version, octets: applyMask($raw.octets, $prefix)};
    # Fold a v4-mapped IPv6 block down to v4 only when it lies wholly inside
    # ::ffff:0:0/96 (prefix >= 96), translating the prefix by the 96-bit offset.
    if ($raw.version == 6 and $prefix >= 96) {
        def folded as Address init unmap($masked);
        if ($folded.version == 4) {
            return Network{addr: $folded, prefix: $prefix - 96};
        }
    }
    return Network{addr: $masked, prefix: $prefix};
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
    return Address{version: $net.addr.version, octets: $out};
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
    return Address{version: $net.addr.version, octets: $out};
}

# --- byte arithmetic (private) ----------------------------------------------

# copyBytes returns a fresh copy of an address's octets (so an in-place edit
# never mutates the caller's value - though value semantics already copy).
func copyBytes(octets as bytes) {
    def out as bytes;
    def i as int init 0;
    while ($i < len($octets)) {
        $out[] = $octets[$i];
        $i = $i + 1;
    }
    return $out;
}

# addAt adds `amount` to byte `idx` (big-endian) and propagates the carry left.
# Callers that stay within the address size (split) never overflow; the general
# increment path (`next`) checks the final carry itself.
func addAt(octets as bytes, idx as int, amount as int) {
    def out as bytes init copyBytes($octets);
    def carry as int init $amount;
    def j as int init $idx;
    while ($j >= 0 and $carry > 0) {
        def v as int init $out[$j] + $carry;
        $out[$j] = $v & 0xff;
        $carry = $v >> 8;
        $j = $j - 1;
    }
    return $out;
}

# incBytes returns octets + 1, or fails when the address is already the last
# (all-ones) address of its version.
func incBytes(octets as bytes) {
    def out as bytes init copyBytes($octets);
    def carry as int init 1;
    def j as int init len($out) - 1;
    while ($j >= 0 and $carry > 0) {
        def v as int init $out[$j] + $carry;
        $out[$j] = $v & 0xff;
        $carry = $v >> 8;
        $j = $j - 1;
    }
    if ($carry > 0) {
        fail("address overflow: already the last address");
    }
    return $out;
}

# decBytes returns octets - 1, or fails when the address is already all-zero.
func decBytes(octets as bytes) {
    def out as bytes init copyBytes($octets);
    def borrow as int init 1;
    def j as int init len($out) - 1;
    while ($j >= 0 and $borrow > 0) {
        def v as int init $out[$j] - $borrow;
        if ($v < 0) {
            $out[$j] = $v + 256;
            $borrow = 1;
        } else {
            $out[$j] = $v;
            $borrow = 0;
        }
        $j = $j - 1;
    }
    if ($borrow > 0) {
        fail("address underflow: already the first address");
    }
    return $out;
}

# bitsFor returns the address width in bits for a version (32 or 128).
func bitsFor(addr as Address) {
    if ($addr.version == 6) {
        return 128;
    }
    return 32;
}

# --- ordering / iteration (exported) ----------------------------------------

/**
 * The next address after `addr` (numerically +1). Throws at the last address of
 * the version (255.255.255.255 / all-ones IPv6) - there is no successor.
 * @param addr {Address} the address
 * @return {Address} the following address
 * @throws {Error} kind "ipnet" at the last address
 */
export func next(addr as Address) {
    return Address{version: $addr.version, octets: incBytes($addr.octets)};
}

/**
 * The previous address before `addr` (numerically -1). Throws at the first
 * address (0.0.0.0 / ::).
 * @param addr {Address} the address
 * @return {Address} the preceding address
 * @throws {Error} kind "ipnet" at the first address
 */
export func prev(addr as Address) {
    return Address{version: $addr.version, octets: decBytes($addr.octets)};
}

/**
 * Order two addresses: -1 if `a < b`, 0 if equal, 1 if `a > b`. Different
 * versions order v4 before v6 (a stable, if arbitrary, total order for sorting).
 * @param a {Address} the first address
 * @param b {Address} the second address
 * @return {int} -1, 0, or 1
 */
export func compare(a as Address, b as Address) {
    if (not ($a.version == $b.version)) {
        if ($a.version < $b.version) {
            return -1;
        }
        return 1;
    }
    def i as int init 0;
    while ($i < len($a.octets)) {
        if ($a.octets[$i] < $b.octets[$i]) {
            return -1;
        }
        if ($a.octets[$i] > $b.octets[$i]) {
            return 1;
        }
        $i = $i + 1;
    }
    return 0;
}

# --- subnet math (exported) -------------------------------------------------

/**
 * The total number of addresses in a network (2 ^ host-bits). Throws when the
 * block is too large to hold in an `int` (an IPv4 prefix is always fine; an IPv6
 * prefix must be >= 66). For usable-host semantics see `firstUsable` /
 * `lastUsable`.
 * @param net {Network} the network
 * @return {int} the count of addresses in the block
 * @throws {Error} kind "ipnet" when the block exceeds the int range
 */
export func hostCount(net as Network) {
    def bits as int init bitsFor($net.addr);
    def hostbits as int init $bits - $net.prefix;
    if ($hostbits > 62) {
        fail("network too large to count (prefix < " + convert.toString($bits - 62) + ")");
    }
    return 1 << $hostbits;
}

/**
 * The first usable host address of a network. For IPv4 this is the address after
 * the network base (the base is the network address), except a /31 (RFC 3021
 * point-to-point, both addresses usable) and a /32 (single host), where it is the
 * base itself. For IPv6 it is the base + 1 (skipping the subnet-router anycast),
 * except /127 and /128 where it is the base.
 * @param net {Network} the network
 * @return {Address} the first usable address
 */
export func firstUsable(net as Network) {
    def bits as int init bitsFor($net.addr);
    if ($net.prefix >= $bits - 1) {
        return $net.addr;
    }
    return next($net.addr);
}

/**
 * The last usable host address of a network. For IPv4 this is the address before
 * the broadcast address, except a /31 or /32 (where the last address is usable).
 * IPv6 has no broadcast address, so the last address of the block is usable.
 * @param net {Network} the network
 * @return {Address} the last usable address
 */
export func lastUsable(net as Network) {
    def bits as int init bitsFor($net.addr);
    def last as Address init broadcast($net);
    if ($net.prefix >= $bits - 1) {
        return $last;
    }
    if ($net.addr.version == 4) {
        return Address{version: 4, octets: decBytes($last.octets)};
    }
    return $last;
}

/**
 * Every usable host address of a network, in ascending order (from `firstUsable`
 * to `lastUsable` inclusive). Capped at 65536 addresses - a larger block throws,
 * so materializing a whole /8 cannot exhaust memory; walk those with `next`
 * instead.
 * @param net {Network} the network
 * @return {list of Address} the usable host addresses
 * @throws {Error} kind "ipnet" when the block has more than 65536 addresses
 */
export func hosts(net as Network) {
    def cnt as int init hostCount($net);
    if ($cnt > 65536) {
        fail("network has too many addresses to list (" + convert.toString($cnt) + "); iterate with next()");
    }
    def out as list of Address init [];
    def cur as Address init firstUsable($net);
    def last as Address init lastUsable($net);
    def more as bool init true;
    while ($more) {
        $out[] = $cur;
        if (equal($cur, $last)) {
            $more = false;
        } else {
            $cur = next($cur);
        }
    }
    return $out;
}

# strideAdd advances an aligned network base by one block of `prefix` (adds
# 2 ^ (bits - prefix)) - used to walk the subnets of a split.
func strideAdd(octets as bytes, prefix as int) {
    def bi as int init ($prefix - 1) // 8;
    def k as int init $prefix - $bi * 8;
    def inc as int init 1 << (8 - $k);
    return addAt($octets, $bi, $inc);
}

/**
 * Divide a network into the equal subnets of a longer prefix (e.g. a /24 into
 * four /26s). Returns them in ascending order. Capped at 65536 subnets (a
 * `newPrefix` no more than 16 bits longer than the network prefix).
 * @param net {Network} the network to split
 * @param newPrefix {int} the subnet prefix length (>= net.prefix, <= version max)
 * @return {list of Network} the subnets
 * @throws {Error} kind "ipnet" on a bad prefix or more than 65536 subnets
 */
export func split(net as Network, newPrefix as int) {
    def bits as int init bitsFor($net.addr);
    if ($newPrefix < $net.prefix or $newPrefix > $bits) {
        fail("split prefix must be between " + convert.toString($net.prefix) + " and " + convert.toString($bits));
    }
    def added as int init $newPrefix - $net.prefix;
    if ($added > 16) {
        fail("split would produce more than 65536 subnets; use a shorter newPrefix");
    }
    def count as int init 1 << $added;
    def out as list of Network init [];
    def cur as bytes init copyBytes($net.addr.octets);
    def i as int init 0;
    while ($i < $count) {
        $out[] = Network{addr: Address{version: $net.addr.version, octets: $cur}, prefix: $newPrefix};
        if ($i + 1 < $count) {
            $cur = strideAdd($cur, $newPrefix);
        }
        $i = $i + 1;
    }
    return $out;
}

/**
 * Whether two networks share any address. Different versions never overlap.
 * @param a {Network} the first network
 * @param b {Network} the second network
 * @return {bool} true if the blocks intersect
 */
export func overlaps(a as Network, b as Network) {
    if (not ($a.addr.version == $b.addr.version)) {
        return false;
    }
    return contains($a, $b.addr) or contains($b, $a.addr);
}

/**
 * Whether `child` is wholly contained in `parent` (a subnet of it, or equal).
 * Different versions are never a subnet.
 * @param child {Network} the candidate subnet
 * @param parent {Network} the enclosing network
 * @return {bool} true if child lies inside parent
 */
export func subnetOf(child as Network, parent as Network) {
    if (not ($child.addr.version == $parent.addr.version)) {
        return false;
    }
    if ($child.prefix < $parent.prefix) {
        return false;
    }
    return contains($parent, $child.addr);
}

# --- aggregation (exported) -------------------------------------------------

# netLess orders networks by base address then prefix (broader block first on an
# equal base), so a covering block sorts ahead of what it covers.
func netLess(a as Network, b as Network) {
    def c as int init compare($a.addr, $b.addr);
    if ($c < 0) {
        return true;
    }
    if ($c > 0) {
        return false;
    }
    return $a.prefix < $b.prefix;
}

# sortNets returns the networks in `netLess` order (insertion sort; the lists are
# small and lists.sort has no Network comparator).
func sortNets(nets as list of Network) {
    def out as list of Network init [];
    for (def nw in $nets) {
        def inserted as bool init false;
        def acc as list of Network init [];
        for (def e in $out) {
            if (not $inserted and netLess($nw, $e)) {
                $acc[] = $nw;
                $inserted = true;
            }
            $acc[] = $e;
        }
        if (not $inserted) {
            $acc[] = $nw;
        }
        $out = $acc;
    }
    return $out;
}

# dropCovered removes any network already contained in another kept (broader or
# equal) network - after sorting, a cover always precedes what it covers.
func dropCovered(sorted as list of Network) {
    def kept as list of Network init [];
    for (def nw in $sorted) {
        def covered as bool init false;
        for (def k in $kept) {
            if ($k.prefix <= $nw.prefix and contains($k, $nw.addr)) {
                $covered = true;
            }
        }
        if (not $covered) {
            $kept[] = $nw;
        }
    }
    return $kept;
}

func normalizeNets(nets as list of Network) {
    return dropCovered(sortNets($nets));
}

# siblings: two equal-prefix networks that are the two halves of one shorter
# block (same /(prefix-1) parent, different bases) - so they merge into it.
func siblings(a as Network, b as Network) {
    if (not ($a.addr.version == $b.addr.version)) {
        return false;
    }
    if (not ($a.prefix == $b.prefix) or $a.prefix < 1) {
        return false;
    }
    if (equal($a.addr, $b.addr)) {
        return false;
    }
    def pa as bytes init applyMask($a.addr.octets, $a.prefix - 1);
    def pb as bytes init applyMask($b.addr.octets, $b.prefix - 1);
    return bytesEqual($pa, $pb);
}

func parentOf(nw as Network) {
    return Network{
        addr: Address{version: $nw.addr.version, octets: applyMask($nw.addr.octets, $nw.prefix - 1)},
        prefix: $nw.prefix - 1
    };
}

# aggregateSame merges one version's networks to the minimal covering set: drop
# contained blocks, then repeatedly fold sibling pairs into their parent until no
# pair remains.
func aggregateSame(nets as list of Network) {
    def cur as list of Network init normalizeNets($nets);
    def changed as bool init true;
    while ($changed) {
        $changed = false;
        def ai as int init -1;
        def bi as int init -1;
        def i as int init 0;
        while ($i < len($cur) and $ai < 0) {
            def j as int init $i + 1;
            while ($j < len($cur) and $ai < 0) {
                if (siblings($cur[$i], $cur[$j])) {
                    $ai = $i;
                    $bi = $j;
                }
                $j = $j + 1;
            }
            $i = $i + 1;
        }
        if ($ai >= 0) {
            def parent as Network init parentOf($cur[$ai]);
            def rest as list of Network init [];
            def k as int init 0;
            while ($k < len($cur)) {
                if (not ($k == $ai) and not ($k == $bi)) {
                    $rest[] = $cur[$k];
                }
                $k = $k + 1;
            }
            $rest[] = $parent;
            $cur = normalizeNets($rest);
            $changed = true;
        }
    }
    return $cur;
}

/**
 * Collapse a list of networks into the minimal equivalent set: contained blocks
 * are dropped and adjacent sibling pairs are merged into their shorter parent
 * (e.g. 192.168.0.0/25 + 192.168.0.128/25 -> 192.168.0.0/24). IPv4 and IPv6
 * entries are aggregated independently; the result is sorted ascending.
 * @param nets {list of Network} the networks to aggregate
 * @return {list of Network} the minimal covering set
 */
export func aggregate(nets as list of Network) {
    def v4 as list of Network init [];
    def v6 as list of Network init [];
    for (def nw in $nets) {
        if ($nw.addr.version == 4) {
            $v4[] = $nw;
        } else {
            $v6[] = $nw;
        }
    }
    def out as list of Network init [];
    for (def r in aggregateSame($v4)) {
        $out[] = $r;
    }
    for (def r in aggregateSame($v6)) {
        $out[] = $r;
    }
    return $out;
}

# --- classification (exported) ----------------------------------------------

/**
 * The address scope: which well-known category an address falls in. A total,
 * disjoint classification - every address is exactly one `Scope` - so the
 * `is*` predicates are thin wrappers over it and a caller can `match` on the
 * whole set. `Reserved` covers the special-purpose ranges that are neither a
 * normal global unicast address nor one of the named categories (documentation,
 * benchmarking, shared CGNAT space, and other reserved blocks).
 */
export def enum Scope {
    Global,
    Private,
    Loopback,
    LinkLocal,
    Multicast,
    Unspecified,
    Reserved
};

# inCidr tests membership in a constant CIDR (parsed fresh; the constants are
# tiny and this keeps the classification table declarative).
func inCidr(addr as Address, cidr as string) {
    return contains(parse($cidr), $addr);
}

# allZero: the unspecified address (0.0.0.0 or ::) has every octet zero.
func allZero(addr as Address) {
    def i as int init 0;
    while ($i < len($addr.octets)) {
        if (not ($addr.octets[$i] == 0)) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

/**
 * Classify an address into its `Scope`. A v4-mapped IPv6 address is folded to v4
 * first (via `unmap`), so `::ffff:10.0.0.1` classifies as `Private`.
 * @param addr {Address} the address
 * @return {Scope} the address's scope
 */
export func scope(addr as Address) {
    def a as Address init unmap($addr);
    if (allZero($a)) {
        return Scope.Unspecified;
    }
    if ($a.version == 4) {
        if (inCidr($a, "127.0.0.0/8")) {
            return Scope.Loopback;
        }
        if (inCidr($a, "169.254.0.0/16")) {
            return Scope.LinkLocal;
        }
        if (inCidr($a, "224.0.0.0/4")) {
            return Scope.Multicast;
        }
        if (inCidr($a, "10.0.0.0/8") or inCidr($a, "172.16.0.0/12") or inCidr($a, "192.168.0.0/16")) {
            return Scope.Private;
        }
        if (inCidr($a, "0.0.0.0/8") or inCidr($a, "100.64.0.0/10") or inCidr($a, "192.0.2.0/24") or inCidr($a, "198.51.100.0/24") or inCidr($a, "203.0.113.0/24") or inCidr($a, "192.88.99.0/24") or inCidr($a, "240.0.0.0/4")) {
            return Scope.Reserved;
        }
        return Scope.Global;
    }
    if (inCidr($a, "::1/128")) {
        return Scope.Loopback;
    }
    if (inCidr($a, "fe80::/10")) {
        return Scope.LinkLocal;
    }
    if (inCidr($a, "ff00::/8")) {
        return Scope.Multicast;
    }
    if (inCidr($a, "fc00::/7")) {
        return Scope.Private;
    }
    if (inCidr($a, "2001:db8::/32")) {
        return Scope.Reserved;
    }
    if (inCidr($a, "2000::/3")) {
        return Scope.Global;
    }
    return Scope.Reserved;
}

/**
 * Whether an address is a loopback address (127.0.0.0/8 or ::1).
 * @param addr {Address} the address
 * @return {bool} true if loopback
 */
export func isLoopback(addr as Address) {
    return scope($addr) == Scope.Loopback;
}

/**
 * Whether an address is in private (RFC 1918) or IPv6 unique-local (fc00::/7)
 * space.
 * @param addr {Address} the address
 * @return {bool} true if private / ULA
 */
export func isPrivate(addr as Address) {
    return scope($addr) == Scope.Private;
}

/**
 * Whether an address is multicast (224.0.0.0/4 or ff00::/8).
 * @param addr {Address} the address
 * @return {bool} true if multicast
 */
export func isMulticast(addr as Address) {
    return scope($addr) == Scope.Multicast;
}

/**
 * Whether an address is link-local (169.254.0.0/16 or fe80::/10).
 * @param addr {Address} the address
 * @return {bool} true if link-local
 */
export func isLinkLocal(addr as Address) {
    return scope($addr) == Scope.LinkLocal;
}

/**
 * Whether an address is the unspecified address (0.0.0.0 or ::).
 * @param addr {Address} the address
 * @return {bool} true if unspecified
 */
export func isUnspecified(addr as Address) {
    return scope($addr) == Scope.Unspecified;
}

/**
 * Whether an address is a normal globally-routable unicast address - i.e. its
 * `scope` is `Global` (not private, loopback, link-local, multicast,
 * unspecified, or a reserved / documentation range). Best-effort, per the
 * well-known special-purpose registries.
 * @param addr {Address} the address
 * @return {bool} true if globally routable
 */
export func isGlobal(addr as Address) {
    return scope($addr) == Scope.Global;
}
