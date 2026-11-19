# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# mcp_test.j - white-box tests for mcp.j. Run with:
#
#     jennifer test modules/mcp_test.j
#
# The overlay splices mcp.j in first, so these tests reach its private helpers
# (findTool, valueText, encodeError) and its exported surface by bare
# identifier. The server `handle` dispatches tool / resource / prompt handler
# func values (the functions below, registered by bare name), which the overlay is
# the entry program for; the live HTTP client round-trip is verified against an
# in-process httpd server in the Go suite (TestMcpRoundTrip).
use testing;
use json;
use net;
use task;
use binary;
use regex;
use fs;
use path;
use time;
use convert;
use strings;

# --- handler func values `handle` dispatches to (registered by bare name) -----

func echoTool(args as json.Value) {
    return json.asString($args, "/text");
}

func addTool2(args as json.Value) {
    def sum as int init json.asInt($args, "/a") + json.asInt($args, "/b");
    def out as json.Value init json.map();
    $out = json.set($out, "/sum", $sum);
    return $out;
}

func boomTool(args as json.Value) {
    throw Error{kind: "demo", message: "kaboom"};
}

func readmeResource(uri as json.Value) {
    return "hello from " + json.asString($uri, "");
}

func greetingPrompt(args as json.Value) {
    # a PromptMessage's content is a content block ({type:"text", text}), not a
    # bare string (MCP requires this; a host rejects a plain string)
    def content as json.Value init json.map();
    $content = json.set($content, "/type", "text");
    $content = json.set($content, "/text", "hi " + json.asString($args, "/who"));
    def msg as json.Value init json.map();
    $msg = json.set($msg, "/role", "user");
    $msg = json.set($msg, "/content", $content);
    def arr as json.Value init json.list();
    $arr = json.append($arr, "", $msg);
    return $arr;
}

# --- server builder -----------------------------------------------------------

func buildServer() {
    def sch as json.Value init property(schema(), "text", "string", "text to echo", true);
    def s as Server init server("demo", "1.0.0");
    $s = addTool($s, "echo", "echo text", $sch, echoTool);
    $s = addTool($s, "add", "add two ints", schema(), addTool2);
    $s = addTool($s, "boom", "always throws", schema(), boomTool);
    $s = addResource($s, "file:///readme", "readme", "the readme", "text/plain", readmeResource);
    $s = addPrompt(
        $s,
        "greet",
        "a greeting",
        [promptArg("who", "who to greet", true)],
        greetingPrompt);
    return $s;
}

# --- schema builder -----------------------------------------------------------

func testSchemaSkeleton() {
    testing.assertEqual(json.encode(schema()), '{"type":"object","properties":{},"required":[]}');
}

func testSchemaProperty() {
    def sch as json.Value init property(schema(), "text", "string", "text to echo", true);
    testing.assertEqual(
        json.encode($sch),
        '{"type":"object","properties":{"text":{"type":"string","description":"text to echo"}},"required":["text"]}');
}

func testSchemaPropertyOptional() {
    def sch as json.Value init property(schema(), "note", "string", "optional note", false);
    # an optional property is not appended to required
    testing.assertEqual(json.length($sch, "/required"), 0);
    testing.assertEqual(json.asString($sch, "/properties/note/type"), "string");
}

# --- initialize handshake -----------------------------------------------------

func testInitialize() {
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}'),
        '{"jsonrpc":"2.0","result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{},"resources":{},"prompts":{}},"serverInfo":{"name":"demo","version":"1.0.0"}},"id":1}');
}

func testPing() {
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"ping","id":2}'),
        '{"jsonrpc":"2.0","result":{},"id":2}');
}

# --- tools --------------------------------------------------------------------

func testToolsList() {
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"tools/list","id":3}'),
        '{"jsonrpc":"2.0","result":{"tools":[{"name":"echo","description":"echo text","inputSchema":{"type":"object","properties":{"text":{"type":"string","description":"text to echo"}},"required":["text"]}},{"name":"add","description":"add two ints","inputSchema":{"type":"object","properties":{},"required":[]}},{"name":"boom","description":"always throws","inputSchema":{"type":"object","properties":{},"required":[]}}]},"id":3}');
}

