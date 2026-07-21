#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The websocket module (modules/websocket.j): an RFC 6455 client. Connect to a
 * WebSocket echo server, send a few messages, and print what comes back. Needs
 * the default `jennifer` binary (net). Defaults to the public echo server
 * wss://ws.postman-echo.com/raw; pass a different ws:// or wss:// URL as the
 * first argument. Requires network access to reach the server.
 * Run: jennifer run examples/modules/websocket_demo.j [wsUrl]
 * @module websocket_demo
 */
use io;
use os;
import "../../modules/websocket.j" as websocket;

def url as string init "wss://ws.postman-echo.com/raw";
if (len(os.ARGS) > 1) { $url = os.ARGS[1]; }

io.printf("connecting to %s ...\n", $url);
def ws as websocket.Conn init websocket.connect($url);
io.printf("connected (handshake ok)\n");

def messages as list of string init ["hello", "from Jennifer", "over WebSocket"];
for (def msg in $messages) {
    websocket.send($ws, $msg);
    def reply as websocket.Message init websocket.receive($ws);
    io.printf("  sent %s|pad=16 -> got [%s] %s\n", $msg, $reply.kind, $reply.text);
}

websocket.close($ws);
io.printf("closed\n");
