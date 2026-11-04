# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * A WebDAV client (RFC 4918): list, stat, read, write, create, move, copy and
 * delete files on any WebDAV server (Nextcloud, ownCloud, Apache `mod_dav`,
 * nginx, rclone, a NAS), plus `propfind` / `proppatch` for any property.
 *
 * The module has two layers. The **protocol layer** (`Resource`, paths and
 * hrefs, the `multistatus` codec, Clark notation for property names) knows
 * nothing about sockets, so a server built on `httpd` can share it: it encodes
 * the same `Resource` the client decodes. The **client** sends the requests
 * over `http` and keeps the session in a value-semantic `Client`. Needs the
 * default `jennifer` binary (`net` via `http`).
 *
 * @module webdav
 * @example
 * def c as webdav.Client init webdav.connect("https://cloud.example.com/dav/files/alice");
 * $c = webdav.withBasicAuth($c, "alice", $password);
 * for (def r in webdav.readDir($c, "/Documents")) { io.printf("%s %d\n", $r.path, $r.size); }
 * webdav.put($c, "/Documents/notes.txt", convert.bytesFromString("hello", "utf-8"));
 */
use convert;
use lists;
use maps;
use regex;
use strings;
use time;
use xml;
import "./http.j" as http;
import "./uri.j" as uri;

/** The `kind` of every error the module throws. */
export def const ERROR_KIND as string init "webdav";

/** The `DAV:` namespace of the WebDAV properties and elements. */
export def const DAV_NS as string init "DAV:";

/** The most entries `walk` collects before it refuses. */
export def const MAX_WALK as int init 100000;

# The idle timeout a Client starts with, and what `withTimeout(c, 0)` restores.
def const DEFAULT_TIMEOUT_MS as int init 30000;

# How `getlastmodified` is written (RFC 1123, always GMT).
def const HTTP_DATE as string init "%a, %d %b %Y %H:%M:%S GMT";

# How much of a caller's or server's string an error message repeats.
def const SHOWN as int init 80;

# Headers that frame the request, which `withHeader` refuses: a wrong one
# would make the server read the body differently from how it was sent.
def const FRAMING_HEADERS as list of string init [
    "host",
    "content-length",
    "transfer-encoding",
    "connection",
    "upgrade",
    "te",
    "trailer"
];

# The properties `readDir`, `stat` and `walk` ask for, in Clark notation.
def const BASIC_PROPS as list of string init [
    "\{DAV:\}resourcetype",
    "\{DAV:\}getcontentlength",
    "\{DAV:\}getlastmodified",
    "\{DAV:\}getetag",
    "\{DAV:\}getcontenttype",
    "\{DAV:\}quota-used-bytes",
    "\{DAV:\}quota-available-bytes"
];

/**
 * A file or collection and its properties, as a `PROPFIND` reports it (and as
 * a server answers one).
 *
 * @field href {string} the server path, percent-decoded, e.g.
 *   `/remote.php/dav/files/alice/a.txt`; a collection's ends in `/`
 * @field path {string} the path below the client's URL, e.g. `/a.txt`; "/" for
 *   the URL itself
 * @field name {string} the last segment; "" for the server root
 * @field isDir {bool} whether it is a collection
 * @field size {int} bytes (`getcontentlength`); 0 for a collection
 * @field modified {int} last change, Unix seconds (`getlastmodified`); 0 when
 *   unknown
 * @field etag {string} the entity tag as sent, quotes included, for `ifMatch`
 * @field contentType {string} the media type; "" when unknown
 * @field quotaUsed {int} bytes used (RFC 4331); -1 when not reported
 * @field quotaAvailable {int} bytes available (RFC 4331); -1 when not reported
 * @field props {map of string to string} every other property by its Clark
 *   name (`{http://owncloud.org/ns}fileid`): its text, or its XML when it holds
 *   elements
 */
export def struct Resource {
    href as string,
    path as string,
    name as string,
    isDir as bool,
    size as int,
    modified as int,
    etag as string,
    contentType as string,
    quotaUsed as int,
    quotaAvailable as int,
    props as map of string to string
};

/**
 * How `putWith` writes a file.
 *
 * @field contentType {string} the media type; "" for `application/octet-stream`
 * @field ifMatch {string} an entity tag the file must still have (from
 *   `Resource.etag`), so a change made meanwhile is not overwritten; "" for
 *   none. It must be strong: a weak `W/"..."` tag can never match
 * @field onlyIfNew {bool} refuse to replace an existing file
 */
export def struct PutOptions {
    contentType as string,
    ifMatch as string,
    onlyIfNew as bool
};

/**
 * A connection to a WebDAV server below a base URL. Value-semantic: the
 * `with*` functions return a changed copy.
 *
 * @field url {string} the base URL without a trailing slash, e.g.
 *   `https://cloud.example.com/remote.php/dav/files/alice`
 * @field basePath {string} its path, percent-decoded; "" for the server root
 * @field headers {map of string to string} sent with every request (the
 *   credential among them)
 * @field tls {http.TlsOptions} certificate checking
 * @field timeoutMs {int} the idle timeout of each request
 */
export def struct Client {
    url as string,
    basePath as string,
    headers as map of string to string,
    tls as http.TlsOptions,
    timeoutMs as int
};

func fail(message as string) {
    throw Error{kind: ERROR_KIND, message: "webdav: " + $message};
}

