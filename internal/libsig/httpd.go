// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("httpd", `
listen(string) httpd.Server
listenWith(string, httpd.Options) httpd.Server
listenTLS(string, bytes, bytes) httpd.Server
listenTLSWith(string, bytes, bytes, httpd.Options) httpd.Server
setMaxBufferBudget(int) null
setMaxBufferBudgetFromRAM(int|float) int
address(httpd.Server) string
maxInFlight(httpd.Server) int
shutdown(httpd.Server) null
accept(httpd.Server) httpd.Request
method(httpd.Request) string
path(httpd.Request) string
query(httpd.Request, string) string
rawQuery(httpd.Request) string
header(httpd.Request, string) string
body(httpd.Request) bytes
remoteAddr(httpd.Request) string
isOpen(httpd.Request) bool
timedOut(httpd.Request) bool
requestValue(httpd.Request, string) string
setRequestValue(httpd.Request, string, string) null
setHeader(httpd.Request, string, string) null
etag(httpd.Request, string) bool
respond(httpd.Request, int, string|bytes) null
serveFile(httpd.Request, string) null
serveDir(httpd.Request, string) null
serveFileEtag(httpd.Request, string) null
serveDirEtag(httpd.Request, string) null
`)
	// M29.48: certificate fields and the etag switch in httpd.Options.
	deprecate("httpd", "listenTLS", "`httpd.listenWith(addr, httpd.Options{certPem: ..., keyPem: ...})`", "0.29.0")
	deprecate("httpd", "listenTLSWith", "`httpd.listenWith` with `certPem` / `keyPem` in its `httpd.Options`", "0.29.0")
	deprecate("httpd", "serveFileEtag", "`httpd.serveFile` on a server opened with `httpd.Options{etag: true}`", "0.29.0")
	deprecate("httpd", "serveDirEtag", "`httpd.serveDir` on a server opened with `httpd.Options{etag: true}`", "0.29.0")
}
