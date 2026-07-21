// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package fslib_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	fslib "jennifer-lang.dev/jennifer/internal/lib/fs"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg parses + runs a Jennifer program with io + task + fs
// installed. Returns captured stdout and the interpreter error. The
// helper wipes the fs handle registry between calls so a leaked
// handle in one test can't leak an id into the next.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	fslib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	fslib.Install(in)
	runErr := in.Run(prog)
	// Drain any lingering unwaited tasks (fs+spawn tests below spawn
	// off filesystem work; loud-fail complaints noise up the output).
	_ = in.UnwaitedTaskErrors()
	return buf.String(), runErr
}

// TestReadWriteStringRoundTrip exercises the happy path for the
// whole-file string API.
func TestReadWriteStringRoundTrip(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "hello.txt")
	src := fmt.Sprintf(`
		use io;
		use fs;
		fs.writeString(%q, "hello, world");
		def s as string init fs.readString(%q);
		io.printf("%%s", $s);
	`, tmp, tmp)
	out, err := runProg(t, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "hello, world" {
		t.Errorf("got %q, want %q", out, "hello, world")
	}
}

// TestReadStringMissingPath: reading a missing file surfaces at the
// boundary with the path in the message.
func TestReadStringMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def s as string init fs.readString(%q);
	`, missing))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.txt") {
		t.Errorf("error should mention the path: %v", err)
	}
}

// TestReadStringInvalidUTF8 - readString must reject a file whose
// content is not valid UTF-8 (strict at the boundary).
func TestReadStringInvalidUTF8(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bin.dat")
	// 0xFF is invalid as a first byte in UTF-8.
	if err := os.WriteFile(tmp, []byte{0xFF, 0xFE, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def s as string init fs.readString(%q);
	`, tmp))
	if err == nil {
		t.Fatal("expected UTF-8 rejection")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error should mention UTF-8: %v", err)
	}
}

// TestReadBytesRoundTrip - readBytes returns raw bytes even when the
// content isn't valid UTF-8; writeBytes accepts them back verbatim.
func TestReadBytesRoundTrip(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bin.dat")
	if err := os.WriteFile(tmp, []byte{0x00, 0xFF, 0x10, 0x7F}, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def b as bytes init fs.readBytes(%q);
		io.printf("%%d", len($b));
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "4" {
		t.Errorf("got %q, want %q", out, "4")
	}
}

// TestAppendString adds content to an existing file rather than
// overwriting.
func TestAppendString(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(tmp, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		fs.appendString(%q, "second\n");
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, rerr := os.ReadFile(tmp)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "first\nsecond\n" {
		t.Errorf("got %q", string(got))
	}
}

// TestExistsAndPredicates covers exists / isFile / isDir on a small
// directory tree.
func TestExistsAndPredicates(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		io.printf("%%t %%t %%t %%t %%t %%t",
			fs.exists(%q), fs.exists(%q),
			fs.isFile(%q), fs.isDir(%q),
			fs.isFile(%q), fs.isDir(%q));
	`, file, filepath.Join(dir, "missing"),
		file, file,
		dir, dir))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "true false true false false true"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestStatShape - fs.stat returns an fs.Stat with the fields readable.
func TestStatShape(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "info.txt")
	if err := os.WriteFile(tmp, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def s as fs.Stat init fs.stat(%q);
		io.printf("size=%%d isDir=%%t mtime>0=%%t", $s.size, $s.isDir, $s.mtimeNanos > 0);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "size=3 isDir=false mtime>0=true"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestMkdirTwoVerbs covers the two-verbs pattern for directory
// creation: mkdir fails when a parent is missing; mkdirAll succeeds.
func TestMkdirTwoVerbs(t *testing.T) {
	base := t.TempDir()
	deep := filepath.Join(base, "a", "b", "c")

	// mkdir refuses to create with missing parents.
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		fs.mkdir(%q);
	`, deep))
	if err == nil {
		t.Fatal("expected fs.mkdir to reject a missing-parent path")
	}

	// mkdirAll creates the whole chain.
	_, err = runProg(t, fmt.Sprintf(`
		use fs;
		fs.mkdirAll(%q);
	`, deep))
	if err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}
	if info, statErr := os.Stat(deep); statErr != nil || !info.IsDir() {
		t.Errorf("expected %s to exist and be a dir, got %v / %v", deep, info, statErr)
	}
}