func testToolsCallString() {
    # a string handler return becomes the text content verbatim
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}},"id":4}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"hi"}],"isError":false},"id":4}');
}

func testToolsCallJsonValue() {
    # a json.Value handler return is JSON-encoded into the text content
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}},"id":5}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"{\"sum\":5}"}],"isError":false},"id":5}');
}

func testToolsCallUnknownIsToolError() {
    # an unknown tool is a tool error (isError true), not a JSON-RPC error
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"nope","arguments":{}},"id":6}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"Unknown tool: nope"}],"isError":true},"id":6}');
}

func testToolsCallThrowIsToolError() {
    # a throwing handler is a tool error; the thrown message ("kaboom") stays
    # server-side and never reaches the wire
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"boom","arguments":{}},"id":7}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"Tool execution failed"}],"isError":true},"id":7}');
}

func testUnregisteredToolNotDispatchable() {
    # SECURITY: a top-level function that exists (echoTool) but is NOT registered
    # under that name cannot be reached by naming it as the tool; only the
    # registered allow-list name ("echo") dispatches. So echoTool is unknown.
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"echoTool","arguments":{"text":"hi"}},"id":8}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"Unknown tool: echoTool"}],"isError":true},"id":8}');
}

# --- resources ----------------------------------------------------------------

func testResourcesList() {
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"resources/list","id":9}'),
        '{"jsonrpc":"2.0","result":{"resources":[{"uri":"file:///readme","name":"readme","description":"the readme","mimeType":"text/plain"}]},"id":9}');
}

func testResourcesRead() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"resources/read","params":{"uri":"file:///readme"},"id":10}'),
        '{"jsonrpc":"2.0","result":{"contents":[{"uri":"file:///readme","mimeType":"text/plain","text":"hello from file:///readme"}]},"id":10}');
}

func testResourcesReadUnknown() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"resources/read","params":{"uri":"file:///nope"},"id":11}'),
        '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Resource not found"},"id":11}');
}

# --- prompts ------------------------------------------------------------------

func testPromptsList() {
    # prompts/list surfaces each prompt's declared arguments so a host knows what
    # to collect before prompts/get
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"prompts/list","id":12}'),
        '{"jsonrpc":"2.0","result":{"prompts":[{"name":"greet","description":"a greeting","arguments":[{"name":"who","description":"who to greet","required":true}]}]},"id":12}');
}

func testPromptArgBuilder() {
    def a as PromptArg init promptArg("topic", "the subject", false);
    testing.assertEqual($a.name, "topic");
    testing.assertEqual($a.description, "the subject");
    testing.assertTrue(not $a.required);
}

func testPromptNoArgsOmitsArguments() {
    # a prompt with no declared arguments omits the `arguments` key entirely
    def s as Server init addPrompt(server("t", "1"), "bare", "no args", [], greetingPrompt);
    testing.assertEqual(
        handle($s, '{"jsonrpc":"2.0","method":"prompts/list","id":30}'),
        '{"jsonrpc":"2.0","result":{"prompts":[{"name":"bare","description":"no args"}]},"id":30}');
}

func testPromptsGet() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"prompts/get","params":{"name":"greet","arguments":{"who":"ada"}},"id":13}'),
        '{"jsonrpc":"2.0","result":{"description":"a greeting","messages":[{"role":"user","content":{"type":"text","text":"hi ada"}}]},"id":13}');
}

func testPromptsGetUnknown() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"prompts/get","params":{"name":"nope","arguments":{}},"id":14}'),
        '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Prompt not found"},"id":14}');
}

# --- protocol-level errors and notifications ----------------------------------

func testNotificationNoReply() {
    # notifications/* owes no reply
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"notifications/initialized"}'),
        "");
}

func testIdlessRequestIsNotification() {
    # a request with no id owes no reply, even for a real method
    testing.assertEqual(handle(buildServer(), '{"jsonrpc":"2.0","method":"ping"}'), "");
}

func testUnknownMethod() {
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"bogus/thing","id":15}'),
        '{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":15}');
}

func testParseError() {
    testing.assertEqual(
        handle(buildServer(), '{bad'),
        '{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}');
}

