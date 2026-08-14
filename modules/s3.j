# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * An S3-compatible object-storage client: get / put / delete objects and list a
 * bucket, signing every request with **AWS Signature Version 4**. The endpoint
 * is configurable, so one module serves AWS S3 and every S3-compatible store
 * (MinIO, Cloudflare R2, Backblaze B2) - a selectable backend, not a module per
 * vendor. Path-style addressing (`{endpoint}/{bucket}/{key}`). SigV4 is
 * HMAC-SHA256 key-chaining, so this builds on `hash.hmac` + `hash.compute` +
 * `encoding` (hex) + `time` (the request timestamp) + `http`. Needs the default
 * `jennifer` binary (`net` via `http`).
 *
 * @module s3
 * @example
 * def c as s3.Client init s3.connect("https://s3.us-east-1.amazonaws.com", "us-east-1", key, secret);
 * def r as http.Response init s3.put($c, "mybucket", "hello.txt", "hi there");
 * def o as http.Response init s3.get($c, "mybucket", "hello.txt");
 */
use hash;
use encoding;
use convert;
use time;
use regex;
use strings;
use lists;
import "./http.j" as http;

def const SERVICE as string init "s3";
def const ALGORITHM as string init "AWS4-HMAC-SHA256";
# The default per-request idle timeout, so a hung S3 endpoint fails instead of
# blocking forever (set `Client.timeout` to override; 0 disables it).
def const DEFAULT_TIMEOUT_MS as int init 30000;

/**
 * A configured S3 client: the endpoint (scheme + host, no trailing slash), the
 * signing region, and the access-key pair. Value-semantic; build with `connect`.
 * @field endpoint {string} e.g. "https://s3.us-east-1.amazonaws.com" or "http://localhost:9000"
 * @field region {string} the signing region, e.g. "us-east-1"
 * @field accessKey {string} the access key id
 * @field secretKey {string} the secret access key
 * @field timeout {int} per-request idle timeout in milliseconds (0 disables it); `connect` defaults it to 30000
 */
export def struct Client {
    endpoint as string,
    region as string,
    accessKey as string,
    secretKey as string,
    timeout as int
};

/**
 * Build a client for an S3 endpoint. The endpoint is any S3-compatible base URL
 * (`scheme://host[:port]`, no trailing slash).
 * @param endpoint {string} the base URL
 * @param region {string} the signing region
 * @param accessKey {string} the access key id
 * @param secretKey {string} the secret access key
 * @return {Client} a configured client (30 s timeout; set `.timeout` to change it)
 */
export func connect(endpoint as string, region as string, accessKey as string, secretKey as string) {
    return Client{
        endpoint: $endpoint,
        region: $region,
        accessKey: $accessKey,
        secretKey: $secretKey,
        timeout: DEFAULT_TIMEOUT_MS
    };
}

# --- low-level crypto helpers -----------------------------------------------

# hexDigest is the lowercase-hex SHA-256 of a string (payloads, canonical request).
func hexDigest(s as string) {
    return encoding.toText(hash.compute(convert.bytesFromString($s, "utf-8"), "sha256"), "hex");
}

# hexDigestBytes is the lowercase-hex SHA-256 of raw bytes (a binary payload, so a
# byte body is signed byte-for-byte rather than through a lossy UTF-8 round-trip).
func hexDigestBytes(b as bytes) {
    return encoding.toText(hash.compute($b, "sha256"), "hex");
}

# hmacRaw is HMAC-SHA256 of a string message under a raw byte key, as bytes (so
# the SigV4 key chain feeds one HMAC's output in as the next one's key).
func hmacRaw(key as bytes, message as string) {
    return hash.hmac($key, convert.bytesFromString($message, "utf-8"), "sha256");
}

# signingKey derives the SigV4 signing key: HMAC("AWS4"+secret, date) chained
# through region, service, and the "aws4_request" terminator.
func signingKey(secret as string, shortDate as string, region as string) {
    def kDate as bytes init hmacRaw(convert.bytesFromString("AWS4" + $secret, "utf-8"), $shortDate);
    def kRegion as bytes init hmacRaw($kDate, $region);
    def kService as bytes init hmacRaw($kRegion, SERVICE);
    return hmacRaw($kService, "aws4_request");
}

