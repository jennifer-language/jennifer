# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# webdav_test.j - white-box tests for webdav.j. Run with:
#
#     jennifer test modules/webdav_test.j
#
# The overlay splices webdav.j in first, so it reaches the private helpers by
# bare identifier. The protocol layer is tested on its own (paths, Clark names,
# the multistatus codec against Nextcloud- and Apache-style replies); the client
# runs against an `httpd` server on a loopback port that answers canned replies
# in order and hands back what it received.
use httpd;
use task;
use testing;

# A canned reply.
def struct Reply {
    status as int,
    headers as map of string to string,
    body as string
};

# One request as the fake server saw it.
def struct Seen {
    method as string,
    path as string,
    headers as map of string to string,
    body as string
};

# A running fake server.
def struct Fake {
    srv as httpd.Server,
    url as string,
    t as task of list of Seen
};

# The request headers the tests look at.
def const WATCHED as list of string init [
    "Authorization",
    "Depth",
    "Destination",
    "Overwrite",
    "If-Match",
    "If-None-Match",
    "Content-Type",
    "X-Extra"
];

func serve(srv as httpd.Server, replies as list of Reply) {
    def seen as list of Seen init [];
    for (def r in $replies) {
        def req as httpd.Request init httpd.accept($srv);
        def headers as map of string to string init {};
        for (def name in WATCHED) {
            def value as string init httpd.header($req, $name);
            if ($value != "") {
                $headers[$name] = $value;
            }
        }
        $seen[] = Seen{
            method: httpd.method($req),
            path: httpd.path($req),
            headers: $headers,
            body: convert.stringFromBytes("utf-8", httpd.body($req))
        };
        for (def name in maps.keys($r.headers)) {
            httpd.setHeader($req, $name, $r.headers[$name]);
        }
        httpd.respond($req, $r.status, $r.body);
    }
    return $seen;
}

func fake(replies as list of Reply) {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    def t as task of list of Seen init spawn {
        return serve($srv, $replies);
    };
    return Fake{srv: $srv, url: "http://" + httpd.address($srv), t: $t};
}

func finish(f as Fake) {
    def seen as list of Seen init task.wait($f.t);
    httpd.shutdown($f.srv);
    return $seen;
}

func reply(status as int, body as string) {
    return Reply{status: $status, headers: {}, body: $body};
}

func missed() {
    throw Error{kind: "test", message: "the call did not fail"};
}

func messageOf(e as WebdavError) {
    testing.assertEqual($e.kind, ERROR_KIND);
    return $e.message;
}

# A resource for the encoder.
func res(href as string, isDir as bool, size as int) {
    def r as Resource init emptyResource($href);
    $r.isDir = $isDir;
    $r.size = $size;
    $r.modified = 1793797509;
    $r.etag = "\"e-" + convert.toString($size) + "\"";
    return $r;
}

# What Nextcloud answers a Depth 1 PROPFIND: `d:` prefix, the oc namespace, a
# 404 propstat for what a file lacks.
def const NEXTCLOUD as string init '<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">
 <d:response>
  <d:href>/remote.php/dav/files/alice/My%20Docs/</d:href>
  <d:propstat>
   <d:prop>
    <d:getlastmodified>Wed, 04 Nov 2026 13:05:09 GMT</d:getlastmodified>
    <d:resourcetype><d:collection/></d:resourcetype>
    <d:quota-used-bytes>1024</d:quota-used-bytes>
    <d:quota-available-bytes>-3</d:quota-available-bytes>
    <d:getetag>"6909"</d:getetag>
   </d:prop>
   <d:status>HTTP/1.1 200 OK</d:status>
  </d:propstat>
  <d:propstat>
   <d:prop><d:getcontentlength/><d:getcontenttype/></d:prop>
   <d:status>HTTP/1.1 404 Not Found</d:status>
  </d:propstat>
 </d:response>
 <d:response>
  <d:href>/remote.php/dav/files/alice/My%20Docs/r%C3%A9sum%C3%A9.pdf</d:href>
  <d:propstat>
   <d:prop>
    <d:getlastmodified>Tue, 03 Nov 2026 08:00:00 GMT</d:getlastmodified>
    <d:getcontentlength>20480</d:getcontentlength>
    <d:resourcetype/>
    <d:getetag>"abc123"</d:getetag>
    <d:getcontenttype>application/pdf</d:getcontenttype>
    <oc:fileid>42</oc:fileid>
    <oc:checksums><oc:checksum>SHA1:da39</oc:checksum></oc:checksums>
   </d:prop>
   <d:status>HTTP/1.1 200 OK</d:status>
  </d:propstat>
 </d:response>