# plain cuts a string for an error message and replaces anything but printable
# ASCII, so a message stays one line and carries no terminal escape.
func plain(text as string, most as int) {
    def cut as string init $text;
    def more as string init "";
    if (len($text) > $most) {
        $cut = strings.substring($text, 0, $most);
        $more = "...";
    }
    return regex.replace("[^ -~]", $cut, "?") + $more;
}

func shown(text as string) {
    return "\"" + plain($text, SHOWN) + "\"";
}

# --- protocol layer: names, paths, hrefs, the multistatus codec ------------

/**
 * A property name in Clark notation, as `Resource.props`, `propfind` and
 * `proppatch` take it.
 *
 * @param ns {string} the namespace URI, e.g. `DAV:`
 * @param local {string} the local name, e.g. `getetag`
 * @return {string} e.g. `{DAV:}getetag`
 */
export func clark(ns as string, local as string) {
    return "\{" + $ns + "\}" + $local;
}

/**
 * Split a Clark name into its namespace and local name.
 *
 * @param name {string} e.g. `{DAV:}getetag`
 * @return {list of string} the namespace and the local name
 */
func clarkParts(name as string) {
    if (not regex.matches("^\{[^\{\}]+\}[A-Za-z_][A-Za-z0-9._-]*$", $name)) {
        fail("a property name is \{namespace\}local, got " + shown($name));
    }
    def close as int init strings.indexOf($name, "\}");
    return [strings.substring($name, 1, $close), strings.substring($name, $close + 1)];
}

/**
 * A path checked and normalized: a leading `/`, no empty, `.` or `..`
 * segment, no control character, no trailing `/`.
 *
 * @param path {string} e.g. `/Documents/a.txt`; "" or `/` for the root
 * @return {string} the path; `/` for the root
 */
export func cleanPath(path as string) {
    def p as string init $path;
    if ($p == "") {
        return "/";
    }
    if (not strings.startsWith($p, "/")) {
        fail("a path starts with /, got " + shown($path));
    }
    if (regex.matches("[\\x00-\\x1f\\x7f]", $p)) {
        fail("a path holds no control characters: " + shown($path));
    }
    while (len($p) > 1 and strings.endsWith($p, "/")) {
        $p = strings.substring($p, 0, len($p) - 1);
    }
    if ($p == "/") {
        return "/";
    }
    for (def segment in strings.split(strings.substring($p, 1), "/")) {
        if ($segment == "" or $segment == "." or $segment == "..") {
            fail("a path has no empty, . or .. segments, got " + shown($path));
        }
    }
    return $p;
}

/**
 * A decoded path as an href: each segment percent-encoded.
 *
 * @param path {string} a clean path
 * @param isDir {bool} add the trailing `/` of a collection
 * @return {string} the href, e.g. `/My%20Files/`
 */
export func encodeHref(path as string, isDir as bool) {
    if ($path == "/" or $path == "") {
        return "/";
    }
    def out as list of string init [];
    for (def segment in strings.split(strings.substring($path, 1), "/")) {
        $out[] = uri.encode($segment);
    }
    def href as string init "/" + strings.join($out, "/");
    if ($isDir) {
        $href = $href + "/";
    }
    return $href;
}

/**
 * The decoded server path of an href, which a server may send as a path or as
 * a full URL.
 *
 * @param href {string} e.g. `/a%20b/` or `https://host/a%20b/`
 * @return {string} the decoded path, e.g. `/a b/`
 */
export func decodeHref(href as string) {
    def h as string init strings.trim($href);
    def scheme as int init strings.indexOf($h, "://");
    if ($scheme >= 0) {
        def rest as string init strings.substring($h, $scheme + 3);
        def slash as int init strings.indexOf($rest, "/");
        if ($slash < 0) {
            return "/";
        }
        $h = strings.substring($rest, $slash);
    }
    if (not strings.startsWith($h, "/")) {
        fail("an href is a path or a URL, got " + shown($href));
    }
    try {
        return uri.decode($h);
    } catch (e) {
        fail("an href has a bad % escape: " + shown($href));
    }
    return "";
}

/**
 * The status code of a `DAV:status` line.
 *
 * @param line {string} e.g. `HTTP/1.1 404 Not Found`
 * @return {int} the code; 0 when the line has none
 */
func statusCode(line as string) {
    def text as string init strings.trim($line);
    def space as int init strings.indexOf($text, " ");
    if ($space < 0 or len($text) < $space + 4) {
        return 0;
    }
    return intOr(strings.substring($text, $space + 1, $space + 4), 0);
}

/**
 * An integer property, or `otherwise` when it is not a number.
 *
 * @param text {string} the text
 * @param otherwise {int} the fallback
 * @return {int} the number
 */
func intOr(text as string, otherwise as int) {
    try {
        return convert.toInt(strings.trim($text));
    } catch (e) {
        return $otherwise;
    }
    return $otherwise;
}

/**
 * An HTTP date as Unix seconds.
 *
 * @param text {string} e.g. `Wed, 04 Nov 2026 13:05:09 GMT`
 * @return {int} the seconds; 0 when it is not an RFC 1123 date
 */
func httpDate(text as string) {
    try {
        return time.unix(time.parse(strings.trim($text), HTTP_DATE));
    } catch (e) {
        return 0;
    }
    return 0;
}

