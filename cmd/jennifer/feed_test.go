// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestFeedFetch drives the feed module's networked path end to end: an
// in-process server serves an RSS document, feed.fetch pulls and parses it over
// the http module, and the .j program asserts the parsed Feed. A non-2xx status
// is surfaced as a catchable error, not a crash. A mismatch throws in the .j
// program and fails loadForTest.
func TestFeedFetch(t *testing.T) {
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
	mux.HandleFunc("/missing.xml", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	feedMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "feed.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use time;
import %q as feed;

def f as feed.Feed init feed.fetch(%q);
testing.assertEqual($f.title, "Example Blog");
testing.assertEqual($f.link, "https://example.org");
testing.assertEqual(time.iso($f.updated), "2026-07-20T14:30:00Z");
testing.assertEqual(len($f.entries), 2);
testing.assertEqual($f.entries[0].title, "First & foremost");
testing.assertEqual($f.entries[0].id, "id-1");
testing.assertEqual($f.entries[0].summary, "Hello <world>");
testing.assertEqual(time.iso($f.entries[0].published), "2026-07-20T09:00:00Z");
testing.assertEqual($f.entries[1].title, "Second");

# A non-2xx status is a catchable error.
def caught as bool init false;
try {
    feed.fetch(%q);
} catch (e) {
    $caught = true;
}
testing.assertTrue($caught);`, feedMod, srv.URL+"/feed.xml", srv.URL+"/missing.xml")
	progPath := filepath.Join(dir, "feed.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("feed program failed with code %d", code)
	}
}