</d:multistatus>';

# What Apache mod_dav answers: the DAV: namespace as the default, its own
# prefixes for the rest, absolute URLs as hrefs, and a member that is gone.
def const APACHE as string init '<?xml version="1.0" encoding="utf-8"?>
<multistatus xmlns="DAV:" xmlns:ns0="DAV:">
<response xmlns:lp1="DAV:" xmlns:lp2="http://apache.org/dav/props/">
<href>http://dav.example.com/share/a%20b.txt</href>
<propstat>
<prop>
<lp1:resourcetype/>
<lp1:getcontentlength>5</lp1:getcontentlength>
<lp1:getlastmodified>Wed, 04 Nov 2026 13:05:09 GMT</lp1:getlastmodified>
<lp2:executable>F</lp2:executable>
</prop>
<status>HTTP/1.1 200 OK</status>
</propstat>
</response>
<response><href>/share/gone.txt</href><status>HTTP/1.1 404 Not Found</status></response>
</multistatus>';

func testPaths() {
    testing.assertEqual(cleanPath(""), "/");
    testing.assertEqual(cleanPath("/"), "/");
    testing.assertEqual(cleanPath("/a/b/"), "/a/b");
    for (def bad in ["a", "/a/../b", "/a//b", "/./a", "/a\nb"]) {
        try {
            cleanPath($bad);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "a path");
        }
    }
    testing.assertEqual(encodeHref("/My Docs/a+b.txt", false), "/My%20Docs/a%2Bb.txt");
    testing.assertEqual(encodeHref("/My Docs", true), "/My%20Docs/");
    testing.assertEqual(encodeHref("/", true), "/");
    testing.assertEqual(decodeHref("/My%20Docs/"), "/My Docs/");
    testing.assertEqual(decodeHref("https://host:8443/a%2Fb"), "/a/b");
    testing.assertEqual(decodeHref("http://host"), "/");
}

func testClarkNames() {
    testing.assertEqual(clark(DAV_NS, "getetag"), "\{DAV:\}getetag");
    testing.assertEqual(
        clarkParts("\{http://owncloud.org/ns\}fileid"),
        ["http://owncloud.org/ns", "fileid"]);
    try {
        clarkParts("getetag");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "a property name is \{namespace\}local");
    }
}

func testParseNextcloud() {
    def got as list of Resource init parseMultistatus(NEXTCLOUD);
    testing.assertEqual(len($got), 2);
    testing.assertEqual($got[0].href, "/remote.php/dav/files/alice/My Docs/");
    testing.assertEqual($got[0].name, "My Docs");
    testing.assertTrue($got[0].isDir);
    testing.assertEqual($got[0].modified, 1793797509);
    testing.assertEqual($got[0].quotaUsed, 1024);
    testing.assertEqual($got[0].quotaAvailable, -3);
    testing.assertEqual($got[0].etag, "\"6909\"");
    testing.assertEqual($got[1].name, "résumé.pdf");
    testing.assertFalse($got[1].isDir);
    testing.assertEqual($got[1].size, 20480);
    testing.assertEqual($got[1].contentType, "application/pdf");
    testing.assertEqual($got[1].quotaUsed, -1);
    testing.assertEqual($got[1].props["\{http://owncloud.org/ns\}fileid"], "42");
    testing.assertContains($got[1].props["\{http://owncloud.org/ns\}checksums"], "SHA1:da39");
}