/**
 * A resource with nothing known yet.
 *
 * @param href {string} its decoded server path
 * @return {Resource} the resource
 */
func emptyResource(href as string) {
    def trimmed as string init $href;
    while (len($trimmed) > 1 and strings.endsWith($trimmed, "/")) {
        $trimmed = strings.substring($trimmed, 0, len($trimmed) - 1);
    }
    def name as string init "";
    if ($trimmed != "/") {
        $name = strings.substring($trimmed, strings.lastIndexOf($trimmed, "/") + 1);
    }
    return Resource{
        href: $href,
        path: "",
        name: $name,
        isDir: strings.endsWith($href, "/"),
        size: 0,
        modified: 0,
        etag: "",
        contentType: "",
        quotaUsed: -1,
        quotaAvailable: -1,
        props: {}
    };
}

/**
 * Take one property element into a resource.
 *
 * @param r {Resource} the resource
 * @param p {xml.Value} the property element
 * @return {Resource} the resource with it
 */
func withProp(r as Resource, p as xml.Value) {
    def key as string init clark(xml.namespace($p), xml.localName($p));
    def text as string init strings.trim(xml.text($p));
    if ($key == "\{DAV:\}resourcetype") {
        $r.isDir = xml.has($p, "d:collection", {"d": DAV_NS});
    } elseif ($key == "\{DAV:\}getcontentlength") {
        $r.size = intOr($text, 0);
    } elseif ($key == "\{DAV:\}getlastmodified") {
        $r.modified = httpDate($text);
    } elseif ($key == "\{DAV:\}getetag") {
        $r.etag = $text;
    } elseif ($key == "\{DAV:\}getcontenttype") {
        $r.contentType = $text;
    } elseif ($key == "\{DAV:\}quota-used-bytes") {
        $r.quotaUsed = intOr($text, -1);
    } elseif ($key == "\{DAV:\}quota-available-bytes") {
        $r.quotaAvailable = intOr($text, -1);
    } elseif (len(xml.children($p)) > 0) {
        $r.props[$key] = xml.encode($p);
    } else {
        $r.props[$key] = $text;
    }
    return $r;
}

/**
 * Decode a `207 Multi-Status` body into its resources. Properties a server
 * reports with a non-2xx status are left out, and so are responses that are
 * a status only (`404` for a member that vanished).
 *
 * @param body {string} the XML
 * @return {list of Resource} the resources, in the order of the reply;
 *   `path` is "" (the client fills it in)
 * @throws {Error} kind `"webdav"` when the body is not a `DAV:multistatus`
 */
export func parseMultistatus(body as string) {
    def doc as xml.Value;
    try {
        $doc = xml.decode($body);
    } catch (e) {
        fail("the reply is not XML: " + plain($e.message, SHOWN));
    }
    if (xml.namespace($doc) != DAV_NS or xml.localName($doc) != "multistatus") {
        fail("the reply is not a DAV:multistatus but " + shown(xml.tag($doc)));
    }
    def ns as map of string to string init {"d": DAV_NS};
    def out as list of Resource init [];
    for (def response in xml.findAll($doc, "d:response", $ns)) {
        if (not xml.has($response, "d:href", $ns)) {
            continue;
        }
        def href as string init xml.text(xml.get($response, "d:href", $ns));
        def r as Resource init emptyResource(decodeHref($href));
        def any as bool init false;
        for (def ps in xml.findAll($response, "d:propstat", $ns)) {
            def code as int init 200;
            if (xml.has($ps, "d:status", $ns)) {
                $code = statusCode(xml.text(xml.get($ps, "d:status", $ns)));
            }
            if ($code < 200 or $code >= 300 or not xml.has($ps, "d:prop", $ns)) {
                continue;
            }
            $any = true;
            for (def p in xml.children(xml.get($ps, "d:prop", $ns))) {
                $r = withProp($r, $p);
            }
        }
        if ($any) {
            $out[] = $r;
        }
    }
    return $out;
}

/**
 * Prefixes for the namespaces of property names, so a document declares each
 * once: `DAV:` is `d`, the others `x0`, `x1`, ...
 *
 * @param names {list of string} Clark names
 * @return {map of string to string} prefix by namespace
 */
func prefixesFor(names as list of string) {
    def out as map of string to string init {"DAV:": "d"};
    for (def name in $names) {
        def ns as string init clarkParts($name)[0];
        if (not maps.has($out, $ns)) {
            $out[$ns] = "x" + convert.toString(len($out) - 1);
        }
    }
    return $out;
}

/**
 * An empty element for a Clark name.
 *
 * @param name {string} the Clark name
 * @param prefixes {map of string to string} prefix by namespace
 * @return {xml.Value} the element
 */
func propElement(name as string, prefixes as map of string to string) {
    def parts as list of string init clarkParts($name);
    return xml.elementNS($parts[0], $prefixes[$parts[0]] + ":" + $parts[1]);
}

/**
 * A `DAV:` element.
 *
 * @param local {string} the local name
 * @return {xml.Value} the element
 */
func dav(local as string) {
    return xml.elementNS(DAV_NS, "d:" + $local);
}

/**
 * A `DAV:` element holding text.
 *
 * @param local {string} the local name
 * @param text {string} the text
 * @return {xml.Value} the element
 */
func davText(local as string, text as string) {
    return xml.setText(dav($local), $text);
}

