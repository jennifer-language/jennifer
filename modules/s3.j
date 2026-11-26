# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0

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
 * The object verbs return the `http.Response`: a 4xx / 5xx is a value to branch
 * on, and `errorOf` reads S3's XML error body (`<Code>NoSuchKey</Code>`, the
 * message and request id) into an `S3Error`, the struct the module throws.
 * Every reply but an object download (`get` / `getBytes`) is capped at 16 MiB
 * (`Error{kind: "limit"}` past it). An endpoint other than `https://` (or
 * `http://` to a loopback host) throws an `S3Error` before a signed request is
 * sent, unless the client sets `allowInsecureHttp`.
 *
 * @module s3
 * @example
 * use os;
 * import "s3.j" as s3;
 * import "http.j" as http;
 * def key as string init os.getEnv("S3_KEY");
 * def secret as string init os.getEnv("S3_SECRET");
 * def c as s3.Client init s3.connect("https://s3.amazonaws.com", "us-east-1", $key, $secret);
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
use math;
import "./http.j" as http;

# The default per-request idle timeout, so a hung S3 endpoint fails instead of
# blocking forever (set `Client.timeoutMs` to override; 0 disables it).
def const DEFAULT_TIMEOUT_MS as int init 30000;
# The response cap for every reply except an object download: a listing page of
# 1000 keys of up to 1024 bytes each, XML-escaped, stays well below it.
def const MAX_REPLY_BYTES as int init 16777216;

# The SigV4 core (signing key, canonical URI / query, endpoint and bucket
# checks, presignAt), shared with s3presign.j.
include "./s3_sigv4.inc.j";

/**
 * A configured S3 client: the endpoint (scheme + host, no trailing slash), the
 * signing region, and the access-key pair. Value-semantic; build with `connect`.
 * @field endpoint {string} e.g. "https://s3.us-east-1.amazonaws.com" or "http://localhost:9000"
 * @field region {string} the signing region, e.g. "us-east-1"
 * @field accessKey {string} the access key id
 * @field secretKey {string} the secret access key
 * @field timeoutMs {int} per-request idle timeout in milliseconds (0 disables it); `connect` defaults it to 30000
 * @field allowInsecureHttp {bool} allow plain `http://` to a non-loopback host (default false)
 */
export def struct Client {
    endpoint as string,
    region as string,
    accessKey as string,
    secretKey as string,
    timeoutMs as int,
    allowInsecureHttp as bool
};

/**
 * Build a client for an S3 endpoint. The endpoint is any S3-compatible base URL,
 * `http://` or `https://` plus a host (a name, an IPv4 address or a bracketed
 * IPv6 address) and an optional port; one trailing `/` is dropped. A path, a
 * query, a fragment, userinfo or a malformed host or port throws.
 * @param endpoint {string} the base URL
 * @param region {string} the signing region
 * @param accessKey {string} the access key id
 * @param secretKey {string} the secret access key
 * @return {Client} a configured client (30 s timeout; set `.timeoutMs` to change it)
 * @throws {S3Error} kind "s3" for a malformed endpoint
 */
export func connect(
    endpoint as string,
    region as string,
    accessKey as string,
    secretKey as string) {
    def base as string init $endpoint;
    if (strings.endsWith($base, "/")) {
        $base = $base[..len($base) - 1];
    }
    hostOf($base);
    return Client{
        endpoint: $base,
        region: $region,
        accessKey: $accessKey,
        secretKey: $secretKey,
        timeoutMs: DEFAULT_TIMEOUT_MS,
        allowInsecureHttp: false
    };
}

/**
 * A failed S3 request, as `errorOf` reads it and the module throws it. Every
 * error of kind `"s3"` is one, so its fields are safe to read once the kind
 * matches.
 * @field kind {string} always `"s3"`
 * @field message {string} `s3: HTTP <status>: <Code>: <Message>`, or what went wrong
 * @field status {int} the response's HTTP status; 0 for a local check
 * @field code {string} S3's error `Code` (`NoSuchKey`, `NoSuchBucket`, `AccessDenied`, `SignatureDoesNotMatch`, `SlowDown`, ...); "" when the body had none
 * @field requestId {string} S3's `RequestId`, for the provider's support; "" when none
 */