func testParseApache() {
    def got as list of Resource init parseMultistatus(APACHE);
    testing.assertEqual(len($got), 1);
    testing.assertEqual($got[0].href, "/share/a b.txt");
    testing.assertEqual($got[0].size, 5);
    testing.assertEqual($got[0].props["\{http://apache.org/dav/props/\}executable"], "F");
    for (def bad in ["not xml", "<other xmlns=\"DAV:\"/>", "<multistatus/>"]) {
        try {
            parseMultistatus($bad);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "the reply is not");
        }
    }
}

func testMultistatusRoundTrip() {
    def dir as Resource init res("/files/My Docs/", true, 0);
    $dir.quotaUsed = 10;
    $dir.quotaAvailable = 90;
    def file as Resource init res("/files/My Docs/a&b.txt", false, 5);
    $file.contentType = "text/plain";
    $file.props["\{http://owncloud.org/ns\}fileid"] = "7";
    def text as string init multistatus([$dir, $file]);
    testing.assertContains($text, "<d:href>/files/My%20Docs/</d:href>");
    testing.assertContains($text, "a%26b.txt");
    def back as list of Resource init parseMultistatus($text);
    testing.assertEqual(len($back), 2);
    testing.assertEqual($back[0], $dir);
    testing.assertEqual($back[1], $file);
}

func testRequestBodies() {
    def body as string init propfindBody(["\{DAV:\}getetag", "\{http://owncloud.org/ns\}fileid"]);
    def doc as xml.Value init xml.decode($body);
    def ns as map of string to string init {"d": DAV_NS, "oc": "http://owncloud.org/ns"};
    testing.assertTrue(xml.has($doc, "d:prop/d:getetag", $ns));
    testing.assertTrue(xml.has($doc, "d:prop/oc:fileid", $ns));
    testing.assertTrue(xml.has(xml.decode(propfindBody([])), "d:allprop", $ns));
    def patch as xml.Value init xml.decode(proppatchBody(
        {
            "\{http://owncloud.org/ns\}favorite": "1"
        },
        ["\{DAV:\}displayname"]));
    testing.assertEqual(xml.text(xml.get($patch, "d:set/d:prop/oc:favorite", $ns)), "1");
    testing.assertTrue(xml.has($patch, "d:remove/d:prop/d:displayname", $ns));
}

func testConnect() {
    def c as Client init connect("https://cloud.example.com/remote.php/dav/files/al%20ice/");
    testing.assertEqual($c.url, "https://cloud.example.com/remote.php/dav/files/al%20ice");
    testing.assertEqual($c.basePath, "/remote.php/dav/files/al ice");
    testing.assertEqual($c.timeoutMs, 30000);
    testing.assertEqual(connect("http://nas:5005").basePath, "");
    testing.assertEqual(withTimeout($c, 0).timeoutMs, 30000);
    testing.assertEqual(withToken($c, "t0k").headers["Authorization"], "Bearer t0k");
    testing.assertEqual(
        withBasicAuth($c, "alice", "pw").headers["Authorization"],
        "Basic YWxpY2U6cHc=");
    testing.assertTrue(withInsecure($c).tls.skipVerify);
    testing.assertFalse(withCaCert(withInsecure($c), convert.bytesFromString("utf-8", "PEM")).tls.skipVerify);
    for (def bad in ["ftp://host", "https://host/a?x=1", "https://host/a#f", "host/a"]) {
        try {
            connect($bad);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "a WebDAV URL is");
        }
    }
    try {
        withHeader($c, "X-Evil", "a\r\nInjected: 1");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "a header is");
    }
    try {
        withBasicAuth($c, "al:ice", "pw");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "holds no :");
    }
}