# --- hostile input: a malformed request must never crash the server -----------
# The request is untrusted; a non-string where a string param is expected used to
# throw an uncaught json.asString error, killing the transport loop (serveStdio).
# These pin that such a request now yields a clean reply instead.

func testToolsCallNonStringNameNoCrash() {
    # a numeric tool name -> a clean "unknown tool" tool error, not a crash
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"tools/call","params":{"name":123},"id":20}'),
        '{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"Unknown tool: "}],"isError":true},"id":20}');
}

func testResourcesReadNonStringUriNoCrash() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"resources/read","params":{"uri":42},"id":21}'),
        '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Resource not found"},"id":21}');
}

func testPromptsGetNonStringNameNoCrash() {
    testing.assertEqual(
        handle(
            buildServer(),
            '{"jsonrpc":"2.0","method":"prompts/get","params":{"name":true},"id":22}'),
        '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Prompt not found"},"id":22}');
}

func testToolsCallScalarParamsNoCrash() {
    # params that are not an object are invalid params, not a dispatch
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"tools/call","params":5,"id":23}'),
        '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params"},"id":23}');
}

# --- private helpers ----------------------------------------------------------

func testFindTool() {
    testing.assertEqual(findTool(buildServer(), "add"), 1);
    testing.assertEqual(findTool(buildServer(), "nope"), -1);
}

func testConstants() {
    testing.assertEqual(PROTOCOL_VERSION, "2025-06-18");
    testing.assertEqual(METHOD_NOT_FOUND, -32601);
    testing.assertEqual(PARSE_ERROR, -32700);
}

# --- stdio client transport (pure request/reply plumbing) ---------------------
# The end-to-end connectStdio path (launch a server, handshake, call) is exercised
# by the demo / manual dogfood; these pin the transport-independent plumbing.

func testStdioRequestFormat() {
    def p as json.Value init json.set(json.map(), "/x", 1);
    testing.assertEqual(
        stdioRequest("tools/list", $p, 7),
        '{"jsonrpc":"2.0","method":"tools/list","params":{"x":1},"id":7}');
}

func testStdioNotificationFormat() {
    testing.assertEqual(
        stdioNotification("notifications/initialized"),
        '{"jsonrpc":"2.0","method":"notifications/initialized"}');
}

func testStdioReplyFindsMatchingId() {
    # stdout carries the initialize reply (id 1) and the op reply (id 2); the op's
    # result is returned, correlated by id regardless of line order
    def out as string init "\{\"jsonrpc\":\"2.0\",\"result\":\{\"a\":1\},\"id\":1\}\n\{\"jsonrpc\":\"2.0\",\"result\":\{\"ok\":true\},\"id\":2\}\n";
    testing.assertTrue(json.asBool(stdioReply($out, 2), "/ok"));
}

func testStdioReplyErrorThrows() {
    def out as string init "\{\"jsonrpc\":\"2.0\",\"error\":\{\"code\":-32601,\"message\":\"nope\"\},\"id\":2\}\n";
    def threw as bool init false;
    try {
        stdioReply($out, 2);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "mcp");
        testing.assertEqual($e.reason, "rpc");
        testing.assertEqual($e.code, -32601);
        testing.assertTrue(json.isNull($e.data, ""));
    }
    testing.assertTrue($threw);
}

# An error reply's data member reaches the McpError.
func testStdioReplyErrorCarriesData() {
    def out as string init '{"jsonrpc":"2.0","error":{"code":-32602,"message":"bad","data":{"arg":"x"}},"id":2}' +
        "\n";
    try {
        stdioReply($out, 2);
        throw Error{kind: "missed", message: "an error reply"};
    } catch (e) {
        testing.assertEqual($e.code, -32602);
        testing.assertEqual(json.asString($e.data, "/arg"), "x");
        testing.assertEqual($e.message, "mcp error -32602: bad");
    }
}

# A server that cannot be run, or exits non-zero, is an McpError with that reason.
func testStdioLaunchAndExitFailures() {
    try {
        stdioCall(["/nonexistent/mcp-server"], "tools/list", json.map(), 0);
        throw Error{kind: "missed", message: "a missing server"};
    } catch (e) {
        testing.assertEqual($e.kind, "mcp");
        testing.assertEqual($e.reason, "launch");
    }
    try {
        stdioCall(["sh", "-c", "exit 3"], "tools/list", json.map(), 0);
        throw Error{kind: "missed", message: "a failing server"};
    } catch (e) {
        testing.assertEqual($e.reason, "exit");
        testing.assertEqual($e.exitCode, 3);
    }
}

