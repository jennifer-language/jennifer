# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# s3presign_test.j - white-box tests for s3presign.j (+ the spliced
# s3_sigv4.inc.j it shares with s3.j). Run with:
#
#     jennifer test modules/s3presign_test.j
#
# The overlay splices s3presign.j in first, so these tests reach presignAt and
# the include's helpers by bare identifier. The vectors are the ones s3_test.j
# checks against an independent (Python) SigV4 implementation, so the two
# modules sign identically. No network: these run on jennifer-tiny-dev too.
use testing;

func sampleClient() {
    return connect(
        "https://s3.amazonaws.com",
        "us-east-1",
        "AKIAIOSFODNN7EXAMPLE",
        "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY");
}

func testPresignVector() {
    def url as string init presignAt(
        sampleClient(),
        "GET",
        "examplebucket",
        "test.txt",
        86400,
        "20130524T000000Z");
    testing.assertEqual(
        $url,
        "https://s3.amazonaws.com/examplebucket/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=733255ef022bec3f2a8701cd61d4b371f3f28c9f193a1f02279211d48d5193d7");
}

func testPresignSpecialKey() {
    def c as Client init connect(
        "https://s3.amazonaws.com",
        "us-east-1",
        "AKIDEXAMPLE",
        "test-secret-key");
    def url as string init presignAt(
        $c,
        "GET",
        "my-bucket",
        "reports/2026 Q1 (final)+draft.pdf",
        900,
        "20260101T000000Z");
    testing.assertEqual(
        $url,
        "https://s3.amazonaws.com/my-bucket/reports/2026%20Q1%20%28final%29%2Bdraft.pdf?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIDEXAMPLE%2F20260101%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20260101T000000Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=ac48b447297c626a38786a3f6abcea0131bf79994f8e486df50dba5383c28c7a");
}

# presign stamps the current time; the URL carries the expiry and a signature.
func testPresignNow() {
    def url as string init presign(sampleClient(), "PUT", "b", "dir/k", 604800);
    testing.assertContains(
        $url,
        "https://s3.amazonaws.com/b/dir/k?X-Amz-Algorithm=AWS4-HMAC-SHA256");
    testing.assertContains($url, "X-Amz-Expires=604800");
    testing.assertContains($url, "&X-Amz-Signature=");
}

# connect only validates and builds the value, with s3.connect's defaults.
func testConnectDefaults() {
    def c as Client init connect("http://127.0.0.1:9000/", "r", "k", "s");
    testing.assertEqual($c.endpoint, "http://127.0.0.1:9000");
    testing.assertEqual($c.timeoutMs, 30000);
    testing.assertFalse($c.allowInsecureHttp);
}

func badEndpoint() {
    connect("https://s3.example.com/base", "r", "k", "s");
}
func presignTooLong() {
    presign(sampleClient(), "GET", "b", "k", 604801);
}
func presignZero() {
    presign(sampleClient(), "GET", "b", "k", 0);
}
func presignBadBucket() {
    presign(sampleClient(), "GET", "a/b", "k", 60);
}
func presignEmptyKey() {
    presign(sampleClient(), "GET", "b", "", 60);
}

func testRefusals() {
    testing.assertThrows("badEndpoint", "s3");
    testing.assertThrows("presignTooLong", "s3");
    testing.assertThrows("presignZero", "s3");
    testing.assertThrows("presignBadBucket", "s3");
    testing.assertThrows("presignEmptyKey", "s3");
}

# Every refusal is this module's S3Error, a local check (status 0).
func testRefusalIsS3Error() {
    def e as S3Error init S3Error{kind: "", message: "", status: -1, code: "", requestId: ""};
    try {
        presignZero();
    } catch (err) {
        $e = $err;
    }
    testing.assertEqual($e.kind, "s3");
    testing.assertEqual($e.status, 0);
}