func testReadDirAndStat() {
    def g as Fake init fake([
        reply(
            207,
            multistatus([
                res("/dav/al ice/Docs/", true, 0),
                res("/dav/al ice/Docs/a.txt", false, 5),
                res("/dav/al ice/Docs/Sub/", true, 0)
            ])),
        reply(207, multistatus([res("/dav/al ice/Docs/a.txt", false, 5)]))
    ]);
    def c as Client init withHeader(
        withBasicAuth(connect($g.url + "/dav/al%20ice"), "alice", "pw"),
        "X-Extra",
        "1");
    def entries as list of Resource init readDir($c, "/Docs");
    def one as Resource init stat($c, "/Docs/a.txt");
    def seen as list of Seen init finish($g);
    testing.assertEqual(len($entries), 2);
    testing.assertEqual($entries[0].path, "/Docs/a.txt");
    testing.assertEqual($entries[1].path, "/Docs/Sub");
    testing.assertTrue($entries[1].isDir);
    testing.assertEqual($one.size, 5);
    testing.assertEqual($seen[0].method, "PROPFIND");
    testing.assertEqual($seen[0].path, "/dav/al ice/Docs/");
    testing.assertEqual($seen[0].headers["Depth"], "1");
    testing.assertEqual($seen[0].headers["Authorization"], "Basic YWxpY2U6cHc=");
    testing.assertEqual($seen[0].headers["X-Extra"], "1");
    testing.assertContains($seen[0].headers["Content-Type"], "application/xml");
    testing.assertContains($seen[0].body, "getetag");
    testing.assertEqual($seen[1].path, "/dav/al ice/Docs/a.txt");
    testing.assertEqual($seen[1].headers["Depth"], "0");
}

func testReadDirOfAFile() {
    def f as Fake init fake([reply(207, multistatus([res("/a.txt", false, 5)]))]);
    try {
        readDir(connect($f.url), "/a.txt");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "not a collection");
    }
    finish($f);
}

# A listed path that is not a direct member (here an entry elsewhere in the
# tree) is refused, so walk cannot wander out of the collection it was given.
func testReadDirRefusesNonMembers() {
    def f as Fake init fake([
        reply(207, multistatus([res("/Docs/", true, 0), res("/Private/key.txt", false, 3)]))
    ]);
    try {
        readDir(connect($f.url), "/Docs");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "not a member");
    }
    finish($f);
}

func testRedirectToTheCollection() {
    # Apache answers a collection named without its slash with a redirect.
    def g as Fake init fake([
        Reply{status: 301, headers: {"Location": "/Docs/"}, body: ""},
        reply(207, multistatus([res("/Docs/", true, 0)]))
    ]);
    def r as Resource init stat(connect($g.url), "/Docs");
    def seen as list of Seen init finish($g);
    testing.assertTrue($r.isDir);
    testing.assertEqual($seen[1].path, "/Docs/");
}

func testExists() {
    def f as Fake init fake([
        reply(207, multistatus([res("/a.txt", false, 1)])),
        reply(404, ""),
        reply(500, "")
    ]);
    def c as Client init connect($f.url);
    testing.assertTrue(exists($c, "/a.txt"));
    testing.assertFalse(exists($c, "/b.txt"));
    try {
        exists($c, "/c.txt");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "PROPFIND /c.txt: HTTP 500");
    }
    finish($f);
}

func testGetAndPut() {
    def f as Fake init fake([
        reply(200, "hello"),
        Reply{status: 201, headers: {"ETag": "\"v1\""}, body: ""},
        reply(412, ""),
        reply(204, ""),
        reply(404, "")
    ]);
    def c as Client init connect($f.url);
    testing.assertEqual(convert.stringFromBytes("utf-8", get($c, "/a b.txt", 0)), "hello");
    def data as bytes init convert.bytesFromString("utf-8", "hi");
    testing.assertEqual(put($c, "/a.txt", $data), "\"v1\"");
    try {
        putWith(
            $c,
            "/a.txt",
            $data,
            PutOptions{contentType: "text/plain", ifMatch: "\"v0\"", onlyIfNew: false});
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "PUT /a.txt: HTTP 412");
    }
    testing.assertEqual(
        putWith($c, "/n.txt", $data, PutOptions{contentType: "", ifMatch: "", onlyIfNew: true}),
        "");
    # A weak tag can never match If-Match: refused before anything is sent.
    try {
        putWith(
            $c,
            "/a.txt",
            $data,
            PutOptions{contentType: "", ifMatch: 'W/"5-6"', onlyIfNew: false});
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "ifMatch needs a strong ETag");
    }
    try {
        get($c, "/missing", 0);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "GET /missing: HTTP 404");
    }
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[0].path, "/a b.txt");
    testing.assertEqual($seen[1].method, "PUT");
    testing.assertEqual($seen[1].body, "hi");
    testing.assertEqual($seen[1].headers["Content-Type"], "application/octet-stream");
    testing.assertEqual($seen[2].headers["If-Match"], "\"v0\"");
    testing.assertEqual($seen[2].headers["Content-Type"], "text/plain");
    testing.assertEqual($seen[3].headers["If-None-Match"], "*");
    try {
        put($c, "/", $data);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "the root is a collection");
    }
}