// TestRemoveTwoVerbs covers the two-verbs pattern for delete: remove
// refuses a non-empty dir; removeAll clears it.
func TestRemoveTwoVerbs(t *testing.T) {
	base := t.TempDir()
	// Nested content so `remove` has something to refuse.
	nested := filepath.Join(base, "target", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "x.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target")

	// remove on a non-empty directory must fail.
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		fs.remove(%q);
	`, target))
	if err == nil {
		t.Fatal("expected fs.remove to refuse a non-empty directory")
	}

	// removeAll clears it.
	_, err = runProg(t, fmt.Sprintf(`
		use fs;
		fs.removeAll(%q);
	`, target))
	if err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("expected %s to be gone, stat err was %v", target, statErr)
	}
}

// TestListSorted - fs.list is deterministic (sorted).
func TestListSorted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def names as list of string init fs.list(%q);
		io.printf("%%a", $names);
	`, dir))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "[alpha, bravo, charlie]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestWalkIncludesRootAndIsSorted - the root itself appears first,
// then entries in per-directory sorted order.
func TestWalkIncludesRootAndIsSorted(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.txt", "z.txt", filepath.Join("sub", "y.txt")} {
		full := filepath.Join(dir, p)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def entries as list of fs.Stat init fs.walk(%q);
		def i as int init 0;
		while ($i < len($entries)) {
			def e as fs.Stat init $entries[$i];
			io.printf("%%s\n", $e.path);
			$i = $i + 1;
		}
	`, dir))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Expected order: root, a.txt, sub, sub/y.txt, z.txt.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 entries, got %d: %q", len(lines), out)
	}
	if lines[0] != dir {
		t.Errorf("root should be first; got %q, want %q", lines[0], dir)
	}
	expected := []string{
		dir,
		filepath.Join(dir, "a.txt"),
		filepath.Join(dir, "sub"),
		filepath.Join(dir, "sub", "y.txt"),
		filepath.Join(dir, "z.txt"),
	}
	for i, want := range expected {
		if lines[i] != want {
			t.Errorf("line %d: got %q, want %q", i, lines[i], want)
		}
	}
}

// TestHandleReadLineLoop exercises the canonical open/readLine/eof
// pattern.
func TestHandleReadLineLoop(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "lines.txt")
	if err := os.WriteFile(tmp, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%q, "read");
		while (not fs.eof($f)) {
			def line as string init fs.readLine($f);
			io.printf("[%%s]", $line);
		}
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "[one][two][three]"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestHandleWriteMode - open("write") + writeString round-trips.
func TestHandleWriteMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "out.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "write");
		fs.writeString($f, "line1\n");
		fs.writeString($f, "line2\n");
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, rerr := os.ReadFile(tmp)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "line1\nline2\n" {
		t.Errorf("got %q", string(got))
	}
}

// TestHandleSync - fs.sync flushes a write handle to disk without closing it,
// and the data is readable afterwards. The handle stays usable (sync, then more
// writes, then close).
func TestHandleSync(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "sync.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "write");
		fs.writeString($f, "durable\n");
		fs.sync($f);
		fs.writeString($f, "more\n");
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, rerr := os.ReadFile(tmp)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "durable\nmore\n" {
		t.Errorf("got %q", string(got))
	}
}

// TestHandleSyncRejectsReadMode - sync on a read handle errors (nothing to
// flush), pointing at the write/append modes.
func TestHandleSyncRejectsReadMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "r.txt")
	if werr := os.WriteFile(tmp, []byte("x"), 0o644); werr != nil {
		t.Fatal(werr)
	}
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "read");
		fs.sync($f);
	`, tmp))
	if err == nil {
		t.Fatal("expected fs.sync to reject a read-mode handle")
	}
	if !strings.Contains(err.Error(), "fs.sync") || !strings.Contains(err.Error(), "write") {
		t.Errorf("error should name fs.sync and the write mode: %v", err)
	}
}