# --- request canonicalization -----------------------------------------------

# hexNibble renders a 4-bit value as one uppercase hex digit.
func hexNibble(n as int) {
    def digits as string init "0123456789ABCDEF";
    return strings.substring($digits, $n, $n + 1);
}

# uriEncodePath percent-encodes a path, leaving the unreserved set and "/" (S3
# object keys keep their slashes) - the AWS canonical-URI rule.
func uriEncodePath(path as string) {
    def raw as bytes init convert.bytesFromString($path, "utf-8");
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
            $out[] = "%" + hexNibble($b // 16) + hexNibble($b % 16);
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# hostOf renders the Host header the `http` module will send for this endpoint:
# the host, plus the port only when it is not the scheme default (matching
# http's own hostHeader, so the signed host equals the sent host).
func hostOf(endpoint as string) {
    def scheme as string init "http";
    def rest as string init $endpoint;
    def sep as int init strings.indexOf($endpoint, "://");
    if ($sep >= 0) {
        $scheme = strings.substring($endpoint, 0, $sep);
        $rest = strings.substring($endpoint, $sep + 3, len($endpoint));
    }
    def slash as int init strings.indexOf($rest, "/");
    def authority as string init $rest;
    if ($slash >= 0) {
        $authority = strings.substring($rest, 0, $slash);
    }
    def colon as int init strings.indexOf($authority, ":");
    if ($colon < 0) {
        return $authority;
    }
    def host as string init strings.substring($authority, 0, $colon);
    def port as int init convert.toInt(strings.substring($authority, $colon + 1, len($authority)));
    if (($scheme == "https" and $port == 443) or ($scheme == "http" and $port == 80)) {
        return $host;
    }
    return $authority;
}

# signCore builds the SigV4 Authorization header value from already-rendered
# canonical-headers / signed-headers strings. The crypto core shared by the
# fixed-header `authorization` and the generalized `prepareHeaders` (metadata /
# copy) paths, so both sign identically.
func signCore(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    payloadHash as string,
    isoDate as string,
    shortDate as string,
    canonicalHeaders as string,
    signedHeaders as string) {
    def canonicalRequest as string init $method + "\n" + $canonicalUri + "\n" + $canonicalQuery +
        "\n" +
        $canonicalHeaders + "\n" + $signedHeaders + "\n" + $payloadHash;
    def scope as string init $shortDate + "/" + $client.region + "/" + SERVICE + "/aws4_request";
    def stringToSign as string init ALGORITHM + "\n" + $isoDate + "\n" + $scope + "\n" +
        hexDigest($canonicalRequest);
    def signature as string init encoding.toText(
        hmacRaw(signingKey($client.secretKey, $shortDate, $client.region), $stringToSign),
        "hex");
    return ALGORITHM + " Credential=" + $client.accessKey + "/" + $scope +
        ", SignedHeaders=" + $signedHeaders + ", Signature=" + $signature;
}

# authorization builds the SigV4 Authorization header value for a request signing
# the fixed header set (host, x-amz-content-sha256, x-amz-date), so any other
# header (e.g. an unsigned Content-Type) may be sent alongside it.
func authorization(
    client as Client,
    method as string,
    host as string,
    canonicalUri as string,
    canonicalQuery as string,
    payloadHash as string,
    isoDate as string,
    shortDate as string) {
    def canonicalHeaders as string init "host:" + $host + "\n" +
        "x-amz-content-sha256:" + $payloadHash + "\n" +
        "x-amz-date:" + $isoDate + "\n";
    def signedHeaders as string init "host;x-amz-content-sha256;x-amz-date";
    return signCore(
        $client,
        $method,
        $canonicalUri,
        $canonicalQuery,
        $payloadHash,
        $isoDate,
        $shortDate,
        $canonicalHeaders,
        $signedHeaders);
}

# --- request dispatch -------------------------------------------------------

# doRequest signs and sends one request, returning the http.Response.
func doRequest(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    body as string) {
    def host as string init hostOf($client.endpoint);
    def payloadHash as string init hexDigest($body);
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    def shortDate as string init strings.substring($isoDate, 0, 8);
    def auth as string init authorization(
        $client,
        $method,
        $host,
        $canonicalUri,
        $canonicalQuery,
        $payloadHash,
        $isoDate,
        $shortDate);
    def headers as map of string to string init {};
    $headers["x-amz-date"] = $isoDate;
    $headers["x-amz-content-sha256"] = $payloadHash;
    $headers["Authorization"] = $auth;
    if ($method == "PUT") {
        $headers["Content-Type"] = "application/octet-stream";
    }
    def url as string init $client.endpoint + $canonicalUri;
    if (not ($canonicalQuery == "")) {
        $url = $url + "?" + $canonicalQuery;
    }
    # -1 lifts http's default body cap: an object-storage GET may legitimately
    # return an object larger than 64 MiB, and the bucket is the caller's own.
    return http.requestWith($method, $url, $headers, $body, $client.timeout, -1);
}

# objectPath is the canonical URI for an object: /{bucket}/{encoded key}.
func objectPath(bucketName as string, key as string) {
    return "/" + $bucketName + "/" + uriEncodePath($key);
}

# sortedKeys returns a map's keys in ascending order (SigV4 needs the signed
# header names and query parameters sorted).
func sortedKeys(m as map of string to string) {
    def ks as list of string init [];
    for (def k in $m) {
        $ks[] = $k;
    }
    return lists.sort($ks);
}

# canonicalHeaderValue trims a header value and collapses each run of spaces to a
# single space - the SigV4 canonical-header-value rule. The value goes onto the
# wire unchanged, but is *signed* in this collapsed form (as S3 re-derives it), so
# a metadata value with internal double spaces still verifies.
func canonicalHeaderValue(s as string) {
    def t as string init strings.trim($s);
    def out as list of string init [];
    def prevSpace as bool init false;
    for (def ch in strings.chars($t)) {
        if ($ch == " ") {
            if (not $prevSpace) {
                $out[] = " ";
            }
            $prevSpace = true;
        } else {
            $out[] = $ch;
            $prevSpace = false;
        }
    }
    return strings.join($out, "");
}

# prepareHeaders signs a request whose signed set is the base three headers plus
# any `extra` headers (content-type, x-amz-meta-*, x-amz-copy-source), and returns
# the full outgoing header map (the extras keep their original casing on the
# wire, but are signed by their lowercased, whitespace-trimmed name/value). This
# is the general path that supports metadata and copy; the fixed-header get / put
# / delete / list stay on `doRequest`.
func prepareHeaders(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    payloadHash as string,
    extra as map of string to string) {
    def host as string init hostOf($client.endpoint);
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    def shortDate as string init strings.substring($isoDate, 0, 8);
    def signed as map of string to string init {};
    $signed["host"] = $host;
    $signed["x-amz-content-sha256"] = $payloadHash;
    $signed["x-amz-date"] = $isoDate;
    for (def k in $extra) {
        $signed[strings.lower($k)] = canonicalHeaderValue($extra[$k]);
    }
    def names as list of string init sortedKeys($signed);
    def canonicalHeaders as string init "";
    for (def n in $names) {
        $canonicalHeaders = $canonicalHeaders + $n + ":" + $signed[$n] + "\n";
    }
    def signedHeaders as string init strings.join($names, ";");
    def auth as string init signCore(
        $client,
        $method,
        $canonicalUri,
        $canonicalQuery,
        $payloadHash,
        $isoDate,
        $shortDate,
        $canonicalHeaders,
        $signedHeaders);
    def out as map of string to string init {};
    $out["x-amz-date"] = $isoDate;
    $out["x-amz-content-sha256"] = $payloadHash;
    for (def k in $extra) {
        $out[$k] = $extra[$k];
    }
    $out["Authorization"] = $auth;
    return $out;
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

# sendSigned signs and sends a request with a string body and extra signed
# headers, returning the http.Response. `-1` lifts http's default body cap.
func sendSigned(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    body as string,
    extra as map of string to string) {
    def headers as map of string to string init prepareHeaders(
        $client,
        $method,
        $canonicalUri,
        $canonicalQuery,
        hexDigest($body),
        $extra);
    return http.requestWith(
        $method,
        requestUrl($client, $canonicalUri, $canonicalQuery),
        $headers,
        $body,
        $client.timeout,
        -1);
}

# sendSignedBytes signs and sends a request with a raw `bytes` body (written
# byte-for-byte) and extra signed headers, returning the http.Response.
func sendSignedBytes(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    body as bytes,
    extra as map of string to string) {
    def headers as map of string to string init prepareHeaders(
        $client,
        $method,
        $canonicalUri,
        $canonicalQuery,
        hexDigestBytes($body),
        $extra);
    return http.requestRawBody(
        $method,
        requestUrl($client, $canonicalUri, $canonicalQuery),
        $headers,
        $body,
        $client.timeout,
        -1);
}

# metadataHeaders builds the extra-signed-header map from an optional content type
# and a user metadata map (keys get the `x-amz-meta-` prefix S3 stores them under).
func metadataHeaders(contentType as string, metadata as map of string to string) {
    def extra as map of string to string init {};
    if (len($contentType) > 0) {
        $extra["Content-Type"] = $contentType;
    }
    for (def k in $metadata) {
        $extra["x-amz-meta-" + $k] = $metadata[$k];
    }
    return $extra;
}

# --- object operations (exported) -------------------------------------------

/**
 * GET an object. The response body is the object's contents; a missing object
 * comes back as a 404 `http.Response`, not an error.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @return {http.Response} the response (body = object contents on 200)
 */
export func get(client as Client, bucketName as string, key as string) {
    return doRequest($client, "GET", objectPath($bucketName, $key), "", "");
}

/**
 * PUT (upload / overwrite) an object with the given body.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param body {string} the object contents
 * @return {http.Response} the response (200 on success)
 */
export func put(client as Client, bucketName as string, key as string, body as string) {
    return doRequest($client, "PUT", objectPath($bucketName, $key), "", $body);
}

/**
 * DELETE an object.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @return {http.Response} the response (204 on success)
 */
export func delete(client as Client, bucketName as string, key as string) {
    return doRequest($client, "DELETE", objectPath($bucketName, $key), "", "");
}

/**
 * GET an object as raw `bytes` (the byte-safe download). Use this for binary
 * objects (images, archives) that a UTF-8 string body cannot hold; `get` stays
 * the text convenience.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @return {http.BytesResponse} the response (body = raw object bytes on 200)
 */
export func getBytes(client as Client, bucketName as string, key as string) {
    def headers as map of string to string init prepareHeaders(
        $client,
        "GET",
        objectPath($bucketName, $key),
        "",
        hexDigest(""),
        {});
    def tls as http.TlsOptions; # zero value: full certificate verification
    return http.requestWithBytes(
        "GET",
        requestUrl($client, objectPath($bucketName, $key), ""),
        $headers,
        "",
        $client.timeout,
        -1,
        $tls);
}

/**
 * PUT (upload / overwrite) an object from a raw `bytes` body, written
 * byte-for-byte so a binary payload round-trips intact.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param data {bytes} the object contents
 * @return {http.Response} the response (200 on success)
 */
export func putBytes(client as Client, bucketName as string, key as string, data as bytes) {
    return sendSignedBytes($client, "PUT", objectPath($bucketName, $key), "", $data, {});
}

/**
 * PUT an object (string body) with an explicit content type and user metadata.
 * The Content-Type and each `x-amz-meta-<key>` header are part of the signature,
 * so they reach S3 intact. Pass `""` for the default content type and `{}` for
 * no metadata.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param body {string} the object contents
 * @param contentType {string} the object content type (e.g. "text/html"; "" = default)
 * @param metadata {map of string to string} user metadata (stored as x-amz-meta-<key>)
 * @return {http.Response} the response (200 on success)
 */
export func putWith(
    client as Client,
    bucketName as string,
    key as string,
    body as string,
    contentType as string,
    metadata as map of string to string) {
    return sendSigned(
        $client,
        "PUT",
        objectPath($bucketName, $key),
        "",
        $body,
        metadataHeaders($contentType, $metadata));
}

/**
 * PUT an object (raw `bytes` body) with an explicit content type and user
 * metadata - the binary counterpart to `putWith`.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param data {bytes} the object contents
 * @param contentType {string} the object content type ("" = default)
 * @param metadata {map of string to string} user metadata (stored as x-amz-meta-<key>)
 * @return {http.Response} the response (200 on success)
 */
export func putBytesWith(
    client as Client,
    bucketName as string,
    key as string,
    data as bytes,
    contentType as string,
    metadata as map of string to string) {
    return sendSignedBytes(
        $client,
        "PUT",
        objectPath($bucketName, $key),
        "",
        $data,
        metadataHeaders($contentType, $metadata));
}

/**
 * HEAD an object: its metadata (status + headers) without the body. A present
 * object returns 200 with `Content-Length` / `Content-Type` / `x-amz-meta-*`
 * headers; a missing one returns 404.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @return {http.Response} the response (headers only, empty body)
 */
export func head(client as Client, bucketName as string, key as string) {
    return doRequest($client, "HEAD", objectPath($bucketName, $key), "", "");
}

/**
 * Copy an object server-side (no download / re-upload): PUT the destination with
 * an `x-amz-copy-source` of `/{srcBucket}/{srcKey}` (signed). Source and
 * destination may share a bucket (a rename when paired with `delete`).
 * @param client {Client} the client
 * @param srcBucket {string} the source bucket
 * @param srcKey {string} the source object key
 * @param dstBucket {string} the destination bucket
 * @param dstKey {string} the destination object key
 * @return {http.Response} the response (200 with a CopyObjectResult body on success)
 */
export func copy(
    client as Client,
    srcBucket as string,
    srcKey as string,
    dstBucket as string,
    dstKey as string) {
    def extra as map of string to string init {};
    $extra["x-amz-copy-source"] = "/" + $srcBucket + "/" + uriEncodePath($srcKey);
    return sendSigned($client, "PUT", objectPath($dstBucket, $dstKey), "", "", $extra);
}

/**
 * List a bucket's objects (S3 ListObjectsV2). The response body is the S3 XML
 * listing; pass it to `s3.objectKeys` to pull out the keys. S3 returns at
 * most 1000 keys per page: check `s3.isTruncated` on the body and, if true,
 * call `s3.listObjectsFrom` with `s3.nextContinuationToken` to fetch the
 * next page (loop until not truncated).
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @return {http.Response} the response (body = ListBucketResult XML on 200)
 */
export func listObjects(client as Client, bucketName as string) {
    return doRequest($client, "GET", "/" + $bucketName, "list-type=2", "");
}

# uriEncodeStrict percent-encodes every byte outside the RFC 3986 unreserved set
# (so `/` is encoded too) - the AWS canonical-query-string rule, and the encoding
# for a presigned URL's query parameters and an opaque continuation token.
func uriEncodeStrict(s as string) {
    def unreserved as string init "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~";
    def raw as bytes init convert.bytesFromString($s, "utf-8");
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def ch as string init convert.fromCodepoint($b);
        if (strings.indexOf($unreserved, $ch) >= 0) {
            $out[] = $ch;
        } else {
            $out[] = "%" + hexNibble($b // 16) + hexNibble($b % 16);
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# tokenEncode percent-encodes a continuation token for the query string (S3
# tokens are opaque base64-ish strings that can carry `+` / `/` / `=`).
func tokenEncode(s as string) {
    return uriEncodeStrict($s);
}

/**
 * Fetch one further page of a ListObjectsV2 listing, starting after `token` (the
 * value from `s3.nextContinuationToken` on the previous page's body).
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param token {string} the continuation token from the previous page
 * @return {http.Response} the response (body = ListBucketResult XML on 200)
 */
export func listObjectsFrom(client as Client, bucketName as string, token as string) {
    # SigV4 requires the canonical query sorted by key: "continuation-token"
    # sorts before "list-type", so build it through canonicalizeQuery rather than
    # hand-ordering (a hand-written "list-type=2&continuation-token=..." signs an
    # out-of-order query and a real S3 / MinIO rejects page 2 with a 403).
    def params as map of string to string init {};
    $params["list-type"] = "2";
    $params["continuation-token"] = $token;
    return doRequest($client, "GET", "/" + $bucketName, canonicalizeQuery($params), "");
}

/**
 * Report whether a ListObjectsV2 XML body was truncated (more pages remain).
 * @param xml {string} the body from a list call
 * @return {bool} true when another page is available
 */
export func isTruncated(xml as string) {
    return regex.find("<IsTruncated>\\s*true\\s*</IsTruncated>", $xml).start >= 0;
}

/**
 * Extract the continuation token to fetch the next page, or "" when none.
 * @param xml {string} the body from a list call
 * @return {string} the next continuation token, or "" if the listing is complete
 */
export func nextContinuationToken(xml as string) {
    def m as regex.Match init regex.find(
        "<NextContinuationToken>([^<]*)</NextContinuationToken>",
        $xml);
    if ($m.start == -1) {
        return "";
    }
    return unescapeXml($m.groups[0]);
}

/**
 * Extract the object keys from a ListObjectsV2 XML body (the `<Key>` elements).
 * @param xml {string} the body from `s3.listObjects`
 * @return {list of string} the object keys, in listing order
 */
export func objectKeys(xml as string) {
    def keys as list of string init [];
    def matches as list of regex.Match init regex.findAll("<Key>([^<]*)</Key>", $xml);
    for (def m in $matches) {
        # XML content escapes the five predefined entities; decode them so a key
        # like "reports&amp;data.txt" round-trips back into s3.get / delete.
        $keys[] = unescapeXml($m.groups[0]);
    }
    return $keys;
}

# unescapeXml decodes the five predefined XML entities. `&amp;` is decoded last
# so an already-decoded `&` in the source is not re-interpreted.
func unescapeXml(s as string) {
    def out as string init strings.replace($s, "&lt;", "<");
    $out = strings.replace($out, "&gt;", ">");
    $out = strings.replace($out, "&quot;", "\"");
    $out = strings.replace($out, "&apos;", "'");
    $out = strings.replace($out, "&amp;", "&");
    return $out;
}

# escapeXml escapes the three text-significant XML characters (`&` first) so a
# value placed in element text cannot break the document.
func escapeXml(s as string) {
    def out as string init strings.replace($s, "&", "&amp;");
    $out = strings.replace($out, "<", "&lt;");
    $out = strings.replace($out, ">", "&gt;");
    return $out;
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

# --- presigned URLs (exported) ----------------------------------------------

/**
 * Build a presigned URL that grants time-limited access to an object without
 * exposing the secret key, using SigV4 query-signing (`X-Amz-*` query
 * parameters). The URL works from any HTTP client until it expires. Pure (no
 * request is sent), so it runs on both binaries.
 * @param client {Client} the client
 * @param method {string} the HTTP method the URL authorizes (e.g. "GET", "PUT")
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param expiresSeconds {int} the validity window in seconds (max 604800 = 7 days)
 * @return {string} the presigned URL
 */
export func presign(
    client as Client,
    method as string,
    bucketName as string,
    key as string,
    expiresSeconds as int) {
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    return presignAt($client, $method, $bucketName, $key, $expiresSeconds, $isoDate);
}

# presignAt is presign with an explicit request timestamp (so the signing is
# deterministically testable against an independent SigV4 vector).
func presignAt(
    client as Client,
    method as string,
    bucketName as string,
    key as string,
    expiresSeconds as int,
    isoDate as string) {
    def host as string init hostOf($client.endpoint);
    def shortDate as string init strings.substring($isoDate, 0, 8);
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
        hmacRaw(signingKey($client.secretKey, $shortDate, $client.region), $stringToSign),
        "hex");
    return requestUrl($client, $canonicalUri, $canonicalQuery) + "&X-Amz-Signature=" + $signature;
}

# --- multipart upload (exported) --------------------------------------------

/**
 * Begin a multipart upload (for objects beyond the ~5 GB single-PUT limit, or to
 * stream parts). Returns the upload id to pass to `uploadPart` /
 * `completeMultipartUpload` / `abortMultipartUpload`.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param contentType {string} the object content type ("" = default)
 * @return {string} the upload id
 * @throws {Error} kind "s3" when the response carries no UploadId
 */
export func createMultipartUpload(
    client as Client,
    bucketName as string,
    key as string,
    contentType as string) {
    def extra as map of string to string init {};
    if (len($contentType) > 0) {
        $extra["Content-Type"] = $contentType;
    }
    def resp as http.Response init sendSigned(
        $client,
        "POST",
        objectPath($bucketName, $key),
        "uploads=",
        "",
        $extra);
    def m as regex.Match init regex.find("<UploadId>([^<]*)</UploadId>", $resp.body);
    if ($m.start == -1) {
        throw Error{
            kind: "s3",
            message: "createMultipartUpload: no UploadId (status " + convert.toString($resp.status) + ")",
            file: "",
            line: 0,
            col: 0
        };
    }
    return unescapeXml($m.groups[0]);
}

/**
 * Upload one part of a multipart upload (each part >= 5 MiB except the last).
 * Returns the part's ETag, which `completeMultipartUpload` needs (collect them
 * in part-number order).
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param uploadId {string} the id from `createMultipartUpload`
 * @param partNumber {int} the 1-based part number
 * @param data {bytes} the part contents
 * @return {string} the part's ETag
 */
export func uploadPart(
    client as Client,
    bucketName as string,
    key as string,
    uploadId as string,
    partNumber as int,
    data as bytes) {
    def params as map of string to string init {};
    $params["partNumber"] = convert.toString($partNumber);
    $params["uploadId"] = $uploadId;
    def resp as http.Response init sendSignedBytes(
        $client,
        "PUT",
        objectPath($bucketName, $key),
        canonicalizeQuery($params),
        $data,
        {});
    return http.header($resp, "ETag");
}

/**
 * Complete a multipart upload, assembling the uploaded parts into the final
 * object. `etags` are the ETags from `uploadPart` in part-number order (part 1
 * first).
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param uploadId {string} the id from `createMultipartUpload`
 * @param etags {list of string} the part ETags, in part-number order
 * @return {http.Response} the response (200 with a CompleteMultipartUploadResult body)
 */
export func completeMultipartUpload(
    client as Client,
    bucketName as string,
    key as string,
    uploadId as string,
    etags as list of string) {
    def params as map of string to string init {};
    $params["uploadId"] = $uploadId;
    return sendSigned(
        $client,
        "POST",
        objectPath($bucketName, $key),
        canonicalizeQuery($params),
        completeXml($etags),
        {});
}

# completeXml renders the CompleteMultipartUpload request body from part ETags.
func completeXml(etags as list of string) {
    # Part elements collect and join once: growing `parts` with `+` per part is
    # O(N^2) over a large multipart upload.
    def parts as list of string init [];
    for (def i as int init 0; $i < len($etags); $i = $i + 1) {
        $parts[] = "<Part><PartNumber>" + convert.toString($i + 1) +
            "</PartNumber><ETag>" + escapeXml($etags[$i]) + "</ETag></Part>";
    }
    return "<CompleteMultipartUpload>" + strings.join($parts, "") + "</CompleteMultipartUpload>";
}

/**
 * Abort a multipart upload, discarding its uploaded parts (call this on failure
 * so the parts do not linger and accrue storage cost).
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param uploadId {string} the id from `createMultipartUpload`
 * @return {http.Response} the response (204 on success)
 */
export func abortMultipartUpload(
    client as Client,
    bucketName as string,
    key as string,
    uploadId as string) {
    def params as map of string to string init {};
    $params["uploadId"] = $uploadId;
    return sendSigned(
        $client,
        "DELETE",
        objectPath($bucketName, $key),
        canonicalizeQuery($params),
        "",
        {});
}
