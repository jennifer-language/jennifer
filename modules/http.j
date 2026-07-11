# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# http.j - an HTTP/1.1 client over the `net` system library. Build a request
# (method, URL, headers, body), send it, and read the response back into a
# `Response` (status, headers, body). `http://` connects in the clear;
# `https://` connects with TLS (`net.connectTLS`). One request per connection
# (the client sends `Connection: close`). Because it uses `net`, this module
# needs the default `jennifer` binary.
#
#     import "http.j" as http;
#
#     def r as http.Response init http.get("http://example.com/", {});
#     io.printf("status %d\n%s\n", $r.status, $r.body);
#
#     def sent as http.Response init http.post("https://api.example.com/items",
#         "application/json", "{\"name\":\"ada\"}", {"Authorization": "Bearer xyz"});
#
# The response body is decoded as text: a JSON / HTML / XML body (valid UTF-8)
# round-trips exactly; a binary body (an image) is not decodable to a string and
# raises an error - a `bytes` body accessor is a planned follow-on. Chunked and
# Content-Length framing are both handled; redirects are returned (3xx), not
# followed automatically.
use net;
use strings;
use convert;
use maps;

# A parsed request URL.
def struct Url {
    scheme as string,
    host as string,
    port as int,
    path as string
};

# An HTTP response. `headers` keys are lowercased (HTTP header names are
# case-insensitive); use `http.header` for a case-insensitive read.
export def struct Response {
    status as int,
    statusText as string,
    headers as map of string to string,
    body as string
};

# --- byte helpers (private) ----------------------------------------

# sliceBytes copies buf[start:end] into a new bytes value.
func sliceBytes(buf as bytes, start as int, end as int) {
    def out as bytes;
    def i as int init $start;
    while ($i < $end) {
        $out[] = $buf[$i];
        $i = $i + 1;
    }
    return $out;
}

# bytesToStr decodes buf[start:end] as UTF-8 text.
func bytesToStr(buf as bytes, start as int, end as int) {
    return convert.stringFromBytes(sliceBytes($buf, $start, $end), "utf-8");
}

