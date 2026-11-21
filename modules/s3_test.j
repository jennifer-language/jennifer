# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# s3_test.j - white-box tests for s3.j's SigV4 signing + canonicalization.
# Run with:
#
#     jennifer test modules/s3_test.j
#
# The overlay splices s3.j in first, so these tests reach its private helpers
# (authorization, hostOf, uriEncodePath, signingKey) by bare identifier. The
# networked get / put / delete / list are verified against an in-process S3-shaped
# server in the Go suite (TestS3Requests); the endpoint, transport and reply
# checks run here against a loopback httpd. s3.j already `use`s hash /
# encoding / convert / time / regex / strings / lists, so the overlay adds testing.
# The signature vector is cross-checked against an independent SigV4 implementation.
use encoding;
use testing;
use maps;
use httpd;
use task;

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
    testing.assertEqual($c.timeoutMs, 30000); # a hung endpoint fails, not hangs
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

# --- presign, byte bodies, metadata, multipart ---

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

# presignAt for a key with a space, parens and `+` - the path is encoded (slashes
# kept) and the whole URL matches the independent (Python) SigV4 reference.
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
    def b as bytes init encoding.encode("utf-8", "hi there");
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
    def h as map of string to string init prepareHeaders(
        sampleClient(),
        "GET",
        "/b/k",
        "",
        hexDigest(""),
        {});
    testing.assertContains(
        $h["Authorization"],
        "SignedHeaders=host;x-amz-content-sha256;x-amz-date");
    testing.assertTrue(maps.has($h, "x-amz-date"));
    testing.assertTrue(maps.has($h, "x-amz-content-sha256"));
}

# prepareHeaders adds an x-amz-meta header to the signed set (sorted in).
func testPrepareHeadersSignsMetadata() {
    def extra as map of string to string init {};
    $extra["x-amz-meta-team"] = "eng";
    def h as map of string to string init prepareHeaders(
        sampleClient(),
        "PUT",
        "/b/k",
        "",
        hexDigest(""),
        $extra);
    testing.assertContains(
        $h["Authorization"],
        "SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-meta-team");
    testing.assertEqual($h["x-amz-meta-team"], "eng");
}

func badBucketSlash() {
    return objectPath("b/../other", "k");
}
func badBucketQuery() {
    return bucketPath("b?x=1");
}
func emptyKey() {
    return objectPath("b", "");
}
# A bucket name goes into the path unencoded, so path characters are refused;
# an empty key would address the bucket itself.
func testBucketAndKeyGuards() {
    testing.assertThrows("badBucketSlash", "s3");
    testing.assertThrows("badBucketQuery", "s3");
    testing.assertThrows("emptyKey", "s3");
    testing.assertEqual(bucketPath("Legacy_Bucket.1"), "/Legacy_Bucket.1");
}

# Numeric character references in a listing decode; &amp; stays one level.
func testObjectKeysCharRefs() {
    def xml as string init "<Key>a&#13;b</Key><Key>c&#x41;</Key><Key>&amp;#13;</Key><Key>bad&#0;</Key>";
    testing.assertEqual(objectKeys($xml), ["a\rb", "cA", "&#13;", "bad&#0;"]);
}

# A decoded "&" is never decoded again: a key that is the text "&lt;" stays it,
# however the server wrote its "&".
func testUnescapeXmlOnePass() {
    testing.assertEqual(unescapeXml("&#38;lt;"), "&lt;");
    testing.assertEqual(unescapeXml("&#x26;amp;"), "&amp;");
    testing.assertEqual(unescapeXml("&amp;lt;"), "&lt;");
    testing.assertEqual(unescapeXml("a&lt;b&gt;&quot;&apos;&#X41;"), "a<b>\"'A");
    testing.assertEqual(unescapeXml("&#55296;&nope;&"), "&#55296;&nope;&");
}

