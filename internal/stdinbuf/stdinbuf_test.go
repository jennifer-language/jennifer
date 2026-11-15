// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package stdinbuf

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// countingReader counts Read calls, to show the pump starts only on demand.
type countingReader struct {
	n     atomic.Int32
	inner io.Reader
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.n.Add(1)
	return c.inner.Read(p)
}

func TestNoReadUntilFirstUse(t *testing.T) {
	ResetForTest()
	src := &countingReader{inner: strings.NewReader("x")}
	r := For(src)
	if For(src) != r {
		t.Fatal("For must return the same reader for the same source")
	}
	time.Sleep(10 * time.Millisecond)
	if got := src.n.Load(); got != 0 {
		t.Fatalf("source read %d times before any read", got)
	}
	if b, err := r.ReadByte(); err != nil || b != 'x' {
		t.Fatalf("ReadByte = %q, %v", b, err)
	}
	if For(nil) != nil {
		t.Error("For(nil) should be nil")
	}
}

// A timed-out read loses nothing: its fill stays in flight, and the byte it
// yields is the next byte a line read sees, in order.
func TestTimeoutThenLine(t *testing.T) {
	ResetForTest()
	pr, pw := io.Pipe()
	defer pw.Close()
	r := For(pr)
	if _, ok, err := r.ReadByteTimeout(20 * time.Millisecond); ok || err != nil {
		t.Fatalf("timed read on an idle pipe: ok=%v err=%v, want a timeout", ok, err)
	}
	go func() { _, _ = pw.Write([]byte("ab\ncd\n")) }()
	line, err := r.ReadDelim('\n', 100)
	if err != nil || string(line) != "ab\n" {
		t.Fatalf("ReadDelim = %q, %v; want \"ab\\n\"", line, err)
	}
	b, ok, err := r.ReadByteTimeout(time.Second)
	if !ok || err != nil || b != 'c' {
		t.Fatalf("next byte = %q ok=%v err=%v; want 'c'", b, ok, err)
	}
	line, err = r.ReadDelim('\n', 100)
	if err != nil || string(line) != "d\n" {
		t.Fatalf("ReadDelim = %q, %v; want \"d\\n\"", line, err)
	}
}

// EOF is sticky and reaches every kind of read, after the buffered bytes.
func TestEOFReachesEveryRead(t *testing.T) {
	ResetForTest()
	r := For(strings.NewReader("tail"))
	line, err := r.ReadDelim('\n', 100)
	if string(line) != "tail" || err != io.EOF {
		t.Fatalf("ReadDelim = %q, %v; want \"tail\", EOF", line, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := r.ReadByte(); err != io.EOF {
			t.Errorf("ReadByte at EOF: %v", err)
		}
		if _, ok, err := r.ReadByteTimeout(time.Second); !ok || err != io.EOF {
			t.Errorf("ReadByteTimeout at EOF: ok=%v %v", ok, err)
		}
		if err := r.Peek(); err != io.EOF {
			t.Errorf("Peek at EOF: %v", err)
		}
		if _, _, err := r.ReadRune(); err != io.EOF {
			t.Errorf("ReadRune at EOF: %v", err)
		}
		if n, err := r.Read(make([]byte, 4)); n != 0 || err != io.EOF {
			t.Errorf("Read at EOF: %d, %v", n, err)
		}
	}
}

// A closed stdin (the read end closed under a pending timed read, as `<&-`
// or a closed descriptor would) reports its error to every reader.
func TestClosedSourceReachesEveryRead(t *testing.T) {
	ResetForTest()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()
	_ = pr.Close()
	r := For(pr)
	if _, _, err := r.ReadByteTimeout(time.Second); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("ReadByteTimeout on a closed file: %v", err)
	}
	if _, err := r.ReadDelim('\n', 100); !errors.Is(err, os.ErrClosed) {
		t.Errorf("ReadDelim on a closed file: %v", err)
	}
	if err := r.Peek(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("Peek on a closed file: %v", err)
	}
}