/**
 * Encode resources as a `207 Multi-Status` body: what a server answers a
 * `PROPFIND`, and the inverse of `parseMultistatus`.
 *
 * @param resources {list of Resource} the resources; `href` is their server
 *   path, decoded; each entry of `props` is written as text
 * @return {string} the XML, with its declaration
 */
export func multistatus(resources as list of Resource) {
    def extra as list of string init [];
    for (def r in $resources) {
        for (def key in maps.keys($r.props)) {
            $extra[] = $key;
        }
    }
    def prefixes as map of string to string init prefixesFor($extra);
    def root as xml.Value init dav("multistatus");
    for (def ns in lists.sort(maps.keys($prefixes))) {
        if ($ns != DAV_NS) {
            $root = xml.setAttr($root, "xmlns:" + $prefixes[$ns], $ns);
        }
    }
    for (def r in $resources) {
        def prop as xml.Value init dav("prop");
        def kind as xml.Value init dav("resourcetype");
        if ($r.isDir) {
            $kind = xml.append($kind, dav("collection"));
        } else {
            $prop = xml.append($prop, davText("getcontentlength", convert.toString($r.size)));
        }
        $prop = xml.append($prop, $kind);
        if ($r.modified > 0) {
            def stamp as string init time.format(time.fromUnix($r.modified), HTTP_DATE);
            $prop = xml.append($prop, davText("getlastmodified", $stamp));
        }
        if ($r.etag != "") {
            $prop = xml.append($prop, davText("getetag", $r.etag));
        }
        if ($r.contentType != "") {
            $prop = xml.append($prop, davText("getcontenttype", $r.contentType));
        }
        if ($r.quotaUsed >= 0) {
            $prop = xml.append($prop, davText("quota-used-bytes", convert.toString($r.quotaUsed)));
        }
        if ($r.quotaAvailable >= 0) {
            def free as string init convert.toString($r.quotaAvailable);
            $prop = xml.append($prop, davText("quota-available-bytes", $free));
        }
        for (def key in lists.sort(maps.keys($r.props))) {
            $prop = xml.append($prop, xml.setText(propElement($key, $prefixes), $r.props[$key]));
        }
        def ps as xml.Value init xml.append(dav("propstat"), $prop);
        $ps = xml.append($ps, davText("status", "HTTP/1.1 200 OK"));
        def path as string init $r.href;
        def isDir as bool init $r.isDir or strings.endsWith($path, "/");
        def response as xml.Value init xml.append(
            dav("response"),
            davText("href", encodeHref(cleanPath($path), $isDir)));
        $root = xml.append($root, xml.append($response, $ps));
    }
    return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + xml.encode($root);
}

/**
 * The body of a `PROPFIND`: the named properties, or `allprop` for none.
 *
 * @param props {list of string} Clark names
 * @return {string} the XML
 */
func propfindBody(props as list of string) {
    def root as xml.Value init dav("propfind");
    if (len($props) == 0) {
        $root = xml.append($root, dav("allprop"));
    } else {
        def prefixes as map of string to string init prefixesFor($props);
        def prop as xml.Value init dav("prop");
        for (def name in $props) {
            $prop = xml.append($prop, propElement($name, $prefixes));
        }
        $root = xml.append($root, $prop);
    }
    return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + xml.encode($root);
}

/**
 * The body of a `PROPPATCH`.
 *
 * @param set {map of string to string} properties to set, by Clark name
 * @param remove {list of string} properties to remove, by Clark name
 * @return {string} the XML
 */
func proppatchBody(set as map of string to string, remove as list of string) {
    def names as list of string init lists.concat(maps.keys($set), $remove);
    def prefixes as map of string to string init prefixesFor($names);
    def root as xml.Value init dav("propertyupdate");
    if (len($set) > 0) {
        def prop as xml.Value init dav("prop");
        for (def name in lists.sort(maps.keys($set))) {
            $prop = xml.append($prop, xml.setText(propElement($name, $prefixes), $set[$name]));
        }
        $root = xml.append($root, xml.append(dav("set"), $prop));
    }
    if (len($remove) > 0) {
        def prop as xml.Value init dav("prop");
        for (def name in $remove) {
            $prop = xml.append($prop, propElement($name, $prefixes));
        }
        $root = xml.append($root, xml.append(dav("remove"), $prop));
    }
    return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + xml.encode($root);
}

# --- client --------------------------------------------------------------

/**
 * A client for a WebDAV server below a URL, with no credential yet.
 *
 * @param url {string} `http://` or `https://`, e.g.
 *   `https://cloud.example.com/remote.php/dav/files/alice`
 * @return {Client} the client; 30 s idle timeout, full certificate checking
 * @throws {Error} kind `"webdav"` on a URL that is not http(s) or carries
 *   credentials (use `withBasicAuth`), a query or a fragment
 */
export func connect(url as string) {
    def u as string init strings.trim($url);
    if (not regex.matches("^https?://[^/\\s?#@]+(/[^\\s?#]*)?$", $u)) {
        fail("a WebDAV URL is http(s)://host[/path] without credentials, query or fragment, got " +
            shown($url));
    }
    while (strings.endsWith($u, "/")) {
        $u = strings.substring($u, 0, len($u) - 1);
    }
    def rest as string init strings.substring($u, strings.indexOf($u, "://") + 3);
    def base as string init "";
    if (strings.contains($rest, "/")) {
        $base = uri.decode(strings.substring($rest, strings.indexOf($rest, "/")));
    }
    def c as Client;
    $c.url = $u;
    $c.basePath = $base;
    $c.headers = {};
    $c.timeoutMs = DEFAULT_TIMEOUT_MS;
    return $c;
}

