// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// Standard-Go implementation of the `net` library. Used by
// the default `jennifer` binary (standard-Go build). Under TinyGo
// (`jennifer-tiny`), the netlib_tinygo.go file is selected instead
// and returns friendly runtime errors from every entry point.

package netlib

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	stdnet "net"
	"sync"
	"syscall"
	"time"

	"jennifer-lang.dev/jennifer/internal/dnsconf"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// clampMillis converts a non-negative millisecond count to a time.Duration,
// clamping a value so large it would overflow int64 nanoseconds to the maximum
// representable duration (~292 years) instead of wrapping to a negative one. A
// "never time out" value thus behaves as an effectively-unbounded deadline, not
// the instant-timeout or infinite-wait the wrap produced.
func clampMillis(ms int64) time.Duration {
	if !limits.DurationFitsUnits(ms, time.Millisecond) {
		return time.Duration(limits.MaxDurationUnits(time.Millisecond)) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

// -------- Registries --------

// connState holds one live TCP connection. The buffered reader
// backs partial-length reads and the sticky-EOF idiom the
// fs handles established.
type connState struct {
	// mu guards the mutable fields below (c / r swap on startTLS, sticky) so a
	// spawned reader task and a main-task startTLS don't race on them. It is
	// held only for the short field-access critical sections, never across a
	// blocking read/write, so a concurrent net.setReadDeadline can still interrupt
	// an in-flight read.
	mu     sync.Mutex
	c      stdnet.Conn
	r      *bufio.Reader
	sticky bool
	host   string // bare hostname dialled (for a later startTLS); empty for accepted conns

	// readMu serializes the blocking reads on r (readBytes and the eof
	// probe). The eof probe arms a short deadline on the shared conn; if it
	// ran while another task sat in a blocking read, that read would fail
	// with a spurious timeout. eof only TryLocks it, so a conn with a read
	// in flight reports "not EOF" instead of blocking.
	readMu sync.Mutex
	// deadline is the read deadline last armed via net.setReadDeadline (zero when
	// cleared), so the eof probe can restore it instead of wiping it.
	deadline time.Time
	// writeDeadline is the write deadline last armed via net.setWriteDeadline,
	// which startTLS restores after its handshake deadline.
	writeDeadline time.Time
	// idle is the idle timeout set via net.setIdleTimeout (0 = none): every
	// read re-arms the read deadline to now+idle before each underlying read.
	idle time.Duration
}

// cancelWatch lets task.cancel interrupt a blocking read, as it does
// time.sleep: when the calling task is cancelled, the read deadline moves into
// the past, which wakes the read. Without it a read in a cancelled (or
// discarded) task runs on until its peer answers or its idle deadline passes,
// holding the connection - a peer trickling a byte per idle period holds it
// indefinitely. nil (no watch) outside a task.
type cancelWatch struct {
	mu    sync.Mutex // orders the cancel against a re-arm of the deadline
	c     deadlineSetter
	fired bool
	done  chan struct{}
	ended chan struct{}
	once  sync.Once
}

func watchCancel(ctx interpreter.BuiltinCtx, c deadlineSetter) *cancelWatch {
	if ctx.Cancel == nil {
		return nil
	}
	w := &cancelWatch{c: c, done: make(chan struct{}), ended: make(chan struct{})}
	go func() {
		defer close(w.ended)
		select {
		case <-ctx.Cancel.CancelSignal():
			w.mu.Lock()
			w.fired = true
			_ = c.SetReadDeadline(time.Unix(1, 0))
			w.mu.Unlock()
		case <-w.done:
		}
	}()
	return w
}

// arm sets the read deadline unless the task was cancelled, so a per-chunk
// idle re-arm cannot undo the cancel.
func (w *cancelWatch) arm(when time.Time) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.fired {
		_ = w.c.SetReadDeadline(when)
	}
}

// stop ends the watch (more calls are no-ops) and reports whether the cancel
// fired; once it returns the deadline is no longer touched.
func (w *cancelWatch) stop() bool {
	if w == nil {
		return false
	}
	w.once.Do(func() { close(w.done) })
	<-w.ended
	return w.fired
}

// idleArmer returns the function a read calls before each underlying read
// when an idle timeout applies: it arms the read deadline to now+idle, or to
// the setReadDeadline deadline `user` when that comes first, so both bounds
// hold and whichever passes first fires. nil when idle is 0. A cancel watch
// arms through w, so a re-arm cannot undo a cancel.
func idleArmer(c deadlineSetter, w *cancelWatch, idle time.Duration, user time.Time) func() {
	if idle <= 0 {
		return nil
	}
	return func() {
		when := time.Now().Add(idle)
		if !user.IsZero() && user.Before(when) {
			when = user
		}
		if w != nil {
			w.arm(when)
		} else {
			_ = c.SetReadDeadline(when)
		}
	}
}

type listenerState struct {
	l stdnet.Listener
}

type udpState struct {
	c stdnet.PacketConn
	// mu guards deadline, the read deadline last armed by net.setReadDeadline
	// (zero when cleared), which a recvFrom woken by task.cancel restores.
	mu       sync.Mutex
	deadline time.Time
}

// maxNetHandles bounds each net registry (conns / listeners / udps) so a script
// that leaks handles (dials / listens without a matching net.close) surfaces a
// catchable error at a friendly limit rather than growing until it hits the OS
// file-descriptor ceiling.
const maxNetHandles = 4096

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
const maxReadBytes = limits.MaxAllocBytes

// eofProbeTimeout bounds net.eof's non-blocking EOF probe: a closed peer's
// pending FIN surfaces immediately (well inside this window); on an open idle
// connection the probe times out and eof reports false rather than blocking.
const eofProbeTimeout = 20 * time.Millisecond

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
		return nil, notOpen(fmt.Sprintf("%s: net.Conn id %d is not open (already closed, or never opened)", fnName, id))
	}
	return s, nil
}

func resolveListener(fnName string, id int64) (*listenerState, error) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	s, ok := listeners[id]
	if !ok {
		return nil, notOpen(fmt.Sprintf("%s: net.Listener id %d is not open (already closed, or never opened)", fnName, id))
	}
	return s, nil
}

func resolveUDP(fnName string, id int64) (*udpState, error) {
	udpsMu.Lock()
	defer udpsMu.Unlock()
	s, ok := udps[id]
	if !ok {
		return nil, notOpen(fmt.Sprintf("%s: net.UDPSocket id %d is not open (already closed, or never opened)", fnName, id))
	}
	return s, nil
}

// -------- TCP --------

// maxDatagram is the largest UDP payload (65,535 minus the 8-byte UDP header;
// an IPv4 datagram tops out lower, an IPv6 jumbogram needs a special path).
const maxDatagram = 65535 - 8

// notOpen is the error for a handle that is closed (or was never opened):
// Error{kind: "closed"}, the kind a call blocked on a socket that gets closed
// raises too (failure), so a loop handles both the same way.
func notOpen(message string) error {
	return interpreter.KindedError("closed", message)
}

