# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# bucket_test.j - white-box tests for bucket.j's SigV4 signing + canonicalization.
# Run with:
#
#     jennifer test modules/bucket_test.j
#
# The overlay splices bucket.j in first, so these tests reach its private helpers
# (authorization, hostOf, uriEncodePath, signingKey) by bare identifier. The
# networked get / put / delete / list are verified against an in-process S3-shaped
# server in the Go suite (TestBucketRequests). bucket.j already `use`s hash /
# encoding / convert / time / regex / strings / lists, so the overlay adds testing.
# The signature vector is cross-checked against an independent SigV4 implementation.
use testing;

func testAuthorizationVector() {
    def c as Client init connect("https://examplebucket.s3.amazonaws.com", "us-east-1",
        "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY");
    def auth as string init authorization($c, "GET", "examplebucket.s3.amazonaws.com",
        "/test.txt", "", hexDigest(""), "20130524T000000Z", "20130524");
    testing.assertEqual($auth,
        "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=df548e2ce037944d03f3e68682813b093763996d597cf890ca3d9037fd231eb4");
}

func testConnectDefaults() {
    def c as Client init connect("https://s3.amazonaws.com", "us-east-1", "k", "s");
    testing.assertEqual($c.timeout, 30000);   # a hung endpoint fails, not hangs
    testing.assertEqual($c.region, "us-east-1");
}

func testHexDigestEmpty() {
    testing.assertEqual(hexDigest(""),
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
}

func testSigningKeyWidth() {
    testing.assertEqual(len(signingKey("sk", "20130524", "us-east-1")), 32);
}

func testHostOf() {
    testing.assertEqual(hostOf("https://s3.amazonaws.com"), "s3.amazonaws.com");        # default https port omitted
    testing.assertEqual(hostOf("https://s3.amazonaws.com:443"), "s3.amazonaws.com");     # explicit default omitted
    testing.assertEqual(hostOf("http://example.com"), "example.com");                    # default http port omitted
    testing.assertEqual(hostOf("http://localhost:9000"), "localhost:9000");              # non-default port kept
    testing.assertEqual(hostOf("https://minio.example.com:9000"), "minio.example.com:9000");
}

func testUriEncodePath() {
    testing.assertEqual(uriEncodePath("simple.txt"), "simple.txt");
    testing.assertEqual(uriEncodePath("a/b/c.txt"), "a/b/c.txt");         # slashes kept
    testing.assertEqual(uriEncodePath("my file.txt"), "my%20file.txt");   # space encoded
    testing.assertEqual(uriEncodePath("a+b&c"), "a%2Bb%26c");             # + and & encoded
    testing.assertEqual(uriEncodePath("na~me-1.0_x"), "na~me-1.0_x");     # unreserved kept
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
