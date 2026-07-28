// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fakeJsonrpc is a minimal in-process JSON-RPC 2.0 server: it parses the POSTed
// request, dispatches `add` (sum of two positional params) and `err` (a JSON-RPC
// error), and returns nothing for a notification (no `id`). It exercises the
// jsonrpc client's request encoding and reply / error decoding on a real socket.
func fakeJsonrpc(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Method string           `json:"method"`
		Params []json.Number    `json:"params"`
		ID     *json.RawMessage `json:"id"`
	}
	if json.Unmarshal(body, &req) != nil {
		fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`)
		return
	}
	if req.ID == nil { // notification: no reply
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := string(*req.ID)
	switch req.Method {
	case "add":
		a, _ := req.Params[0].Int64()
		b, _ := req.Params[1].Int64()
		fmt.Fprintf(w, `{"jsonrpc":"2.0","result":%d,"id":%s}`, a+b, id)
	case "err":
		fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32000,"message":"boom"},"id":%s}`, id)
	default:
		fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":%s}`, id)
	}
}

// A .j program drives the jsonrpc client against the in-process server: a `call`
// returns the result, an error reply throws `Error{kind: "jsonrpc"}` (caught by
// assertThrows), and a `notify` returns without error. A mismatch throws and
// fails loadForTest. Runs the real HTTP dialogue in CI with no external server.
func TestJsonrpcClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", fakeJsonrpc)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rpcMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "jsonrpc.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use json;
import %q as jsonrpc;
def c as jsonrpc.Client init jsonrpc.client(%q);

def p as json.Value init json.list();
$p = json.append($p, "", 2);
$p = json.append($p, "", 40);
def r as json.Value init jsonrpc.call($c, "add", $p);
testing.assertEqual(json.asInt($r, ""), 42);

testing.assertThrows("callErr", "jsonrpc");
jsonrpc.notify($c, "add", $p);

func callErr() {
    jsonrpc.call($c, "err", json.list());
}`, rpcMod, srv.URL+"/rpc")
	progPath := filepath.Join(dir, "rpc.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("jsonrpc client program failed with code %d", code)
	}
}
