// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

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

	"github.com/mplx/jennifer-lang/internal/interpreter"
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
}

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
		body, rerr := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if rerr != nil {
			// A mid-body client disconnect leaves a truncated body; answer 400
			// here rather than hand the interpreter a silently-short request.
			http.Error(w, "error reading request body", http.StatusBadRequest)
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
		}

		select {
		case <-rs.done:
		case <-st.closing:
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		}

		// Response I/O happens here, on the handler goroutine only.
		rs.mu.Lock()
		useFile, filePath := rs.useServeFile, rs.serveFile
		status, respBody := rs.status, rs.respBody
		headers := rs.respHeaders
		rs.mu.Unlock()

		if useFile {
			http.ServeFile(w, r, filePath)
			return
		}
		for _, h := range headers {
			w.Header().Set(h.key, h.value)
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
	if network == "unix" {
		// Clear a stale socket left by a prior crash; a graceful shutdown
		// unlinks its own socket, so this only matters after an abnormal exit.
		_ = os.Remove(address)
	}
	ln, err := net.Listen(network, address)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("httpd.listen: %v", err)
	}
	st := newServer(ln)
	go func() { _ = st.srv.Serve(ln) }()
	return makeServer(registerServer(st)), nil
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
	st.srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
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
	}
	st.srv = &http.Server{
		Handler:           makeHandler(st),
		ReadHeaderTimeout: readHeaderTimeout,
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
