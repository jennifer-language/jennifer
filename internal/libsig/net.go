// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addErrorStruct("net", "NetError")
	add("net", `
# The trailing timeoutMs ints of connect / connectTLS / startTLS and the
# trailing idle timeout of readAll / readN are deprecated shapes (lint L301,
# removed in 0.29.0); the signatures keep accepting them until then.
connect(string, int?) net.Conn
connectWith(string, net.ConnectOptions) net.Conn
connectTLS(string, net.TLSOptions|int?) net.Conn
connectTLS(string, net.TLSOptions, int) net.Conn
startTLS(net.Conn, net.TLSOptions|int?) net.Conn
startTLS(net.Conn, net.TLSOptions, int) net.Conn
startTLSWith(net.Conn, net.ConnectOptions) net.Conn
listen(string) net.Listener
accept(net.Listener) net.Conn
readBytes(net.Conn, int) bytes
readAll(net.Conn, int?, int?) bytes
readN(net.Conn, int, int?) bytes
writeBytes(net.Conn, bytes) null
setReadDeadline(net.Conn|net.UDPSocket, int) null
setWriteDeadline(net.Conn|net.UDPSocket, int) null
setIdleTimeout(net.Conn, int) null
eof(net.Conn) bool
abort(net.Conn) null
address(net.Conn|net.Listener|net.UDPSocket) string
listenUDP(string) net.UDPSocket
sendTo(net.UDPSocket, string, bytes) null
recvFrom(net.UDPSocket, int) net.Datagram
setBroadcast(net.UDPSocket, bool) null
bindToDevice(net.UDPSocket, string) null
lookup(string) list of string
lookupWith(string, net.LookupOptions) list of string
reverseLookup(string) list of string
reverseLookupWith(string, net.LookupOptions) list of string
close(net.Conn|net.Listener|net.UDPSocket) null
`)
	// M29.46: a 20 ms guess named like the exact fs.eof.
	deprecate("net", "eof", "an empty `net.readBytes` (the peer closed)", "0.29.0")
}
