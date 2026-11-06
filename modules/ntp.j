# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
# pragma-jennifer-capability: net

/**
 * A simple SNTP client (RFC 4330 / 5905): query a time server over UDP and get
 * back its time plus the local clock offset and round-trip delay. This is the
 * one-shot query half of NTP - it speaks the standard NTP wire protocol (the
 * 48-byte packet on port 123) but does not discipline the clock or run as a
 * daemon; it reports the measurement and leaves acting on it to the caller.
 *
 * The packet is packed / unpacked with `bytes` and the bitwise operators, and
 * NTP timestamps (seconds since 1900) are converted to `time.Time` through
 * `time`. Needs the default `jennifer` binary (`net`); a query throws
 * `Error{kind: "ntp"}` on timeout or a short reply.
 * @module ntp
 * @example
 * import "ntp.j" as ntp;
 * use io; use time;
 * def r as ntp.Result init ntp.query("pool.ntp.org");
 * io.printf("server time: %s  offset: %d ms\n", time.iso($r.serverTime), time.milliseconds($r.offset));
 */
use net;
use time;
use binary;
use convert;

# NTP counts seconds from 1900-01-01; Unix time from 1970-01-01.
def const NTP_UNIX_OFFSET as int init 2208988800;
def const DEFAULT_TIMEOUT as int init 5000;
def const ONE_SECOND as int init 1000000000;

/**
 * The result of a query.
 * @field serverTime {time.Time} the server's transmit time
 * @field offset {time.Duration} the local clock offset (server minus local)
 * @field delay {time.Duration} the measured round-trip delay
 */
export def struct Result {
    serverTime as time.Time,
    offset as time.Duration,
    delay as time.Duration
};

func fail(msg as string) {
    throw Error{kind: "ntp", message: $msg};
}

# --- packet codec (private) -------------------------------------------------

# appendUint appends a 32-bit big-endian value to a byte buffer.
func appendUint(b as bytes, v as int) {
    $b[] = ($v >> 24) & 0xff;
    $b[] = ($v >> 16) & 0xff;
    $b[] = ($v >> 8) & 0xff;
    $b[] = $v & 0xff;
    return $b;
}

# readUint reads a 32-bit big-endian value at a byte offset.
func readUint(b as bytes, off as int) {
    return ($b[$off] << 24) | ($b[$off + 1] << 16) | ($b[$off + 2] << 8) | $b[$off + 3];
}

# buildRequest builds a 48-byte client request (LI=0, version 4, mode 3 client)
# with the transmit timestamp set to the given instant.
func buildRequest(orig as time.Time) {
    def pkt as bytes;
    $pkt[] = 0x23;
    for (def i in 1..40) {
        $pkt[] = 0;
    }
    def secs as int init time.unix($orig) + NTP_UNIX_OFFSET;
    def frac as int init ((time.unixNanos($orig) % ONE_SECOND) << 32) // ONE_SECOND;
    $pkt = appendUint($pkt, $secs);
    $pkt = appendUint($pkt, $frac);
    return $pkt;
}

# readTimestamp decodes the 64-bit NTP timestamp at a byte offset into a Time.
# The 32-bit seconds wrap on 2036-02-07 (RFC 5905 eras): a value with the top
# bit clear is read as era 1, so 1968-2036 and 2036-2104 both decode.
func readTimestamp(b as bytes, off as int) {
    def secs as int init readUint($b, $off);
    if ($secs < 0x80000000) {
        $secs = $secs + 0x100000000;
    }
    def frac as int init readUint($b, $off + 4);
    def nanos as int init ($frac * ONE_SECOND) >> 32;
    return time.fromUnixNanos(($secs - NTP_UNIX_OFFSET) * ONE_SECOND + $nanos);
}