func testMkdir() {
    def f as Fake init fake([
        reply(201, ""),
        reply(405, ""),
        reply(207, multistatus([res("/Backups/", true, 0)])),
        reply(201, ""),
        reply(201, ""),
        reply(405, ""),
        reply(207, multistatus([res("/x", false, 3)]))
    ]);
    def c as Client init connect($f.url);
    mkdir($c, "/New");
    mkdirAll($c, "/Backups/2026/11");
    try {
        mkdirAll($c, "/x/y");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "MKCOL /x: a file is in the way");
    }
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[0].method, "MKCOL");
    testing.assertEqual($seen[1].path, "/Backups");
    testing.assertEqual($seen[2].method, "PROPFIND");
    testing.assertEqual($seen[3].path, "/Backups/2026");
    testing.assertEqual($seen[4].path, "/Backups/2026/11");
}

func testDeleteMoveCopy() {
    def partial as string init '<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response>' +
        '<d:href>/Old/locked.txt</d:href><d:status>HTTP/1.1 423 Locked</d:status></d:response>' +
        '</d:multistatus>';
    def f as Fake init fake([
        reply(204, ""),
        reply(207, $partial),
        reply(201, ""),
        reply(204, ""),
        reply(412, "")
    ]);
    def c as Client init connect($f.url + "/dav");
    delete($c, "/a.txt");
    try {
        delete($c, "/Old");
        missed();
    } catch (e) {
        testing.assertContains(
            messageOf($e),
            "kept part of it, e.g. \"/Old/locked.txt\" (HTTP 423)");
        # a WebdavError: the request, and the failed member with its status
        testing.assertEqual($e.method, "DELETE");
        testing.assertEqual($e.path, "/Old");
        testing.assertEqual($e.status, 423);
        testing.assertEqual($e.member, "/Old/locked.txt");
    }
    move($c, "/Inbox/scan.pdf", "/Docs/scan 1.pdf", false);
    copy($c, "/Templates", "/Docs/Templates", true);
    try {
        move($c, "/a", "/b", false);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "MOVE /a: HTTP 412");
        testing.assertEqual($e.method, "MOVE");
        testing.assertEqual($e.status, 412);
        testing.assertEqual($e.member, "");
    }
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[0].method, "DELETE");
    testing.assertEqual($seen[2].method, "MOVE");
    testing.assertEqual($seen[2].headers["Destination"], $f.url + "/dav/Docs/scan%201.pdf");
    testing.assertEqual($seen[2].headers["Overwrite"], "F");
    testing.assertEqual($seen[3].method, "COPY");
    testing.assertEqual($seen[3].headers["Overwrite"], "T");
    testing.assertEqual($seen[3].headers["Depth"], "infinity");
    for (def bad in ["/", ""]) {
        try {
            delete($c, $bad);
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "refusing to delete");
        }
    }
}