/**
 * Send a header with every request, such as `OCS-APIRequest: true`.
 *
 * @param c {Client} the client
 * @param name {string} the header name
 * @param value {string} its value
 * @return {Client} the changed copy
 * @throws {Error} kind `"webdav"` on a bad name, a value with a line break,
 *   or a header that frames the request (`Host`, `Content-Length`, ...)
 */
export func withHeader(c as Client, name as string, value as string) {
    def broken as bool init regex.matches("[\\x00-\\x1f\\x7f]", $value);
    if (not regex.matches("^[A-Za-z0-9-]+$", $name) or $broken) {
        fail("a header is a token name and a one-line value, got " + shown($name));
    }
    if (lists.contains(FRAMING_HEADERS, strings.lower($name))) {
        fail("the client sets " + $name + " itself");
    }
    $c.headers[$name] = $value;
    return $c;
}

/**
 * Authenticate with a user name and password (HTTP Basic). Over `http://`
 * they travel readable; use `https://`.
 *
 * @param c {Client} the client
 * @param user {string} the user name, without `:`
 * @param password {string} the password; an app password where the server
 *   offers them
 * @return {Client} the changed copy
 * @throws {Error} kind `"webdav"` on a `:` in the name or a control character
 */
export func withBasicAuth(c as Client, user as string, password as string) {
    if (strings.contains($user, ":") or regex.matches("[\\x00-\\x1f\\x7f]", $user + $password)) {
        fail("a Basic user name holds no :, and neither holds control characters");
    }
    $c.headers["Authorization"] = http.basic($user, $password);
    return $c;
}

/**
 * Authenticate with a bearer token.
 *
 * @param c {Client} the client
 * @param token {string} the token
 * @return {Client} the changed copy
 * @throws {Error} kind `"webdav"` on a space or control character in it
 */
export func withToken(c as Client, token as string) {
    if ($token == "" or regex.matches("[\\s\\x00-\\x1f\\x7f]", $token)) {
        fail("a token is not empty and holds no spaces or control characters");
    }
    $c.headers["Authorization"] = "Bearer " + $token;
    return $c;
}

/**
 * Trust a certificate (PEM) in addition to the system's.
 *
 * @param c {Client} the client
 * @param pem {bytes} the certificate
 * @return {Client} the changed copy
 */
export func withCaCert(c as Client, pem as bytes) {
    $c.tls.caCert = $pem;
    $c.tls.skipVerify = false;
    return $c;
}

/**
 * Accept any certificate. The connection stays encrypted but is no longer
 * authenticated; prefer `withCaCert`.
 *
 * @param c {Client} the client
 * @return {Client} the changed copy
 */
export func withInsecure(c as Client) {
    $c.tls.skipVerify = true;
    return $c;
}

/**
 * Set the idle timeout of each request.
 *
 * @param c {Client} the client
 * @param ms {int} milliseconds; 0 for the default, 30 s
 * @return {Client} the changed copy
 */
export func withTimeout(c as Client, ms as int) {
    $c.timeoutMs = $ms;
    if ($ms <= 0) {
        $c.timeoutMs = DEFAULT_TIMEOUT_MS;
    }
    return $c;
}

/**
 * The URL of a path below the client's.
 *
 * @param c {Client} the client
 * @param path {string} a clean path
 * @param isDir {bool} add a trailing `/`
 * @return {string} the URL
 */
func urlOf(c as Client, path as string, isDir as bool) {
    if ($path == "/") {
        return $c.url + "/";
    }
    return $c.url + encodeHref($path, $isDir);
}

/**
 * Send a request below the client's URL. A redirect to the same URL with a
 * trailing `/` (how Apache answers a collection named without one) is
 * followed once; any other redirect is returned.
 *
 * @param c {Client} the client
 * @param method {string} the method
 * @param path {string} a clean path
 * @param isDir {bool} address it as a collection
 * @param extra {map of string to string} headers for this request
 * @param body {string} the body; "" for none
 * @return {http.Response} the response
 */
func exchange(
    c as Client,
    method as string,
    path as string,
    isDir as bool,
    extra as map of string to string,
    body as string) {
    def headers as map of string to string init $c.headers;
    for (def key in maps.keys($extra)) {
        $headers[$key] = $extra[$key];
    }
    if ($body != "") {
        $headers["Content-Type"] = "application/xml; charset=utf-8";
    }
    def o as http.Options;
    $o.tls = $c.tls;
    $o.timeoutMs = $c.timeoutMs;
    def url as string init urlOf($c, $path, $isDir);
    def r as http.Response;
    try {
        $r = http.send($method, $url, $headers, $body, $o);
        if (lists.contains([301, 302, 307, 308], $r.status) and not $isDir) {
            def location as string init strings.trim(http.header($r, "Location"));
            if (sameTarget($location, $url + "/")) {
                $r = http.send($method, $url + "/", $headers, $body, $o);
            }
        }
    } catch (e) {
        if ($e.kind == ERROR_KIND) {
            throw $e;
        }
        fail($method + " " + $path + ": " + $e.message);
    }
    return $r;
}

