# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0
#
# s3_sigv4.inc.j - the pure AWS Signature Version 4 core for S3: the signing-key
# chain, the canonical URI and query encodings, endpoint and bucket validation,
# and the query-signed (presigned) URL. Spliced into s3.j (the client, which
# signs every request with it) and s3presign.j (presigned URLs alone, no
# network) via `include`; not a standalone module. It declares no `use` of its
# own and relies on the includer's `use hash; use encoding; use convert;
# use regex; use strings; use lists;`, its `Client` struct (endpoint, region,
# accessKey, secretKey) and its `s3Error(message)`, which builds the includer's
# `S3Error`.

def const SERVICE as string init "s3";
def const ALGORITHM as string init "AWS4-HMAC-SHA256";

# --- signing-key chain ------------------------------------------------------

# hexDigest is the lowercase-hex SHA-256 of a string (payloads, canonical request).
func hexDigest(s as string) {
    return encoding.toText("hex", hash.compute("sha256", encoding.encode("utf-8", $s)));
}

# hmacRaw is HMAC-SHA256 of a string message under a raw byte key, as bytes (so
# the SigV4 key chain feeds one HMAC's output in as the next one's key).
func hmacRaw(key as bytes, message as string) {
    return hash.hmac("sha256", encoding.encode("utf-8", $message), $key);
}

# signingKey derives the SigV4 signing key: HMAC("AWS4"+secret, date) chained
# through region, service, and the "aws4_request" terminator.
func signingKey(secret as string, shortDate as string, region as string) {
    def kDate as bytes init hmacRaw(encoding.encode("utf-8", "AWS4" + $secret), $shortDate);
    def kRegion as bytes init hmacRaw($kDate, $region);
    def kService as bytes init hmacRaw($kRegion, SERVICE);
    return hmacRaw($kService, "aws4_request");
}

# --- canonical URI and endpoint ---------------------------------------------

