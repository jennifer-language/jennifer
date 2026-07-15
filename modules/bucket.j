# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
 * Named `bucket` (not `s3`) because a module namespace is letters-only.
 * @module bucket
 * @example
 * def c as bucket.Client init bucket.connect("https://s3.us-east-1.amazonaws.com", "us-east-1", key, secret);
 * def r as http.Response init bucket.put($c, "mybucket", "hello.txt", "hi there");
 * def o as http.Response init bucket.get($c, "mybucket", "hello.txt");
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
    return Client{ endpoint: $endpoint, region: $region, accessKey: $accessKey, secretKey: $secretKey, timeout: DEFAULT_TIMEOUT_MS };
}

# --- low-level crypto helpers -----------------------------------------------

# hexDigest is the lowercase-hex SHA-256 of a string (payloads, canonical request).
func hexDigest(s as string) {
    return encoding.toText(hash.compute(convert.bytesFromString($s, "utf-8"), "sha256"), "hex");
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
    def out as string init "";
    def i as int init 0;
    while ($i < len($raw)) {
        def b as int init $raw[$i];
        def unreserved as bool init ($b >= 65 and $b <= 90) or ($b >= 97 and $b <= 122) or
            ($b >= 48 and $b <= 57) or $b == 45 or $b == 46 or $b == 95 or $b == 126 or $b == 47;
        if ($unreserved) {
            $out = $out + convert.fromCodepoint($b);
        } else {
            $out = $out + "%" + hexNibble($b // 16) + hexNibble($b % 16);
        }
        $i = $i + 1;
    }
    return $out;
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

# authorization builds the SigV4 Authorization header value for a request. The
# signed header set is fixed (host, x-amz-content-sha256, x-amz-date), so any
# other header (e.g. Content-Type) may be sent unsigned.
func authorization(client as Client, method as string, host as string, canonicalUri as string,
    canonicalQuery as string, payloadHash as string, isoDate as string, shortDate as string) {
    def canonicalHeaders as string init "host:" + $host + "\n" +
        "x-amz-content-sha256:" + $payloadHash + "\n" +
        "x-amz-date:" + $isoDate + "\n";
    def signedHeaders as string init "host;x-amz-content-sha256;x-amz-date";
    def canonicalRequest as string init $method + "\n" + $canonicalUri + "\n" + $canonicalQuery + "\n" +
        $canonicalHeaders + "\n" + $signedHeaders + "\n" + $payloadHash;
    def scope as string init $shortDate + "/" + $client.region + "/" + SERVICE + "/aws4_request";
    def stringToSign as string init ALGORITHM + "\n" + $isoDate + "\n" + $scope + "\n" +
        hexDigest($canonicalRequest);
    def signature as string init encoding.toText(
        hmacRaw(signingKey($client.secretKey, $shortDate, $client.region), $stringToSign), "hex");
    return ALGORITHM + " Credential=" + $client.accessKey + "/" + $scope +
        ", SignedHeaders=" + $signedHeaders + ", Signature=" + $signature;
}

# --- request dispatch -------------------------------------------------------

# doRequest signs and sends one request, returning the http.Response.
func doRequest(client as Client, method as string, canonicalUri as string, canonicalQuery as string, body as string) {
    def host as string init hostOf($client.endpoint);
    def payloadHash as string init hexDigest($body);
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    def shortDate as string init strings.substring($isoDate, 0, 8);
    def auth as string init authorization($client, $method, $host, $canonicalUri,
        $canonicalQuery, $payloadHash, $isoDate, $shortDate);
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
    return http.requestWith($method, $url, $headers, $body, $client.timeout);
}

# objectPath is the canonical URI for an object: /{bucket}/{encoded key}.
func objectPath(bucketName as string, key as string) {
    return "/" + $bucketName + "/" + uriEncodePath($key);
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
 * List a bucket's objects (S3 ListObjectsV2). The response body is the S3 XML
 * listing; pass it to `bucket.objectKeys` to pull out the keys.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @return {http.Response} the response (body = ListBucketResult XML on 200)
 */
export func listObjects(client as Client, bucketName as string) {
    return doRequest($client, "GET", "/" + $bucketName, "list-type=2", "");
}

/**
 * Extract the object keys from a ListObjectsV2 XML body (the `<Key>` elements).
 * @param xml {string} the body from `bucket.listObjects`
 * @return {list of string} the object keys, in listing order
 */
export func objectKeys(xml as string) {
    def keys as list of string init [];
    def matches as list of regex.Match init regex.findAll("<Key>([^<]*)</Key>", $xml);
    for (def m in $matches) {
        $keys[] = $m.groups[0];
    }
    return $keys;
}
