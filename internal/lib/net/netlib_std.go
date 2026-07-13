// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build !tinygo

// Standard-Go implementation of the `net` library. Used by
// the default `jennifer` binary (standard-Go build). Under TinyGo
// (`jennifer-tiny`), the netlib_tinygo.go file is selected instead
// and returns friendly runtime errors from every entry point.

package netlib

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"sync"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// -------- Registries --------

// connState holds one live TCP connection. The buffered reader
// backs partial-length reads and the sticky-EOF idiom the
// fs handles established.
type connState struct {
	c      stdnet.Conn
	r      *bufio.Reader
	sticky bool
	host   string // bare hostname dialled (for a later startTLS); empty for accepted conns
}

type listenerState struct {
	l stdnet.Listener
}

type udpState struct {
	c stdnet.PacketConn
}

var (
	connsMu    sync.Mutex
	conns      = map[int64]*connState{}
	nextConnID int64

	listenersMu    sync.Mutex
	listeners      = map[int64]*listenerState{}
	nextListenerID int64

	udpsMu    sync.Mutex
	udps      = map[int64]*udpState{}
	nextUDPID int64
)

// maxReadBytes caps a single caller-requested read so a huge `n` cannot force a
// multi-gigabyte up-front allocation (or a makeslice panic). Read in chunks for
// more.
const maxReadBytes = 256 << 20

// ResetForTest wipes all three registries between runs. Exported
// so the _test package can drive it.
func ResetForTest() {
	connsMu.Lock()
	for _, s := range conns {
		if s != nil && s.c != nil {
			_ = s.c.Close()
		}
	}
	conns = map[int64]*connState{}
	nextConnID = 0
	connsMu.Unlock()

	listenersMu.Lock()
	for _, s := range listeners {
		if s != nil && s.l != nil {
			_ = s.l.Close()
		}
	}
	listeners = map[int64]*listenerState{}
	nextListenerID = 0
	listenersMu.Unlock()

	udpsMu.Lock()
	for _, s := range udps {
		if s != nil && s.c != nil {
			_ = s.c.Close()
		}
	}
	udps = map[int64]*udpState{}
	nextUDPID = 0
	udpsMu.Unlock()
}

func resolveConn(fnName string, id int64) (*connState, error) {
	connsMu.Lock()
	defer connsMu.Unlock()
	s, ok := conns[id]
	if !ok {
		return nil, fmt.Errorf("%s: net.Conn id %d is not open (already closed, or never opened)", fnName, id)
	}
	return s, nil
}

func resolveListener(fnName string, id int64) (*listenerState, error) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	s, ok := listeners[id]
	if !ok {
		return nil, fmt.Errorf("%s: net.Listener id %d is not open (already closed, or never opened)", fnName, id)
	}
	return s, nil
}

func resolveUDP(fnName string, id int64) (*udpState, error) {
	udpsMu.Lock()
	defer udpsMu.Unlock()
	s, ok := udps[id]
	if !ok {
		return nil, fmt.Errorf("%s: net.UDPSocket id %d is not open (already closed, or never opened)", fnName, id)
	}
	return s, nil
}

// -------- TCP --------

func connectFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.connect expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.connect", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	c, dialErr := stdnet.Dial("tcp", addr)
	if dialErr != nil {
		return interpreter.Null(), fmt.Errorf("net.connect: %s: %v", addr, dialErr)
	}
	host, _, _ := stdnet.SplitHostPort(addr) // recorded so a later startTLS can reuse it
	connsMu.Lock()
	nextConnID++
	id := nextConnID
	conns[id] = &connState{c: c, r: bufio.NewReader(c), host: host}
	connsMu.Unlock()
	return makeConn(id), nil
}

// -------- TLS --------

// testRootCAs, when non-nil, is added to the client config's trusted
// roots so tests can trust a self-signed local server. Nil in production.
var testRootCAs *x509.CertPool

// clientTLSConfig builds the client config for host (used for SNI and
// certificate-hostname checks). Verification is on unless the caller's
// net.TLSOptions set skipVerify; a non-empty caCert (PEM) is trusted in
// place of the system roots.
func clientTLSConfig(fnName, host string, skipVerify bool, caCert []byte) (*tls.Config, error) {
	cfg := &tls.Config{ServerName: host, InsecureSkipVerify: skipVerify}
	switch {
	case len(caCert) > 0:
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("%s: caCert is not a valid PEM certificate", fnName)
		}
		cfg.RootCAs = pool
	case testRootCAs != nil:
		cfg.RootCAs = testRootCAs
	}
	return cfg, nil
}