# checkReply applies the RFC 4330 section 5 sanity checks to a server reply: the
# originate timestamp must echo the request's transmit timestamp (a reply to
# some other request, or a spoofed one, does not), the mode must be server (4)
# or broadcast (5), the server must be synchronised (leap indicator not 3), a
# stratum of 0 is a Kiss-o'-Death whose code is in the reference id, and the
# transmit timestamp must be set.
func checkReply(resp as bytes, req as bytes, address as string) {
    if (not binary.slice($resp, 24, 32) == binary.slice($req, 40, 48)) {
        fail("reply from " + $address + " does not answer this request");
    }
    def mode as int init $resp[0] & 7;
    if ($mode != 4 and $mode != 5) {
        fail("reply from " + $address + " is not a server reply (mode {$mode})");
    }
    if (($resp[0] >> 6) == 3) {
        fail("server " + $address + " is not synchronised");
    }
    if ($resp[1] == 0) {
        # The kiss code is four ASCII letters (RATE, DENY, ...); anything else in
        # the reference id shows as '?'.
        def code as string init "";
        for (def i in 12..16) {
            def c as int init $resp[$i];
            if ($c >= 32 and $c < 127) {
                $code = $code + convert.fromCodepoint($c);
            } else {
                $code = $code + "?";
            }
        }
        fail("server " + $address + " sent a Kiss-o'-Death (" + $code + ")");
    }
    if (readUint($resp, 40) == 0 and readUint($resp, 44) == 0) {
        fail("reply from " + $address + " has no transmit timestamp");
    }
}

# --- query (exported) -------------------------------------------------------

/**
 * Query an NTP server by host name (port 123, a 5-second timeout).
 * @param host {string} the server host (e.g. "pool.ntp.org")
 * @return {Result} the server time, clock offset, and round-trip delay
 * @throws {Error} kind "ntp" on timeout or a malformed reply
 */
export func query(host as string) {
    return queryWith($host + ":123", DEFAULT_TIMEOUT);
}

/**
 * Query an NTP server at a full `host:port` address with a timeout in
 * milliseconds.
 * @param address {string} the server "host:port"
 * @param timeoutMs {int} the receive timeout in milliseconds
 * @return {Result} the server time, clock offset, and round-trip delay
 * @throws {Error} kind "ntp" on timeout or a malformed reply
 */
export func queryWith(address as string, timeoutMs as int) {
    def sock as net.UDPSocket init net.listenUDP(":0");
    defer net.close($sock); # closed however the query exits
    net.setReadDeadline($sock, $timeoutMs);
    def orig as time.Time init time.now();
    net.sendTo($sock, $address, buildRequest($orig));
    def resp as bytes;
    def dst as time.Time init $orig;
    try {
        def dg as net.Datagram init net.recvFrom($sock, 48);
        $resp = $dg.data;
        $dst = time.now();
    } catch (e) {
        fail("no response from {$address} within {$timeoutMs}ms");
    }
    if (len($resp) < 48) {
        fail("short NTP response from " + $address);
    }
    checkReply($resp, buildRequest($orig), $address);
    def rec as time.Time init readTimestamp($resp, 32);
    def xmt as time.Time init readTimestamp($resp, 40);
    def nOrig as int init time.unixNanos($orig);
    def nRec as int init time.unixNanos($rec);
    def nXmt as int init time.unixNanos($xmt);
    def nDst as int init time.unixNanos($dst);
    # offset = midpoint(receive, transmit) - midpoint(originate, destination);
    # delay  = (destination - originate) - (transmit - receive).
    def serverMid as time.Time init time.fromUnixNanos(($nRec + $nXmt) // 2);
    def localMid as time.Time init time.fromUnixNanos(($nOrig + $nDst) // 2);
    def offset as time.Duration init time.sub($serverMid, $localMid);
    def delayNanos as int init ($nDst - $nOrig) - ($nXmt - $nRec);
    def delay as time.Duration init time.sub(
        time.fromUnixNanos($delayNanos),
        time.fromUnixNanos(0));
    return Result{serverTime: $xmt, offset: $offset, delay: $delay};
}