/**
 * Whether a redirect's `Location` names the given URL, as a full URL or as a
 * path. An unreadable `Location` names nothing.
 *
 * @param location {string} the header
 * @param url {string} the URL
 * @return {bool} whether they match
 */
func sameTarget(location as string, url as string) {
    if ($location == $url) {
        return true;
    }
    try {
        return $location != "" and decodeHref($location) == decodeHref($url);
    } catch (e) {
        return false;
    }
    return false;
}

/**
 * Refuse a response whose status is not one of `wanted`.
 *
 * @param r {http.Response} the response
 * @param method {string} the method, for the message
 * @param path {string} the path, for the message
 * @param wanted {list of int} the statuses that mean success
 */
func expect(r as http.Response, method as string, path as string, wanted as list of int) {
    if (not lists.contains($wanted, $r.status)) {
        fail($method + " " + $path + ": " + statusText($r.status, $r.statusText));
    }
}

/**
 * A refused status for an error message.
 *
 * @param status {int} the status
 * @param reason {string} the server's reason phrase
 * @return {string} e.g. `HTTP 404 Not Found`
 */
func statusText(status as int, reason as string) {
    return strings.trim("HTTP " + convert.toString($status) + " " +
        plain(strings.trim($reason), SHOWN));
}

/**
 * The `path` of a resource below the client's URL.
 *
 * @param c {Client} the client
 * @param href {string} the decoded server path
 * @return {string} the path; the href itself when it is not below the URL
 */
func relative(c as Client, href as string) {
    def p as string init $href;
    if ($c.basePath != "" and ($p == $c.basePath or strings.startsWith($p, $c.basePath + "/"))) {
        $p = strings.substring($p, len($c.basePath));
    }
    while (len($p) > 1 and strings.endsWith($p, "/")) {
        $p = strings.substring($p, 0, len($p) - 1);
    }
    if ($p == "") {
        return "/";
    }
    return $p;
}

/**
 * `PROPFIND` a path at a depth.
 *
 * @param c {Client} the client
 * @param path {string} a clean path
 * @param depth {int} 0 or 1
 * @param props {list of string} Clark names; `[]` for all
 * @param isDir {bool} address it as a collection
 * @return {list of Resource} the resources, `path` filled in
 */
func find(c as Client, path as string, depth as int, props as list of string, isDir as bool) {
    def r as http.Response init exchange(
        $c,
        "PROPFIND",
        $path,
        $isDir,
        {"Depth": convert.toString($depth)},
        propfindBody($props));
    expect($r, "PROPFIND", $path, [207]);
    def out as list of Resource init [];
    for (def one in parseMultistatus($r.body)) {
        $one.path = relative($c, $one.href);
        $out[] = $one;
    }
    return $out;
}

/**
 * Ask for properties of a path, and of its members with depth 1.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents`
 * @param depth {int} 0 for the path, 1 for it and its members
 * @param props {list of string} Clark names, e.g.
 *   `{http://owncloud.org/ns}fileid`; `[]` for all the server offers
 * @return {list of Resource} the path first, then its members
 * @throws {Error} kind `"webdav"` on a bad path, depth or name, or when the
 *   server refuses
 */
export func propfind(c as Client, path as string, depth as int, props as list of string) {
    if ($depth != 0 and $depth != 1) {
        fail("a depth is 0 or 1; walk goes deeper");
    }
    for (def name in $props) {
        clarkParts($name);
    }
    return find($c, cleanPath($path), $depth, $props, false);
}

/**
 * One file or collection.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`
 * @return {Resource} it
 * @throws {Error} kind `"webdav"` when it does not exist (HTTP 404), or when
 *   the server refuses
 */
export func stat(c as Client, path as string) {
    def p as string init cleanPath($path);
    def found as list of Resource init find($c, $p, 0, BASIC_PROPS, false);
    if (len($found) == 0) {
        fail("PROPFIND " + $p + ": the reply names no resource");
    }
    return $found[0];
}

/**
 * Whether a path exists.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`
 * @return {bool} whether it does
 * @throws {Error} kind `"webdav"` on a bad path or any answer but 207 and 404
 */
export func exists(c as Client, path as string) {
    def p as string init cleanPath($path);
    def body as string init propfindBody(["\{DAV:\}resourcetype"]);
    def r as http.Response init exchange($c, "PROPFIND", $p, false, {"Depth": "0"}, $body);
    if ($r.status == 404) {
        return false;
    }
    expect($r, "PROPFIND", $p, [207]);
    return true;
}

/**
 * The members of a collection, without the collection itself.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents`; `/` for the client's URL
 * @return {list of Resource} the members, in the server's order
 * @throws {Error} kind `"webdav"` when it is a file, does not exist, or the
 *   server refuses
 */
export func readDir(c as Client, path as string) {
    def p as string init cleanPath($path);
    def found as list of Resource init find($c, $p, 1, BASIC_PROPS, true);
    def out as list of Resource init [];
    for (def r in $found) {
        if ($r.path != $p) {
            $out[] = $r;
        } elseif (not $r.isDir) {
            fail("PROPFIND " + $p + ": not a collection");
        }
    }
    return $out;
}