// tlsOptions reads the optional net.TLSOptions argument at args[idx]:
// skipVerify (opt out of verification) and caCert (a PEM certificate to
// trust). Absent -> verify against the system roots.
func tlsOptions(fnName string, args []Value, idx int) (skipVerify bool, caCert []byte, err error) {
	if len(args) <= idx {
		return false, nil, nil
	}
	v := args[idx]
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "TLSOptions" {
		return false, nil, fmt.Errorf("%s: options must be a net.TLSOptions, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "skipVerify":
			if f.Value.Kind != interpreter.KindBool {
				return false, nil, fmt.Errorf("%s: net.TLSOptions.skipVerify must be bool, got %s", fnName, f.Value.Kind)
			}
			skipVerify = f.Value.Bool
		case "caCert":
			if f.Value.Kind != interpreter.KindBytes {
				return false, nil, fmt.Errorf("%s: net.TLSOptions.caCert must be bytes, got %s", fnName, f.Value.Kind)
			}
			caCert = f.Value.Bytes
		}
	}
	return skipVerify, caCert, nil
}

// bufferedConn presents a bufio.Reader's already-buffered bytes (then the
// raw connection) as a net.Conn, so a TLS handshake begun mid-stream
// (startTLS) never drops plaintext the reader read ahead.
type bufferedConn struct {
	r *bufio.Reader
	stdnet.Conn
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// connectTLSFn implements `net.connectTLS(address [, net.TLSOptions]) ->
// net.Conn`: dial `address` (host:port, same as net.connect) and complete
// a TLS handshake, verifying the server certificate against the address's
// host.
func connectTLSFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("net.connectTLS expects 1 or 2 arguments (address[, net.TLSOptions]), got %d", len(args))
	}
	addr, err := takeStringArg("net.connectTLS", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	skip, caCert, err := tlsOptions("net.connectTLS", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	host, _, splitErr := stdnet.SplitHostPort(addr)
	if splitErr != nil {
		return interpreter.Null(), fmt.Errorf("net.connectTLS: address must be host:port: %v", splitErr)
	}
	cfg, err := clientTLSConfig("net.connectTLS", host, skip, caCert)
	if err != nil {
		return interpreter.Null(), err
	}
	c, dialErr := tls.Dial("tcp", addr, cfg)
	if dialErr != nil {
		return interpreter.Null(), fmt.Errorf("net.connectTLS: %s: %v", addr, dialErr)
	}
	connsMu.Lock()
	nextConnID++
	id := nextConnID
	conns[id] = &connState{c: c, r: bufio.NewReader(c), host: host}
	connsMu.Unlock()
	return makeConn(id), nil
}

// startTLSFn implements `net.startTLS(conn [, net.TLSOptions]) ->
// net.Conn`: upgrade an open plaintext connection to TLS in place (for
// STARTTLS). The server certificate is verified against the hostname the
// connection was opened with (net.connect); returns the same handle.
func startTLSFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("net.startTLS expects 1 or 2 arguments (net.Conn[, net.TLSOptions]), got %d", len(args))
	}
	id, err := extractID("net.startTLS", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	skip, caCert, err := tlsOptions("net.startTLS", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveConn("net.startTLS", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if s.host == "" {
		return interpreter.Null(), fmt.Errorf("net.startTLS: this connection has no recorded hostname to verify against; startTLS upgrades a connection opened with net.connect")
	}
	cfg, err := clientTLSConfig("net.startTLS", s.host, skip, caCert)
	if err != nil {
		return interpreter.Null(), err
	}
	// Hand the handshake the buffered reader + raw conn so no read-ahead
	// plaintext is lost, then swap the registry entry to the TLS conn.
	tlsConn := tls.Client(&bufferedConn{r: s.r, Conn: s.c}, cfg)
	if hErr := tlsConn.Handshake(); hErr != nil {
		return interpreter.Null(), fmt.Errorf("net.startTLS: %v", hErr)
	}
	connsMu.Lock()
	s.c = tlsConn
	s.r = bufio.NewReader(tlsConn)
	s.sticky = false
	connsMu.Unlock()
	return makeConn(id), nil
}

func listenFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.listen expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.listen", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	l, lErr := stdnet.Listen("tcp", addr)
	if lErr != nil {
		return interpreter.Null(), fmt.Errorf("net.listen: %s: %v", addr, lErr)
	}
	listenersMu.Lock()
	nextListenerID++
	id := nextListenerID
	listeners[id] = &listenerState{l: l}
	listenersMu.Unlock()
	return makeListener(id), nil
}

func acceptFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.accept expects 1 argument (net.Listener), got %d", len(args))
	}
	id, err := extractID("net.accept", "Listener", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveListener("net.accept", id)
	if err != nil {
		return interpreter.Null(), err
	}
	c, aErr := s.l.Accept()
	if aErr != nil {
		return interpreter.Null(), fmt.Errorf("net.accept: %v", aErr)
	}
	connsMu.Lock()
	nextConnID++
	newID := nextConnID
	conns[newID] = &connState{c: c, r: bufio.NewReader(c)}
	connsMu.Unlock()
	return makeConn(newID), nil
}