# errorOf reads S3's XML error body; a body without one gives the status alone.
func testErrorOfReadsTheXmlError() {
    def r as http.Response init http.Response{
        status: 404,
        statusText: "Not Found",
        headers: {},
        body: '<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code>' +
            '<Message>The specified key does not exist.</Message><Key>cat.jpg</Key>' +
            '<RequestId>4442587FB7D0A2F9</RequestId></Error>'
    };
    def e as S3Error init errorOf($r);
    testing.assertEqual($e.kind, "s3");
    testing.assertEqual($e.status, 404);
    testing.assertEqual($e.code, "NoSuchKey");
    testing.assertEqual($e.requestId, "4442587FB7D0A2F9");
    testing.assertEqual($e.message, "s3: HTTP 404: NoSuchKey: The specified key does not exist.");
    $r.status = 403;
    $r.body = "";
    testing.assertEqual(errorOf($r).message, "s3: HTTP 403");
    testing.assertEqual(errorOf($r).code, "");
    testing.assertThrows("badBucket", "s3");
}

func badBucket() {
    bucketPath("a/b");
}

# An expiry outside SigV4's 1 s .. 7 days is refused when the URL is built.
func presignTooLong() {
    def c as Client init connect("https://s3.example.com", "us-east-1", "AK", "SK");
    return presign($c, "GET", "b", "k", 604801);
}
func presignZero() {
    def c as Client init connect("https://s3.example.com", "us-east-1", "AK", "SK");
    return presign($c, "GET", "b", "k", 0);
}
func testPresignExpiryRange() {
    testing.assertThrows("presignTooLong", "s3");
    testing.assertThrows("presignZero", "s3");
    def c as Client init connect("https://s3.example.com", "us-east-1", "AK", "SK");
    testing.assertTrue(strings.contains(
        presign($c, "GET", "b", "k", 604800),
        "X-Amz-Expires=604800"));
}

# --- endpoint, transport and reply checks (against a loopback httpd) -------

# A canned reply: status, headers, body.
def struct Canned {
    status as int,
    headers as map of string to string,
    body as string
};

# A running fake server and the task that answers the canned replies.
def struct Fake {
    srv as httpd.Server,
    url as string,
    t as task of int
};

func serveCanned(srv as httpd.Server, replies as list of Canned) {
    for (def r in $replies) {
        def req as httpd.Request init httpd.accept($srv);
        for (def name in maps.keys($r.headers)) {
            httpd.setHeader($req, $name, $r.headers[$name]);
        }
        httpd.respond($req, $r.status, $r.body);
    }
    return len($replies);
}

func fake(replies as list of Canned) {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    def t as task of int init spawn {
        return serveCanned($srv, $replies);
    };
    return Fake{srv: $srv, url: "http://" + httpd.address($srv), t: $t};
}

func finish(f as Fake) {
    task.wait($f.t);
    httpd.shutdown($f.srv);
}

# thrown runs one failing call against a fake answering `reply` and returns
# the S3Error it threw (an "unthrown" one when the call succeeded).
func thrown(op as string, reply as Canned) {
    def f as Fake init fake([$reply]);
    def c as Client init connect($f.url, "us-east-1", "AK", "SK");
    def data as bytes init encoding.encode("utf-8", "part");
    def got as S3Error init S3Error{
        kind: "unthrown",
        message: "",
        status: 0,
        code: "",
        requestId: ""
    };
    try {
        if ($op == "uploadPart") {
            uploadPart($c, "b", "k", "UID", 1, $data);
        } elseif ($op == "complete") {
            completeMultipartUpload($c, "b", "k", "UID", ["\"e1\""]);
        } else {
            copy($c, "b", "src", "b", "dst");
        }
    } catch (e) {
        $got = $e;
    }
    finish($f);
    return $got;
}

