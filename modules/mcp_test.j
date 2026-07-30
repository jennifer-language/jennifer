# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# mcp_test.j - white-box tests for mcp.j. Run with:
#
#     jennifer test modules/mcp_test.j
#
# The overlay splices mcp.j in first, so these tests reach its private helpers
# (findTool, valueText, encodeError) and its exported surface by bare
# identifier. The server `handle` dispatches tool / resource / prompt handlers
# by name via meta.callMain to the methods below, which the overlay is the entry
# program for; the live HTTP client round-trip is verified against an in-process
# httpd server in the Go suite (TestMcpRoundTrip).
use testing;
use json;

# --- handler methods `handle` dispatches to (named in the registry) ----------

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
    throw Error{kind: "demo", message: "kaboom", file: "", line: 0, col: 0};
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
    $s = addTool($s, "echo", "echo text", $sch, "echoTool");
    $s = addTool($s, "add", "add two ints", schema(), "addTool2");
    $s = addTool($s, "boom", "always throws", schema(), "boomTool");
    $s = addResource($s, "file:///readme", "readme", "the readme", "text/plain", "readmeResource");
    $s = addPrompt($s, "greet", "a greeting", [promptArg("who", "who to greet", true)], "greetingPrompt");
    return $s;
}

# --- schema builder -----------------------------------------------------------

func testSchemaSkeleton() {
    testing.assertEqual(
        json.encode(schema()),
        "{\"type\":\"object\",\"properties\":{},\"required\":[]}");
}

func testSchemaProperty() {
    def sch as json.Value init property(schema(), "text", "string", "text to echo", true);
    testing.assertEqual(
        json.encode($sch),
        "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\",\"description\":\"text to echo\"}},\"required\":[\"text\"]}");
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
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"initialize\",\"params\":{},\"id\":1}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{\"tools\":{},\"resources\":{},\"prompts\":{}},\"serverInfo\":{\"name\":\"demo\",\"version\":\"1.0.0\"}},\"id\":1}");
}

func testPing() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":2}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{},\"id\":2}");
}

# --- tools --------------------------------------------------------------------

func testToolsList() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/list\",\"id\":3}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"tools\":[{\"name\":\"echo\",\"description\":\"echo text\",\"inputSchema\":{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\",\"description\":\"text to echo\"}},\"required\":[\"text\"]}},{\"name\":\"add\",\"description\":\"add two ints\",\"inputSchema\":{\"type\":\"object\",\"properties\":{},\"required\":[]}},{\"name\":\"boom\",\"description\":\"always throws\",\"inputSchema\":{\"type\":\"object\",\"properties\":{},\"required\":[]}}]},\"id\":3}");
}

func testToolsCallString() {
    # a string handler return becomes the text content verbatim
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"echo\",\"arguments\":{\"text\":\"hi\"}},\"id\":4}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"hi\"}],\"isError\":false},\"id\":4}");
}

func testToolsCallJsonValue() {
    # a json.Value handler return is JSON-encoded into the text content
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"add\",\"arguments\":{\"a\":2,\"b\":3}},\"id\":5}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"sum\\\":5}\"}],\"isError\":false},\"id\":5}");
}

func testToolsCallUnknownIsToolError() {
    # an unknown tool is a tool error (isError true), not a JSON-RPC error
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"nope\",\"arguments\":{}},\"id\":6}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Unknown tool: nope\"}],\"isError\":true},\"id\":6}");
}

func testToolsCallThrowIsToolError() {
    # a throwing handler is a tool error; the thrown message ("kaboom") stays
    # server-side and never reaches the wire
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"boom\",\"arguments\":{}},\"id\":7}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Tool execution failed\"}],\"isError\":true},\"id\":7}");
}

func testUnregisteredToolNotDispatchable() {
    # SECURITY: a top-level method that exists (echoTool) but is NOT registered
    # under that name cannot be reached by naming it as the tool; only the
    # registered allow-list name ("echo") dispatches. So "echoTool" is unknown.
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"echoTool\",\"arguments\":{\"text\":\"hi\"}},\"id\":8}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Unknown tool: echoTool\"}],\"isError\":true},\"id\":8}");
}

# --- resources ----------------------------------------------------------------

func testResourcesList() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"resources/list\",\"id\":9}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"resources\":[{\"uri\":\"file:///readme\",\"name\":\"readme\",\"description\":\"the readme\",\"mimeType\":\"text/plain\"}]},\"id\":9}");
}

func testResourcesRead() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"resources/read\",\"params\":{\"uri\":\"file:///readme\"},\"id\":10}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"contents\":[{\"uri\":\"file:///readme\",\"mimeType\":\"text/plain\",\"text\":\"hello from file:///readme\"}]},\"id\":10}");
}

func testResourcesReadUnknown() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"resources/read\",\"params\":{\"uri\":\"file:///nope\"},\"id\":11}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32602,\"message\":\"Resource not found\"},\"id\":11}");
}

# --- prompts ------------------------------------------------------------------