// TestHandleSyncAfterCloseErrors - sync on a closed handle is a positioned
// error, not a crash (same use-after-close contract as the other verbs).
func TestHandleSyncAfterCloseErrors(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "c.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "write");
		fs.close($f);
		fs.sync($f);
	`, tmp))
	if err == nil {
		t.Fatal("expected fs.sync after close to error")
	}
	if !strings.Contains(err.Error(), "not open") {
		t.Errorf("error should say the handle is not open: %v", err)
	}
}

// TestHandleSyncAppendMode - sync is allowed on an append-mode handle (append
// writes are durable too), not just write mode.
func TestHandleSyncAppendMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "app.txt")
	if werr := os.WriteFile(tmp, []byte("base\n"), 0o644); werr != nil {
		t.Fatal(werr)
	}
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "append");
		fs.writeString($f, "added\n");
		fs.sync($f);
		fs.sync($f);
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v (append-mode sync + double-sync should be fine)", err)
	}
	got, rerr := os.ReadFile(tmp)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "base\nadded\n" {
		t.Errorf("got %q", string(got))
	}
}

// fs.sync from a spawned task, concurrent with writes from the main task on the
// same shared handle, must not race: syncFn takes the same per-handle mutex the
// write path does, and a sync that loses the race to a concurrent close returns
// a catchable error rather than crashing. Run under -race to verify.
func TestHandleSyncConcurrentIsRaceFree(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "sync-race.txt")
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		use task;
		def f as fs.File init fs.open(%q, "write");
		def t as task of int init spawn {
			def n as int init 0;
			while ($n < 100) {
				try { fs.sync($f); } catch (e) { break; }
				$n = $n + 1;
			}
			return $n;
		};
		def m as int init 0;
		while ($m < 100) {
			fs.writeString($f, "x");
			$m = $m + 1;
		}
		def n as int init task.wait($t);
		io.printf("%%d", $m + $n);
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "200" {
		t.Errorf("total = %s, want 200 (100 writes + 100 syncs)", out)
	}
}

// TestHandleUnknownMode - unknown mode string errors at the boundary
// with the supported set listed.
func TestHandleUnknownMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "x.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "rw");
	`, tmp))
	if err == nil {
		t.Fatal("expected mode boundary error")
	}
	if !strings.Contains(err.Error(), "read") || !strings.Contains(err.Error(), "write") || !strings.Contains(err.Error(), "append") {
		t.Errorf("error should list known modes: %v", err)
	}
}

