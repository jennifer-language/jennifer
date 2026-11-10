# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * URL parsing, building, and query-string handling (RFC 3986). The shared URL
 * layer the network modules (`http`, `rest`, `s3`, `oauth`, `influxdb`, ...)
 * build on instead of re-splitting strings and hand-rolling percent-encoding.
 *
 * `parse` splits a URL into a `Uri` (scheme / user / host / port / path / query
 * / fragment); `build` reassembles one. `encode` / `decode` are RFC 3986
 * percent-encoding (over the `encoding` library's `uri-percent` codec), and
 * `encodeForm` / `decodeForm` are the `application/x-www-form-urlencoded` variant
 * (space as "+", the `uri-form` codec). `buildQuery` / `parseQuery` convert
 * between a `map of string to string` and a `key=value&...` string using form
 * encoding (the query-string convention). `resolve` applies a relative
 * reference to a base URL (RFC 3986 section 5), the operation a feed reader or
 * crawler needs to turn a relative link absolute.
 *
 * Pure Jennifer over `strings` + `encoding` + `convert` + `regex` - no Go, no network -
 * so it runs on both binaries. An IPv6 literal host keeps its brackets in `host`
 * (`[::1]`) with the port split out; everything else follows RFC 3986.
 * @module uri
 */

use strings;
use encoding;
use convert;
use regex;

/**
 * The parts of a parsed URL. Missing components are the empty string (so a
 * relative URL has an empty scheme and host). `port` is a string ("" when
 * absent) rather than an int, to distinguish "no port" from port 0.
 * @field scheme {string} the scheme without the trailing ":" (e.g. "https")
 * @field user {string} the userinfo before the last "@" ("" when absent)
 * @field host {string} the host name or IP literal ("" when absent)
 * @field port {string} the port after ":" ("" when absent)
 * @field path {string} the path, including its leading "/" when present
 * @field query {string} the raw query string without the leading "?"
 * @field fragment {string} the fragment without the leading "#"
 */
export def struct Uri {
    scheme as string,
    user as string,
    host as string,
    port as string,
    path as string,
    query as string,
    fragment as string
};

/**
 * Percent-encode a string per RFC 3986 (component encoding): every byte outside
 * the unreserved set `A-Za-z0-9-._~` becomes `%XX`, space becomes `%20`. Safe in
 * any URL position (a path segment, a query value).
 * @param s {string} the text to encode
 * @return {string} the percent-encoded text
 */
export func encode(s as string) {
    return encoding.toText("uri-percent", convert.bytesFromString("utf-8", $s));
}

/**
 * Reverse percent-encoding: `%XX` triples decode to their byte, and a literal
 * "+" is left as "+" (RFC 3986, not form encoding - use parseQuery for query
 * strings, which treats "+" as a space).
 * @param s {string} the percent-encoded text
 * @return {string} the decoded text
 * @throws {Error} kind "uri" on a malformed "%" escape, or when the decoded bytes
 *   are not valid UTF-8
 */
export func decode(s as string) {
    return unescape($s, "uri-percent", "decode");
}

/**
 * Form-encode a string per `application/x-www-form-urlencoded`: like `encode`,
 * but a space becomes "+" instead of "%20". This is the encoding for query-string
 * values and HTML form bodies (`buildQuery` uses it).
 * @param s {string} the text to encode
 * @return {string} the form-encoded text
 */
export func encodeForm(s as string) {
    return encoding.toText("uri-form", convert.bytesFromString("utf-8", $s));
}

/**
 * Reverse form-encoding: `%XX` triples decode to their byte and a "+" decodes to
 * a space (the form-urlencoded convention). Use this for query-string values;
 * use `decode` for RFC 3986 path segments where "+" is literal.
 * @param s {string} the form-encoded text
 * @return {string} the decoded text
 * @throws {Error} kind "uri" on a malformed "%" escape, or when the decoded bytes
 *   are not valid UTF-8
 */
export func decodeForm(s as string) {
    return unescape($s, "uri-form", "decodeForm");
}

/**
 * Decode `s` with the `encoding` codec `format`, raising a failure as kind "uri"
 * under the public function `fn` the caller used, not the codec's internals.
 * @param s {string} the encoded text
 * @param format {string} "uri-percent" or "uri-form"
 * @param fn {string} the public function name for the error message
 * @return {string} the decoded text
 * @throws {Error} kind "uri" on a malformed escape or invalid UTF-8
 */
func unescape(s as string, format as string, fn as string) {
    def raw as bytes;
    try {
        $raw = encoding.fromText($format, $s);
    } catch (e) {
        def at as int init strings.indexOf($e.message, " decode: ");
        def detail as string init $e.message;
        if ($at >= 0) {
            $detail = $e.message[$at + 9..];
        }
        throw Error{kind: "uri", message: "uri.{$fn}: {$detail}"};
    }
    try {
        return convert.stringFromBytes("utf-8", $raw);
    } catch (e) {
        throw Error{kind: "uri", message: "uri.{$fn}: decoded bytes are not valid UTF-8"};
    }
}

/**
 * Parse a URL into its parts. Absent components are "". The authority
 * (user/host/port) is only populated when the URL has a "//" authority, so a
 * scheme-only URL like "mailto:x@y" keeps "x@y" in `path`. The text before
 * the first ":" is a scheme only when it is a valid scheme name, so
 * "user@host:22/x" is a relative path. The userinfo ends at the last "@".
 * Parsing splits; it does not validate a host or port.
 * @param raw {string} the URL text
 * @return {Uri} the parsed parts
 */
export func parse(raw as string) {
    def s as string init $raw;
    def fragment as string init "";
    def query as string init "";

    def hi as int init strings.indexOf($s, "#");
    if ($hi >= 0) {
        $fragment = $s[$hi + 1..];
        $s = $s[..$hi];
    }
    def qi as int init strings.indexOf($s, "?");
    if ($qi >= 0) {
        $query = $s[$qi + 1..];
        $s = $s[..$qi];
    }

    # A scheme is present when ":" appears before the first "/" and what
    # precedes it is a scheme name (RFC 3986: a letter, then letters, digits,
    # "+", "-" or "."); otherwise the text is a relative reference.
    def scheme as string init "";
    def rest as string init $s;
    def ci as int init strings.indexOf($s, ":");
    def firstSlash as int init strings.indexOf($s, "/");
    if ($ci > 0 and ($firstSlash < 0 or $ci < $firstSlash) and
        regex.matches('^[A-Za-z][A-Za-z0-9+.-]*$', $s[..$ci])) {
        $scheme = $s[..$ci];
        $rest = $s[$ci + 1..];
    }

    def user as string init "";
    def host as string init "";
    def port as string init "";
    def path as string init $rest;
    if (strings.startsWith($rest, "//")) {
        def auth as string init $rest[2..];
        def ps as int init strings.indexOf($auth, "/");
        if ($ps >= 0) {
            $path = $auth[$ps..];
            $auth = $auth[..$ps];
        } else {
            $path = "";
        }
        # The last "@" ends the userinfo, as in `http` and browsers: a
        # password may hold an "@", a host never does.
        def at as int init strings.lastIndexOf($auth, "@");
        if ($at >= 0) {
            $user = $auth[..$at];
            $auth = $auth[$at + 1..];
        }
        $host = $auth;
        if (strings.startsWith($auth, "[")) {
            # IPv6 literal: host is "[...]" up to and including "]", and a port
            # (if any) follows the "]".
            def close as int init strings.indexOf($auth, "]");
            if ($close >= 0) {
                $host = $auth[..$close + 1];
                def afterClose as string init $auth[$close + 1..];
                if (strings.startsWith($afterClose, ":")) {
                    $port = $afterClose[1..];
                }
            }
        } else {
            def pc as int init strings.indexOf($auth, ":");
            if ($pc >= 0) {
                $host = $auth[..$pc];
                $port = $auth[$pc + 1..];
            }
        }
    }

    return Uri{
        scheme: $scheme,
        user: $user,
        host: $host,
        port: $port,
        path: $path,
        query: $query,
        fragment: $fragment
    };
}

/**
 * Reassemble a URL string from its parts (the inverse of parse). Components are
 * emitted verbatim (already-encoded); build does not re-encode. An empty
 * query, fragment or port marker ("?", "#", ":") is not kept, and a `file:`
 * URL with a path and no host is written with "//" ("file:///etc").
 * @param u {Uri} the parts to assemble
 * @return {string} the URL string
 */
export func build(u as Uri) {
    def s as string init "";
    if ($u.scheme != "") {
        $s = $u.scheme + ":";
    }
    # A `file:` URL keeps its empty authority ("file:///etc"), which RFC 8089
    # treats as the same as "file:/etc".
    def fileRoot as bool init strings.lower($u.scheme) == "file" and
        strings.startsWith($u.path, "/");
    if ($u.host != "" or $u.user != "" or $fileRoot) {
        $s = $s + "//";
        if ($u.user != "") {
            $s = $s + $u.user + "@";
        }
        $s = $s + $u.host;
        if ($u.port != "") {
            $s = $s + ":" + $u.port;
        }
    }
    $s = $s + $u.path;
    if ($u.query != "") {
        $s = $s + "?" + $u.query;
    }
    if ($u.fragment != "") {
        $s = $s + "#" + $u.fragment;
    }
    return $s;
}

/**
 * Build a `key=value&...` query string from a map, form-encoding every key and
 * value (the query-string convention: a space becomes "+"). Pairs keep the map's
 * insertion order.
 * @param params {map of string to string} the query parameters
 * @return {string} the encoded query string (without a leading "?")
 */
export func buildQuery(params as map of string to string) {
    def parts as list of string init [];
    for (def key in $params) {
        $parts[] = encodeForm($key) + "=" + encodeForm($params[$key]);
    }
    return strings.join($parts, "&");
}

/**
 * Parse a `key=value&...` query string into a map, decoding each component. A
 * "+" decodes to a space (the form-urlencoded convention), a repeated key keeps
 * its last value, and a bare "key" (no "=") maps to "".
 * @param q {string} the query string (without a leading "?")
 * @return {map of string to string} the decoded parameters
 * @throws {Error} kind "uri" on a malformed "%" escape, or when a decoded
 *   component is not valid UTF-8
 */
export func parseQuery(q as string) {
    def out as map of string to string init {};
    if ($q == "") {
        return $out;
    }
    for (def pair in strings.split($q, "&")) {
        if ($pair == "") {
            continue;
        }
        def eq as int init strings.indexOf($pair, "=");
        if ($eq >= 0) {
            def k as string init unescape($pair[..$eq], "uri-form", "parseQuery");
            def v as string init unescape($pair[$eq + 1..], "uri-form", "parseQuery");
            $out[$k] = $v;
        } else {
            $out[unescape($pair, "uri-form", "parseQuery")] = "";
        }
    }
    return $out;
}

/**
 * Resolve a (possibly relative) reference against a base URL, per RFC 3986
 * section 5: turn "../img.png" plus "https://h/a/b/page" into
 * "https://h/a/img.png". An absolute reference (with its own scheme) is returned
 * as-is.
 * @param base {string} the absolute base URL
 * @param ref {string} the reference to resolve (absolute or relative)
 * @return {string} the resolved absolute URL
 */
export func resolve(base as string, ref as string) {
    def b as Uri init parse($base);
    def r as Uri init parse($ref);
    def out as Uri init Uri{
        scheme: "",
        user: "",
        host: "",
        port: "",
        path: "",
        query: "",
        fragment: ""
    };

    if ($r.scheme != "") {
        $out = $r;
        $out.path = removeDotSegments($r.path);
    } else {
        $out.scheme = $b.scheme;
        if ($r.host != "" or $r.user != "") {
            $out.user = $r.user;
            $out.host = $r.host;
            $out.port = $r.port;
            $out.path = removeDotSegments($r.path);
            $out.query = $r.query;
        } else {
            $out.user = $b.user;
            $out.host = $b.host;
            $out.port = $b.port;
            if ($r.path == "") {
                $out.path = $b.path;
                if ($r.query != "") {
                    $out.query = $r.query;
                } else {
                    $out.query = $b.query;
                }
            } else {
                if (strings.startsWith($r.path, "/")) {
                    $out.path = removeDotSegments($r.path);
                } else {
                    $out.path = removeDotSegments(mergePath($b, $r.path));
                }
                $out.query = $r.query;
            }
        }
    }
    $out.fragment = $r.fragment;
    return build($out);
}

# mergePath joins a relative reference path onto the base path: keep everything
# up to (and including) the base's last "/", then append the reference. If the
# base has an authority but an empty path, the merged path is "/" + ref.
func mergePath(b as Uri, refPath as string) {
    if (($b.host != "" or $b.user != "") and $b.path == "") {
        return "/" + $refPath;
    }
    def slash as int init strings.lastIndexOf($b.path, "/");
    if ($slash < 0) {
        return $refPath;
    }
    return $b.path[..$slash + 1] + $refPath;
}

# removeDotSegments applies the RFC 3986 section 5.2.4 algorithm, collapsing
# "." and ".." segments (so "/a/b/../c" becomes "/a/c"). It walks the segments
# of one split with a stack whose top is a counter, so a pop copies nothing:
# slicing the remaining input or popping a list per step is O(N^2) on a long
# path. Each output entry carries its own leading "/" (the first segment of a
# relative path has none), as in the RFC's output buffer.
func removeDotSegments(path as string) {
    def segs as list of string init strings.split($path, "/");
    def output as list of string init [];
    def top as int init 0;
    def i as int init 0;
    def count as int init len($segs);
    if (strings.startsWith($path, "/")) {
        $i = 1;
    } else {
        # A relative path drops its leading "./" and "../" (rule A) and a
        # whole "." or ".." (rule D); its first other segment has no "/".
        while ($i < $count and ($segs[$i] == "." or $segs[$i] == "..")) {
            $i = $i + 1;
        }
        if ($i >= $count) {
            return "";
        }
        $output[] = $segs[$i];
        $top = 1;
        $i = $i + 1;
    }
    while ($i < $count) {
        def seg as string init $segs[$i];
        def last as bool init $i == $count - 1;
        def entry as string init "/" + $seg;
        def keep as bool init true;
        if ($seg == "." or $seg == "..") {
            # "/./" and "/../" become "/"; a final "/." or "/.." leaves "/".
            if ($seg == ".." and $top > 0) {
                $top = $top - 1;
            }
            $entry = "/";
            $keep = $last;
        }
        if ($keep) {
            if ($top < len($output)) {
                $output[$top] = $entry;
            } else {
                $output[] = $entry;
            }
            $top = $top + 1;
        }
        $i = $i + 1;
    }
    return strings.join($output[..$top], "");
}