# uriEncodePath percent-encodes a path, leaving the unreserved set and "/" (S3
# object keys keep their slashes) - the AWS canonical-URI rule.
func uriEncodePath(path as string) {
    def raw as bytes init encoding.encode("utf-8", $path);
    # every byte as two uppercase hex digits, sliced per escaped byte
    def hex as string init strings.upper(encoding.toText("hex", $raw));
    # Bytes collect and join once: growing a string with `+` per input byte is
    # O(N^2) over a long object key.
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def unreserved as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or
            ($b >= 48 and $b <= 57) or $b == 45 or $b == 46 or $b == 95 or $b == 126 or $b == 47;
        if ($unreserved) {
            $out[] = convert.fromCodepoint($b);
        } else {
            $out[] = "%" + $hex[2 * $i..2 * $i + 2];
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# ENDPOINT_RE is the whole endpoint: scheme, host (a name or IPv4 address, or
# a bracketed IPv6 address) and an optional port, nothing after it. The signed
# canonical URI is the path alone, so a base path would sign a different path
# than the one sent.
def const ENDPOINT_RE as string init '^([Hh][Tt][Tt][Pp][Ss]?)://' +
    '(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9._-]+)(:[1-9][0-9]{0,4})?$';

# hostOf validates an endpoint and renders the Host header the `http` module
# will send for it: the host, plus the port only when it is not the scheme
# default (matching http's own hostHeader, so the signed host equals the sent
# host). The endpoint stays out of the message: it may be a private address.
func hostOf(endpoint as string) {
    def m as regex.Match init regex.find(ENDPOINT_RE, $endpoint);
    if ($m.start == -1) {
        throw s3Error("s3: endpoint must be http(s)://host[:port], with no path");
    }
    def scheme as string init strings.lower($m.groups[0]);
    def host as string init $m.groups[1];
    if ($m.groups[2] == "") {
        return $host;
    }
    def port as int init convert.toInt($m.groups[2][1..]);
    if ($port > 65535) {
        throw s3Error("s3: endpoint port must be 1 to 65535");
    }
    if (($scheme == "https" and $port == 443) or ($scheme == "http" and $port == 80)) {
        return $host;
    }
    return $host + ":" + convert.toString($port);
}

# BUCKET_RE is the bucket-name charset (S3's current rules plus the legacy
# us-east-1 / MinIO uppercase and "_"): a name goes into the path unencoded, so
# "/", "?", "#", "%" and whitespace must never reach it.
def const BUCKET_RE as string init '^[A-Za-z0-9._-]{1,255}$';

# bucketPath is the canonical URI for a bucket: /{bucket}.
func bucketPath(bucketName as string) {
    if (not regex.matches(BUCKET_RE, $bucketName) or $bucketName == "." or $bucketName == "..") {
        throw s3Error("s3: invalid bucket name: \"" + $bucketName + "\"");
    }
    return "/" + $bucketName;
}

# objectPath is the canonical URI for an object: /{bucket}/{encoded key}. An
# empty key is refused: "/{bucket}/" addresses the bucket, so a PUT or DELETE
# would create or delete the bucket itself.
func objectPath(bucketName as string, key as string) {
    if ($key == "") {
        throw s3Error("s3: empty object key");
    }
    return bucketPath($bucketName) + "/" + uriEncodePath($key);
}

# --- canonical query --------------------------------------------------------

# sortedKeys returns a map's keys in ascending order (SigV4 needs the signed
# header names and query parameters sorted).
func sortedKeys(m as map of string to string) {
    def ks as list of string init [];
    for (def k in $m) {
        $ks[] = $k;
    }
    return lists.sort($ks);
}

# uriEncodeStrict percent-encodes every byte outside the RFC 3986 unreserved set
# (so `/` is encoded too) - the AWS canonical-query-string rule, and the encoding
# for a presigned URL's query parameters and an opaque continuation token.
func uriEncodeStrict(s as string) {
    def unreserved as string init "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~";
    def raw as bytes init encoding.encode("utf-8", $s);
    # every byte as two uppercase hex digits, sliced per escaped byte
    def hex as string init strings.upper(encoding.toText("hex", $raw));
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def ch as string init convert.fromCodepoint($b);
        if (strings.indexOf($unreserved, $ch) >= 0) {
            $out[] = $ch;
        } else {
            $out[] = "%" + $hex[2 * $i..2 * $i + 2];
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# canonicalizeQuery renders a query parameter map as the AWS canonical query
# string: names sorted ascending, each name and value strict-URI-encoded, joined
# with `&` (an empty value yields `name=`).
func canonicalizeQuery(params as map of string to string) {
    def parts as list of string init [];
    for (def k in sortedKeys($params)) {
        $parts[] = uriEncodeStrict($k) + "=" + uriEncodeStrict($params[$k]);
    }
    return strings.join($parts, "&");
}

# requestUrl assembles the request URL from the endpoint, canonical URI, and an
# optional canonical query string.
func requestUrl(client as Client, canonicalUri as string, canonicalQuery as string) {
    def url as string init $client.endpoint + $canonicalUri;
    if (not ($canonicalQuery == "")) {
        $url = $url + "?" + $canonicalQuery;
    }
    return $url;
}

# --- presigned URL ----------------------------------------------------------

# presignAt is presign with an explicit request timestamp (so the signing is
# deterministically testable against an independent SigV4 vector).
func presignAt(
    client as Client,
    method as string,
    bucketName as string,
    key as string,
    expiresSeconds as int,
    isoDate as string) {
    # SigV4 query signing allows 1 second to 7 days; S3 refuses any other
    # X-Amz-Expires only when the URL is used.
    if ($expiresSeconds < 1 or $expiresSeconds > 604800) {
        throw s3Error("s3: presign expiry must be 1 to 604800 seconds, got {$expiresSeconds}");
    }
    def host as string init hostOf($client.endpoint);
    def shortDate as string init $isoDate[..8];
    def canonicalUri as string init objectPath($bucketName, $key);
    def scope as string init $shortDate + "/" + $client.region + "/" + SERVICE + "/aws4_request";
    def params as map of string to string init {};
    $params["X-Amz-Algorithm"] = ALGORITHM;
    $params["X-Amz-Credential"] = $client.accessKey + "/" + $scope;
    $params["X-Amz-Date"] = $isoDate;
    $params["X-Amz-Expires"] = convert.toString($expiresSeconds);
    $params["X-Amz-SignedHeaders"] = "host";
    def canonicalQuery as string init canonicalizeQuery($params);
    def canonicalHeaders as string init "host:" + $host + "\n";
    def canonicalRequest as string init $method + "\n" + $canonicalUri + "\n" + $canonicalQuery +
        "\n" + $canonicalHeaders + "\n" + "host" + "\n" + "UNSIGNED-PAYLOAD";
    def stringToSign as string init ALGORITHM + "\n" + $isoDate + "\n" + $scope + "\n" +
        hexDigest($canonicalRequest);
    def signature as string init encoding.toText(
        "hex",
        hmacRaw(signingKey($client.secretKey, $shortDate, $client.region), $stringToSign));
    return requestUrl($client, $canonicalUri, $canonicalQuery) + "&X-Amz-Signature=" + $signature;
}