// TestHandleReadOnWriteMode - reading from a write-mode handle errors
// with a message pointing at the corrective open call.
func TestHandleReadOnWriteMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "x.txt")
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "write");
		def s as string init fs.readLine($f);
	`, tmp))
	if err == nil {
		t.Fatal("expected read-on-write boundary error")
	}
	if !strings.Contains(err.Error(), `"write"`) || !strings.Contains(err.Error(), `"read"`) {
		t.Errorf("error should point at the mode mismatch: %v", err)
	}
}

// TestHandleUseAfterClose - operations on a closed handle error with
// a message that names the id.
func TestHandleUseAfterClose(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runProg(t, fmt.Sprintf(`
		use fs;
		def f as fs.File init fs.open(%q, "read");
		fs.close($f);
		def s as string init fs.readLine($f);
	`, tmp))
	if err == nil {
		t.Fatal("expected use-after-close error")
	}
	if !strings.Contains(err.Error(), "not open") {
		t.Errorf("error should mention 'not open': %v", err)
	}
}

// TestSpawnFsCompose confirms the composition story: a `spawn`
// body can read a file and return its content via task.wait.
func TestSpawnFsCompose(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "concurrent.txt")
	if err := os.WriteFile(tmp, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		use task;
		def t as task of string init spawn {
			return fs.readString(%q);
		};
		def s as string init task.wait($t);
		io.printf("%%s", $s);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "payload" {
		t.Errorf("got %q, want %q", out, "payload")
	}
}

// TestReadBytesPartialViaHandle exercises the two-arg form of the
// polymorphic `fs.readBytes` verb.
func TestReadBytesPartialViaHandle(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chunk.dat")
	if err := os.WriteFile(tmp, []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%q, "read");
		def first as bytes init fs.readBytes($f, 4);
		def second as bytes init fs.readBytes($f, 4);
		def third as bytes init fs.readBytes($f, 4);   # partial: only 2 bytes left
		fs.close($f);
		io.printf("%%d %%d %%d", len($first), len($second), len($third));
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "4 4 2" {
		t.Errorf("got %q, want %q", out, "4 4 2")
	}
}

// A file handle shared between a spawned task and the main task must not race:
// handleState's per-handle mutex guards sticky and the bufio.Reader, so
// concurrent readLine / eof calls are serialized rather than corrupting reader
// state (bufio.Reader is not goroutine-safe). Run under -race to verify.
func TestHandleConcurrentAccessIsRaceFree(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "lines.txt")
	var content strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&content, "line-%d\n", i)
	}
	if err := os.WriteFile(tmp, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		use task;
		def f as fs.File init fs.open(%q, "read");
		def t as task of int init spawn {
			def n as int init 0;
			while (not fs.eof($f)) {
				try { fs.readLine($f); $n = $n + 1; } catch (e) { break; }
			}
			return $n;
		};
		def m as int init 0;
		while (not fs.eof($f)) {
			try { fs.readLine($f); $m = $m + 1; } catch (e) { break; }
		}
		def n as int init task.wait($t);
		io.printf("%%d", $n + $m);
		fs.close($f);
	`, tmp))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Between them the two tasks must have read every line exactly once.
	if out != "500" {
		t.Errorf("total lines read = %s, want 500", out)
	}
}

// TestChmodTightensPermissions - fs.chmod changes a file's mode bits, the
// piece the fixed-0644 write verbs can't express (the primitive oauth's 0600
// token store leans on).
func TestChmodTightensPermissions(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "secret.json")
	src := fmt.Sprintf(`
		use fs;
		fs.writeString(%q, "token");
		fs.chmod(%q, 0o600);
	`, tmp, tmp)
	if _, err := runProg(t, src); err != nil {
		t.Fatalf("run: %v", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0600", got)
	}
}

// TestChmodRejectsBadMode - a mode outside [0, 0o7777] is a catchable error,
// not a silent no-op.
func TestChmodRejectsBadMode(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runProg(t, fmt.Sprintf(`use fs; fs.chmod(%q, 12288);`, tmp)) // 0o30000 > 0o7777
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
}

// TestChownNoopSucceeds - fs.chown with (-1, -1) changes neither owner nor
// group (chown(2) semantics), so it succeeds for any caller without privilege.
// Owner/group semantics are Unix/Linux; the platform target is Linux.
func TestChownNoopSucceeds(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runProg(t, fmt.Sprintf(`use fs; fs.chown(%q, -1, -1);`, tmp)); err != nil {
		t.Fatalf("chown(-1,-1) should be a no-op success, got %v", err)
	}
}

// TestChownRejectsBadArity - a wrong argument count is a catchable error.
func TestChownRejectsBadArity(t *testing.T) {
	_, err := runProg(t, `use fs; fs.chown("/tmp/x", 0);`)
	if err == nil || !strings.Contains(err.Error(), "3 arguments") {
		t.Fatalf("expected arity error, got %v", err)
	}
}