// readBytesFn reads *up to* n bytes from a TCP connection. Unlike
// fs.readBytes (which uses io.ReadFull because files have
// known sizes), TCP is a stream where "up to N" is the natural
// unit: block for at least one byte to arrive, then return
// whatever the buffered reader has, capped at n. Callers who need
// exactly N bytes call it in a loop until they've assembled the
// payload.
func readBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.readBytes expects 2 arguments (net.Conn, n), got %d", len(args))
	}
	id, err := extractID("net.readBytes", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := takeIntArg("net.readBytes", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if n > maxReadBytes {
		return interpreter.Null(), fmt.Errorf("net.readBytes: %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	s, err := resolveConn("net.readBytes", id)
	if err != nil {
		return interpreter.Null(), err
	}
	buf := make([]byte, n)
	read, rerr := s.r.Read(buf)
	if rerr != nil {
		if errors.Is(rerr, io.EOF) {
			s.sticky = true
			return interpreter.BytesVal(buf[:read]), nil
		}
		return interpreter.Null(), fmt.Errorf("net.readBytes: %v", rerr)
	}
	return interpreter.BytesVal(buf[:read]), nil
}

func writeBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.writeBytes expects 2 arguments (net.Conn, bytes), got %d", len(args))
	}
	id, err := extractID("net.writeBytes", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := takeBytesArg("net.writeBytes", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveConn("net.writeBytes", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, werr := s.c.Write(data); werr != nil {
		return interpreter.Null(), fmt.Errorf("net.writeBytes: %v", werr)
	}
	return interpreter.Null(), nil
}

func eofFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.eof expects 1 argument (net.Conn), got %d", len(args))
	}
	id, err := extractID("net.eof", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveConn("net.eof", id)
	if err != nil {
		return interpreter.Null(), err
	}
	if s.sticky {
		return interpreter.BoolVal(true), nil
	}
	if _, peekErr := s.r.Peek(1); errors.Is(peekErr, io.EOF) {
		s.sticky = true
		return interpreter.BoolVal(true), nil
	}
	return interpreter.BoolVal(false), nil
}

// addressFn is polymorphic on the handle kind. For a net.Conn it
// returns the peer's remote address (who you're talking to). For a
// net.Listener or net.UDPSocket it returns the local bound
// address; that's the one servers need after binding to `:0` to
// discover which ephemeral port the kernel picked.
func addressFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.address expects 1 argument (net.Conn, net.Listener, or net.UDPSocket), got %d", len(args))
	}
	v := args[0]
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName {
		return interpreter.Null(), fmt.Errorf("net.address: argument must be a net.Conn, net.Listener, or net.UDPSocket; got %s", v.Kind)
	}
	switch v.StructName {
	case "Conn":
		id, err := extractID("net.address", "Conn", v)
		if err != nil {
			return interpreter.Null(), err
		}
		s, err := resolveConn("net.address", id)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.StringVal(s.c.RemoteAddr().String()), nil
	case "Listener":
		id, err := extractID("net.address", "Listener", v)
		if err != nil {
			return interpreter.Null(), err
		}
		s, err := resolveListener("net.address", id)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.StringVal(s.l.Addr().String()), nil
	case "UDPSocket":
		id, err := extractID("net.address", "UDPSocket", v)
		if err != nil {
			return interpreter.Null(), err
		}
		s, err := resolveUDP("net.address", id)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.StringVal(s.c.LocalAddr().String()), nil
	default:
		return interpreter.Null(), fmt.Errorf("net.address: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.%s", v.StructName)
	}
}

func closeConn(id int64) error {
	connsMu.Lock()
	s, ok := conns[id]
	if !ok {
		connsMu.Unlock()
		return fmt.Errorf("net.close: net.Conn id %d is not open (already closed?)", id)
	}
	delete(conns, id)
	connsMu.Unlock()
	if s.c != nil {
		if err := s.c.Close(); err != nil {
			return fmt.Errorf("net.close: %v", err)
		}
	}
	return nil
}

func closeListener(id int64) error {
	listenersMu.Lock()
	s, ok := listeners[id]
	if !ok {
		listenersMu.Unlock()
		return fmt.Errorf("net.close: net.Listener id %d is not open (already closed?)", id)
	}
	delete(listeners, id)
	listenersMu.Unlock()
	if s.l != nil {
		if err := s.l.Close(); err != nil {
			return fmt.Errorf("net.close: %v", err)
		}
	}
	return nil
}

// -------- UDP --------

func listenUDPFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.listenUDP expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.listenUDP", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	c, lErr := stdnet.ListenPacket("udp", addr)
	if lErr != nil {
		return interpreter.Null(), fmt.Errorf("net.listenUDP: %s: %v", addr, lErr)
	}
	udpsMu.Lock()
	nextUDPID++
	id := nextUDPID
	udps[id] = &udpState{c: c}
	udpsMu.Unlock()
	return makeUDPSocket(id), nil
}

func sendToFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("net.sendTo expects 3 arguments (net.UDPSocket, peer, bytes), got %d", len(args))
	}
	id, err := extractID("net.sendTo", "UDPSocket", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	peer, err := takeStringArg("net.sendTo", args, 1, "peer")
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := takeBytesArg("net.sendTo", args, 2, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveUDP("net.sendTo", id)
	if err != nil {
		return interpreter.Null(), err
	}
	peerAddr, resolveErr := stdnet.ResolveUDPAddr("udp", peer)
	if resolveErr != nil {
		return interpreter.Null(), fmt.Errorf("net.sendTo: %s: %v", peer, resolveErr)
	}
	if _, werr := s.c.WriteTo(data, peerAddr); werr != nil {
		return interpreter.Null(), fmt.Errorf("net.sendTo: %v", werr)
	}
	return interpreter.Null(), nil
}

func recvFromFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.recvFrom expects 2 arguments (net.UDPSocket, n), got %d", len(args))
	}
	id, err := extractID("net.recvFrom", "UDPSocket", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := takeIntArg("net.recvFrom", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if n > maxReadBytes {
		return interpreter.Null(), fmt.Errorf("net.recvFrom: %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	s, err := resolveUDP("net.recvFrom", id)
	if err != nil {
		return interpreter.Null(), err
	}
	buf := make([]byte, n)
	read, peer, rerr := s.c.ReadFrom(buf)
	if rerr != nil {
		return interpreter.Null(), fmt.Errorf("net.recvFrom: %v", rerr)
	}
	return makeDatagram(buf[:read], peer.String()), nil
}

func closeUDP(id int64) error {
	udpsMu.Lock()
	s, ok := udps[id]
	if !ok {
		udpsMu.Unlock()
		return fmt.Errorf("net.close: net.UDPSocket id %d is not open (already closed?)", id)
	}
	delete(udps, id)
	udpsMu.Unlock()
	if s.c != nil {
		if err := s.c.Close(); err != nil {
			return fmt.Errorf("net.close: %v", err)
		}
	}
	return nil
}

// -------- DNS --------

func lookupFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.lookup expects 1 argument (host), got %d", len(args))
	}
	host, err := takeStringArg("net.lookup", args, 0, "host")
	if err != nil {
		return interpreter.Null(), err
	}
	addrs, dErr := stdnet.LookupHost(host)
	if dErr != nil {
		return interpreter.Null(), fmt.Errorf("net.lookup: %s: %v", host, dErr)
	}
	return stringSlice(addrs), nil
}

func reverseLookupFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.reverseLookup expects 1 argument (ip), got %d", len(args))
	}
	ip, err := takeStringArg("net.reverseLookup", args, 0, "ip")
	if err != nil {
		return interpreter.Null(), err
	}
	names, dErr := stdnet.LookupAddr(ip)
	if dErr != nil {
		return interpreter.Null(), fmt.Errorf("net.reverseLookup: %s: %v", ip, dErr)
	}
	return stringSlice(names), nil
}
