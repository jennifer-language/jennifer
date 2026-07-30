// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
        def body as string init convert.stringFromBytes(httpd.body($req), "utf-8");
        def reply as string init mcp.handle($srv, $body);
        httpd.respond($req, 200, $reply);
    }
}

def sch as json.Value init mcp.property(mcp.schema(), "text", "string", "text to echo", true);
def srv as mcp.Server init mcp.addTool(mcp.server("demo", "1.0.0"), "echo", "echo text", $sch, "echoTool");

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
