# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `fs` library.
 * Shows whole-file read/write/append, metadata predicates + fs.stat, the
 * two-verbs pattern for directory create (mkdir vs mkdirAll) and delete
 * (remove vs removeAll), directory listing + walk, buffered file handles for a
 * line-by-line read loop, and fs.makeTempFile / fs.makeTempDir for uniquely-
 * named scratch entries. Uses a fresh subdirectory next to the source file so
 * re-runs stay clean; wipes it on the way out with fs.removeAll.
 * @module fs
 */

use io;
use fs;

/** The scratch directory this example creates, writes into, and cleans up. */
def const ROOT as string init "fs-example-tmp";

# Fresh slate: if a prior run left the dir around, clear it. The
# canonical "safe if missing" idiom checks fs.exists first.
if (fs.exists(ROOT)) {
    fs.removeAll(ROOT);
}

# ---- one-shot whole-file round trip ----
fs.mkdirAll(ROOT);
fs.writeString(ROOT + "/note.txt", "hello, fs\n");
def content as string init fs.readString(ROOT + "/note.txt");
io.printf("read: %s", $content);

# Append doesn't overwrite; whole-file re-read shows both lines.
fs.appendString(ROOT + "/note.txt", "second line\n");
io.printf("after append:\n%s", fs.readString(ROOT + "/note.txt"));

# ---- metadata ----
io.printf("exists(note.txt) = %t\n", fs.exists(ROOT + "/note.txt"));
io.printf("exists(missing)  = %t\n", fs.exists(ROOT + "/missing.txt"));
io.printf("isFile(note.txt) = %t\n", fs.isFile(ROOT + "/note.txt"));
io.printf("isDir(root)      = %t\n", fs.isDir(ROOT));

def s as fs.Stat init fs.stat(ROOT + "/note.txt");
# The mode field is host-dependent (umask), so we don't print it
# here; a real program would pass it to convert.toString / mask
# the bits it cares about.
io.printf("stat size=%d isDir=%t\n", $s.size, $s.isDir);

# ---- two-verbs pattern: mkdir vs mkdirAll ----
# mkdir refuses to create with missing parents; mkdirAll does the
# mkdir -p thing. Two names, so the risky call is grep-visible.
try {
    fs.mkdir(ROOT + "/a/b/c");
} catch (e) {
    io.printf("mkdir refused missing parents (good)\n");
}
fs.mkdirAll(ROOT + "/a/b/c");
io.printf("mkdirAll created nested dir: %t\n", fs.isDir(ROOT + "/a/b/c"));

# ---- two-verbs pattern: remove vs removeAll ----
# Populate one leaf so remove has a non-empty dir to refuse.
fs.writeString(ROOT + "/a/b/leaf.txt", "x");
try {
    fs.remove(ROOT + "/a");
} catch (e) {
    io.printf("remove refused non-empty dir (good)\n");
}
fs.removeAll(ROOT + "/a");
io.printf("after removeAll: exists(a) = %t\n", fs.exists(ROOT + "/a"));

# ---- directory listing (sorted, non-recursive) ----
fs.writeString(ROOT + "/alpha",   "one");
fs.writeString(ROOT + "/bravo",   "two");
fs.writeString(ROOT + "/charlie", "three");
def names as list of string init fs.list(ROOT);
io.printf("list: %a\n", $names);

# ---- walk (depth-first, sorted, includes root) ----
fs.mkdirAll(ROOT + "/sub");
fs.writeString(ROOT + "/sub/y.txt", "under-sub");
def entries as list of fs.Stat init fs.walk(ROOT);
io.printf("walk entries: %d\n", len($entries));
for (def e in $entries) {
    io.printf("  isDir=%t size=%d\n", $e.isDir, $e.size);
}

# ---- buffered file handle: canonical read-loop ----
fs.writeString(ROOT + "/lines.txt", "one\ntwo\nthree\n");
def f as fs.File init fs.open(ROOT + "/lines.txt", "read");
while (not fs.eof($f)) {
    def line as string init fs.readLine($f);
    io.printf("line: %s\n", $line);
}
fs.close($f);

# ---- temp files / dirs (unique names in the system temp dir) ----
# makeTempFile / makeTempDir create a fresh, uniquely-named entry atomically
# (0600 file / 0700 dir) and return its path; the caller removes it (no
# finally in the language). dir "" means the system temp dir (os.tempDir()).
def tf as string init fs.makeTempFile("", "fsdemo-", ".txt");
fs.writeString($tf, "scratch\n");
io.printf("makeTempFile isFile = %t\n", fs.isFile($tf));
fs.remove($tf);
def td as string init fs.makeTempDir("", "fsdemo-");
io.printf("makeTempDir  isDir  = %t\n", fs.isDir($td));
fs.removeAll($td);
io.printf("temp cleaned        = %t\n", not fs.exists($tf) and not fs.exists($td));

# ---- cleanup ----
fs.removeAll(ROOT);
io.printf("cleanup: exists = %t\n", fs.exists(ROOT));
