#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Model Context Protocol server side, self-contained: build a Server with a
 * tool, a resource, and a prompt, then answer a full `initialize` /
 * `tools/list` / `tools/call` sequence with `mcp.handle` inline. No network -
 * `handle` is the transport-agnostic dispatcher; a real server wires it to
 * `mcp.serveStdio` (stdio) or `httpd` (HTTP).
 * @module mcp_demo
 */
use io;
use json;
import "../../modules/mcp.j" as mcp;

# --- tool / resource / prompt handlers (reached by name via meta.callMain) ---

func echoTool(args as json.Value) {
    return json.asString($args, "/text");
}

func addTool2(args as json.Value) {
    def out as json.Value init json.map();
    $out = json.set($out, "/sum", json.asInt($args, "/a") + json.asInt($args, "/b"));
    return $out;
}

func readmeResource(uri as json.Value) {
    return "This is the readme for " + json.asString($uri, "");
}

func greetingPrompt(args as json.Value) {
    # a PromptMessage's content is a content block ({type:"text", text}) per MCP
    def content as json.Value init json.map();
    $content = json.set($content, "/type", "text");
    $content = json.set($content, "/text", "Please greet " + json.asString($args, "/who"));
    def msg as json.Value init json.map();
    $msg = json.set($msg, "/role", "user");
    $msg = json.set($msg, "/content", $content);
    def arr as json.Value init json.list();
    $arr = json.append($arr, "", $msg);
    return $arr;
}

# --- build the server --------------------------------------------------------

def echoSchema as json.Value init mcp.property(mcp.schema(), "text", "string", "the text to echo", true);
def addSchema as json.Value init mcp.property(
    mcp.property(mcp.schema(), "a", "integer", "first addend", true),
    "b", "integer", "second addend", true);

def srv as mcp.Server init mcp.server("demo-server", "1.0.0");
$srv = mcp.addTool($srv, "echo", "Echo the given text", $echoSchema, "echoTool");
$srv = mcp.addTool($srv, "add", "Add two integers", $addSchema, "addTool2");
$srv = mcp.addResource($srv, "file:///readme", "readme", "The project readme", "text/plain", "readmeResource");
$srv = mcp.addPrompt(
    $srv,
    "greet",
    "A friendly greeting",
    [mcp.promptArg("who", "the name to greet", true)],
    "greetingPrompt");

# --- answer a request sequence with mcp.handle (no network) ------------------

io.printf("MCP server side (mcp.handle):\n\n");

io.printf("initialize    -> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}'));

io.printf("tools/list    -> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"tools/list","id":2}'));

io.printf("tools/call    -> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}},"id":3}'));

io.printf("resources/read-> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"resources/read","params":{"uri":"file:///readme"},"id":4}'));

io.printf("prompts/get   -> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"prompts/get","params":{"name":"greet","arguments":{"who":"Ada"}},"id":5}'));

io.printf("unknown method-> %s\n\n",
    mcp.handle($srv, '{"jsonrpc":"2.0","method":"bogus","id":6}'));

io.printf("notification  -> (no reply owed; handle returned an empty string)\n");