# findCRLF returns the index of the next CRLF at or after `start`, or -1.
func findCRLF(buf as bytes, start as int) {
    def i as int init $start;
    def n as int init len($buf) - 1;
    while ($i < $n) {
        if ($buf[$i] == 13 and $buf[$i + 1] == 10) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# headerEnd returns the index of the CRLFCRLF separating headers from body, -1
# if not present.
func headerEnd(buf as bytes) {
    def n as int init len($buf) - 3;
    def i as int init 0;
    while ($i < $n) {
        if ($buf[$i] == 13 and $buf[$i + 1] == 10) {
            if ($buf[$i + 2] == 13 and $buf[$i + 3] == 10) {
                return $i;
            }
        }
        $i = $i + 1;
    }
    return -1;
}

# hexToInt parses a hex string (a chunk-size line), stopping at the first
# non-hex character (a chunk extension after ";").
func hexToInt(s as string) {
    def result as int init 0;
    for (def c in strings.chars(strings.lower($s))) {
        def d as int init strings.indexOf("0123456789abcdef", $c);
        if ($d < 0) {
            return $result;
        }
        $result = $result * 16 + $d;
    }
    return $result;
}

# dechunk decodes a chunked transfer-encoded body (all bytes already read).
func dechunk(body as bytes) {
    def out as bytes;
    def pos as int init 0;
    def n as int init len($body);
    while ($pos < $n) {
        def lineEnd as int init findCRLF($body, $pos);
        if ($lineEnd < 0) {
            return $out;
        }
        def size as int init hexToInt(strings.trim(bytesToStr($body, $pos, $lineEnd)));
        if ($size == 0) {
            return $out;
        }
        def dataStart as int init $lineEnd + 2;
        def dataEnd as int init $dataStart + $size;
        if ($dataEnd > $n) {
            $dataEnd = $n;
        }
        def j as int init $dataStart;
        while ($j < $dataEnd) {
            $out[] = $body[$j];
            $j = $j + 1;
        }
        $pos = $dataEnd + 2;
    }
    return $out;
}

# --- request / response (private) ----------------------------------

# parseUrl splits a URL into scheme / host / port / path (with query). Defaults:
# scheme "http", port 80 (443 for https), path "/".
func parseUrl(url as string) {
    def scheme as string init "http";
    def rest as string init $url;
    def si as int init strings.indexOf($url, "://");
    if ($si >= 0) {
        $scheme = strings.lower(strings.substring($url, 0, $si));
        $rest = strings.substring($url, $si + 3, len($url));
    }
    def authority as string init $rest;
    def path as string init "/";
    def slash as int init strings.indexOf($rest, "/");
    if ($slash >= 0) {
        $authority = strings.substring($rest, 0, $slash);
        $path = strings.substring($rest, $slash, len($rest));
    }
    def host as string init $authority;
    def port as int init 80;
    if ($scheme == "https") {
        $port = 443;
    }
    def colon as int init strings.indexOf($authority, ":");
    if ($colon >= 0) {
        $host = strings.substring($authority, 0, $colon);
        $port = convert.toInt(strings.substring($authority, $colon + 1, len($authority)));
    }
    return Url{scheme: $scheme, host: $host, port: $port, path: $path};
}

# hostHeader renders the Host header value (host, plus port when non-default).
func hostHeader(u as Url) {
    if ($u.scheme == "http" and $u.port == 80) {
        return $u.host;
    }
    if ($u.scheme == "https" and $u.port == 443) {
        return $u.host;
    }
    return $u.host + ":" + convert.toString($u.port);
}

# buildRequest renders the full request text for a method / URL / headers / body.
func buildRequest(method as string, u as Url, headers as map of string to string, body as string) {
    def out as string init $method + " " + $u.path + " HTTP/1.1\r\n";
    $out = $out + "Host: " + hostHeader($u) + "\r\n";
    $out = $out + "Connection: close\r\n";
    if (not maps.has($headers, "User-Agent")) {
        $out = $out + "User-Agent: jennifer-http\r\n";
    }
    for (def k in $headers) {
        $out = $out + $k + ": " + $headers[$k] + "\r\n";
    }
    if (len($body) > 0) {
        def blen as int init len(convert.bytesFromString($body, "utf-8"));
        $out = $out + "Content-Length: " + convert.toString($blen) + "\r\n";
    }
    return $out + "\r\n" + $body;
}

# parseHeaders parses the header lines (after the status line) into a lowercased
# map.
func parseHeaders(lines as list of string) {
    def headers as map of string to string init {};
    def i as int init 1;
    while ($i < len($lines)) {
        def line as string init $lines[$i];
        def colon as int init strings.indexOf($line, ":");
        if ($colon > 0) {
            def raw as string init strings.substring($line, 0, $colon);
            def name as string init strings.lower(strings.trim($raw));
            $headers[$name] = strings.trim(strings.substring($line, $colon + 1, len($line)));
        }
        $i = $i + 1;
    }
    return $headers;
}

# parseResponse parses a complete raw response (headers + body bytes) into a
# Response, de-chunking or trimming the body to its framed length.
func parseResponse(raw as bytes) {
    def hend as int init headerEnd($raw);
    if ($hend < 0) {
        throw Error{kind: "http", message: "malformed response (no header terminator)",
            file: "", line: 0, col: 0};
    }
    def headerText as string init bytesToStr($raw, 0, $hend);
    def lines as list of string init strings.split($headerText, "\r\n");
    def statusLine as string init $lines[0];
    def protoEnd as int init strings.indexOf($statusLine, " ");
    def afterProto as string init strings.substring($statusLine, $protoEnd + 1, len($statusLine));
    def codeEnd as int init strings.indexOf($afterProto, " ");
    def status as int init convert.toInt(strings.substring($afterProto, 0, $codeEnd));
    def statusText as string init strings.substring($afterProto, $codeEnd + 1, len($afterProto));
    def headers as map of string to string init parseHeaders($lines);
    def bodyBytes as bytes init sliceBytes($raw, $hend + 4, len($raw));
    if (isChunked($headers)) {
        $bodyBytes = dechunk($bodyBytes);
    } elseif (maps.has($headers, "content-length")) {
        def cl as int init convert.toInt($headers["content-length"]);
        if (len($bodyBytes) > $cl) {
            $bodyBytes = sliceBytes($bodyBytes, 0, $cl);
        }
    }
    return Response{status: $status, statusText: $statusText, headers: $headers,
        body: convert.stringFromBytes($bodyBytes, "utf-8")};
}

# isChunked reports whether the response uses chunked transfer-encoding.
func isChunked(headers as map of string to string) {
    if (not maps.has($headers, "transfer-encoding")) {
        return false;
    }
    return strings.contains(strings.lower($headers["transfer-encoding"]), "chunked");
}

# --- net (private) -------------------------------------------------

# readToEOF reads the whole connection (the server closes after the response
# because we send Connection: close).
func readToEOF(conn as net.Conn) {
    def buf as bytes;
    while (true) {
        def chunk as bytes init net.readBytes($conn, 4096);
        if (len($chunk) == 0) {
            return $buf;
        }
        def i as int init 0;
        def m as int init len($chunk);
        while ($i < $m) {
            $buf[] = $chunk[$i];
            $i = $i + 1;
        }
    }
    return $buf;
}

func dial(u as Url) {
    def addr as string init $u.host + ":" + convert.toString($u.port);
    if ($u.scheme == "https") {
        return net.connectTLS($addr);
    }
    return net.connect($addr);
}

# --- API (exported) ------------------------------------------------

# request sends one HTTP request and returns the response. `headers` is a
# `map of string to string` ({} for none); `body` is "" for no body.
export func request(method as string, url as string,
    headers as map of string to string, body as string) {
    def u as Url init parseUrl($url);
    def conn as net.Conn init dial($u);
    def wire as string init buildRequest($method, $u, $headers, $body);
    net.writeBytes($conn, convert.bytesFromString($wire, "utf-8"));
    def resp as Response init parseResponse(readToEOF($conn));
    net.close($conn);
    return $resp;
}

# get issues a GET request.
export func get(url as string, headers as map of string to string) {
    return request("GET", $url, $headers, "");
}

# post issues a POST request with `contentType` and `body`.
export func post(url as string, contentType as string, body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("POST", $url, $h, $body);
}

# put issues a PUT request with `contentType` and `body`.
export func put(url as string, contentType as string, body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("PUT", $url, $h, $body);
}

# patch issues a PATCH request (a partial update) with `contentType` and `body`.
export func patch(url as string, contentType as string, body as string,
    headers as map of string to string) {
    def h as map of string to string init $headers;
    $h["Content-Type"] = $contentType;
    return request("PATCH", $url, $h, $body);
}

# delete issues a DELETE request.
export func delete(url as string, headers as map of string to string) {
    return request("DELETE", $url, $headers, "");
}

# head issues a HEAD request (status and headers, no body).
export func head(url as string, headers as map of string to string) {
    return request("HEAD", $url, $headers, "");
}

# options issues an OPTIONS request (capability probe; read the `Allow` header).
export func options(url as string, headers as map of string to string) {
    return request("OPTIONS", $url, $headers, "");
}

# header reads a response header by name, case-insensitively, or "" if absent.
export func header(resp as Response, name as string) {
    def key as string init strings.lower($name);
    if (maps.has($resp.headers, $key)) {
        return $resp.headers[$key];
    }
    return "";
}
