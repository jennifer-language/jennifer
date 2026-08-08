# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
 * Pure Jennifer over `strings` + `encoding` + `convert` - no Go, no network -
 * so it runs on both binaries. An IPv6 literal host keeps its brackets in `host`
 * (`[::1]`) with the port split out; everything else follows RFC 3986.
 * @module uri
 */

use strings;
use encoding;
use convert;

/**
 * The parts of a parsed URL. Missing components are the empty string (so a
 * relative URL has an empty scheme and host). `port` is a string ("" when
 * absent) rather than an int, to distinguish "no port" from port 0.
 * @field scheme {string} the scheme without the trailing ":" (e.g. "https")
 * @field user {string} the userinfo before "@" ("" when absent)
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
    return encoding.toText(convert.bytesFromString($s, "utf-8"), "uri-percent");
}

/**
 * Reverse percent-encoding: `%XX` triples decode to their byte, and a literal
 * "+" is left as "+" (RFC 3986, not form encoding - use parseQuery for query
 * strings, which treats "+" as a space).
 * @param s {string} the percent-encoded text
 * @return {string} the decoded text
 * @throws {Error} on a malformed "%" escape
 */
export func decode(s as string) {
    return convert.stringFromBytes(encoding.fromText($s, "uri-percent"), "utf-8");
}

/**
 * Form-encode a string per `application/x-www-form-urlencoded`: like `encode`,
 * but a space becomes "+" instead of "%20". This is the encoding for query-string
 * values and HTML form bodies (`buildQuery` uses it).
 * @param s {string} the text to encode
 * @return {string} the form-encoded text
 */
export func encodeForm(s as string) {
    return encoding.toText(convert.bytesFromString($s, "utf-8"), "uri-form");
}

/**
 * Reverse form-encoding: `%XX` triples decode to their byte and a "+" decodes to
 * a space (the form-urlencoded convention). Use this for query-string values;
 * use `decode` for RFC 3986 path segments where "+" is literal.
 * @param s {string} the form-encoded text
 * @return {string} the decoded text
 * @throws {Error} on a malformed "%" escape
 */
export func decodeForm(s as string) {
    return convert.stringFromBytes(encoding.fromText($s, "uri-form"), "utf-8");
}

/**
 * Parse a URL into its parts. Absent components are "". The authority
 * (user/host/port) is only populated when the URL has a "//" authority, so a
 * scheme-only URL like "mailto:x@y" keeps "x@y" in `path`.
 * @param raw {string} the URL text
 * @return {Uri} the parsed parts
 */
