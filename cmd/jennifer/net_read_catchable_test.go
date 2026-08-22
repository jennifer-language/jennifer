// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNetReadErrorsAreCatchableAcrossModuleBoundaries pins the catchability
// contract a field report questioned: a net read error raised deep inside a
// module (a `net.readBytes` timeout, or a mid-frame connection drop) must unwind
// through nested user methods, across MORE THAN ONE module boundary, and through
// `defer` / `errdefer` cleanup, into an enclosing `try` in the entry program.
//
// The shape mirrors the reported stack: entry program -> a "routeros"-style
// module method (with its own `errdefer`) -> a "mikrotik"-style module method
// whose `readN` wraps the blocking read in a `defer` that clears the deadline.
// Two drop modes are exercised against a loopback server:
//
//   - silent stall: the peer accepts, reads the command, then never replies -
//     the read blocks until the deadline fires ("net.readBytes: read timed out",
//     the exact message the report saw). This is what an active connection
//     silently dropped by the far end (a firewall/NAT state drop, a router
//     reboot mid-session) looks like to the reader.
//   - clean close: the peer closes mid-exchange (FIN -> EOF -> a `throw`).
//
// Both MUST be caught (program exits 0, the catch block runs). The one path that
// is uncatchable by design - a read error inside an unobserved `spawn` surfacing
// as the exit-time loud-fail - is covered separately; see the package's spawn
// loud-fail tests.
func TestNetReadErrorsAreCatchableAcrossModuleBoundaries(t *testing.T) {
	dir := t.TempDir()

	// Innermost module: the mikrotik-style readN with a deadline-clearing defer.
	inner := `use net;
use binary;
def const READ_TIMEOUT_MS as int init 300;
export func readN(sock as net.Conn, n as int) {
    def parts as list of bytes init [];
    def got as int init 0;
    defer net.setDeadline($sock, 0);
    while ($got < $n) {
        net.setDeadline($sock, READ_TIMEOUT_MS);
        def chunk as bytes init net.readBytes($sock, $n - $got);
        if (len($chunk) == 0) {
            throw Error{kind: "mikrotik", message: "connection closed mid-sentence", file: "", line: 0, col: 0};
        }
        $parts[] = $chunk;
        $got = $got + len($chunk);
    }
    return binary.join($parts);
}
`
	// Middle module: the routeros-style wrapper, with its own errdefer, that
	// calls across a second boundary into inner.readN.
	middle := `use net;
import "./inner.j" as inner;
export func identity(sock as net.Conn) {
    errdefer net.setDeadline($sock, 0);
    def b as bytes init inner.readN($sock, 8);
    return len($b);
}
`
	// Entry program: a loopback server that drops the connection one of two ways
	// (chosen by a JENNIFER_TEST_DROP env value), and a caller that wraps the
	// synchronous two-boundary call in a try. It prints a CAUGHT marker per
	// caught error and OK at the end - reaching OK proves nothing escaped Run.
	app := `use net;
use io;
use os;
use convert;
use task;
import "./middle.j" as mt;

def mode as string init os.getEnv("JENNIFER_TEST_DROP");
def srv as net.Listener init net.listen("127.0.0.1:0");
def addr as string init net.address($srv);
def server as task of null init spawn {
    def c as net.Conn init net.accept($srv);
    net.readBytes($c, 1);        # receive the one-byte command
    if ($mode == "close") {
        net.close($c);           # clean close mid-exchange (FIN)
    } else {
        net.readBytes($c, 1);    # silent stall: block, never reply
    }
    return;
};
def c as net.Conn init net.connect($addr);
net.writeBytes($c, convert.bytesFromString("x", "utf-8"));
def settled as bool init false;
try {
    mt.identity($c);
    $settled = true;
} catch (e) {
    io.printf("CAUGHT {$e.kind}\n");
}
io.printf("OK settled={$settled}\n");
task.discard($server);
net.close($c);
`
	write := func(name, src string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("inner.j", inner)
	write("middle.j", middle)
	appPath := write("app.j", app)

	for _, mode := range []string{"stall", "close"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("JENNIFER_TEST_DROP", mode)
			var code int
			out := captureStdout(t, func() { code = runFile(appPath, nil, "") })
			if code != 0 {
				t.Fatalf("%s: exit code %d, want 0 (a read error escaped try/catch)\noutput:\n%s", mode, code, out)
			}
			if !strings.Contains(out, "CAUGHT") {
				t.Errorf("%s: expected a CAUGHT marker (the catch block never ran); got:\n%s", mode, out)
			}
			if !strings.Contains(out, "OK settled=false") {
				t.Errorf("%s: program did not reach OK with settled=false; got:\n%s", mode, out)
			}
		})
	}
}