// failure turns a socket error into the error a Jennifer program sees. A
// deadline or dial timeout (a net.Error whose Timeout() is true) becomes a
// catchable Error{kind: "timeout"} reading "<prefix>: <op> timed out", the same
// shape for every call, TCP and UDP alike, so a poll loop can tell "nothing
// arrived yet" from a closed or broken socket by kind rather than by message. A
// socket closed under a blocked call - `net.close` from another task, the way
// to stop a task parked in `accept` or a read - becomes Error{kind: "closed"}
// reading "<prefix>: <op> on a closed socket", the kind a call on an
// already-closed handle raises too (notOpen). Anything else is a transport
// failure: a net.NetError (kind "net") whose reason says what went wrong and
// whose message is the OS text, as it read when this was a runtime error.
func failure(ctx interpreter.BuiltinCtx, prefix, op string, err error) error {
	var ne stdnet.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return interpreter.RaiseError("timeout", fmt.Sprintf("%s: %s timed out", prefix, op), ctx.File, ctx.Line, ctx.Col)
	}
	if errors.Is(err, stdnet.ErrClosed) {
		return interpreter.RaiseError("closed", fmt.Sprintf("%s: %s on a closed socket", prefix, op), ctx.File, ctx.Line, ctx.Col)
	}
	if isAddrError(err) {
		return fmt.Errorf("%s: %v", prefix, err)
	}
	reason := failureReason(op, err)
	msg := fmt.Sprintf("%s: %v", prefix, err)
	if reason == "dns" {
		msg = dnsconf.Setup().Explain(msg)
	}
	return newNetError(ctx, op, reason, msg)
}

// isAddrError reports a malformed address (no port, a bad port): the caller's
// input, not the network, so it stays a runtime error.
func isAddrError(err error) bool {
	var addrErr *stdnet.AddrError
	return errors.As(err, &addrErr)
}

// failureReason classifies a socket error for NetError.reason: a failed name
// lookup is "dns"; refused, reset (a broken pipe and an aborted connection
// too: the peer tore the connection down) and unreachable come from the OS
// error number (errnoReason, per platform); a certificate or TLS protocol
// error, or anything else that fails a handshake, is "tls"; the rest "other".
func failureReason(op string, err error) string {
	var dnsErr *stdnet.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if r := errnoReason(errno); r != "" {
			return r
		}
	}
	if isTLSError(err) || op == "handshake" {
		return "tls"
	}
	return "other"
}

// isTLSError reports whether err comes from certificate verification or the
// TLS protocol (a bad record, an alert from the peer).
func isTLSError(err error) bool {
	var certErr *tls.CertificateVerificationError
	var authErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	var recordErr tls.RecordHeaderError
	var alertErr tls.AlertError
	var opErr *stdnet.OpError
	switch {
	case errors.As(err, &certErr), errors.As(err, &authErr), errors.As(err, &hostErr),
		errors.As(err, &invalidErr), errors.As(err, &recordErr), errors.As(err, &alertErr):
		return true
	case errors.As(err, &opErr) && opErr.Op == "remote error":
		return true
	}
	return false
}

// tlsDialOp names the step a failed tls.Dial stopped at: "connect" when the
// TCP dial (or its name lookup) failed, else "handshake".
func tlsDialOp(err error) string {
	var opErr *stdnet.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return "connect"
	}
	return "handshake"
}

// DefaultTimeoutMs is the deadline a zero (or absent) timeoutMs stands for: it
// bounds a connect together with its TLS handshake, a startTLS handshake and a
// lookup, never a later read or write.
const DefaultTimeoutMs = 30000

// noDeadlineMs is the timeoutMs that asks for no deadline: the largest int.
const noDeadlineMs = math.MaxInt64

// defaultTimeout is DefaultTimeoutMs as a Duration; a variable so tests can
// lower it instead of waiting 30 s.
var defaultTimeout = DefaultTimeoutMs * time.Millisecond

// resolveTimeout turns a caller's timeoutMs into the deadline to arm: 0 is the
// default, the largest int is none (returned as 0), a negative value throws.
func resolveTimeout(fnName string, ms int64) (time.Duration, error) {
	switch {
	case ms < 0:
		return 0, fmt.Errorf("%s: timeoutMs must be >= 0 (0 = the default %d, %d = none), got %d", fnName, DefaultTimeoutMs, int64(noDeadlineMs), ms)
	case ms == 0:
		return defaultTimeout, nil
	case ms == noDeadlineMs:
		return 0, nil
	}
	return clampMillis(ms), nil
}

// dialOptions is a net.ConnectOptions read into Go: the resolved deadline (0 =
// none), the tls flag and the TLSOptions fields.
type dialOptions struct {
	timeout    time.Duration
	tls        bool
	skipVerify bool
	caCert     []byte
	dnsServers []string // host:port; only lookupWith asks them
}

// maxDNSServers bounds ConnectOptions.dnsServers: each is tried in turn, so a
// long list only multiplies the wait.
const maxDNSServers = 16

// connectOptions reads a net.ConnectOptions argument.
func connectOptions(fnName string, v Value) (dialOptions, error) {
	var o dialOptions
	if v.Kind != interpreter.KindStruct || v.StructNS() != LibraryName || v.StructName() != "ConnectOptions" {
		return o, fmt.Errorf("%s: options must be a net.ConnectOptions, got %s", fnName, typeName(v))
	}
	ms := int64(0)
	for _, f := range v.Fields() {
		switch f.Name {
		case "timeoutMs":
			if f.Value.Kind != interpreter.KindInt {
				return o, fmt.Errorf("%s: net.ConnectOptions.timeoutMs must be int, got %s", fnName, f.Value.Kind)
			}
			ms = f.Value.Int()
		case "tls":
			if f.Value.Kind != interpreter.KindBool {
				return o, fmt.Errorf("%s: net.ConnectOptions.tls must be bool, got %s", fnName, f.Value.Kind)
			}
			o.tls = f.Value.Bool()
		case "tlsOptions":
			if f.Value.Kind == interpreter.KindNull {
				continue
			}
			skip, caCert, err := tlsOptions(fnName, []Value{f.Value}, 0)
			if err != nil {
				return o, err
			}
			o.skipVerify, o.caCert = skip, caCert
		case "dnsServers":
			servers, err := dnsServers(fnName, f.Value)
			if err != nil {
				return o, err
			}
			o.dnsServers = servers
		}
	}
	timeout, err := resolveTimeout(fnName, ms)
	if err != nil {
		return o, err
	}
	o.timeout = timeout
	return o, nil
}

// dnsServers reads ConnectOptions.dnsServers: each "ip" or "ip:port" (port 53
// by default) as host:port; a host name is an error.
func dnsServers(fnName string, v Value) ([]string, error) {
	if v.Kind == interpreter.KindNull {
		return nil, nil
	}
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("%s: net.ConnectOptions.dnsServers must be list of string, got %s", fnName, v.Kind)
	}
	list := v.List()
	if len(list) > maxDNSServers {
		return nil, fmt.Errorf("%s: net.ConnectOptions.dnsServers names %d servers, at most %d", fnName, len(list), maxDNSServers)
	}
	var servers []string
	for _, e := range list {
		if e.Kind != interpreter.KindString {
			return nil, fmt.Errorf("%s: net.ConnectOptions.dnsServers must be list of string, got an element of %s", fnName, e.Kind)
		}
		addr, err := dnsconf.ServerAddr(e.Str())
		if err != nil {
			return nil, fmt.Errorf("%s: %v", fnName, err)
		}
		servers = append(servers, addr)
	}
	return servers, nil
}