func testStdioReplyMissingThrows() {
    def out as string init "\{\"jsonrpc\":\"2.0\",\"result\":\{\},\"id\":1\}\n";
    def threw as bool init false;
    try {
        stdioReply($out, 2);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "mcp");
        testing.assertEqual($e.reason, "noreply");
    }
    testing.assertTrue($threw);
}

# stdout carries only MCP messages (the stdio transport's rule): a stray log
# line, a bare value or a batch is a protocol failure, not skipped.
func testStdioReplyRefusesNonMessages() {
    def reply as string init '{"jsonrpc":"2.0","result":{"v":9},"id":2}';
    for (def junk in [
        "starting up...",
        "42",
        "[1,2]",
        '{"result":1,"id":2}',
        '{"jsonrpc":"1.0","result":1,"id":2}'
    ]) {
        testing.assertEqual(stdioReason($junk + "\n" + $reply + "\n"), "protocol");
    }
    # a server's own request or notification is a message, passed over
    def note as string init '{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"hi"}}';
    testing.assertEqual(json.asInt(stdioReply($note + "\n" + $reply + "\n", 2), "/v"), 9);
}

# A reply line past json's nesting cap is kind "limit", not a skipped line
# that ends as "noreply".
func testStdioReplyPassesADecodingCapOn() {
    def out as string init strings.repeat("[", 1001) + "\n";
    try {
        stdioReply($out, 2);
        throw Error{kind: "missed", message: "a decoding cap"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
}

# stdioReason scans stdout as stdioReply does and returns the McpError's
# reason ("" when a result came back).
func stdioReason(out as string) {
    try {
        stdioReply($out, 2);
    } catch (e) {
        testing.assertEqual($e.kind, "mcp");
        def me as McpError init $e;
        return $me.reason;
    }
    return "";
}

# A reply that is not a well-formed JSON-RPC response is an McpError with
# reason "protocol", never a runtime error.
func testStdioReplyIsStrict() {
    for (def bad in [
        '{"jsonrpc":"2.0","id":2}',
        '{"jsonrpc":"2.0","result":1,"error":{"code":1,"message":"x"},"id":2}',
        '{"jsonrpc":"2.0","error":"boom","id":2}',
        '{"jsonrpc":"2.0","error":{"code":1},"id":2}',
        '{"jsonrpc":"2.0","result":1,"id":7}',
        '{"jsonrpc":"2.0","result":1,"id":"2"}'
    ]) {
        testing.assertEqual(stdioReason($bad + "\n"), "protocol");
    }
    # a failed handshake, or an error tied to no request, ends the exchange
    testing.assertEqual(
        stdioReason('{"jsonrpc":"2.0","error":{"code":-32602,"message":"v"},"id":1}' + "\n"),
        "rpc");
    testing.assertEqual(
        stdioReason('{"jsonrpc":"2.0","error":{"code":-32700,"message":"p"},"id":null}' + "\n"),
        "rpc");
    testing.assertEqual(stdioReason('{"jsonrpc":"2.0","result":{},"id":1}' + "\n"), "noreply");
}

# --- the stdio exchange: a server that never exits, or never answers ----------

# shQuote quotes text for sh; shEcho is the line that prints it.
func shQuote(text as string) {
    return "'" + $text + "'";
}

func shEcho(text as string) {
    return "echo " + shQuote($text);
}

# writeServer writes a shell script stdio server and returns its path.
func writeServer(dir as string, script as string) {
    def file as string init path.join($dir, "srv.sh");
    fs.writeString($file, "#!/bin/sh\n" + $script + "\n");
    fs.chmod($file, 448);
    return $file;
}

# A server that answers and then keeps running after stdin's EOF is stopped,
# and its reply is used.
func testStdioServerThatKeepsRunning() {
    def dir as string init fs.makeTempDir();
    defer fs.removeAll($dir);
    def srv as string init writeServer(
        $dir,
        strings.join(
            [
                'cat >/dev/null',
                shEcho('{"jsonrpc":"2.0","result":{},"id":1}'),
                shEcho('{"jsonrpc":"2.0","result":{"tools":[]},"id":2}'),
                "trap '' TERM",
                'while :; do sleep 1; done'
            ],
            "\n"));
    def start as int init time.unixMillis(time.now());
    def c as Client init connectStdio([$srv]);
    $c.stdioTimeoutMs = 20000;
    testing.assertEqual(json.length(listTools($c), ""), 0);
    # replied at once: the grace and the kill, not the timeout
    testing.assertTrue(time.unixMillis(time.now()) - $start < 10000);
}

# A server that never answers is stopped at the deadline: kind "timeout".
func testStdioServerThatNeverAnswers() {
    def dir as string init fs.makeTempDir();
    defer fs.removeAll($dir);
    def srv as string init writeServer($dir, "cat >/dev/null\nexec sleep 30");
    def c as Client init connectStdio([$srv]);
    $c.stdioTimeoutMs = 300;
    def start as int init time.unixMillis(time.now());
    try {
        listTools($c);
        throw Error{kind: "missed", message: "a timeout"};
    } catch (e) {
        testing.assertEqual($e.kind, "timeout");
    }
    testing.assertTrue(time.unixMillis(time.now()) - $start < 10000);
}

# A server that replies and exits, with a log line on stderr, works as before.
func testStdioServerRoundTrip() {
    def dir as string init fs.makeTempDir();
    defer fs.removeAll($dir);
    def srv as string init writeServer(
        $dir,
        strings.join(
            [
                'cat >/dev/null',
                'echo starting >&2',
                shEcho('{"jsonrpc":"2.0","result":{"protocolVersion":"2025-06-18"},"id":1}'),
                "printf %s " + shQuote('{"jsonrpc":"2.0","result":{"tools":[{"name":"t"}]},"id":2}')
            ],
            "\n"));
    def tools as json.Value init listTools(connectStdio([$srv]));
    testing.assertEqual(json.asString($tools, "/0/name"), "t");
    testing.assertEqual(
        json.asString(initialize(connectStdio([$srv])), "/protocolVersion"),
        "2025-06-18");
}

# --- the server refuses requests that are not JSON-RPC 2.0 ---------------------

func testHandleRefusesInvalidRequests() {
    def invalid as string init '{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":';
    testing.assertEqual(handle(buildServer(), '{"method":"ping","id":1}'), $invalid + '1}');
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"1.0","method":"ping","id":1}'),
        $invalid + '1}');
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"ping","id":{"a":1}}'),
        $invalid + 'null}');
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"ping","id":1.5}'),
        $invalid + 'null}');
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"ping","id":null}'),
        $invalid + 'null}');
    testing.assertEqual(handle(buildServer(), '[1]'), $invalid + 'null}');
    def badParams as string init '{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params"},"id":';
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"tools/call","params":[1],"id":3}'),
        $badParams + '3}');
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"resources/read","params":"x","id":"k"}'),
        $badParams + '"k"}');
    # still fine: a string id, no params
    testing.assertEqual(
        handle(buildServer(), '{"jsonrpc":"2.0","method":"ping","id":"a"}'),
        '{"jsonrpc":"2.0","result":{},"id":"a"}');
}

