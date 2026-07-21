// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Standard-Go implementation of the `httpd` library, wrapping `net/http`. Used
// by the default `jennifer` binary. Under TinyGo (`jennifer-tiny`),
// httpdlib_tinygo.go is selected instead and returns friendly runtime errors.
//
// The pull loop rests on one invariant that keeps it correct under `net/http`:
// **the ResponseWriter is only ever touched from the handler goroutine.**
// `httpd.respond` (called on the interpreter / accept side) does not write to
// the ResponseWriter; it fills in the pending status / headers / body on the
// shared request state and closes a `done` channel. The parked handler
// goroutine wakes on that channel and writes the response itself. Cross-
// goroutine ResponseWriter use - which `net/http` forbids - never happens.

package httpdlib

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// parseListenAddr splits a listen address into a net.Listen network + address.
// A `unix:` prefix selects a Unix domain socket (`httpd.listen("unix:/run/app.sock")`),
// which reverse proxies like nginx can `proxy_pass` to; anything else is TCP.
func parseListenAddr(addr string) (string, string) {
	if strings.HasPrefix(addr, "unix:") {
		return "unix", strings.TrimPrefix(addr, "unix:")
	}
	return "tcp", addr
}

// maxBodyBytes caps how much request body the engine buffers per request. A
// tunable knob is a planned follow-up; the default keeps a runaway upload from
// exhausting memory.
const maxBodyBytes = 10 << 20 // 10 MiB

// readHeaderTimeout bounds how long a slow client may take to send request
// headers (Slowloris protection).
const readHeaderTimeout = 15 * time.Second

// bodyReadTimeout bounds how long a client may take to deliver its request
// body. The body is buffered while holding one of the maxInFlight admission
// slots, so without this bound 256 deliberately slow uploads would pin every
// slot indefinitely and stall the whole server (body-read Slowloris). 60 s
// for up to maxBodyBytes still admits a ~1.4 Mbps uplink. A var so tests can
// lower it.
var bodyReadTimeout = 60 * time.Second

// writeTimeout bounds how long the response write may take. Like the body
// read, the write happens while holding an admission slot, so without this a
// client that stops reading the response (a slow-read Slowloris, or just a
// stalled large download) pins its slot until the socket send buffer drains -
// 256 such clients stall the whole server. Armed per-response via
// ResponseController rather than http.Server.WriteTimeout so it bounds the
// whole write from its start, not from request receipt, and can be generous
// enough for large legitimate downloads. A var so tests can lower it.
var writeTimeout = 5 * time.Minute

// idleTimeout bounds how long an idle keep-alive connection may sit between
// requests. ReadHeaderTimeout does not cover the idle gap, so without this a
// client can open many keep-alives, send one request each, then idle forever -
// a goroutine + fd per connection with no bound (connection-exhaustion
// Slowloris). A var so tests can lower it (read at server construction).
var idleTimeout = 2 * time.Minute

// respondTimeout bounds how long a handler goroutine parks waiting for the
// program to answer an accepted request. If the program never responds (e.g.
// it threw between accept and respond), the handler answers 500 and unparks so
// the goroutine and client connection don't leak until server shutdown. It is
// a generous safety net, not a per-request SLA. A var so tests can lower it.
var respondTimeout = 60 * time.Second

// -------- Registries --------

// serverState holds one live listening server and the channel the handler
// goroutines hand accepted requests across.
type serverState struct {
	srv       *http.Server
	ln        net.Listener
	addr      string
	reqs      chan *reqState
	closing   chan struct{}
	closeOnce sync.Once
	// sem bounds how many requests may buffer a body concurrently, capping
	// worst-case buffered memory at maxInFlight * maxBodyBytes rather than
	// growing with the connection count.
	sem chan struct{}
}

// maxInFlight caps concurrent in-flight (body-buffered) requests. Bounds
// worst-case buffered RSS to maxInFlight * maxBodyBytes.
const maxInFlight = 256