/**
 * Everything below a collection, collections before their members. It asks
 * with depth 1 per collection, since many servers refuse `Depth: infinity`.
 *
 * @param c {Client} the client
 * @param path {string} the collection
 * @return {list of Resource} the files and collections below it
 * @throws {Error} kind `"webdav"` past `MAX_WALK` entries, or as `readDir`
 */
export func walk(c as Client, path as string) {
    def out as list of Resource init [];
    def pending as list of string init [cleanPath($path)];
    def next as int init 0;
    while ($next < len($pending)) {
        def dir as string init $pending[$next];
        $next = $next + 1;
        for (def r in readDir($c, $dir)) {
            $out[] = $r;
            if (len($out) > MAX_WALK) {
                fail("walk " + cleanPath($path) + ": more than " + convert.toString(MAX_WALK) +
                    " entries");
            }
            if ($r.isDir) {
                $pending[] = $r.path;
            }
        }
    }
    return $out;
}

/**
 * Read a file.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`
 * @param maxBytes {int} the largest file accepted; 0 for 64 MiB, negative
 *   for no limit
 * @return {bytes} its content
 * @throws {Error} kind `"webdav"` when it does not exist, is larger than
 *   `maxBytes`, or the server refuses
 */
export func get(c as Client, path as string, maxBytes as int) {
    def p as string init cleanPath($path);
    def headers as map of string to string init $c.headers;
    $headers["Accept"] = "*/*";
    def r as http.BytesResponse;
    try {
        $r = http.requestWithBytes(
            "GET",
            urlOf($c, $p, false),
            $headers,
            "",
            $c.timeoutMs,
            $maxBytes,
            $c.tls);
    } catch (e) {
        fail("GET " + $p + ": " + $e.message);
    }
    if ($r.status != 200) {
        fail("GET " + $p + ": " + statusText($r.status, $r.statusText));
    }
    return $r.body;
}

/**
 * Write a file, replacing it when it exists.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`; its collection must exist
 * @param data {bytes} the content
 * @return {string} the new entity tag when the server sends one; ""
 *   otherwise
 * @throws {Error} kind `"webdav"` when the collection does not exist (HTTP
 *   409), or the server refuses
 */
export func put(c as Client, path as string, data as bytes) {
    return putWith($c, $path, $data, PutOptions{contentType: "", ifMatch: "", onlyIfNew: false});
}

/**
 * Write a file with a media type or a condition.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`
 * @param data {bytes} the content
 * @param options {PutOptions} the media type, and `ifMatch` / `onlyIfNew`
 * @return {string} the new entity tag when the server sends one; ""
 *   otherwise
 * @throws {Error} kind `"webdav"` when a condition fails (HTTP 412), on a
 *   weak `ifMatch` tag, or as `put`
 */
export func putWith(c as Client, path as string, data as bytes, options as PutOptions) {
    def p as string init cleanPath($path);
    if ($p == "/") {
        fail("PUT /: the root is a collection");
    }
    def headers as map of string to string init $c.headers;
    $headers["Content-Type"] = "application/octet-stream";
    if ($options.contentType != "") {
        $headers["Content-Type"] = $options.contentType;
    }
    if ($options.ifMatch != "") {
        # If-Match compares strongly (RFC 7232), so a weak tag never matches.
        if (strings.startsWith($options.ifMatch, "W/")) {
            fail("PUT " + $p + ": ifMatch needs a strong ETag, got the weak " +
                shown($options.ifMatch) + "; stat the file again later");
        }
        $headers["If-Match"] = $options.ifMatch;
    }
    if ($options.onlyIfNew) {
        $headers["If-None-Match"] = "*";
    }
    def r as http.Response;
    try {
        $r = http.requestRawBodyTls(
            "PUT",
            urlOf($c, $p, false),
            $headers,
            $data,
            $c.timeoutMs,
            0,
            $c.tls);
    } catch (e) {
        fail("PUT " + $p + ": " + $e.message);
    }
    expect($r, "PUT", $p, [200, 201, 204]);
    return strings.trim(http.header($r, "ETag"));
}

/**
 * Create a collection. Its parent must exist.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/2026`
 * @throws {Error} kind `"webdav"` when it exists (HTTP 405), the parent does
 *   not (HTTP 409), or the server refuses
 */
export func mkdir(c as Client, path as string) {
    def p as string init cleanPath($path);
    expect(exchange($c, "MKCOL", $p, false, {}, ""), "MKCOL", $p, [201]);
}

/**
 * Create a collection and any missing parents. An existing one is no error.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Backups/2026/11`
 * @throws {Error} kind `"webdav"` when a part of the path is a file, or the
 *   server refuses
 */
export func mkdirAll(c as Client, path as string) {
    def p as string init cleanPath($path);
    if ($p == "/") {
        return;
    }
    def at as string init "";
    for (def segment in strings.split(strings.substring($p, 1), "/")) {
        $at = $at + "/" + $segment;
        def r as http.Response init exchange($c, "MKCOL", $at, false, {}, "");
        if ($r.status == 405) {
            if (not stat($c, $at).isDir) {
                fail("MKCOL " + $at + ": a file is in the way");
            }
        } else {
            expect($r, "MKCOL", $at, [201]);
        }
    }
}

/**
 * Delete a file, or a collection with everything in it.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/old`
 * @throws {Error} kind `"webdav"` on the root, when it does not exist (HTTP
 *   404), or when the server cannot delete all of it (HTTP 207, naming the
 *   first member it kept)
 */
