// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestFeedOverHTTP drives the documented way to read a feed off the web end to
// end: an in-process server serves an RSS document, the .j program fetches it
// with the http module and hands the body to feed.parse, then asserts the parsed
// Feed. A mismatch throws in the .j program and fails loadForTest.
func TestFeedOverHTTP(t *testing.T) {
	const rss = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>Example Blog</title>
  <link>https://example.org</link>
  <lastBuildDate>Mon, 20 Jul 2026 14:30:00 +0000</lastBuildDate>
  <item>
    <title>First &amp; foremost</title>
    <link>https://example.org/1</link>
    <guid>id-1</guid>
    <pubDate>Mon, 20 Jul 2026 09:00:00 +0000</pubDate>
    <description>Hello &lt;world&gt;</description>
  </item>
  <item>
    <title>Second</title>
    <link>https://example.org/2</link>
    <guid>id-2</guid>
  </item>
</channel></rss>`

	mux := http.NewServeMux()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rss)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	feedMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "feed.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use time;
import %q as feed;
import %q as http;

def resp as http.Response init http.get(%q, {});
testing.assertEqual($resp.status, 200);
def f as feed.Feed init feed.parse($resp.body);
testing.assertEqual($f.title, "Example Blog");
testing.assertEqual($f.link, "https://example.org");
testing.assertEqual(time.iso($f.updated), "2026-07-20T14:30:00Z");
testing.assertEqual(len($f.entries), 2);
testing.assertEqual($f.entries[0].title, "First & foremost");
testing.assertEqual($f.entries[0].id, "id-1");
testing.assertEqual($f.entries[0].summary, "Hello <world>");
testing.assertEqual(time.iso($f.entries[0].published), "2026-07-20T09:00:00Z");
testing.assertEqual($f.entries[1].title, "Second");`, feedMod, httpMod, srv.URL+"/feed.xml")
	progPath := filepath.Join(dir, "feed.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("feed program failed with code %d", code)
	}
}
