// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// A .j program builds an MCP server, serves mcp.handle over httpd on an
// ephemeral port in a spawned task, and acts as its own MCP client: it runs the
// `initialize` handshake and calls a registered `echo` tool, asserting the
// round-trip. This exercises the whole chain - the mcp HTTP client (over the
// jsonrpc client over http), the transport-agnostic server `handle`, and
// cross-boundary tool-handler dispatch via meta.callMain (an echo handler
// defined in this entry program, reached from a request parsed inside the mcp
// module). Runs the real HTTP dialogue in CI with no external server.
func TestMcpRoundTrip(t *testing.T) {
	mcpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mcp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use json;
use convert;
import %q as mcp;

func echoTool(args as json.Value) {
    return json.asString($args, "/text");
}

func serve(hsrv as httpd.Server, srv as mcp.Server) {
    while (true) {
        def req as httpd.Request;
        try {
            $req = httpd.accept($hsrv);
        } catch (e) {
            return;
        }
        def body as string init convert.stringFromBytes("utf-8", httpd.body($req));
        def reply as string init mcp.handle($srv, $body);
        httpd.respond($req, 200, $reply);
    }
}

def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "text to echo", true);
def srv as mcp.Server init mcp.addTool(mcp.server("demo", "1.0.0"), "echo", "echo text", $sch, echoTool);

def hsrv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($hsrv);
def server as task of null init spawn { serve($hsrv, $srv); };

def c as mcp.Client init mcp.connect("http://" + $addr + "/");

def info as json.Value init mcp.initialize($c);
testing.assertEqual(json.asString($info, "/protocolVersion"), "2025-06-18");
testing.assertEqual(json.asString($info, "/serverInfo/name"), "demo");

def tools as json.Value init mcp.listTools($c);
testing.assertEqual(json.asString($tools, "/0/name"), "echo");

def args as json.Value init json.set(json.map(), "/text", "hello");
def result as json.Value init mcp.callTool($c, "echo", $args);
testing.assertEqual(json.asBool($result, "/isError"), false);
testing.assertEqual(json.asString($result, "/content/0/text"), "hello");

httpd.shutdown($hsrv);
task.wait($server);`, mcpMod)

	progPath := filepath.Join(dir, "mcpapp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mcp round-trip program failed with code %d", code)
	}
}

// serveStdio answers a stdin line that is not UTF-8 with a parse error and
// keeps serving: the next request still gets its reply. It used to let
// io.readLine's error end the loop, and the program with it. Runs a built
// binary, since the program reads the process's own stdin.
func TestMcpServeStdioSurvivesBadLine(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	mcpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mcp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "jennifer")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	prog := fmt.Sprintf(`import %q as mcp;
mcp.serveStdio(mcp.server("demo", "1.0.0"));
`, mcpMod)
	progPath := filepath.Join(dir, "stdio.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", progPath)
	cmd.Stdin = strings.NewReader("\xff\xfe\n{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"ping\"}\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("serveStdio program failed: %v; output %q", err, out)
	}
	if !strings.Contains(string(out), `"code":-32700`) || !strings.Contains(string(out), `"id":7`) {
		t.Fatalf("want a parse error, then the ping reply; got %q", out)
	}

	// A line over io.readLine's cap is one parse error, and its tail (padded
	// past the reader's chunk so the cut falls in the padding, then a ping
	// with id 99) is skipped rather than run. The line is streamed, not held.
	cmd = exec.Command(bin, "run", progPath)
	pr, pw := io.Pipe()
	cmd.Stdin = pr
	go func() {
		filler := bytes.Repeat([]byte("a"), 1<<20)
		for n := 0; n < limits.MaxAllocBytes; n += len(filler) {
			if _, err := pw.Write(filler); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		_, _ = pw.Write(bytes.Repeat([]byte(" "), 1<<16))
		_, _ = io.WriteString(pw, `{"jsonrpc":"2.0","id":99,"method":"ping"}`+"\n")
		_, _ = io.WriteString(pw, `{"jsonrpc":"2.0","id":8,"method":"ping"}`+"\n")
		pw.Close()
	}()
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("serveStdio program failed on an over-cap line: %v; output %q", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"code":-32700`) || !strings.Contains(lines[1], `"id":8`) {
		t.Fatalf("want one parse error, then the id 8 ping reply; got %q", out)
	}
}
