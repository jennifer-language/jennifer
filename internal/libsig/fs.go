// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("fs", `
readString(string) string
appendString(string, string) null
appendBytes(string, bytes) null
writeNew(string, string) null
exists(string) bool
isFile(string) bool
isDir(string) bool
stat(string) fs.Stat
realpath(string) string
symlink(string, string) null
readlink(string) string
chmod(string, int) null
chown(string, int, int) null
mkdir(string) null
mkdirAll(string) null
remove(string) null
removeAll(string) null
rename(string, string) null
list(string) list of string
walk(string) list of fs.Stat
makeTempFile(string?, string?, string?) string
makeTempDir(string?, string?) string
open(string, string) fs.File
close(fs.File|fs.Watcher) null
readLine(fs.File) string
readChars(fs.File, int) string
# whole-file form (path) or partial read (fs.File, n)
readBytes(string) bytes
readBytes(fs.File, int) bytes
writeString(string|fs.File, string) null
writeBytes(string|fs.File, bytes) null
sync(fs.File) null
eof(fs.File) bool
watch(string, int?) fs.Watcher
next(fs.Watcher) fs.Event
hasEvent(fs.Watcher) bool
`)
}