# --- the HTTP client -------------------------------------------------------------

# readRequest reads one HTTP request: the head, then Content-Length bytes.
func readRequest(conn as net.Conn) {
    def got as bytes init net.readBytes($conn, 65536);
    def crlf as bytes init convert.bytesFromString("utf-8", "\r\n\r\n");
    while (binary.indexOf($got, $crlf) < 0) {
        $got = binary.concat($got, net.readBytes($conn, 65536));
    }
    def headEnd as int init binary.indexOf($got, $crlf) + 4;
    def head as string init strings.lower(convert.stringFromBytes(
        "utf-8",
        binary.slice($got, 0, $headEnd)));
    def m as regex.Match init regex.find('content-length: *([0-9]+)', $head);
    def want as int init $headEnd + convert.toInt($m.groups[0]);
    while (len($got) < $want) {
        $got = binary.concat($got, net.readBytes($conn, 65536));
    }
    return convert.stringFromBytes("utf-8", $got);
}

# answerHttp accepts one request on `l`, answers it with status / type / body
# and returns the request head and body as received.
func answerHttp(l as net.Listener, status as int, ctype as string, body as string) {
    def conn as net.Conn init net.accept($l);
    def got as string init readRequest($conn);
    net.writeBytes(
        $conn,
        convert.bytesFromString(
            "utf-8",
            "HTTP/1.1 " + convert.toString($status) + " X\r\nContent-Type: " + $ctype +
                "\r\nContent-Length: " +
                convert.toString(len(convert.bytesFromString("utf-8", $body))) +
                "\r\nConnection: close\r\n\r\n" + $body));
    net.close($conn);
    return $got;
}