func testConnectValidatesEndpoint() {
    # One trailing "/" is dropped, so the signed path equals the sent one.
    testing.assertEqual(
        connect("http://127.0.0.1:9000/", "r", "k", "s").endpoint,
        "http://127.0.0.1:9000");
    # An IPv6 endpoint keeps its brackets in the signed Host.
    testing.assertEqual(hostOf("http://[::1]:9000"), "[::1]:9000");
    testing.assertEqual(hostOf("https://[2001:db8::1]"), "[2001:db8::1]");
    testing.assertEqual(hostOf("HTTPS://s3.example.com:443"), "s3.example.com");
    for (def bad in [
        "https://s3.example.com/base",
        "https://s3.example.com//",
        "https://s3.example.com?x=1",
        "https://s3.example.com#f",
        "https://user:pw@s3.example.com",
        "ftp://s3.example.com",
        "s3.example.com:9000",
        "https://s3.example.com:",
        "https://s3.example.com:0",
        "https://s3.example.com:65536",
        "https://[::1",
        "https://",
        ""
    ]) {
        def kind as string init "";
        try {
            connect($bad, "r", "k", "s");
        } catch (e) {
            $kind = $e.kind;
        }
        testing.assertEqual($kind, "s3");
    }
}

func testPlainHttpToRemoteHostIsRefused() {
    # s3.invalid never resolves: an "s3" kind proves the refusal came before a dial.
    def c as Client init connect("http://s3.invalid:9000", "us-east-1", "AK", "SK");
    def kinds as list of string init [];
    for (def op in ["put", "putBytes", "getBytes"]) {
        try {
            if ($op == "put") {
                put($c, "b", "k", "x");
            } elseif ($op == "putBytes") {
                putBytes($c, "b", "k", encoding.encode("utf-8", "x"));
            } else {
                getBytes($c, "b", "k");
            }
        } catch (e) {
            $kinds[] = $e.kind;
        }
    }
    testing.assertEqual($kinds, ["s3", "s3", "s3"]);
    # The opt-in lets the request through to the (failing) dial.
    $c.allowInsecureHttp = true;
    def kind as string init "";
    try {
        put($c, "b", "k", "x");
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertTrue($kind != "s3" and $kind != "");
    # A presigned URL sends nothing, so it is still built.
    $c.allowInsecureHttp = false;
    testing.assertTrue(strings.startsWith(
        presign($c, "GET", "b", "k", 60),
        "http://s3.invalid:9000/b/k?"));
}

func testUploadPartChecksTheStatus() {
    def e as S3Error init thrown(
        "uploadPart",
        Canned{status: 403, headers: {}, body: "<Error><Code>AccessDenied</Code></Error>"});
    testing.assertEqual($e.kind, "s3");
    testing.assertEqual($e.status, 403);
    testing.assertEqual($e.code, "AccessDenied");
    # A 200 without an ETag cannot be completed later.
    testing.assertEqual(
        thrown("uploadPart", Canned{status: 200, headers: {}, body: ""}).kind,
        "s3");
}

func testErrorUnderA200IsThrown() {
    def errBody as string init '<?xml version="1.0" encoding="UTF-8"?>' + "\n  \n" +
        '<Error><Code>InternalError</Code>' +
        '<Message>We encountered an internal error.</Message><RequestId>R1</RequestId></Error>';
    def e as S3Error init thrown("complete", Canned{status: 200, headers: {}, body: $errBody});
    testing.assertEqual($e.kind, "s3");
    testing.assertEqual($e.status, 200);
    testing.assertEqual($e.code, "InternalError");
    testing.assertEqual($e.requestId, "R1");
    testing.assertEqual(
        thrown("copy", Canned{status: 200, headers: {}, body: $errBody}).code,
        "InternalError");
    # A real result passes.
    def done as string init "<CompleteMultipartUploadResult><ETag>x</ETag>" +
        "</CompleteMultipartUploadResult>";
    testing.assertEqual(
        thrown("complete", Canned{status: 200, headers: {}, body: $done}).kind,
        "unthrown");
}

func testListReplyIsCapped() {
    def f as Fake init fake([
        Canned{status: 200, headers: {}, body: strings.replicate("x", MAX_REPLY_BYTES + 1)}
    ]);
    def c as Client init connect($f.url, "us-east-1", "AK", "SK");
    def kind as string init "";
    try {
        listObjects($c, "b");
    } catch (e) {
        $kind = $e.kind;
    }
    finish($f);
    testing.assertEqual($kind, "limit");
}