// typeName names a value's type for an error: net.X for a net struct.
func typeName(v Value) string {
	if v.Kind == interpreter.KindStruct && v.StructName() != "" {
		if v.StructNS() != "" {
			return v.StructNS() + "." + v.StructName()
		}
		return v.StructName()
	}
	return v.Kind.String()
}

// legacyTimeout reads the deprecated trailing timeoutMs int of connect /
// connectTLS / startTLS (removed in 0.29.0) with the ConnectOptions rule.
func legacyTimeout(fnName string, v Value) (time.Duration, error) {
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: timeout must be int milliseconds, got %s", fnName, v.Kind)
	}
	return resolveTimeout(fnName, v.Int())
}

// connectFn implements `net.connect(address) -> net.Conn`, a plaintext
// connectWith with the zero ConnectOptions. The deprecated
// `net.connect(address, timeoutMs)` still works until 0.29.0.
func connectFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("net.connect expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.connect", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	o := dialOptions{timeout: defaultTimeout}
	if len(args) == 2 {
		if o.timeout, err = legacyTimeout("net.connect", args[1]); err != nil {
			return interpreter.Null(), err
		}
	}
	return dial(ctx, "net.connect", addr, o)
}

// connectWithFn implements `net.connectWith(address, net.ConnectOptions) ->
// net.Conn`: connect, with TLS when the options say so, within timeoutMs.
func connectWithFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.connectWith expects 2 arguments (address, net.ConnectOptions), got %d", len(args))
	}
	addr, err := takeStringArg("net.connectWith", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	o, err := connectOptions("net.connectWith", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return dial(ctx, "net.connectWith", addr, o)
}

// dial opens a connection: TCP, then a TLS handshake when o.tls, both within
// o.timeout (0 = no deadline). The deadline ends with the handshake; later
// reads and writes are bounded only by setReadDeadline / setWriteDeadline.
func dial(ctx interpreter.BuiltinCtx, fnName, addr string, o dialOptions) (Value, error) {
	host, _, splitErr := stdnet.SplitHostPort(addr) // recorded so a later startTLS can reuse it
	dialer := &stdnet.Dialer{Timeout: o.timeout}
	var c stdnet.Conn
	if o.tls {
		if splitErr != nil {
			return interpreter.Null(), fmt.Errorf("%s: address must be host:port: %v", fnName, splitErr)
		}
		cfg, err := clientTLSConfig(fnName, host, o.skipVerify, o.caCert)
		if err != nil {
			return interpreter.Null(), err
		}
		// DialWithDialer applies the dialer's timeout to the connect and the
		// handshake as a whole, and leaves no deadline on the connection.
		tc, dialErr := tls.DialWithDialer(dialer, "tcp", addr, cfg)
		if dialErr != nil {
			return interpreter.Null(), tlsDialFailure(ctx, fnName+": "+addr, dialErr)
		}
		c = tc
	} else {
		tcp, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			return interpreter.Null(), failure(ctx, fnName+": "+addr, "connect", dialErr)
		}
		c = tcp
	}
	connsMu.Lock()
	if len(conns) >= maxNetHandles {
		connsMu.Unlock()
		_ = c.Close()
		return interpreter.Null(), limits.Capf("net: too many open connections (limit %d); each connect needs a matching net.close", maxNetHandles)
	}
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
	if v.Kind != interpreter.KindStruct || v.StructNS() != LibraryName || v.StructName() != "TLSOptions" {
		return false, nil, fmt.Errorf("%s: options must be a net.TLSOptions, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields() {
		switch f.Name {
		case "skipVerify":
			if f.Value.Kind != interpreter.KindBool {
				return false, nil, fmt.Errorf("%s: net.TLSOptions.skipVerify must be bool, got %s", fnName, f.Value.Kind)
			}
			skipVerify = f.Value.Bool()
		case "caCert":
			if f.Value.Kind != interpreter.KindBytes {
				return false, nil, fmt.Errorf("%s: net.TLSOptions.caCert must be bytes, got %s", fnName, f.Value.Kind)
			}
			caCert = f.Value.Bytes()
		}
	}
	return skipVerify, caCert, nil
}

// tlsArgLayout classifies the optional trailing args of connectTLS / startTLS,
// which share the shape `fn(head[, net.TLSOptions][, timeoutMs])` (head at
// index 0; the trailing timeoutMs is deprecated, removed in 0.29.0). It
// returns the index of the TLSOptions argument and of the timeoutMs argument,
// each set to len(args) (the "absent" sentinel tlsOptions reads as
// not-present) when that argument is omitted. A timeout is recognised by being
// an int, TLSOptions by being a struct; when both are present the order is
// fixed (options, then timeout).
func tlsArgLayout(fnName string, args []Value) (optsIdx, timeoutIdx int, err error) {
	optsIdx, timeoutIdx = len(args), len(args)
	switch len(args) {
	case 1:
		// head only
	case 2:
		if args[1].Kind == interpreter.KindInt {
			timeoutIdx = 1
		} else {
			optsIdx = 1
		}
	case 3:
		if args[1].Kind == interpreter.KindInt {
			return 0, 0, fmt.Errorf("%s: with three arguments the order is (..., net.TLSOptions, timeoutMs)", fnName)
		}
		optsIdx, timeoutIdx = 1, 2
	}
	return optsIdx, timeoutIdx, nil
}

// bufferedConn presents a bufio.Reader's already-buffered bytes (then the
// raw connection) as a net.Conn, so a TLS handshake begun mid-stream
// (startTLS) never drops plaintext the reader read ahead.
type bufferedConn struct {
	r *bufio.Reader
	stdnet.Conn
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// connectTLSFn implements `net.connectTLS(address[, net.TLSOptions]) ->
// net.Conn`, connectWith with `tls: true`: dial `address` (host:port) and
// complete a TLS handshake within the default deadline, verifying the server
// certificate against the address's host. The deprecated trailing timeoutMs
// still works until 0.29.0.
func connectTLSFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("net.connectTLS expects 1 or 2 arguments (address[, net.TLSOptions]), got %d", len(args))
	}
	addr, err := takeStringArg("net.connectTLS", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	optsIdx, timeoutIdx, err := tlsArgLayout("net.connectTLS", args)
	if err != nil {
		return interpreter.Null(), err
	}
	o := dialOptions{timeout: defaultTimeout, tls: true}
	if o.skipVerify, o.caCert, err = tlsOptions("net.connectTLS", args, optsIdx); err != nil {
		return interpreter.Null(), err
	}
	if timeoutIdx < len(args) {
		if o.timeout, err = legacyTimeout("net.connectTLS", args[timeoutIdx]); err != nil {
			return interpreter.Null(), err
		}
	}
	return dial(ctx, "net.connectTLS", addr, o)
}

// listenFailure is the error of a failed listen: a malformed address stays a
// runtime error, anything else (the address in use, not available here, a
// name that does not resolve) is a NetError with op "listen".
func listenFailure(ctx interpreter.BuiltinCtx, msg string, err error) error {
	if isAddrError(err) {
		return errors.New(msg)
	}
	return newNetError(ctx, "listen", failureReason("listen", err), msg)
}

// tlsDialFailure is failure for tls.Dial, which dials and shakes hands in one
// call: a timeout or closed socket reads "connect" as before, and a NetError
// names the step that failed (tlsDialOp).
func tlsDialFailure(ctx interpreter.BuiltinCtx, prefix string, err error) error {
	op := tlsDialOp(err)
	if op == "connect" {
		return failure(ctx, prefix, "connect", err)
	}
	var ne stdnet.Error
	if (errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, stdnet.ErrClosed) {
		return failure(ctx, prefix, "connect", err)
	}
	return newNetError(ctx, op, failureReason(op, err), fmt.Sprintf("%s: %v", prefix, err))
}

// readAllFn implements `net.readAll(conn [, maxBytes [, idleTimeoutMs]]) ->
// bytes`: read from the connection until EOF and return the whole remaining
// stream as one bytes, grown once in a Go loop (32 KiB chunks) instead of the
// per-byte interpreted accumulation a `.j` read-to-EOF loop pays. This is the
// throughput fix for whole-body reads (an HTTP / S3 object download).
//
// maxBytes controls the ceiling: omitted or 0 uses the default 256 MiB cap
// (matching net.readBytes / readN / recvFrom, so an attacker-shaped endless
// stream is a catchable error, not an OOM of the recover-less interpreter); a
// value > 0 sets an explicit cap; a *negative* value is the explicit "no limit"
// sentinel for the rare trusted-peer case. The idle timeout of
// net.setIdleTimeout (or the deprecated trailing idleTimeoutMs, which takes
// precedence for the call) re-arms the read deadline before each chunk.
func readAllFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("net.readAll expects 1 to 3 arguments (net.Conn[, maxBytes[, idleTimeoutMs]]), got %d", len(args))
	}
	id, err := extractID("net.readAll", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	// Default finite cap; omitted or 0 keeps it, >0 overrides it, <0 disables it
	// (the explicit unlimited sentinel). readAll extracts the arg itself instead
	// of via takeIntArg so a negative sentinel is accepted here only.
	capped := true
	maxBytes := int64(maxReadBytes)
	if len(args) >= 2 {
		if args[1].Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("net.readAll: maxBytes must be int, got %s", args[1].Kind)
		}
		switch m := args[1].Int(); {
		case m > 0:
			maxBytes = m
		case m < 0:
			capped = false
		}
	}
	idleMs := int64(0)
	if len(args) == 3 {
		if idleMs, err = takeIntArg("net.readAll", args, 2, "idleTimeoutMs"); err != nil {
			return interpreter.Null(), err
		}
		if idleMs < 0 {
			return interpreter.Null(), fmt.Errorf("net.readAll: idleTimeoutMs must be >= 0, got %d", idleMs)
		}
	}
	s, err := resolveConn("net.readAll", id)
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	r := s.r
	c := s.c
	sticky := s.sticky
	idle, userDeadline := s.idle, s.deadline
	s.mu.Unlock()
	if sticky {
		return interpreter.BytesVal([]byte{}), nil
	}
	if idleMs > 0 {
		idle = clampMillis(idleMs)
	}
	var out []byte
	tmp := make([]byte, 32*1024)
	s.readMu.Lock()
	defer s.readMu.Unlock()
	w := watchCancel(ctx, c)
	if idle > 0 || w != nil {
		// The idle re-arm below (or a cancel) overwrites the conn's read
		// deadline; restore the one armed via net.setReadDeadline (zero when
		// none) on every exit so a later plain read doesn't inherit a stale
		// deadline and spuriously time out. Same restore contract as the eof
		// probe. Deferred first, so it runs after the watch has stopped.
		defer func() { _ = c.SetReadDeadline(userDeadline) }()
	}
	defer w.stop()
	arm := idleArmer(c, w, idle, userDeadline)
	for {
		if arm != nil {
			arm()
		}
		nread, rerr := r.Read(tmp)
		if rerr != nil && w.stop() {
			return interpreter.Null(), interpreter.ErrTaskCancelled()
		}
		if nread > 0 {
			if capped && int64(len(out))+int64(nread) > maxBytes {
				// kind "limit": a cap was reached, so a caller tells it from an
				// I/O failure by kind, not by the message text
				return interpreter.Null(), interpreter.KindedError("limit", fmt.Sprintf("net.readAll: stream exceeds the %d-byte limit", maxBytes))
			}
			out = append(out, tmp[:nread]...)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				s.mu.Lock()
				s.sticky = true
				s.mu.Unlock()
				return interpreter.BytesVal(out), nil
			}
			return interpreter.Null(), failure(ctx, "net.readAll", "read", rerr)
		}
	}
}

