#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * List a WebDAV folder, write a file into it, read it back and remove it.
 * Needs a server, so it reads the URL and credentials from the environment
 * and prints usage when they are absent:
 *
 *   WEBDAV_URL=https://cloud.example.com/remote.php/dav/files/alice \
 *   WEBDAV_USER=alice WEBDAV_PASSWORD=app-password jennifer run webdav_demo.j
 *
 * The encoding half needs no server, so it always runs: it builds the
 * `207 Multi-Status` a server would answer and decodes it again.
 * @module webdav_demo
 */
use convert;
use io;
use os;
import "../../modules/webdav.j" as webdav;

def sample as webdav.Resource init webdav.Resource{
    href: "/files/alice/Notes/todo.txt",
    path: "",
    name: "todo.txt",
    isDir: false,
    size: 42,
    modified: 1793797509,
    etag: "\"v7\"",
    contentType: "text/plain",
    quotaUsed: -1,
    quotaAvailable: -1,
    props: {}
};
def reply as string init webdav.multistatus([$sample]);
io.printf("%s\n\n", $reply);
for (def r in webdav.parseMultistatus($reply)) {
    io.printf("decoded: %s, %d bytes, etag %s\n\n", $r.href, $r.size, $r.etag);
}

/**
 * List the root, write a file, read it back and remove it.
 *
 * @param c {webdav.Client} the client
 */
func tryServer(c as webdav.Client) {
    for (def r in webdav.readDir($c, "/")) {
        def kind as string init "file";
        if ($r.isDir) {
            $kind = "dir ";
        }
        io.printf("%s %s %d\n", $kind, $r.path, $r.size);
    }
    def name as string init "/jennifer-webdav-demo.txt";
    def etag as string init webdav.put(
        $c,
        $name,
        convert.bytesFromString("utf-8", "hello from Jennifer\n"));
    io.printf("wrote %s (etag %s)\n", $name, $etag);
    io.printf("read back: %s", convert.stringFromBytes("utf-8", webdav.get($c, $name, 0)));
    webdav.delete($c, $name);
    io.printf("removed %s\n", $name);
}

if (os.hasEnv("WEBDAV_URL")) {
    def c as webdav.Client init webdav.connect(os.getEnv("WEBDAV_URL"));
    if (os.hasEnv("WEBDAV_USER")) {
        $c = webdav.withBasicAuth($c, os.getEnv("WEBDAV_USER"), os.getEnv("WEBDAV_PASSWORD"));
    }
    tryServer($c);
} else {
    io.printf("set WEBDAV_URL (and WEBDAV_USER / WEBDAV_PASSWORD) to try a server\n");
}