type respHeader struct{ key, value string }

// reqState is the shared state for one in-flight request. The handler goroutine
// creates it, parks on done, and writes the response; the interpreter side
// reads the request and fills in the response fields before closing done.
type reqState struct {
	id   int64
	r    *http.Request
	body []byte
	done chan struct{}
	srv  *serverState

	mu           sync.Mutex
	responded    bool
	status       int
	respBody     []byte
	respHeaders  []respHeader
	useServeFile bool
	serveFile    string
}

var (
	serversMu    sync.Mutex
	servers      = map[int64]*serverState{}
	nextServerID int64

	reqsMu    sync.Mutex
	reqStates = map[int64]*reqState{}
	nextReqID int64
)

// ResetForTest wipes both registries between test runs.
func ResetForTest() {
	serversMu.Lock()
	for _, s := range servers {
		s.closeOnce.Do(func() { close(s.closing) })
		if s.srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = s.srv.Shutdown(ctx)
			cancel()
		}
	}
	servers = map[int64]*serverState{}
	nextServerID = 0
	serversMu.Unlock()

	reqsMu.Lock()
	reqStates = map[int64]*reqState{}
	nextReqID = 0
	reqsMu.Unlock()
}

func registerServer(st *serverState) int64 {
	serversMu.Lock()
	defer serversMu.Unlock()
	id := nextServerID
	nextServerID++
	servers[id] = st
	return id
}

func resolveServer(fnName string, id int64) (*serverState, error) {
	serversMu.Lock()
	defer serversMu.Unlock()
	st, ok := servers[id]
	if !ok {
		return nil, fmt.Errorf("%s: httpd.Server id %d is not running (already shut down, or never started)", fnName, id)
	}
	return st, nil
}

func registerReq(rs *reqState) int64 {
	reqsMu.Lock()
	defer reqsMu.Unlock()
	id := nextReqID
	nextReqID++
	rs.id = id
	reqStates[id] = rs
	return id
}

func unregisterReq(id int64) {
	reqsMu.Lock()
	delete(reqStates, id)
	reqsMu.Unlock()
}

func resolveReq(fnName string, id int64) (*reqState, error) {
	reqsMu.Lock()
	defer reqsMu.Unlock()
	rs, ok := reqStates[id]
	if !ok {
		return nil, fmt.Errorf("%s: httpd.Request id %d is not open (already responded, or never accepted)", fnName, id)
	}
	return rs, nil
}

// -------- handler --------

