# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.28.0

/**
 * Presigned URLs for S3-compatible object storage (AWS S3 / MinIO / R2 / B2),
 * with no network: the local half of the `s3` module. A presigned URL grants
 * one HTTP method on one object for a limited time, signed with AWS Signature
 * Version 4 query parameters, so any HTTP client (a browser, `curl`, another
 * service) can use it without the secret key. Building one sends nothing, and
 * this module imports no network module, so it runs on **both** binaries.
 *
 * It is a subset of `s3` with the same names: `Client`, `connect`, `S3Error`
 * and `presign` behave as they do there, and share its signing code, so a
 * program that only presigns moves between the two by changing the import.
 * `connect` only validates the endpoint and builds the value; nothing is
 * contacted.
 * @module s3presign
 * @example
 * use os;
 * import "s3presign.j" as s3;
 * def c as s3.Client init s3.connect("https://s3.amazonaws.com", "us-east-1",
 *     os.getEnv("S3_KEY"), os.getEnv("S3_SECRET"));
 * def url as string init s3.presign($c, "GET", "mybucket", "report.pdf", 3600);
 */
use hash;
use encoding;
use convert;
use time;
use regex;
use strings;
use lists;

# The SigV4 core (signing key, canonical URI / query, endpoint and bucket
# checks, presignAt), shared with s3.j.
include "./s3_sigv4.inc.j";

# The timeout `connect` sets, as `s3.connect` does (unused here).
def const DEFAULT_TIMEOUT_MS as int init 30000;

/**
 * An S3 endpoint and its credentials: the endpoint (scheme + host, no trailing
 * slash), the signing region, and the access-key pair. The same shape as
 * `s3.Client`; `timeoutMs` and `allowInsecureHttp` only matter to `s3`'s
 * requests. Value-semantic; build with `connect`.
 * @field endpoint {string} e.g. "https://s3.us-east-1.amazonaws.com" or "http://localhost:9000"
 * @field region {string} the signing region, e.g. "us-east-1"
 * @field accessKey {string} the access key id
 * @field secretKey {string} the secret access key
 * @field timeoutMs {int} per-request timeout of `s3`'s requests (unused here); `connect` sets 30000
 * @field allowInsecureHttp {bool} allow plain `http://` to a non-loopback host in `s3`'s requests (unused here)
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
 * A failed presign: a malformed endpoint, bucket name, key or expiry. The same
 * shape as `s3.S3Error`; every error of kind `"s3"` this module throws is one.
 * @field kind {string} always `"s3"`
 * @field message {string} what went wrong
 * @field status {int} always 0 (a local check)
 * @field code {string} always ""
 * @field requestId {string} always ""
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

/**
 * Build a client value for an S3 endpoint, as `s3.connect` does: the endpoint
 * is `http://` or `https://` plus a host (a name, an IPv4 address or a
 * bracketed IPv6 address) and an optional port; one trailing `/` is dropped. A
 * path, a query, a fragment, userinfo or a malformed host or port throws.
 * Nothing is contacted.
 * @param endpoint {string} the base URL
 * @param region {string} the signing region
 * @param accessKey {string} the access key id
 * @param secretKey {string} the secret access key
 * @return {Client} the client value
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
 * Build a presigned URL that grants time-limited access to an object without
 * exposing the secret key, using SigV4 query-signing (`X-Amz-*` query
 * parameters) stamped with the current time. The URL works from any HTTP
 * client until it expires.
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