// A read error arriving while a timed read is pending reaches the next reader.
func TestErrorAfterTimeout(t *testing.T) {
	ResetForTest()
	pr, pw := io.Pipe()
	r := For(pr)
	if _, ok, _ := r.ReadByteTimeout(10 * time.Millisecond); ok {
		t.Fatal("want a timeout")
	}
	boom := errors.New("boom")
	_ = pw.CloseWithError(boom)
	if _, err := r.ReadDelim('\n', 100); !errors.Is(err, boom) {
		t.Errorf("ReadDelim after the error: %v", err)
	}
	if _, err := r.ReadByte(); !errors.Is(err, boom) {
		t.Errorf("ReadByte after the error: %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }

func TestNoProgress(t *testing.T) {
	ResetForTest()
	if _, err := For(zeroReader{}).ReadByte(); !errors.Is(err, io.ErrNoProgress) {
		t.Errorf("ReadByte over a (0, nil) source: %v", err)
	}
}

// oneByteReader hands out one byte per Read, so runes and lines span fills.
type oneByteReader struct{ s string }

func (o *oneByteReader) Read(p []byte) (int, error) {
	if o.s == "" {
		return 0, io.EOF
	}
	p[0] = o.s[0]
	o.s = o.s[1:]
	return 1, nil
}

func TestReadRuneAcrossFills(t *testing.T) {
	ResetForTest()
	r := For(&oneByteReader{s: "é€\xffa\xe2\x82"})
	for _, want := range []struct {
		r    rune
		size int
	}{{'é', 2}, {'€', 3}, {utf8.RuneError, 1}, {'a', 1}, {utf8.RuneError, 1}, {utf8.RuneError, 1}} {
		ch, size, err := r.ReadRune()
		if err != nil || ch != want.r || size != want.size {
			t.Fatalf("ReadRune = %q/%d/%v; want %q/%d", ch, size, err, want.r, want.size)
		}
	}
	if _, _, err := r.ReadRune(); err != io.EOF {
		t.Errorf("ReadRune at EOF: %v", err)
	}
}

func TestReadDelimLimit(t *testing.T) {
	ResetForTest()
	r := For(&oneByteReader{s: "abcdef\nok\n"})
	if _, err := r.ReadDelim('\n', 4); !errors.Is(err, limits.ErrTooLarge) {
		t.Fatalf("over-long line: %v", err)
	}
	// The line was consumed up to the limit; the rest stays readable.
	line, err := r.ReadDelim('\n', 100)
	if err != nil || !strings.HasSuffix(string(line), "\n") {
		t.Fatalf("after the limit: %q, %v", line, err)
	}
	line, err = r.ReadDelim('\n', 100)
	if err != nil || string(line) != "ok\n" {
		t.Errorf("next line = %q, %v", line, err)
	}
}

// Stdin that is a file or a pipe reads through the same layer.
func TestFileAndPipeSources(t *testing.T) {
	ResetForTest()
	path := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(path, []byte("one\ntwo"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := For(f)
	if b, _ := r.ReadByte(); b != 'o' {
		t.Fatalf("first byte %q", b)
	}
	if line, _ := r.ReadDelim('\n', 100); string(line) != "ne\n" {
		t.Fatalf("line %q", line)
	}
	if rest, err := io.ReadAll(r); err != nil || string(rest) != "two" {
		t.Fatalf("rest %q, %v", rest, err)
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	pr2 := For(pr)
	go func() {
		_, _ = pw.Write([]byte("k"))
		time.Sleep(30 * time.Millisecond)
		_, _ = pw.Write([]byte("line\n"))
		_ = pw.Close()
	}()
	if b, ok, err := pr2.ReadByteTimeout(time.Second); !ok || err != nil || b != 'k' {
		t.Fatalf("pipe key = %q ok=%v err=%v", b, ok, err)
	}
	if line, err := pr2.ReadDelim('\n', 100); err != nil || string(line) != "line\n" {
		t.Fatalf("pipe line = %q, %v", line, err)
	}
	if err := pr2.Peek(); err != io.EOF {
		t.Errorf("pipe end: %v", err)
	}
}

// Concurrent readers of any kind get each byte exactly once (run under -race).
func TestConcurrentReadersShareBytes(t *testing.T) {
	ResetForTest()
	const n = 20000
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i%250) + 1
	}
	pr, pw := io.Pipe()
	go func() {
		for i := 0; i < n; i += 997 {
			_, _ = pw.Write(data[i:min(i+997, n)])
		}
		_ = pw.Close()
	}()
	r := For(pr)
	var mu sync.Mutex
	seen := 0
	sum := 0
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for {
				var got []byte
				switch g % 4 {
				case 0:
					b, err := r.ReadByte()
					if err != nil {
						return
					}
					got = []byte{b}
				case 1:
					b, ok, err := r.ReadByteTimeout(time.Millisecond)
					if err != nil {
						return
					}
					if ok {
						got = []byte{b}
					}
				case 2:
					buf := make([]byte, 7)
					k, err := r.Read(buf)
					if err != nil {
						return
					}
					got = buf[:k]
				case 3:
					line, err := r.ReadDelim(250, 1<<20)
					got = line
					if err != nil {
						mu.Lock()
						seen += len(got)
						for _, c := range got {
							sum += int(c)
						}
						mu.Unlock()
						return
					}
				}
				mu.Lock()
				seen += len(got)
				for _, c := range got {
					sum += int(c)
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()
	want := 0
	for _, c := range data {
		want += int(c)
	}
	if seen != n || sum != want {
		t.Errorf("readers saw %d bytes (sum %d), want %d (sum %d)", seen, sum, n, want)
	}
}
