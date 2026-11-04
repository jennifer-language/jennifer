// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package regexlib

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPatternProgramSizeCap(t *testing.T) {
	resetCacheForTest()
	saved := maxProgramSize
	maxProgramSize = 100
	defer func() { maxProgramSize = saved }()
	if _, err := compilePattern("regex.test", "a{1000}"); err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("oversized pattern: expected a program-size error, got %v", err)
	}
	if _, err := compilePattern("regex.test", "abc"); err != nil {
		t.Errorf("small pattern should compile, got %v", err)
	}
}

// TestPatternLengthCap: a pattern past maxPatternBytes is refused before it is
// parsed. Parsing a 1 MiB literal to measure it cost about 480 MB of
// allocation, so the length check has to come first.
func TestPatternLengthCap(t *testing.T) {
	resetCacheForTest()
	big := strings.Repeat("a", maxPatternBytes+1)
	t0 := time.Now()
	_, err := compilePattern("regex.test", big)
	if err == nil || !strings.Contains(err.Error(), "bytes, over the limit") {
		t.Fatalf("over-long pattern: expected a length error, got %v", err)
	}
	if d := time.Since(t0); d > 50*time.Millisecond {
		t.Errorf("refusing an over-long pattern took %v; it should not parse it", d)
	}
	if _, err := compilePattern("regex.test", strings.Repeat("a", 1000)); err != nil {
		t.Errorf("a 1000-byte pattern should compile, got %v", err)
	}
}

// TestErrorQuotesShortPattern: an error quotes at most maxQuotedPattern runes
// of the pattern, Go-escaped.
func TestErrorQuotesShortPattern(t *testing.T) {
	resetCacheForTest()
	_, err := compilePattern("regex.test", "("+strings.Repeat("x", 500)+"\n")
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if len(err.Error()) > 200 || !strings.Contains(err.Error(), `"...`) {
		t.Errorf("message not cut short: %q", err.Error())
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("message holds a raw newline: %q", err.Error())
	}
	if got := quoted("a\nb"); got != `"a\nb"` {
		t.Errorf("quoted = %s", got)
	}
}

// TestConcurrentCompile: compiling happens outside the cache lock, so many
// goroutines missing at once must still agree on one cached entry per pattern
// and keep the LRU bounded (run with -race).
func TestConcurrentCompile(t *testing.T) {
	resetCacheForTest()
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				p := fmt.Sprintf("x%d+y", i%200)
				re, err := compilePattern("regex.test", p)
				if err != nil || !re.MatchString(fmt.Sprintf("x%dy", i%200)) {
					t.Errorf("pattern %s: %v", p, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cacheLRU.Len() > cacheCap || len(cacheMap) != cacheLRU.Len() {
		t.Errorf("cache out of step: lru %d, map %d, cap %d", cacheLRU.Len(), len(cacheMap), cacheCap)
	}
}