# The client sends the Streamable HTTP headers: Accept for JSON and an event
# stream, and MCP-Protocol-Version once initialized; it reads a response sent
# as an event stream.
func testHttpHeadersAndEventStream() {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def stream as string init "event: message\n" +
        'data: {"jsonrpc":"2.0","method":"notifications/progress","params":{}}' + "\n\n" +
        "event: message\r\n" + 'data: {"jsonrpc":"2.0","result":' + "\r\n" +
        'data: {"tools":[{"name":"x"}]},"id":1}' + "\r\n\r\n";
    def t as task of string init spawn {
        return answerHttp($l, 200, "text/event-stream", $stream);
    };
    def tools as json.Value init listTools(connect("http://" + net.address($l) + "/mcp"));
    testing.assertEqual(json.asString($tools, "/0/name"), "x");
    def got as string init strings.lower(task.wait($t));
    testing.assertContains($got, "accept: application/json, text/event-stream");
    testing.assertContains($got, "mcp-protocol-version: 2025-06-18");
    # initialize goes without the version header, its notification with it
    $t = spawn {
        return answerHttp(
            $l,
            200,
            "application/json",
            '{"jsonrpc":"2.0","result":{"protocolVersion":"2025-06-18"},"id":1}');
    };
    def n as task of string init spawn {
        task.wait($t);
        return answerHttp($l, 202, "text/plain", "");
    };
    initialize(connect("http://" + net.address($l) + "/mcp"));
    testing.assertFalse(strings.contains(strings.lower(task.wait($t)), "mcp-protocol-version"));
    def note as string init strings.lower(task.wait($n));
    testing.assertContains($note, "notifications/initialized");
    testing.assertContains($note, "mcp-protocol-version: 2025-06-18");
    net.close($l);
}

# An event stream without the response is a protocol failure.
func testHttpEventStreamWithoutResponse() {
    def l as net.Listener init net.listen("127.0.0.1:0");
    def t as task of string init spawn {
        return answerHttp($l, 200, "text/event-stream", ": keep-alive\n\n");
    };
    try {
        listTools(connect("http://" + net.address($l) + "/mcp"));
        throw Error{kind: "missed", message: "a protocol failure"};
    } catch (e) {
        testing.assertEqual($e.kind, "jsonrpc");
        def re as jsonrpc.RpcError init $e;
        testing.assertEqual($re.reason, "protocol");
    }
    task.wait($t);
    net.close($l);
}

# A bearer token to a non-loopback http:// endpoint is refused before any
# connection (192.0.2.1 is TEST-NET); allowInsecureHttp lets it through.
func testCredentialsNeedHttps() {
    def c as Client init connectWith("http://192.0.2.1/mcp", {"Authorization": "Bearer t"});
    for (def op in ["initialize", "list"]) {
        try {
            if ($op == "initialize") {
                initialize($c);
            } else {
                listTools($c);
            }
            throw Error{kind: "missed", message: "a cleartext refusal"};
        } catch (e) {
            testing.assertEqual($e.kind, "jsonrpc");
            def re as jsonrpc.RpcError init $e;
            testing.assertEqual($re.reason, "insecure");
        }
    }
    def lan as Client init connectWith("http://host.invalid/mcp", {"Authorization": "Bearer t"});
    $lan.allowInsecureHttp = true;
    try {
        listTools($lan);
        throw Error{kind: "missed", message: "a DNS failure"};
    } catch (e) {
        testing.assertEqual($e.kind, "net");
    }
}
