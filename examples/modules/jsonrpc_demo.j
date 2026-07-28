#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * JSON-RPC 2.0: the server side (dispatch requests to named methods with
 * `jsonrpc.handle`) runs self-contained here; the client side (`jsonrpc.call`)
 * runs against a real endpoint when JSONRPC_URL is set.
 * @module jsonrpc_demo
 */
use io;
use os;
use json;
use strings;
import "../../modules/jsonrpc.j" as jsonrpc;

# --- server: methods `handle` dispatches to, by name ---

func add(params as json.Value) {
    return json.asInt($params, "/0") + json.asInt($params, "/1");
}

func upper(params as json.Value) {
    return strings.upper(json.asString($params, "/text"));
}

io.printf("server side (jsonrpc.handle):\n");
io.printf(
    "  add(2, 3)      -> %s\n",
    jsonrpc.handle("{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[2,3],\"id\":1}"));
io.printf(
    "  upper(\"hi\")    -> %s\n",
    jsonrpc.handle("{\"jsonrpc\":\"2.0\",\"method\":\"upper\",\"params\":{\"text\":\"hi\"},\"id\":2}"));
io.printf(
    "  unknownMethod  -> %s\n",
    jsonrpc.handle("{\"jsonrpc\":\"2.0\",\"method\":\"nope\",\"id\":3}"));
io.printf(
    "  a batch        -> %s\n",
    jsonrpc.handle("[{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[10,20],\"id\":1},{\"jsonrpc\":\"2.0\",\"method\":\"add\",\"params\":[1,1]}]"));

# --- client: call a real endpoint if one is configured ---

def url as string init os.getEnv("JSONRPC_URL");
if ($url == "") {
    io.printf("\nclient side: set JSONRPC_URL to a JSON-RPC endpoint to call it live.\n");
    exit;
}

def c as jsonrpc.Client init jsonrpc.client($url);
def params as json.Value init json.list();
$params = json.append($params, "", 2);
$params = json.append($params, "", 3);
def method as string init os.getEnv("JSONRPC_METHOD");
if ($method == "") {
    $method = "add";
}
def result as json.Value init jsonrpc.call($c, $method, $params);
io.printf("\nclient side: %s([2, 3]) -> %s\n", $method, json.encode($result));