// makeHandler builds the net/http handler for a server. Each request runs on
// its own Go goroutine: it buffers the body, hands a reqState to the pull loop,
// parks until the interpreter responds (or the server shuts down), then writes
// the response from this goroutine.
func makeHandler(st *serverState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Admission control: bound how many requests buffer a body (and stay
		// in flight) concurrently, so N connections can't each buffer up to
		// maxBodyBytes before backpressure (~N * 10 MiB of RSS). Wait for a
		// slot or bail on shutdown; the slot is held for the whole handler and
		// released when the response is written / the request times out.
		select {
		case st.sem <- struct{}{}:
			defer func() { <-st.sem }()
		case <-st.closing:
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		}
		// Read one byte past the cap so an over-limit body is detectable:
		// it must be REJECTED with 413, never silently truncated - a
		// truncated-but-complete-looking body defeats body-signature
		// verification and smuggles content past inspection. The read runs
		// under bodyReadTimeout (armed on the connection, cleared after) so a
		// trickling client can't hold its admission slot forever.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Now().Add(bodyReadTimeout))
		body, rerr := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		_ = rc.SetReadDeadline(time.Time{})
		if rerr != nil {
			// A mid-body client disconnect (or bodyReadTimeout expiry) leaves a
			// truncated body; answer 400 here rather than hand the interpreter
			// a silently-short request. Connection: close makes the server skip
			// its keepalive body drain, which would otherwise block on the
			// bytes that never arrived (the deadline is already cleared).
			w.Header().Set("Connection", "close")
			http.Error(w, "error reading request body", http.StatusBadRequest)
			return
		}
		if int64(len(body)) > maxBodyBytes {
			// Same drain concern: the client may still be streaming the rest.
			w.Header().Set("Connection", "close")
			http.Error(w, "request body exceeds the server's limit", http.StatusRequestEntityTooLarge)
			return
		}
		rs := &reqState{r: r, body: body, done: make(chan struct{}), status: 200, srv: st}
		id := registerReq(rs)
		defer unregisterReq(id)

		select {
		case st.reqs <- rs:
		case <-st.closing:
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		case <-time.After(respondTimeout):
			// The program isn't draining the accept queue (stuck between
			// listen and accept, or accepting too slowly). Without this arm
			// the handler goroutine, its admission slot, and the client
			// connection would all park until server shutdown.
			http.Error(w, "server not accepting requests", http.StatusServiceUnavailable)
			return
		}

		select {
		case <-rs.done:
		case <-st.closing:
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		case <-time.After(respondTimeout):
			// The program accepted this request but never answered it (e.g. it
			// threw between accept and respond). Claim the response so a late
			// respond can't double-write, then answer 500 and unpark -
			// otherwise this handler goroutine and the client connection leak
			// until server shutdown.
			rs.mu.Lock()
			claimed := !rs.responded
			rs.responded = true
			rs.mu.Unlock()
			if claimed {
				http.Error(w, "handler did not respond in time", http.StatusInternalServerError)
				return
			}
			// The program responded within the timeout race window; fall
			// through and write its response (rs.done is closed / closing).
		}

		// Response I/O happens here, on the handler goroutine only.
		rs.mu.Lock()
		useFile, filePath := rs.useServeFile, rs.serveFile
		status, respBody := rs.status, rs.respBody
		headers := rs.respHeaders
		rs.mu.Unlock()

		// Bound the response write so a client that stops reading can't pin
		// this handler's admission slot (held until return) until the socket
		// send buffer drains. Armed here, at the start of the write, so a large
		// legitimate download gets the full writeTimeout rather than a budget
		// shared with request receipt.
		_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))

		// Apply the handler's headers first, so a Cache-Control / CORS /
		// Set-Cookie set before serveFile / serveDir is preserved (ServeFile
		// adds its own headers without clearing these).
		for _, h := range headers {
			// Set-Cookie is the canonical multi-value response header: a handler
			// may emit several. Add() keeps each; Set() (used for every other
			// header) would collapse them to the last one.
			if strings.EqualFold(h.key, "Set-Cookie") {
				w.Header().Add(h.key, h.value)
			} else {
				w.Header().Set(h.key, h.value)
			}
		}
		if useFile {
			http.ServeFile(w, r, filePath)
			return
		}
		w.WriteHeader(status)
		if len(respBody) > 0 {
			_, _ = w.Write(respBody)
		}
	})
}

// -------- lifecycle --------

func listenFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.listen expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("httpd.listen", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	network, address := parseListenAddr(addr)
	ln, err := net.Listen(network, address)
	if err != nil {
		// A unix socket path from a prior crash lingers and makes Listen fail
		// with EADDRINUSE. Only clear it when it is provably stale: the path is
		// a socket AND nothing is listening (a probe dial is refused). This
		// avoids deleting a live server's socket (which would silently break it).
		if network == "unix" && isStaleUnixSocket(address) {
			_ = os.Remove(address)
			ln, err = net.Listen(network, address)
		}
		if err != nil {
			return interpreter.Null(), fmt.Errorf("httpd.listen: %v", err)
		}
	}
	st := newServer(ln)
	go func() { _ = st.srv.Serve(ln) }()
	return makeServer(registerServer(st)), nil
}

// isStaleUnixSocket reports whether path is a leftover unix socket with no live
// server behind it: the path is a socket file and a probe dial is refused. A
// path that isn't a socket, or one that accepts a connection, is left untouched
// so a second instance never deletes a running server's socket.
func isStaleUnixSocket(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	c, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err != nil {
		return true // nobody is listening: the socket is stale
	}
	_ = c.Close()
	return false // a live server owns this socket
}

func listenTLSFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("httpd.listenTLS expects 3 arguments (address, cert, key), got %d", len(args))
	}
	addr, err := takeStringArg("httpd.listenTLS", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	certPEM, err := takeBytesArg("httpd.listenTLS", args, 1, "cert")
	if err != nil {
		return interpreter.Null(), err
	}
	keyPEM, err := takeBytesArg("httpd.listenTLS", args, 2, "key")
	if err != nil {
		return interpreter.Null(), err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("httpd.listenTLS: bad certificate / key pair: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("httpd.listenTLS: %v", err)
	}
	st := newServer(ln)
	// Floor at TLS 1.2: 1.0/1.1 are deprecated (RFC 8996) and fail compliance
	// scans.
	st.srv.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	go func() { _ = st.srv.ServeTLS(ln, "", "") }()
	return makeServer(registerServer(st)), nil
}

// newServer builds the serverState + http.Server for an already-open listener.
func newServer(ln net.Listener) *serverState {
	st := &serverState{
		ln:      ln,
		addr:    ln.Addr().String(),
		reqs:    make(chan *reqState),
		closing: make(chan struct{}),
		sem:     make(chan struct{}, maxInFlight),
	}
	st.srv = &http.Server{
		Handler:           makeHandler(st),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
	return st
}

func addressFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.address expects 1 argument (httpd.Server), got %d", len(args))
	}
	id, err := extractID("httpd.address", "Server", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	st, err := resolveServer("httpd.address", id)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(st.addr), nil
}

func shutdownFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.shutdown expects 1 argument (httpd.Server), got %d", len(args))
	}
	id, err := extractID("httpd.shutdown", "Server", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	st, err := resolveServer("httpd.shutdown", id)
	if err != nil {
		return interpreter.Null(), err
	}
	// Unblock parked handlers and accept() callers, then drain gracefully.
	st.closeOnce.Do(func() { close(st.closing) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = st.srv.Shutdown(ctx)
	serversMu.Lock()
	delete(servers, id)
	serversMu.Unlock()
	return interpreter.Null(), nil
}

// -------- pull loop --------

func acceptFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.accept expects 1 argument (httpd.Server), got %d", len(args))
	}
	id, err := extractID("httpd.accept", "Server", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	st, err := resolveServer("httpd.accept", id)
	if err != nil {
		return interpreter.Null(), err
	}
	select {
	case rs := <-st.reqs:
		return makeRequest(rs.id), nil
	case <-st.closing:
		return interpreter.Null(), fmt.Errorf("httpd.accept: server has been shut down")
	}
}

// -------- request accessors --------

// reqField resolves a Request handle to its state for a read accessor.
func reqField(fnName string, args []Value) (*reqState, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s expects a httpd.Request argument", fnName)
	}
	id, err := extractID(fnName, "Request", args[0])
	if err != nil {
		return nil, err
	}
	return resolveReq(fnName, id)
}

func methodFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.method expects 1 argument (httpd.Request), got %d", len(args))
	}
	rs, err := reqField("httpd.method", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(rs.r.Method), nil
}

func pathFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.path expects 1 argument (httpd.Request), got %d", len(args))
	}
	rs, err := reqField("httpd.path", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(rs.r.URL.Path), nil
}

func queryFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("httpd.query expects 2 arguments (httpd.Request, name), got %d", len(args))
	}
	rs, err := reqField("httpd.query", args)
	if err != nil {
		return interpreter.Null(), err
	}
	name, err := takeStringArg("httpd.query", args, 1, "name")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(rs.r.URL.Query().Get(name)), nil
}

func headerFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("httpd.header expects 2 arguments (httpd.Request, name), got %d", len(args))
	}
	rs, err := reqField("httpd.header", args)
	if err != nil {
		return interpreter.Null(), err
	}
	name, err := takeStringArg("httpd.header", args, 1, "name")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(rs.r.Header.Get(name)), nil
}

func bodyFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.body expects 1 argument (httpd.Request), got %d", len(args))
	}
	rs, err := reqField("httpd.body", args)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]byte, len(rs.body))
	copy(out, rs.body)
	return interpreter.BytesVal(out), nil
}

func remoteAddrFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("httpd.remoteAddr expects 1 argument (httpd.Request), got %d", len(args))
	}
	rs, err := reqField("httpd.remoteAddr", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(rs.r.RemoteAddr), nil
}

// -------- response --------

func setHeaderFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("httpd.setHeader expects 3 arguments (httpd.Request, name, value), got %d", len(args))
	}
	rs, err := reqField("httpd.setHeader", args)
	if err != nil {
		return interpreter.Null(), err
	}
	name, err := takeStringArg("httpd.setHeader", args, 1, "name")
	if err != nil {
		return interpreter.Null(), err
	}
	value, err := takeStringArg("httpd.setHeader", args, 2, "value")
	if err != nil {
		return interpreter.Null(), err
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.responded {
		return interpreter.Null(), fmt.Errorf("httpd.setHeader: request already answered")
	}
	rs.respHeaders = append(rs.respHeaders, respHeader{key: name, value: value})
	return interpreter.Null(), nil
}

func respondFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("httpd.respond expects 3 arguments (httpd.Request, status, body), got %d", len(args))
	}
	rs, err := reqField("httpd.respond", args)
	if err != nil {
		return interpreter.Null(), err
	}
	status, err := takeIntArg("httpd.respond", args, 1, "status")
	if err != nil {
		return interpreter.Null(), err
	}
	// net/http's WriteHeader panics for a status outside [100, 999]; reject it
	// at the boundary so a bad status is a catchable error, not an aborted
	// connection with a logged panic.
	if status < 100 || status > 999 {
		return interpreter.Null(), fmt.Errorf("httpd.respond: status %d out of range [100, 999]", status)
	}
	body, err := takeBodyArg("httpd.respond", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	rs.mu.Lock()
	if rs.responded {
		rs.mu.Unlock()
		return interpreter.Null(), fmt.Errorf("httpd.respond: request already answered")
	}
	rs.responded = true
	rs.status = int(status)
	rs.respBody = body
	rs.mu.Unlock()
	close(rs.done)
	return interpreter.Null(), nil
}

func serveFileFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("httpd.serveFile expects 2 arguments (httpd.Request, path), got %d", len(args))
	}
	rs, err := reqField("httpd.serveFile", args)
	if err != nil {
		return interpreter.Null(), err
	}
	p, err := takeStringArg("httpd.serveFile", args, 1, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	return answerWithFile(rs, p)
}

func serveDirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("httpd.serveDir expects 2 arguments (httpd.Request, root), got %d", len(args))
	}
	rs, err := reqField("httpd.serveDir", args)
	if err != nil {
		return interpreter.Null(), err
	}
	root, err := takeStringArg("httpd.serveDir", args, 1, "root")
	if err != nil {
		return interpreter.Null(), err
	}
	// path.Clean on a rooted path collapses any ".." so the request cannot
	// escape root; then map slashes to the host separator under root.
	clean := path.Clean("/" + rs.r.URL.Path)
	full := filepath.Join(root, filepath.FromSlash(clean))
	return answerWithFile(rs, full)
}

// answerWithFile marks a request to be answered by ServeFile from the handler
// goroutine.
func answerWithFile(rs *reqState, p string) (Value, error) {
	rs.mu.Lock()
	if rs.responded {
		rs.mu.Unlock()
		return interpreter.Null(), fmt.Errorf("request already answered")
	}
	rs.responded = true
	rs.useServeFile = true
	rs.serveFile = p
	rs.mu.Unlock()
	close(rs.done)
	return interpreter.Null(), nil
}
