# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# s3_test.j - white-box tests for s3.j's SigV4 signing + canonicalization.
# Run with:
#
#     jennifer test modules/s3_test.j
#
# The overlay splices s3.j in first, so these tests reach its private helpers
# (authorization, hostOf, uriEncodePath, signingKey) by bare identifier. The
# networked get / put / delete / list are verified against an in-process S3-shaped
# server in the Go suite (TestS3Requests). s3.j already `use`s hash /
# encoding / convert / time / regex / strings / lists, so the overlay adds testing.
# The signature vector is cross-checked against an independent SigV4 implementation.
use testing;
use maps;

func testAuthorizationVector() {
    def c as Client init connect(
        "https://examplebucket.s3.amazonaws.com",
        "us-east-1",
        "AKIAIOSFODNN7EXAMPLE",
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY");
    def auth as string init authorization(
        $c,
        "GET",
        "examplebucket.s3.amazonaws.com",
        "/test.txt",
        "",
        hexDigest(""),
        "20130524T000000Z",
        "20130524");
    testing.assertEqual(
        $auth,
        "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=df548e2ce037944d03f3e68682813b093763996d597cf890ca3d9037fd231eb4");
}

func testConnectDefaults() {
    def c as Client init connect("https://s3.amazonaws.com", "us-east-1", "k", "s");
    testing.assertEqual($c.timeout, 30000); # a hung endpoint fails, not hangs
    testing.assertEqual($c.region, "us-east-1");
}

func testHexDigestEmpty() {
    testing.assertEqual(
        hexDigest(""),
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
}

func testSigningKeyWidth() {
    testing.assertEqual(len(signingKey("sk", "20130524", "us-east-1")), 32);
}

func testHostOf() {
    testing.assertEqual(hostOf("https://s3.amazonaws.com"), "s3.amazonaws.com"); # default https port omitted
    testing.assertEqual(hostOf("https://s3.amazonaws.com:443"), "s3.amazonaws.com"); # explicit default omitted
    testing.assertEqual(hostOf("http://example.com"), "example.com"); # default http port omitted
    testing.assertEqual(hostOf("http://localhost:9000"), "localhost:9000"); # non-default port kept
    testing.assertEqual(hostOf("https://minio.example.com:9000"), "minio.example.com:9000");
}

func testUriEncodePath() {
    testing.assertEqual(uriEncodePath("simple.txt"), "simple.txt");
    testing.assertEqual(uriEncodePath("a/b/c.txt"), "a/b/c.txt"); # slashes kept
    testing.assertEqual(uriEncodePath("my file.txt"), "my%20file.txt"); # space encoded
    testing.assertEqual(uriEncodePath("a+b&c"), "a%2Bb%26c"); # + and & encoded
    testing.assertEqual(uriEncodePath("na~me-1.0_x"), "na~me-1.0_x"); # unreserved kept
}

func testObjectPath() {
    testing.assertEqual(objectPath("mybucket", "path/to/obj.txt"), "/mybucket/path/to/obj.txt");
    testing.assertEqual(objectPath("b", "a b.txt"), "/b/a%20b.txt");
}

func testObjectKeys() {
    def xml as string init "<ListBucketResult><Contents><Key>a.txt</Key></Contents><Contents><Key>dir/b.txt</Key></Contents></ListBucketResult>";
    def keys as list of string init objectKeys($xml);
    testing.assertEqual(len($keys), 2);
    testing.assertEqual($keys[0], "a.txt");
    testing.assertEqual($keys[1], "dir/b.txt");
    testing.assertEqual(len(objectKeys("<ListBucketResult></ListBucketResult>")), 0);
}

# XML-escaped keys are decoded so they round-trip back into get / delete.
func testObjectKeysDecodesEntities() {
    def xml as string init "<ListBucketResult><Contents><Key>reports&amp;data.txt</Key></Contents><Contents><Key>a&lt;b&gt;c.txt</Key></Contents></ListBucketResult>";
    def keys as list of string init objectKeys($xml);
    testing.assertEqual($keys[0], "reports&data.txt");
    testing.assertEqual($keys[1], "a<b>c.txt");
}

# --- M23.6: presign, byte bodies, metadata, multipart ---

# A configured client for the deterministic signing tests.
func sampleClient() {
    return connect(
        "https://s3.amazonaws.com",
        "us-east-1",
        "AKIAIOSFODNN7EXAMPLE",
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY");
}

# presignAt is cross-checked against an independent (Python) SigV4 query-signing
# implementation for these exact path-style inputs.
func testPresignVector() {
    def url as string init presignAt(
        sampleClient(), "GET", "examplebucket", "test.txt", 86400, "20130524T000000Z");
    testing.assertEqual(
        $url,
        "https://s3.amazonaws.com/examplebucket/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=733255ef022bec3f2a8701cd61d4b371f3f28c9f193a1f02279211d48d5193d7");
}

# presignAt for a key with a space, parens and `+` - the path is encoded (slashes
# kept) and the whole URL matches the independent (Python) SigV4 reference.
func testPresignSpecialKey() {
    def c as Client init connect("https://s3.amazonaws.com", "us-east-1", "AKIDEXAMPLE", "test-secret-key");
    def url as string init presignAt($c, "GET", "my-bucket", "reports/2026 Q1 (final)+draft.pdf", 900, "20260101T000000Z");
    testing.assertEqual(
        $url,
        "https://s3.amazonaws.com/my-bucket/reports/2026%20Q1%20%28final%29%2Bdraft.pdf?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIDEXAMPLE%2F20260101%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20260101T000000Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=ac48b447297c626a38786a3f6abcea0131bf79994f8e486df50dba5383c28c7a");
}

# The SigV4 canonical header value collapses internal space runs (so a metadata
# value with double spaces signs the way S3 re-derives it).
func testCanonicalHeaderValueCollapse() {
    testing.assertEqual(canonicalHeaderValue("  a  b   c  "), "a b c");
    testing.assertEqual(canonicalHeaderValue("plain"), "plain");
}

func testUriEncodeStrict() {
    testing.assertEqual(uriEncodeStrict("a/b c+d"), "a%2Fb%20c%2Bd"); # slash encoded too
    testing.assertEqual(uriEncodePath("a/b c"), "a/b%20c"); # path keeps slashes
    testing.assertEqual(uriEncodeStrict("na~me-1.0_x"), "na~me-1.0_x"); # unreserved kept
}

# The canonical query must be sorted by key ("continuation-token" < "list-type"),
# which is the page-2 pagination signing correctness.
func testCanonicalizeQuery() {
    def q as map of string to string init {};
    $q["list-type"] = "2";
    $q["continuation-token"] = "a/b+c";
    testing.assertEqual(canonicalizeQuery($q), "continuation-token=a%2Fb%2Bc&list-type=2");
    def one as map of string to string init {};
    $one["uploads"] = "";
    testing.assertEqual(canonicalizeQuery($one), "uploads="); # empty value keeps the =
}

func testHexDigestBytesMatchesString() {
    def b as bytes init convert.bytesFromString("hi there", "utf-8");
    testing.assertEqual(hexDigestBytes($b), hexDigest("hi there"));
}

func testCompleteXml() {
    def etags as list of string init ["\"aaa\"", "\"bbb\""];
    testing.assertEqual(
        completeXml($etags),
        "<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>\"aaa\"</ETag></Part><Part><PartNumber>2</PartNumber><ETag>\"bbb\"</ETag></Part></CompleteMultipartUpload>");
}

func testMetadataHeaders() {
    def md as map of string to string init {};
    $md["author"] = "jane";
    def extra as map of string to string init metadataHeaders("text/html", $md);
    testing.assertEqual($extra["Content-Type"], "text/html");
    testing.assertEqual($extra["x-amz-meta-author"], "jane");
    # empty content type is omitted (only metadata remains)
    def only as map of string to string init metadataHeaders("", $md);
    testing.assertFalse(maps.has($only, "Content-Type"));
}

# prepareHeaders with no extras signs the same fixed header set as authorization.
func testPrepareHeadersFixedSet() {
    def h as map of string to string init prepareHeaders(sampleClient(), "GET", "/b/k", "", hexDigest(""), {});
    testing.assertContains($h["Authorization"], "SignedHeaders=host;x-amz-content-sha256;x-amz-date");
    testing.assertTrue(maps.has($h, "x-amz-date"));
    testing.assertTrue(maps.has($h, "x-amz-content-sha256"));
}

# prepareHeaders adds an x-amz-meta header to the signed set (sorted in).
func testPrepareHeadersSignsMetadata() {
    def extra as map of string to string init {};
    $extra["x-amz-meta-team"] = "eng";
    def h as map of string to string init prepareHeaders(sampleClient(), "PUT", "/b/k", "", hexDigest(""), $extra);
    testing.assertContains($h["Authorization"], "SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-meta-team");
    testing.assertEqual($h["x-amz-meta-team"], "eng");
}