// readNFn implements `net.readN(conn, n [, idleTimeoutMs]) -> bytes`: read
// exactly n bytes for a length-prefixed frame (an MQTT remaining-length body,
// an IMAP `{N}` literal, a RESP bulk string) in one Go loop, not a per-byte
// interpreted accumulation. A peer that closes after fewer than n bytes is a
// catchable "closed mid-frame" error, never a truncated return. The idle
// timeout works as in readAll.
func readNFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("net.readN expects 2 or 3 arguments (net.Conn, n[, idleTimeoutMs]), got %d", len(args))
	}
	id, err := extractID("net.readN", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := takeIntArg("net.readN", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("net.readN: n must be >= 0, got %d", n)
	}
	// Same per-call ceiling readBytes / recvFrom use: a caller-supplied (often
	// wire-derived) length can't force an unbounded up-front allocation.
	if n > maxReadBytes {
		return interpreter.Null(), limits.Capf("net.readN: %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	idleMs := int64(0)
	if len(args) == 3 {
		if idleMs, err = takeIntArg("net.readN", args, 2, "idleTimeoutMs"); err != nil {
			return interpreter.Null(), err
		}
		if idleMs < 0 {
			return interpreter.Null(), fmt.Errorf("net.readN: idleTimeoutMs must be >= 0, got %d", idleMs)
		}
	}
	s, err := resolveConn("net.readN", id)
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	r := s.r
	c := s.c
	idle, userDeadline := s.idle, s.deadline
	s.mu.Unlock()
	if idleMs > 0 {
		idle = clampMillis(idleMs)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	w := watchCancel(ctx, c)
	if idle > 0 || w != nil {
		// Restore the net.setReadDeadline deadline (zero when none) on every exit,
		// as in readAll: the per-chunk idle re-arm (or a cancel) must not leak
		// into later reads on this conn.
		defer func() { _ = c.SetReadDeadline(userDeadline) }()
	}
	arm := idleArmer(c, w, idle, userDeadline)
	// The buffer grows with the bytes that arrive, so a wire-derived n from a
	// peer that then stalls does not commit n bytes up front.
	buf, rerr := limits.ReadN(r, n, arm)
	if w.stop() {
		return interpreter.Null(), interpreter.ErrTaskCancelled()
	}
	if rerr != nil {
		if errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF) {
			s.mu.Lock()
			s.sticky = true
			s.mu.Unlock()
			return interpreter.Null(), interpreter.KindedError("closed", fmt.Sprintf("net.readN: connection closed after %d of %d bytes", len(buf), n))
		}
		return interpreter.Null(), failure(ctx, "net.readN", "read", rerr)
	}
	return interpreter.BytesVal(buf), nil
}

// startTLSFn implements `net.startTLS(conn[, net.TLSOptions]) -> net.Conn`:
// upgrade an open plaintext connection to TLS in place (for STARTTLS) within
// the default handshake deadline. The deprecated trailing timeoutMs still
// works until 0.29.0.
func startTLSFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("net.startTLS expects 1 or 2 arguments (net.Conn[, net.TLSOptions]), got %d", len(args))
	}
	id, err := extractID("net.startTLS", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	optsIdx, timeoutIdx, err := tlsArgLayout("net.startTLS", args)
	if err != nil {
		return interpreter.Null(), err
	}
	o := dialOptions{timeout: defaultTimeout, tls: true}
	if o.skipVerify, o.caCert, err = tlsOptions("net.startTLS", args, optsIdx); err != nil {
		return interpreter.Null(), err
	}
	if timeoutIdx < len(args) {
		if o.timeout, err = legacyTimeout("net.startTLS", args[timeoutIdx]); err != nil {
			return interpreter.Null(), err
		}
	}
	return upgradeTLS(ctx, "net.startTLS", id, o)
}

// startTLSWithFn implements `net.startTLSWith(conn, net.ConnectOptions) ->
// net.Conn`: startTLS with the options' timeoutMs and tlsOptions. The tls
// flag is ignored: the call is the request for TLS.
func startTLSWithFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.startTLSWith expects 2 arguments (net.Conn, net.ConnectOptions), got %d", len(args))
	}
	id, err := extractID("net.startTLSWith", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	o, err := connectOptions("net.startTLSWith", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return upgradeTLS(ctx, "net.startTLSWith", id, o)
}

// upgradeTLS runs the client handshake on connection id within o.timeout (0 =
// no deadline) and swaps the registry entry to the TLS connection. The
// server certificate is verified against the hostname the connection was
// opened with; the same handle is returned.
func upgradeTLS(ctx interpreter.BuiltinCtx, fnName string, id int64, o dialOptions) (Value, error) {
	s, err := resolveConn(fnName, id)
	if err != nil {
		return interpreter.Null(), err
	}
	if s.host == "" {
		return interpreter.Null(), fmt.Errorf("%s: this connection has no recorded hostname to verify against; startTLS upgrades a connection opened with net.connect", fnName)
	}
	cfg, err := clientTLSConfig(fnName, s.host, o.skipVerify, o.caCert)
	if err != nil {
		return interpreter.Null(), err
	}
	// Hold readMu across the whole handshake-and-swap so no blocking read is in
	// flight while the transport is upgraded: a reader parked in net.readBytes
	// keeps its snapshot of the pre-upgrade *bufio.Reader and would consume raw
	// TLS record bytes as if they were plaintext after the swap. Snapshot the
	// mutable fields under s.mu rather than reading them unlocked - a concurrent
	// net.setReadDeadline writes s.deadline under mu, so an unlocked read races it.
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.mu.Lock()
	r := s.r
	c := s.c
	readDeadline, writeDeadline := s.deadline, s.writeDeadline
	s.mu.Unlock()

	// Hand the handshake the buffered reader + raw conn so no read-ahead
	// plaintext is lost, then swap the registry entry to the TLS conn. The
	// timeout arms a combined handshake deadline on the underlying conn (a
	// stalled peer otherwise hangs the upgrade, which reads and writes), then
	// restores the deadlines the caller armed with setRead/WriteDeadline.
	tlsConn := tls.Client(&bufferedConn{r: r, Conn: c}, cfg)
	if o.timeout > 0 {
		_ = c.SetDeadline(time.Now().Add(o.timeout))
	}
	hErr := tlsConn.Handshake()
	if o.timeout > 0 {
		_ = c.SetReadDeadline(readDeadline)
		_ = c.SetWriteDeadline(writeDeadline)
	}
	if hErr != nil {
		return interpreter.Null(), failure(ctx, fnName, "handshake", hErr)
	}
	s.mu.Lock()
	s.c = tlsConn
	s.r = bufio.NewReader(tlsConn)
	s.sticky = false
	s.mu.Unlock()
	return makeConn(id), nil
}

func listenFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.listen expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.listen", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	l, lErr := stdnet.Listen("tcp", addr)
	if lErr != nil {
		return interpreter.Null(), listenFailure(ctx, fmt.Sprintf("net.listen: %s: %v", addr, lErr), lErr)
	}
	listenersMu.Lock()
	if len(listeners) >= maxNetHandles {
		listenersMu.Unlock()
		_ = l.Close()
		return interpreter.Null(), limits.Capf("net.listen: too many open listeners (limit %d); each needs a matching net.close", maxNetHandles)
	}
	nextListenerID++
	id := nextListenerID
	listeners[id] = &listenerState{l: l}
	listenersMu.Unlock()
	return makeListener(id), nil
}

func acceptFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
		return interpreter.Null(), failure(ctx, "net.accept", "accept", aErr)
	}
	connsMu.Lock()
	if len(conns) >= maxNetHandles {
		connsMu.Unlock()
		_ = c.Close()
		return interpreter.Null(), limits.Capf("net.accept: too many open connections (limit %d); each accepted conn needs a matching net.close", maxNetHandles)
	}
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
func readBytesFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
		return interpreter.Null(), limits.Capf("net.readBytes: %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	s, err := resolveConn("net.readBytes", id)
	if err != nil {
		return interpreter.Null(), err
	}
	buf := make([]byte, n)
	// Snapshot the reader pointer under the lock (a concurrent startTLS may
	// swap it); do the blocking read outside the lock so net.setReadDeadline can
	// still interrupt it.
	s.mu.Lock()
	r := s.r
	c := s.c
	idle, userDeadline := s.idle, s.deadline
	s.mu.Unlock()
	s.readMu.Lock()
	w := watchCancel(ctx, c)
	// An idle timeout arms the deadline for this read (one underlying read at
	// most) and the setReadDeadline one is restored after it.
	if arm := idleArmer(c, w, idle, userDeadline); arm != nil && r.Buffered() == 0 {
		arm()
		defer func() { _ = c.SetReadDeadline(userDeadline) }()
	}
	read, rerr := r.Read(buf)
	if w.stop() {
		_ = c.SetReadDeadline(userDeadline)
		s.readMu.Unlock()
		return interpreter.Null(), interpreter.ErrTaskCancelled()
	}
	s.readMu.Unlock()
	if rerr != nil {
		if errors.Is(rerr, io.EOF) {
			s.mu.Lock()
			s.sticky = true
			s.mu.Unlock()
			return interpreter.BytesVal(buf[:read]), nil
		}
		// A deadline set by net.setReadDeadline surfaces as a catchable
		// Error{kind: "timeout"}, so a poll-with-timeout loop can tell "no data
		// yet, send a keepalive" apart from a real connection failure. The
		// deadline is not cleared; the caller re-arms it (or clears it with ms
		// 0) before the next read.
		return interpreter.Null(), failure(ctx, "net.readBytes", "read", rerr)
	}
	return interpreter.BytesVal(buf[:read]), nil
}

