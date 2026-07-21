// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"
)

// TestTelegramBotAPI drives the telegram module against an in-process fake Bot
// API server (reached via botWith's overridable base URL). It covers getMe,
// sendMessage (asserting the form params round-trip), getUpdates (a parsed
// update), and the {"ok":false} error path throwing in the .j program.
func TestTelegramBotAPI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		method := path.Base(r.URL.Path) // /bot<token>/<method>
		switch method {
		case "getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Bot","username":"testbot"}}`)
		case "sendMessage":
			_ = r.ParseForm()
			if r.PostFormValue("chat_id") != "555" || r.PostFormValue("text") != "hello world" {
				fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
				return
			}
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":10,"chat":{"id":555},"date":1700000000,"text":"hello world"}}`)
		case "getUpdates":
			fmt.Fprint(w, `{"ok":true,"result":[{"update_id":100,"message":{"message_id":1,"chat":{"id":5},"date":1,"text":"ping"}}]}`)
		default:
			fmt.Fprint(w, `{"ok":false,"description":"unknown method"}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tgMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "telegram.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as telegram;
def bot as telegram.Bot init telegram.botWith("TESTTOKEN", %q);

def me as telegram.User init telegram.getMe($bot);
testing.assertEqual($me.username, "testbot");
testing.assertTrue($me.isBot);

def m as telegram.Message init telegram.sendMessage($bot, 555, "hello world");
testing.assertEqual($m.messageId, 10);
testing.assertEqual($m.chatId, 555);
testing.assertEqual($m.text, "hello world");

def us as list of telegram.Update init telegram.getUpdates($bot, 0, 0);
testing.assertEqual(len($us), 1);
testing.assertEqual($us[0].updateId, 100);
testing.assertTrue($us[0].hasMessage);
testing.assertEqual($us[0].message.text, "ping");

def threw as bool init false;
try {
    telegram.sendMessage($bot, 999, "x");
} catch (e) {
    $threw = true;
    testing.assertEqual($e.kind, "telegram");
}
testing.assertTrue($threw);`, tgMod, srv.URL)
	progPath := filepath.Join(dir, "tg.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("telegram program failed with code %d", code)
	}
}