export def struct S3Error {
    kind as string,
    message as string,
    status as int,
    code as string,
    requestId as string
};

# s3Error is the S3Error of a local check.
func s3Error(message as string) {
    return S3Error{kind: "s3", message: $message, status: 0, code: "", requestId: ""};
}

# xmlField reads the text of the first <name>...</name> element, unescaped, or "".
func xmlField(body as string, name as string) {
    def m as regex.Match init regex.find("<" + $name + ">([^<]*)</" + $name + ">", $body);
    if ($m.start == -1) {
        return "";
    }
    return unescapeXml($m.groups[0]);
}

/**
 * Read a failed response into an `S3Error`: its status and, from S3's XML error
 * body, the `Code`, `Message` and `RequestId`. A HEAD or a non-XML body gives
 * the status alone.
 * @param resp {http.Response} a response, typically with a 4xx / 5xx status
 * @return {S3Error} the error, to inspect or `throw`
 * @example
 * def r as http.Response init s3.get($c, "photos", "cat.jpg");
 * if ($r.status != 200) {
 *     def e as s3.S3Error init s3.errorOf($r);
 *     if ($e.code == "NoSuchKey") {
 *         io.printf("no such photo\n");
 *     } else {
 *         throw $e;
 *     }
 * }
 */
export func errorOf(resp as http.Response) {
    def code as string init xmlField($resp.body, "Code");
    def text as string init xmlField($resp.body, "Message");
    def message as string init "s3: HTTP {$resp.status}";
    if ($code != "") {
        $message = $message + ": " + $code;
    }
    if ($text != "") {
        $message = $message + ": " + $text;
    }
    return S3Error{
        kind: "s3",
        message: $message,
        status: $resp.status,
        code: $code,
        requestId: xmlField($resp.body, "RequestId")
    };
}

# hexDigestBytes is the lowercase-hex SHA-256 of raw bytes (a binary payload, so a
# byte body is signed byte-for-byte rather than through a lossy UTF-8 round-trip).
func hexDigestBytes(b as bytes) {
    return encoding.toText("hex", hash.compute("sha256", $b));
}

# --- request canonicalization -----------------------------------------------

# requestHost is hostOf for a request about to be sent: it also refuses plain
# http:// to a non-loopback host while the client holds keys, before any
# connection. SigV4 never sends the secret, but the access key id, every object
# and listing travel in the clear, and an on-path attacker can replay a captured
# signed PUT or DELETE for 15 minutes or answer a GET with data of their own.
func requestHost(client as Client) {
    def host as string init hostOf($client.endpoint);
    if (($client.accessKey != "" or $client.secretKey != "") and not $client.allowInsecureHttp and
        http.isCleartextRemote($client.endpoint)) {
        throw s3Error(
            "s3: refusing a signed request over plain http:// to a non-loopback host " +
                "(set Client.allowInsecureHttp to allow it)");
    }
    return $host;
}

# isErrorDocument reports whether a 2xx body is an S3 <Error> document: AWS
# answers CompleteMultipartUpload and CopyObject with 200 before the work is
# done, so a failure arrives as an <Error> body under that 200.
func isErrorDocument(body as string) {
    return regex.matches('^\s*(<\?xml[^>]*\?>\s*)?<Error>', $body);
}

# checkDeferredError throws the S3Error of a 2xx reply whose body is <Error>.
func checkDeferredError(resp as http.Response) {
    if ($resp.status >= 200 and $resp.status < 300 and isErrorDocument($resp.body)) {
        throw errorOf($resp);
    }
    return $resp;
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
        "hex",
        hmacRaw(signingKey($client.secretKey, $shortDate, $client.region), $stringToSign));
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

# httpOptions is the http.Options for a request of `client` with body cap
# `maxBytes` (math.MAX_INT: none); the client's timeoutMs 0 means no timeout.
func httpOptions(client as Client, maxBytes as int) {
    def o as http.Options;
    $o.timeoutMs = $client.timeoutMs;
    if ($client.timeoutMs == 0) {
        $o.timeoutMs = math.MAX_INT;
    }
    $o.maxBytes = $maxBytes;
    return $o;
}

