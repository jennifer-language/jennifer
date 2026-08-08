#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The uri module: parse a URL into its parts and rebuild it, the two web
 * encodings (RFC 3986 percent vs application/x-www-form-urlencoded), query-string
 * build / parse, and RFC 3986 relative-reference resolution. Pure Jennifer over
 * strings + encoding, so it runs on either binary.
 * @module uri_demo
 */
use io;
import "../../modules/uri.j" as uri;

# --- parse / build round-trip ---
def raw as string init "https://user@example.com:8443/a/b?x=1&y=2#top";
def u as uri.Uri init uri.parse($raw);
io.printf("scheme=%s user=%s host=%s port=%s\n", $u.scheme, $u.user, $u.host, $u.port);
io.printf("path=%s query=%s fragment=%s\n", $u.path, $u.query, $u.fragment);
io.printf("rebuilt -> %s\n", uri.build($u));

# An IPv6 literal keeps its brackets in host, with the port split out.
def v6 as uri.Uri init uri.parse("http://[::1]:9000/x");
io.printf("ipv6 host=%s port=%s\n", $v6.host, $v6.port);

# --- percent vs form encoding ---
# encode/decode are RFC 3986 (space -> %20); encodeForm/decodeForm are
# form-urlencoded (space -> +).
io.printf("encode(\"a b/c\")     -> %s\n", uri.encode("a b/c"));
io.printf("encodeForm(\"a b/c\") -> %s\n", uri.encodeForm("a b/c"));
io.printf("decodeForm(\"a+b\")   -> %s\n", uri.decodeForm("a+b"));

# --- query strings (form-encoded, insertion order) ---
def params as map of string to string init {};
$params["q"] = "jennifer lang";
$params["page"] = "2";
def qs as string init uri.buildQuery($params);
io.printf("buildQuery -> %s\n", $qs);
def back as map of string to string init uri.parseQuery($qs);
io.printf("parseQuery q=%s page=%s\n", $back["q"], $back["page"]);

# --- relative-reference resolution (RFC 3986 section 5) ---
def base as string init "http://h/a/b/page.html";
io.printf("resolve ../img.png   -> %s\n", uri.resolve($base, "../img.png"));
io.printf("resolve /root/x      -> %s\n", uri.resolve($base, "/root/x"));
io.printf("resolve //other/z    -> %s\n", uri.resolve($base, "//other/z"));
io.printf("resolve https://x/y  -> %s\n", uri.resolve($base, "https://x/y"));
