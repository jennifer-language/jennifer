// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// webhookRecorder is a fake webhook endpoint that records every POST body (JSON
// content type required) and answers with a fixed status. Bodies are collected
// under a mutex so the test goroutine reads them safely after loadForTest runs
// the synchronous .j program.
func webhookRecorder(status int) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var bodies []string
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			fmt.Fprint(w, "ok")
		}
	})
	srv := httptest.NewServer(mux)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(bodies))
		copy(out, bodies)
		return out
	}
}

// TestSlackWebhook drives slack.send (plain) and slack.sendMessage (Block Kit)
// against a fake Incoming Webhook, asserting the exact JSON payloads.
func TestSlackWebhook(t *testing.T) {
	srv, got := webhookRecorder(http.StatusOK)
	defer srv.Close()

	slackMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "slack.j"))
	if err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/webhook"
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as slack;
slack.send(%q, "hello");
def m as slack.Message init slack.divider(slack.section(slack.header(slack.message(), "Deploy"), "body"));
slack.sendMessage(%q, $m);`, slackMod, url, url)
	progPath := filepath.Join(dir, "slack.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("slack program failed with code %d", code)
	}

	bodies := got()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 webhook posts, got %d: %q", len(bodies), bodies)
	}
	if bodies[0] != `{"text":"hello"}` {
		t.Errorf("send body = %q", bodies[0])
	}
	want := `{"blocks":[{"type":"header","text":{"type":"plain_text","text":"Deploy"}},` +
		`{"type":"section","text":{"type":"mrkdwn","text":"body"}},{"type":"divider"}]}`
	if bodies[1] != want {
		t.Errorf("sendMessage body = %q, want %q", bodies[1], want)
	}
}

// TestDiscordWebhook drives discord.send (plain) and discord.sendMessage (embed)
// against a fake channel Webhook (which answers 204), asserting the payloads.
func TestDiscordWebhook(t *testing.T) {
	srv, got := webhookRecorder(http.StatusNoContent)
	defer srv.Close()

	discordMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "discord.j"))
	if err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/webhook"
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as discord;
discord.send(%q, "hello");
def m as discord.Message init discord.embed(discord.content(discord.message(), "heads up"), "Deploy", "live", 3066993);
discord.sendMessage(%q, $m);`, discordMod, url, url)
	progPath := filepath.Join(dir, "discord.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("discord program failed with code %d", code)
	}

	bodies := got()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 webhook posts, got %d: %q", len(bodies), bodies)
	}
	if bodies[0] != `{"content":"hello"}` {
		t.Errorf("send body = %q", bodies[0])
	}
	want := `{"content":"heads up","embeds":[{"title":"Deploy","description":"live","color":3066993}]}`
	if bodies[1] != want {
		t.Errorf("sendMessage body = %q, want %q", bodies[1], want)
	}
}