# doRequest signs and sends one request, returning the http.Response. maxBytes
# caps the response body (math.MAX_INT lifts it for an object download).
func doRequest(
    client as Client,
    method as string,
    canonicalUri as string,
    canonicalQuery as string,
    body as string,
    maxBytes as int) {
    def host as string init requestHost($client);
    def payloadHash as string init hexDigest($body);
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    def shortDate as string init $isoDate[..8];
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
    return http.send($method, $url, $headers, $body, httpOptions($client, $maxBytes));
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
    def host as string init requestHost($client);
    def isoDate as string init time.format(time.utc(), "%Y%m%dT%H%M%SZ");
    def shortDate as string init $isoDate[..8];
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

# sendSigned signs and sends a request with a string body and extra signed
# headers, returning the http.Response (its body capped at MAX_REPLY_BYTES).
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
    return http.send(
        $method,
        requestUrl($client, $canonicalUri, $canonicalQuery),
        $headers,
        $body,
        httpOptions($client, MAX_REPLY_BYTES));
}

# sendSignedBytes signs and sends a request with a raw `bytes` body (written
# byte-for-byte) and extra signed headers, returning the http.Response (its body
# capped at MAX_REPLY_BYTES).
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
    def r as http.BytesResponse init http.sendBytes(
        $method,
        requestUrl($client, $canonicalUri, $canonicalQuery),
        $headers,
        $body,
        httpOptions($client, MAX_REPLY_BYTES));
    return textResponse($r);
}

# textResponse decodes a BytesResponse's body as UTF-8, throwing kind "http" for
# one that is not, as http's text verbs do.
func textResponse(r as http.BytesResponse) {
    def text as string init "";
    try {
        $text = encoding.decode("utf-8", $r.body);
    } catch (e) {
        throw Error{
            kind: "http",
            message: "http: response body is not valid UTF-8 (use the byte verbs)"
        };
    }
    return http.Response{
        status: $r.status,
        statusText: $r.statusText,
        headers: $r.headers,
        body: $text
    };
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
    # -1 lifts http's body cap: an object may be larger than any fixed cap, and
    # the bucket is the caller's own.
    return doRequest($client, "GET", objectPath($bucketName, $key), "", "", math.MAX_INT);
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
    return doRequest($client, "PUT", objectPath($bucketName, $key), "", $body, MAX_REPLY_BYTES);
}

/**
 * DELETE an object.
 * @param client {Client} the client
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @return {http.Response} the response (204 on success)
 */