func testPropfindAndProppatch() {
    def refused as string init '<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" ' +
        'xmlns:oc="http://owncloud.org/ns"><d:response><d:href>/a.txt</d:href><d:propstat>' +
        '<d:prop><oc:fileid/></d:prop><d:status>HTTP/1.1 403 Forbidden</d:status>' +
        '</d:propstat></d:response></d:multistatus>';
    def withId as Resource init res("/a.txt", false, 1);
    $withId.props["\{http://owncloud.org/ns\}fileid"] = "42";
    def f as Fake init fake([
        reply(207, multistatus([$withId])),
        reply(207, multistatus([$withId])),
        reply(207, $refused)
    ]);
    def c as Client init connect($f.url);
    def got as list of Resource init propfind(
        $c,
        "/a.txt",
        0,
        ["\{http://owncloud.org/ns\}fileid"]);
    testing.assertEqual($got[0].props["\{http://owncloud.org/ns\}fileid"], "42");
    proppatch($c, "/a.txt", {"\{http://owncloud.org/ns\}favorite": "1"}, []);
    try {
        proppatch($c, "/a.txt", {"\{http://owncloud.org/ns\}fileid": "9"}, []);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "HTTP 403 for \{http://owncloud.org/ns\}fileid");
    }
    def seen as list of Seen init finish($f);
    testing.assertEqual($seen[1].method, "PROPPATCH");
    testing.assertContains($seen[1].body, "favorite");
    try {
        propfind($c, "/", 2, []);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "a depth is 0 or 1");
    }
}

func testWalk() {
    def f as Fake init fake([
        reply(
            207,
            multistatus([res("/", true, 0), res("/a.txt", false, 1), res("/Sub/", true, 0)])),
        reply(207, multistatus([res("/Sub/", true, 0), res("/Sub/b.txt", false, 2)]))
    ]);
    def all as list of Resource init walk(connect($f.url), "/");
    def seen as list of Seen init finish($f);
    testing.assertEqual(len($all), 3);
    testing.assertEqual($all[2].path, "/Sub/b.txt");
    testing.assertEqual($seen[1].path, "/Sub/");
}

func testMessagesStayOneLine() {
    def f as Fake init fake([Reply{status: 500, headers: {}, body: ""}]);
    try {
        stat(connect($f.url), "/a.txt");
        missed();
    } catch (e) {
        testing.assertFalse(strings.contains(messageOf($e), convert.fromCodepoint(10)));
    }
    finish($f);
    def long as string init "/" + strings.repeat("x", 200) + convert.fromCodepoint(27) + "[31m";
    try {
        cleanPath(strings.substring($long, 1));
        missed();
    } catch (e) {
        testing.assertFalse(strings.contains(messageOf($e), convert.fromCodepoint(27)));
        testing.assertContains(messageOf($e), "...");
    }
}

func testNoExternalEntities() {
    def body as string init '<?xml version="1.0"?><!DOCTYPE d:multistatus [<!ENTITY e SYSTEM ' +
        '"file:///etc/hostname">]><d:multistatus xmlns:d="DAV:"><d:response><d:href>/&e;' +
        '</d:href></d:response></d:multistatus>';
    try {
        parseMultistatus($body);
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "the reply is not XML");
    }
}

func testValidationFixes() {
    # A base path matches whole segments only.
    def c as Client init connect("https://h/dav");
    testing.assertEqual(relative($c, "/dav"), "/");
    testing.assertEqual(relative($c, "/dav/x"), "/x");
    testing.assertEqual(relative($c, "/davfoo/x"), "/davfoo/x");
    # A bad escape from a server is a webdav error, not a uri one.
    try {
        decodeHref("/a%zzb");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "a bad % escape");
    }
    # Credentials in the URL would travel in every Destination header.
    try {
        connect("https://user:pw@host/dav");
        missed();
    } catch (e) {
        testing.assertContains(messageOf($e), "without credentials");
    }
    for (def name in ["Content-Length", "transfer-encoding", "Host", "Connection"]) {
        try {
            withHeader($c, $name, "0");
            missed();
        } catch (e) {
            testing.assertContains(messageOf($e), "the client sets");
        }
    }
    testing.assertEqual(statusCode("HTTP/1.1 423 Locked"), 423);
    testing.assertEqual(statusCode("HTTP/1.1"), 0);
    testing.assertEqual(statusCode("HTTP/1.1 abc"), 0);
    testing.assertTrue(sameTarget("/Docs/", "http://h/Docs/"));
    testing.assertFalse(sameTarget("%zz", "http://h/Docs/"));
    testing.assertFalse(sameTarget("", "http://h/Docs/"));
}