export func parse(raw as string) {
    def s as string init $raw;
    def fragment as string init "";
    def query as string init "";

    def hi as int init strings.indexOf($s, "#");
    if ($hi >= 0) {
        $fragment = strings.substring($s, $hi + 1, len($s));
        $s = strings.substring($s, 0, $hi);
    }
    def qi as int init strings.indexOf($s, "?");
    if ($qi >= 0) {
        $query = strings.substring($s, $qi + 1, len($s));
        $s = strings.substring($s, 0, $qi);
    }

    # A scheme is present when ":" appears before the first "/".
    def scheme as string init "";
    def rest as string init $s;
    def ci as int init strings.indexOf($s, ":");
    def firstSlash as int init strings.indexOf($s, "/");
    if ($ci > 0 and ($firstSlash < 0 or $ci < $firstSlash)) {
        $scheme = strings.substring($s, 0, $ci);
        $rest = strings.substring($s, $ci + 1, len($s));
    }

    def user as string init "";
    def host as string init "";
    def port as string init "";
    def path as string init $rest;
    if (strings.startsWith($rest, "//")) {
        def auth as string init strings.substring($rest, 2, len($rest));
        def ps as int init strings.indexOf($auth, "/");
        if ($ps >= 0) {
            $path = strings.substring($auth, $ps, len($auth));
            $auth = strings.substring($auth, 0, $ps);
        } else {
            $path = "";
        }
        def at as int init strings.indexOf($auth, "@");
        if ($at >= 0) {
            $user = strings.substring($auth, 0, $at);
            $auth = strings.substring($auth, $at + 1, len($auth));
        }
        $host = $auth;
        if (strings.startsWith($auth, "[")) {
            # IPv6 literal: host is "[...]" up to and including "]", and a port
            # (if any) follows the "]".
            def close as int init strings.indexOf($auth, "]");
            if ($close >= 0) {
                $host = strings.substring($auth, 0, $close + 1);
                def afterClose as string init strings.substring($auth, $close + 1, len($auth));
                if (strings.startsWith($afterClose, ":")) {
                    $port = strings.substring($afterClose, 1, len($afterClose));
                }
            }
        } else {
            def pc as int init strings.indexOf($auth, ":");
            if ($pc >= 0) {
                $host = strings.substring($auth, 0, $pc);
                $port = strings.substring($auth, $pc + 1, len($auth));
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
 * emitted verbatim (already-encoded); build does not re-encode.
 * @param u {Uri} the parts to assemble
 * @return {string} the URL string
 */
export func build(u as Uri) {
    def s as string init "";
    if ($u.scheme != "") {
        $s = $u.scheme + ":";
    }
    if ($u.host != "" or $u.user != "") {
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
 * @throws {Error} on a malformed "%" escape
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
            def k as string init decodeForm(strings.substring($pair, 0, $eq));
            def v as string init decodeForm(strings.substring($pair, $eq + 1, len($pair)));
            $out[$k] = $v;
        } else {
            $out[decodeForm($pair)] = "";
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
    def out as Uri init Uri{scheme: "", user: "", host: "", port: "", path: "", query: "", fragment: ""};

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
    def slash as int init lastIndexOf($b.path, "/");
    if ($slash < 0) {
        return $refPath;
    }
    return strings.substring($b.path, 0, $slash + 1) + $refPath;
}

# lastIndexOf returns the byte index of the last "/" in s, or -1. strings has no
# lastIndexOf, so scan forward and keep the highest hit.
func lastIndexOf(s as string, sub as string) {
    def found as int init -1;
    def i as int init 0;
    while ($i < len($s)) {
        if (strings.substring($s, $i, $i + 1) == $sub) {
            $found = $i;
        }
        $i = $i + 1;
    }
    return $found;
}

# removeDotSegments applies the RFC 3986 section 5.2.4 algorithm, collapsing
# "." and ".." segments (so "/a/b/../c" becomes "/a/c").
func removeDotSegments(path as string) {
    def input as string init $path;
    def output as string init "";
    while (len($input) > 0) {
        if (strings.startsWith($input, "../")) {
            $input = strings.substring($input, 3, len($input));
        } elseif (strings.startsWith($input, "./")) {
            $input = strings.substring($input, 2, len($input));
        } elseif (strings.startsWith($input, "/./")) {
            $input = "/" + strings.substring($input, 3, len($input));
        } elseif ($input == "/.") {
            $input = "/";
        } elseif (strings.startsWith($input, "/../")) {
            $input = "/" + strings.substring($input, 4, len($input));
            $output = dropLastSegment($output);
        } elseif ($input == "/..") {
            $input = "/";
            $output = dropLastSegment($output);
        } elseif ($input == "." or $input == "..") {
            $input = "";
        } else {
            # Move the first path segment (the leading "/" plus chars up to but
            # not including the next "/") from input to output.
            def start as int init 0;
            if (strings.startsWith($input, "/")) {
                $start = 1;
            }
            def next as int init indexFrom($input, "/", $start);
            if ($next < 0) {
                $output = $output + $input;
                $input = "";
            } else {
                $output = $output + strings.substring($input, 0, $next);
                $input = strings.substring($input, $next, len($input));
            }
        }
    }
    return $output;
}

# dropLastSegment removes the last "/segment" from s (used by the ".." rules).
func dropLastSegment(s as string) {
    def slash as int init lastIndexOf($s, "/");
    if ($slash < 0) {
        return "";
    }
    return strings.substring($s, 0, $slash);
}

# indexFrom is strings.indexOf starting at offset `from` (returns an absolute
# index, or -1).
func indexFrom(s as string, sub as string, from as int) {
    def rel as int init strings.indexOf(strings.substring($s, $from, len($s)), $sub);
    if ($rel < 0) {
        return -1;
    }
    return $from + $rel;
}