// deadlineKind selects which direction a deadline governs. There is no combined
// form: a read deadline and a write deadline are separate things (stance 1), so a
// caller that wants both arms both. This is deliberate - a single combined
// deadline left expired by the poll idiom would silently break the next write.
type deadlineKind int

const (
	dlRead  deadlineKind = iota // read only (net.setReadDeadline)
	dlWrite                     // write only (net.setWriteDeadline)
)

// deadlineSetter is the deadline surface shared by a stream net.Conn and a
// datagram net.PacketConn, so one helper arms either handle.
type deadlineSetter interface {
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

func armDeadline(d deadlineSetter, when time.Time, kind deadlineKind) error {
	if kind == dlRead {
		return d.SetReadDeadline(when)
	}
	return d.SetWriteDeadline(when)
}

// applyDeadline is the shared body of net.setReadDeadline / setWriteDeadline. A
// positive `ms` sets an absolute deadline that many milliseconds from now; once
// it passes, a pending or subsequent operation in the governed direction fails
// with a distinguishable timeout error until the deadline is reset. `ms == 0`
// clears it. `kind` selects the direction: the read-only form is what lets a
// single-threaded client poll for a packet with a timeout WITHOUT arming a write
// deadline - so a keepalive / reply after a read timeout still succeeds.
func applyDeadline(fnName string, args []Value, kind deadlineKind) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (net.Conn or net.UDPSocket, ms), got %d", fnName, len(args))
	}
	ms, err := takeIntArg(fnName, args, 1, "ms")
	if err != nil {
		return interpreter.Null(), err
	}
	var when time.Time // the zero Time clears any existing deadline
	if ms > 0 {
		when = time.Now().Add(clampMillis(ms))
	}
	// Dispatch on the handle kind: a stream net.Conn or a datagram
	// net.UDPSocket (its PacketConn honours the same deadline methods).
	v := args[0]
	if v.Kind == interpreter.KindStruct && v.StructNS() == LibraryName && v.StructName() == "UDPSocket" {
		id, err := extractID(fnName, "UDPSocket", v)
		if err != nil {
			return interpreter.Null(), err
		}
		s, err := resolveUDP(fnName, id)
		if err != nil {
			return interpreter.Null(), err
		}
		if kind == dlRead {
			s.mu.Lock()
			s.deadline = when
			s.mu.Unlock()
		}
		if derr := armDeadline(s.c, when, kind); derr != nil {
			return interpreter.Null(), fmt.Errorf("%s: %v", fnName, derr)
		}
		return interpreter.Null(), nil
	}
	id, err := extractID(fnName, "Conn", v)
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveConn(fnName, id)
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	c := s.c
	// s.deadline is the read-deadline baseline the eof probe / readAll / readN
	// save and restore (they only ever touch the read side), so a write-only call
	// must not update it - else a later read would inherit the write deadline.
	if kind == dlRead {
		s.deadline = when
	} else {
		s.writeDeadline = when
	}
	s.mu.Unlock()
	if derr := armDeadline(c, when, kind); derr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fnName, derr)
	}
	return interpreter.Null(), nil
}

// setReadDeadlineFn arms or clears a read-only deadline: the poll-with-timeout
// primitive. On a timeout the write side is untouched, so a reply / keepalive
// still goes out.
func setReadDeadlineFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return applyDeadline("net.setReadDeadline", args, dlRead)
}

// setWriteDeadlineFn arms or clears a write-only deadline, bounding a slow / stalled
// send without affecting reads.
func setWriteDeadlineFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return applyDeadline("net.setWriteDeadline", args, dlWrite)
}

// setIdleTimeoutFn implements `net.setIdleTimeout(conn, ms)`: while set,
// every read on the connection (readBytes, readAll, readN) re-arms the read
// deadline to `ms` from now before each underlying read, so a stall of `ms`
// throws kind "timeout" and a slow but steady transfer is never cut. A
// setReadDeadline deadline still applies: whichever passes first fires. 0 or
// the largest int clears it; a negative value throws.
func setIdleTimeoutFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.setIdleTimeout expects 2 arguments (net.Conn, ms), got %d", len(args))
	}
	id, err := extractID("net.setIdleTimeout", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("net.setIdleTimeout: ms must be int, got %s", args[1].Kind)
	}
	ms := args[1].Int()
	if ms < 0 {
		return interpreter.Null(), fmt.Errorf("net.setIdleTimeout: ms must be >= 0 (0 or %d = none), got %d", int64(noDeadlineMs), ms)
	}
	var idle time.Duration
	if ms != noDeadlineMs {
		idle = clampMillis(ms)
	}
	s, err := resolveConn("net.setIdleTimeout", id)
	if err != nil {
		return interpreter.Null(), err
	}
	s.mu.Lock()
	s.idle = idle
	s.mu.Unlock()
	return interpreter.Null(), nil
}

func writeBytesFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	s.mu.Lock()
	c := s.c
	s.mu.Unlock()
	if _, werr := c.Write(data); werr != nil {
		return interpreter.Null(), failure(ctx, "net.writeBytes", "write", werr)
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
	s.mu.Lock()
	if s.sticky {
		s.mu.Unlock()
		return interpreter.BoolVal(true), nil
	}
	r, c := s.r, s.c
	s.mu.Unlock()
	// If another task is blocked in readBytes right now, answer "not EOF"
	// without touching the reader at all: bufio.Reader is not
	// goroutine-safe (even Buffered() races a blocked Read), and arming the
	// probe deadline would spuriously time that read out. The blocked
	// reader discovers EOF itself (sticky).
	if !s.readMu.TryLock() {
		return interpreter.BoolVal(false), nil
	}
	// Buffered data means definitely not EOF, decided without any I/O.
	if r.Buffered() > 0 {
		s.readMu.Unlock()
		return interpreter.BoolVal(false), nil
	}
	// Otherwise probe *without blocking indefinitely on the peer*: a bare
	// Peek(1) blocks until the peer sends or closes, so `while (not
	// net.eof($c))` would deadlock any protocol where it is the local side's
	// turn to write. A short read deadline makes Peek return io.EOF when the
	// peer has closed (the pending FIN surfaces at once, well inside the
	// window) or a timeout ("no data yet", not EOF) on an open, idle
	// connection. Afterwards the deadline armed via net.setReadDeadline (if any)
	// is restored.
	s.mu.Lock()
	userDeadline := s.deadline
	s.mu.Unlock()
	_ = c.SetReadDeadline(time.Now().Add(eofProbeTimeout))
	_, peekErr := r.Peek(1)
	_ = c.SetReadDeadline(userDeadline)
	s.readMu.Unlock()
	if errors.Is(peekErr, io.EOF) {
		s.mu.Lock()
		s.sticky = true
		s.mu.Unlock()
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
	if v.Kind != interpreter.KindStruct || v.StructNS() != LibraryName {
		return interpreter.Null(), fmt.Errorf("net.address: argument must be a net.Conn, net.Listener, or net.UDPSocket; got %s", v.Kind)
	}
	switch v.StructName() {
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
		return interpreter.Null(), fmt.Errorf("net.address: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.%s", v.StructName())
	}
}

func closeConn(id int64) error {
	connsMu.Lock()
	s, ok := conns[id]
	if !ok {
		connsMu.Unlock()
		return nil // a second close does nothing (M29.46)
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

// abortFn implements `net.abort(conn)`: close a TCP connection with a reset
// instead of the orderly FIN of net.close. SO_LINGER set to 0 makes the
// kernel discard unsent data and answer with RST, so the peer's next read or
// write fails with a NetError of reason "reset" - how a test or a fake server
// produces the failure a crashed or overloaded peer causes. A TLS connection
// is aborted at its TCP layer, without a close_notify. The handle is retired
// as by net.close.
func abortFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.abort expects 1 argument (net.Conn), got %d", len(args))
	}
	id, err := extractID("net.abort", "Conn", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	connsMu.Lock()
	s, ok := conns[id]
	if !ok {
		connsMu.Unlock()
		return interpreter.Null(), nil // already closed or aborted: nothing to do (M29.46)
	}
	delete(conns, id)
	connsMu.Unlock()
	s.mu.Lock()
	c := s.c
	s.mu.Unlock()
	raw := tcpOf(c)
	if raw == nil {
		_ = c.Close()
		return interpreter.Null(), fmt.Errorf("net.abort: not a TCP connection")
	}
	if err := raw.SetLinger(0); err != nil {
		_ = raw.Close()
		return interpreter.Null(), fmt.Errorf("net.abort: %v", err)
	}
	if err := raw.Close(); err != nil {
		return interpreter.Null(), fmt.Errorf("net.abort: %v", err)
	}
	return interpreter.Null(), nil
}

// tcpOf finds the TCP connection under c: c itself, or the one a TLS
// connection (connectTLS, or startTLS over a bufferedConn) runs on.
func tcpOf(c stdnet.Conn) *stdnet.TCPConn {
	for c != nil {
		switch v := c.(type) {
		case *stdnet.TCPConn:
			return v
		case *tls.Conn:
			c = v.NetConn()
		case *bufferedConn:
			c = v.Conn
		default:
			return nil
		}
	}
	return nil
}

func closeListener(id int64) error {
	listenersMu.Lock()
	s, ok := listeners[id]
	if !ok {
		listenersMu.Unlock()
		return nil // a second close does nothing (M29.46)
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

func listenUDPFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.listenUDP expects 1 argument (address), got %d", len(args))
	}
	addr, err := takeStringArg("net.listenUDP", args, 0, "address")
	if err != nil {
		return interpreter.Null(), err
	}
	c, lErr := stdnet.ListenPacket("udp", addr)
	if lErr != nil {
		return interpreter.Null(), listenFailure(ctx, fmt.Sprintf("net.listenUDP: %s: %v", addr, lErr), lErr)
	}
	udpsMu.Lock()
	if len(udps) >= maxNetHandles {
		udpsMu.Unlock()
		_ = c.Close()
		return interpreter.Null(), limits.Capf("net.listenUDP: too many open UDP sockets (limit %d); each needs a matching net.close", maxNetHandles)
	}
	nextUDPID++
	id := nextUDPID
	udps[id] = &udpState{c: c}
	udpsMu.Unlock()
	return makeUDPSocket(id), nil
}

func sendToFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
		msg := fmt.Sprintf("net.sendTo: %s: %v", peer, resolveErr)
		var dnsErr *stdnet.DNSError
		if !errors.As(resolveErr, &dnsErr) {
			return interpreter.Null(), errors.New(msg)
		}
		return interpreter.Null(), newNetError(ctx, "lookup", "dns", dnsconf.Setup().Explain(msg))
	}
	if _, werr := s.c.WriteTo(data, peerAddr); werr != nil {
		return interpreter.Null(), failure(ctx, "net.sendTo", "write", werr)
	}
	return interpreter.Null(), nil
}

func recvFromFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
		return interpreter.Null(), limits.Capf("net.recvFrom: %d exceeds the %d-byte per-call limit", n, maxReadBytes)
	}
	s, err := resolveUDP("net.recvFrom", id)
	if err != nil {
		return interpreter.Null(), err
	}
	// Read into a buffer of the largest possible datagram, whatever n is: a
	// datagram longer than the buffer is cut short by the kernel and the rest
	// discarded without a word, so a buffer of only n bytes could not tell a
	// truncated datagram from a short one. The result carries the first n bytes
	// and `truncated` reports whether any were dropped. The buffer is pooled and
	// the result copied out of it, so a call costs the datagram, not 64 KiB.
	bp := datagramPool.Get().(*[]byte)
	defer datagramPool.Put(bp)
	buf := *bp
	// task.cancel wakes the read, as it does a TCP read; the deadline it moved
	// is then restored.
	w := watchCancel(ctx, s.c)
	read, peer, rerr := s.c.ReadFrom(buf)
	if w.stop() {
		s.mu.Lock()
		_ = s.c.SetReadDeadline(s.deadline)
		s.mu.Unlock()
		return interpreter.Null(), interpreter.ErrTaskCancelled()
	}
	if rerr != nil {
		return interpreter.Null(), failure(ctx, "net.recvFrom", "read", rerr)
	}
	truncated := int64(read) > n
	if truncated {
		read = int(n)
	}
	return makeDatagram(bytes.Clone(buf[:read]), peer.String(), truncated), nil
}

// datagramPool holds maxDatagram-byte receive buffers for recvFrom.
var datagramPool = sync.Pool{New: func() any {
	b := make([]byte, maxDatagram)
	return &b
}}

// setBroadcastFn toggles the SO_BROADCAST socket option on a UDP socket, so a
// sendTo may target a subnet or the limited broadcast address (255.255.255.255).
// The kernel refuses a broadcast send on a socket without it; receiving a
// broadcast needs no option. The actual setsockopt lives in a platform helper
// (setSocketBroadcast) so the best-effort Windows build still compiles.
func setBroadcastFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast expects 2 arguments (net.UDPSocket, enabled), got %d", len(args))
	}
	id, err := extractID("net.setBroadcast", "UDPSocket", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBool {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast: second argument (enabled) must be bool, got %s", args[1].Kind)
	}
	s, err := resolveUDP("net.setBroadcast", id)
	if err != nil {
		return interpreter.Null(), err
	}
	sc, ok := s.c.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast: this UDP socket does not support broadcast control")
	}
	rc, cerr := sc.SyscallConn()
	if cerr != nil {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast: %v", cerr)
	}
	val := 0
	if args[1].Bool() {
		val = 1
	}
	var opErr error
	if ctlErr := rc.Control(func(fd uintptr) { opErr = setSocketBroadcast(fd, val) }); ctlErr != nil {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast: %v", ctlErr)
	}
	if opErr != nil {
		return interpreter.Null(), fmt.Errorf("net.setBroadcast: %v", opErr)
	}
	return interpreter.Null(), nil
}

