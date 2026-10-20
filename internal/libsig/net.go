// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("net", `
connect(string, int?) net.Conn
# (address[, net.TLSOptions][, timeoutMs]): a lone trailing int is the timeout
connectTLS(string, net.TLSOptions|int?) net.Conn
connectTLS(string, net.TLSOptions, int) net.Conn
startTLS(net.Conn, net.TLSOptions|int?) net.Conn
startTLS(net.Conn, net.TLSOptions, int) net.Conn
listen(string) net.Listener
accept(net.Listener) net.Conn
readBytes(net.Conn, int) bytes
readAll(net.Conn, int?, int?) bytes
readN(net.Conn, int, int?) bytes
writeBytes(net.Conn, bytes) null
setReadDeadline(net.Conn|net.UDPSocket, int) null
setWriteDeadline(net.Conn|net.UDPSocket, int) null
eof(net.Conn) bool
address(net.Conn|net.Listener|net.UDPSocket) string
listenUDP(string) net.UDPSocket
sendTo(net.UDPSocket, string, bytes) null
recvFrom(net.UDPSocket, int) net.Datagram
setBroadcast(net.UDPSocket, bool) null
bindToDevice(net.UDPSocket, string) null
lookup(string) list of string
reverseLookup(string) list of string
close(net.Conn|net.Listener|net.UDPSocket) null
`)
}