export func delete(client as Client, bucketName as string, key as string) {
    return doRequest($client, "DELETE", objectPath($bucketName, $key), "", "", MAX_REPLY_BYTES);
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
    def none as bytes;
    return http.sendBytes(
        "GET",
        requestUrl($client, objectPath($bucketName, $key), ""),
        $headers,
        $none,
        httpOptions($client, math.MAX_INT));
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
    return doRequest($client, "HEAD", objectPath($bucketName, $key), "", "", MAX_REPLY_BYTES);
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
 * @throws {S3Error} kind "s3" for an `<Error>` body under a 200 (a copy that failed after S3 answered)
 */
export func copy(
    client as Client,
    srcBucket as string,
    srcKey as string,
    dstBucket as string,
    dstKey as string) {
    def extra as map of string to string init {};
    $extra["x-amz-copy-source"] = objectPath($srcBucket, $srcKey);
    return checkDeferredError(sendSigned(
        $client,
        "PUT",
        objectPath($dstBucket, $dstKey),
        "",
        "",
        $extra));
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
    return doRequest($client, "GET", bucketPath($bucketName), "list-type=2", "", MAX_REPLY_BYTES);
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
    return doRequest(
        $client,
        "GET",
        bucketPath($bucketName),
        canonicalizeQuery($params),
        "",
        MAX_REPLY_BYTES);
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

# unescapeXml decodes the five predefined XML entities and numeric character
# references (S3 writes a key's control characters as `&#13;` / `&#x0D;`) in
# one left-to-right pass, so a decoded `&` is never read again: `&#38;lt;` and
# `&amp;lt;` are the text `&lt;`. A reference out of range is left as is.
func unescapeXml(s as string) {
    if (strings.indexOf($s, "&") < 0) {
        return $s;
    }
    def parts as list of string init [];
    def pos as int init 0;
    for (def m in regex.findAll('&(#[xX][0-9A-Fa-f]{1,6}|#[0-9]{1,7}|lt|gt|quot|apos|amp);', $s)) {
        def text as string init entityText($m.groups[0]);
        if ($text != "") {
            $parts[] = $s[$pos..$m.start];
            $parts[] = $text;
            $pos = $m.end;
        }
    }
    $parts[] = $s[$pos..];
    return strings.join($parts, "");
}

# entityText is the character an entity name (`lt`, `#38`, `#x26`) stands for,
# or "" for a character reference out of range.
func entityText(name as string) {
    if ($name == "lt") {
        return "<";
    } elseif ($name == "gt") {
        return ">";
    } elseif ($name == "quot") {
        return "\"";
    } elseif ($name == "apos") {
        return "'";
    } elseif ($name == "amp") {
        return "&";
    }
    def cp as int init -1;
    if (strings.startsWith(strings.lower($name), "#x")) {
        $cp = convert.toInt($name[2..], 16);
    } else {
        $cp = convert.toInt($name[1..]);
    }
    if ($cp < 1 or $cp > 1114111 or ($cp >= 55296 and $cp <= 57343)) {
        return "";
    }
    return convert.fromCodepoint($cp);
}

# escapeXml escapes the three text-significant XML characters (`&` first) so a
# value placed in element text cannot break the document.
func escapeXml(s as string) {
    def out as string init strings.replace($s, "&", "&amp;");
    $out = strings.replace($out, "<", "&lt;");
    $out = strings.replace($out, ">", "&gt;");
    return $out;
}

# --- presigned URLs (exported) ----------------------------------------------

/**
 * Build a presigned URL that grants time-limited access to an object without
 * exposing the secret key, using SigV4 query-signing (`X-Amz-*` query
 * parameters). The URL works from any HTTP client until it expires. No request
 * is sent; on `jennifer-tiny`, where this module does not load (it imports
 * `http`), the `s3presign` module has the same `presign`.
 * @param client {Client} the client
 * @param method {string} the HTTP method the URL authorizes (e.g. "GET", "PUT")
 * @param bucketName {string} the bucket
 * @param key {string} the object key
 * @param expiresSeconds {int} the validity window in seconds (1 to 604800 = 7 days)
 * @return {string} the presigned URL
 * @throws {S3Error} kind "s3" for an expiry outside 1..604800, a bad bucket name or an empty key
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
 * @throws {S3Error} kind "s3" when S3 refuses the request (its `Code` and status) or the response carries no UploadId
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
    if ($resp.status < 200 or $resp.status >= 300) {
        throw errorOf($resp);
    }
    def m as regex.Match init regex.find("<UploadId>([^<]*)</UploadId>", $resp.body);
    if ($m.start == -1) {
        def e as S3Error init errorOf($resp);
        $e.message = "s3: createMultipartUpload: no UploadId (status {$resp.status})";
        throw $e;
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
 * @throws {S3Error} kind "s3" when S3 refuses the part (`Code`, status) or sends no ETag
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
    if ($resp.status < 200 or $resp.status >= 300) {
        throw errorOf($resp);
    }
    def etag as string init http.header($resp, "ETag");
    if ($etag == "") {
        def e as S3Error init errorOf($resp);
        $e.message = "s3: uploadPart: no ETag (status {$resp.status})";
        throw $e;
    }
    return $etag;
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
 * @throws {S3Error} kind "s3" for an `<Error>` body under a 200 (a failure after S3 answered)
 */
export func completeMultipartUpload(
    client as Client,
    bucketName as string,
    key as string,
    uploadId as string,
    etags as list of string) {
    def params as map of string to string init {};
    $params["uploadId"] = $uploadId;
    return checkDeferredError(sendSigned(
        $client,
        "POST",
        objectPath($bucketName, $key),
        canonicalizeQuery($params),
        completeXml($etags),
        {}));
}

# completeXml renders the CompleteMultipartUpload request body from part ETags.
func completeXml(etags as list of string) {
    # Part elements collect and join once: growing `parts` with `+` per part is
    # O(N^2) over a large multipart upload.
    def parts as list of string init [];
    for (def i in 0..len($etags)) {
        $parts[] = "<Part><PartNumber>{$i + 1}</PartNumber><ETag>" + escapeXml($etags[$i]) +
            "</ETag></Part>";
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