// bindToDeviceFn binds a UDP socket to a specific network interface via
// SO_BINDTODEVICE, so a sendTo leaves on that link and only datagrams arriving
// on it are received - the interface selection a wildcard bind (0.0.0.0:0)
// otherwise leaves to the kernel routing table. This is what lets a broadcast
// to 255.255.255.255 reach a device on a chosen NIC of a multi-homed host (a
// `@jennifer/mactelnet` bootstrap of a router with no IP yet). An empty iface clears the
// binding. Linux-only; unprivileged on Linux 5.7+ for a socket not yet bound
// to a device, otherwise CAP_NET_RAW (EPERM without). The setsockopt lives in a platform helper
// (setSocketBindToDevice) so the non-Linux best-effort builds still compile.
func bindToDeviceFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("net.bindToDevice expects 2 arguments (net.UDPSocket, iface), got %d", len(args))
	}
	id, err := extractID("net.bindToDevice", "UDPSocket", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	iface, err := takeStringArg("net.bindToDevice", args, 1, "iface")
	if err != nil {
		return interpreter.Null(), err
	}
	s, err := resolveUDP("net.bindToDevice", id)
	if err != nil {
		return interpreter.Null(), err
	}
	sc, ok := s.c.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return interpreter.Null(), fmt.Errorf("net.bindToDevice: this UDP socket does not support interface binding")
	}
	rc, cerr := sc.SyscallConn()
	if cerr != nil {
		return interpreter.Null(), fmt.Errorf("net.bindToDevice: %v", cerr)
	}
	var opErr error
	if ctlErr := rc.Control(func(fd uintptr) { opErr = setSocketBindToDevice(fd, iface) }); ctlErr != nil {
		return interpreter.Null(), fmt.Errorf("net.bindToDevice: %v", ctlErr)
	}
	if opErr != nil {
		return interpreter.Null(), fmt.Errorf("net.bindToDevice: %v", opErr)
	}
	return interpreter.Null(), nil
}

func closeUDP(id int64) error {
	udpsMu.Lock()
	s, ok := udps[id]
	if !ok {
		udpsMu.Unlock()
		return nil // a second close does nothing (M29.46)
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

// resolver answers net.lookup / reverseLookup; a variable so tests
// can hand in one that never answers. It is the process-wide resolver, which
// setupDNS routes through the configured files.
var resolver = stdnet.DefaultResolver

// setupDNS picks the resolver configuration once per process and, when it is
// not the system's, routes net.DefaultResolver through it (internal/dnsconf).
func setupDNS() { dnsconf.Setup() }

// lookupDeadline runs a lookup under timeout (0 = no deadline). A lookup cut
// off by the deadline is kind "timeout"; any other failure a NetError with
// op "lookup" and reason "dns".
func lookupDeadline(ctx interpreter.BuiltinCtx, fnName, name string, timeout time.Duration,
	run func(context.Context, string) ([]string, error)) (Value, error) {
	return lookupWithin(ctx, fnName, name, timeout, false, run)
}

// lookupWithin is lookupDeadline; explicitServers (lookupWith's dnsServers)
// leaves out the configuration hint, which is about the configured resolver.
func lookupWithin(ctx interpreter.BuiltinCtx, fnName, name string, timeout time.Duration, explicitServers bool,
	run func(context.Context, string) ([]string, error)) (Value, error) {
	c := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, timeout)
		defer cancel()
	}
	out, err := run(c, name)
	if err != nil {
		var dnsErr *stdnet.DNSError
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &dnsErr) && dnsErr.IsTimeout) {
			return interpreter.Null(), interpreter.RaiseError("timeout", fmt.Sprintf("%s: %s: lookup timed out", fnName, name), ctx.File, ctx.Line, ctx.Col)
		}
		msg := fmt.Sprintf("%s: %s: %v", fnName, name, err)
		if !explicitServers {
			msg = dnsconf.Setup().Explain(msg)
		}
		return interpreter.Null(), newNetError(ctx, "lookup", "dns", msg)
	}
	return stringSlice(out), nil
}

// lookupArgs reads `(name)` or, for the With form, `(name, net.ConnectOptions)`
// and returns the name and the options (the zero form: the default deadline).
func lookupArgs(fnName, role string, args []Value, with bool) (string, dialOptions, error) {
	want, shape := 1, "1 argument ("+role+")"
	if with {
		want, shape = 2, "2 arguments ("+role+", net.ConnectOptions)"
	}
	if len(args) != want {
		return "", dialOptions{}, fmt.Errorf("%s expects %s, got %d", fnName, shape, len(args))
	}
	name, err := takeStringArg(fnName, args, 0, role)
	if err != nil {
		return "", dialOptions{}, err
	}
	if !with {
		return name, dialOptions{timeout: defaultTimeout}, nil
	}
	o, err := connectOptions(fnName, args[1])
	if err != nil {
		return "", dialOptions{}, err
	}
	return name, o, nil
}

func lookupFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	host, o, err := lookupArgs("net.lookup", "host", args, false)
	if err != nil {
		return interpreter.Null(), err
	}
	return lookupDeadline(ctx, "net.lookup", host, o.timeout, resolver.LookupHost)
}

// lookupWithFn implements `net.lookupWith(host, net.ConnectOptions)`: lookup
// within the options' timeoutMs. With dnsServers it asks only those (no hosts
// file, no search list, A then AAAA); tls and tlsOptions are ignored.
func lookupWithFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fnName = "net.lookupWith"
	host, o, err := lookupArgs(fnName, "host", args, true)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(o.dnsServers) == 0 {
		return lookupDeadline(ctx, fnName, host, o.timeout, resolver.LookupHost)
	}
	return lookupWithin(ctx, fnName, host, o.timeout, true, func(c context.Context, name string) ([]string, error) {
		return dnsconf.Query(c, o.dnsServers, name)
	})
}

func reverseLookupFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	ip, o, err := lookupArgs("net.reverseLookup", "ip", args, false)
	if err != nil {
		return interpreter.Null(), err
	}
	return lookupDeadline(ctx, "net.reverseLookup", ip, o.timeout, resolver.LookupAddr)
}

// reverseLookupWithFn implements `net.reverseLookupWith(ip,
// net.ConnectOptions)`: reverseLookup within the options' timeoutMs, through
// the configured resolver (dnsServers is ignored, as are tls and tlsOptions).
func reverseLookupWithFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	ip, o, err := lookupArgs("net.reverseLookupWith", "ip", args, true)
	if err != nil {
		return interpreter.Null(), err
	}
	return lookupDeadline(ctx, "net.reverseLookupWith", ip, o.timeout, resolver.LookupAddr)
}