export func delete(c as Client, path as string) {
    def p as string init cleanPath($path);
    if ($p == "/") {
        fail("DELETE /: refusing to delete the client's root");
    }
    def r as http.Response init exchange($c, "DELETE", $p, false, {}, "");
    if ($r.status == 207) {
        fail("DELETE " + $p + ": the server kept part of it" + firstFailure($r.body));
    }
    expect($r, "DELETE", $p, [200, 204]);
}

/**
 * The first member a `207` reply reports as failed, for an error message.
 *
 * @param body {string} the multistatus XML
 * @return {string} `, e.g. "/a" (HTTP 423)`, or "" when it names none
 */
func firstFailure(body as string) {
    try {
        def doc as xml.Value init xml.decode($body);
        def ns as map of string to string init {"d": DAV_NS};
        for (def response in xml.findAll($doc, "d:response", $ns)) {
            if (xml.has($response, "d:status", $ns) and xml.has($response, "d:href", $ns)) {
                def code as int init statusCode(xml.text(xml.get($response, "d:status", $ns)));
                def href as string init decodeHref(xml.text(xml.get($response, "d:href", $ns)));
                return ", e.g. " + shown($href) + " (HTTP " + convert.toString($code) + ")";
            }
        }
    } catch (e) {
        return "";
    }
    return "";
}

/**
 * `MOVE` or `COPY` a path to another below the same URL.
 *
 * @param c {Client} the client
 * @param method {string} `MOVE` or `COPY`
 * @param source {string} the source
 * @param target {string} the destination
 * @param overwrite {bool} replace an existing destination
 */
func transfer(
    c as Client,
    method as string,
    source as string,
    target as string,
    overwrite as bool) {
    def from as string init cleanPath($source);
    def dest as string init cleanPath($target);
    if ($from == "/" or $dest == "/") {
        fail($method + " " + $from + ": the client's root cannot be moved or copied");
    }
    def flag as string init "F";
    if ($overwrite) {
        $flag = "T";
    }
    def extra as map of string to string init {
        "Destination": urlOf($c, $dest, false),
        "Overwrite": $flag
    };
    if ($method == "COPY") {
        $extra["Depth"] = "infinity";
    }
    def r as http.Response init exchange($c, $method, $from, false, $extra, "");
    if ($r.status == 207) {
        fail($method + " " + $from + ": the server did not finish" + firstFailure($r.body));
    }
    expect($r, $method, $from, [201, 204]);
}

/**
 * Move or rename a file or collection.
 *
 * @param c {Client} the client
 * @param source {string} e.g. `/Inbox/scan.pdf`
 * @param target {string} e.g. `/Documents/scan.pdf`
 * @param overwrite {bool} replace an existing destination
 * @throws {Error} kind `"webdav"` when the destination exists and `overwrite`
 *   is false (HTTP 412), its parent does not (HTTP 409), or the server
 *   refuses
 */
export func move(c as Client, source as string, target as string, overwrite as bool) {
    transfer($c, "MOVE", $source, $target, $overwrite);
}

/**
 * Copy a file, or a collection with everything in it.
 *
 * @param c {Client} the client
 * @param source {string} e.g. `/Templates/report.odt`
 * @param target {string} e.g. `/Documents/report.odt`
 * @param overwrite {bool} replace an existing destination
 * @throws {Error} kind `"webdav"` as `move`
 */
export func copy(c as Client, source as string, target as string, overwrite as bool) {
    transfer($c, "COPY", $source, $target, $overwrite);
}

/**
 * Set or remove properties of a path, all or none.
 *
 * @param c {Client} the client
 * @param path {string} e.g. `/Documents/a.txt`
 * @param set {map of string to string} text properties to set, by Clark
 *   name, e.g. `{http://owncloud.org/ns}favorite: "1"`
 * @param remove {list of string} properties to remove, by Clark name
 * @throws {Error} kind `"webdav"` on a bad name, or when the server refuses
 *   a property (naming it)
 */
export func proppatch(
    c as Client,
    path as string,
    set as map of string to string,
    remove as list of string) {
    def p as string init cleanPath($path);
    if (len($set) == 0 and len($remove) == 0) {
        return;
    }
    def r as http.Response init exchange(
        $c,
        "PROPPATCH",
        $p,
        false,
        {},
        proppatchBody($set, $remove));
    expect($r, "PROPPATCH", $p, [200, 207]);
    if ($r.status == 200) {
        return;
    }
    def doc as xml.Value;
    try {
        $doc = xml.decode($r.body);
    } catch (e) {
        fail("PROPPATCH " + $p + ": the reply is not XML");
    }
    def ns as map of string to string init {"d": DAV_NS};
    for (def ps in xml.findAll($doc, "d:response/d:propstat", $ns)) {
        def code as int init 200;
        if (xml.has($ps, "d:status", $ns)) {
            $code = statusCode(xml.text(xml.get($ps, "d:status", $ns)));
        }
        if ($code < 200 or $code >= 300) {
            def names as list of string init [];
            for (def one in xml.children(xml.get($ps, "d:prop", $ns))) {
                $names[] = clark(xml.namespace($one), xml.localName($one));
            }
            fail("PROPPATCH " + $p + ": HTTP " + convert.toString($code) + " for " +
                plain(strings.join($names, ", "), SHOWN));
        }
    }
}