func testPromptsList() {
    # prompts/list surfaces each prompt's declared arguments so a host knows what
    # to collect before prompts/get
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"prompts/list\",\"id\":12}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"prompts\":[{\"name\":\"greet\",\"description\":\"a greeting\",\"arguments\":[{\"name\":\"who\",\"description\":\"who to greet\",\"required\":true}]}]},\"id\":12}");
}

func testPromptArgBuilder() {
    def a as PromptArg init promptArg("topic", "the subject", false);
    testing.assertEqual($a.name, "topic");
    testing.assertEqual($a.description, "the subject");
    testing.assertTrue(not $a.required);
}

func testPromptNoArgsOmitsArguments() {
    # a prompt with no declared arguments omits the `arguments` key entirely
    def s as Server init addPrompt(server("t", "1"), "bare", "no args", [], "greetingPrompt");
    testing.assertEqual(
        handle($s, "{\"jsonrpc\":\"2.0\",\"method\":\"prompts/list\",\"id\":30}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"prompts\":[{\"name\":\"bare\",\"description\":\"no args\"}]},\"id\":30}");
}

func testPromptsGet() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"prompts/get\",\"params\":{\"name\":\"greet\",\"arguments\":{\"who\":\"ada\"}},\"id\":13}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"description\":\"a greeting\",\"messages\":[{\"role\":\"user\",\"content\":{\"type\":\"text\",\"text\":\"hi ada\"}}]},\"id\":13}");
}

func testPromptsGetUnknown() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"prompts/get\",\"params\":{\"name\":\"nope\",\"arguments\":{}},\"id\":14}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32602,\"message\":\"Prompt not found\"},\"id\":14}");
}

# --- protocol-level errors and notifications ----------------------------------

func testNotificationNoReply() {
    # notifications/* owes no reply
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}"),
        "");
}

func testIdlessRequestIsNotification() {
    # a request with no id owes no reply, even for a real method
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"ping\"}"),
        "");
}

func testUnknownMethod() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"bogus/thing\",\"id\":15}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"Method not found\"},\"id\":15}");
}

func testParseError() {
    testing.assertEqual(
        handle(buildServer(), "{bad"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32700,\"message\":\"Parse error\"},\"id\":null}");
}

# --- hostile input: a malformed request must never crash the server -----------
# The request is untrusted; a non-string where a string param is expected used to
# throw an uncaught json.asString error, killing the transport loop (serveStdio).
# These pin that such a request now yields a clean reply instead.

func testToolsCallNonStringNameNoCrash() {
    # a numeric tool name -> a clean "unknown tool" tool error, not a crash
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":123},\"id\":20}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Unknown tool: \"}],\"isError\":true},\"id\":20}");
}

func testResourcesReadNonStringUriNoCrash() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"resources/read\",\"params\":{\"uri\":42},\"id\":21}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32602,\"message\":\"Resource not found\"},\"id\":21}");
}

func testPromptsGetNonStringNameNoCrash() {
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"prompts/get\",\"params\":{\"name\":true},\"id\":22}"),
        "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32602,\"message\":\"Prompt not found\"},\"id\":22}");
}

func testToolsCallScalarParamsNoCrash() {
    # params that is a scalar (not an object) resolves to an empty name -> unknown
    testing.assertEqual(
        handle(buildServer(), "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":5,\"id\":23}"),
        "{\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Unknown tool: \"}],\"isError\":true},\"id\":23}");
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
        "{\"jsonrpc\":\"2.0\",\"method\":\"tools/list\",\"params\":{\"x\":1},\"id\":7}");
}

func testStdioNotificationFormat() {
    testing.assertEqual(
        stdioNotification("notifications/initialized"),
        "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}");
}

func testStdioReplyFindsMatchingId() {
    # stdout carries the initialize reply (id 1) and the op reply (id 2); the op's
    # result is returned, correlated by id regardless of line order
    def out as string init "{\"jsonrpc\":\"2.0\",\"result\":{\"a\":1},\"id\":1}\n{\"jsonrpc\":\"2.0\",\"result\":{\"ok\":true},\"id\":2}\n";
    testing.assertTrue(json.asBool(stdioReply($out, 2), "/ok"));
}

func testStdioReplyErrorThrows() {
    def out as string init "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"nope\"},\"id\":2}\n";
    def threw as bool init false;
    try {
        stdioReply($out, 2);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "mcp");
    }
    testing.assertTrue($threw);
}

func testStdioReplyMissingThrows() {
    def out as string init "{\"jsonrpc\":\"2.0\",\"result\":{},\"id\":1}\n";
    def threw as bool init false;
    try {
        stdioReply($out, 2);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "mcp");
    }
    testing.assertTrue($threw);
}

func testStdioReplySkipsNonJsonLines() {
    # a stray non-JSON line (a server writing a log to stdout) is skipped, the real
    # reply is still found
    def out as string init "starting up...\n{\"jsonrpc\":\"2.0\",\"result\":{\"v\":9},\"id\":2}\n";
    testing.assertEqual(json.asInt(stdioReply($out, 2), "/v"), 9);
}
