// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package kv

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"jennifer-lang.dev/jennifer/internal/dataerr"
)

// fileVersion identifies the content of a store file: whether it exists and the
// SHA-256 of its bytes (the rule `flatdb.save` uses). The zero value is "no
// file".
type fileVersion struct {
	exists bool
	sum    [sha256.Size]byte
}

func versionOf(exists bool, data []byte) fileVersion {
	if !exists {
		return fileVersion{}
	}
	return fileVersion{exists: true, sum: sha256.Sum256(data)}
}

// currentVersion reads the version of the file at path now.
func currentVersion(path string) (fileVersion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileVersion{}, nil
		}
		return fileVersion{}, err
	}
	return versionOf(true, data), nil
}

// Write-lock timing: the lock is held only for a read, a hash and a rename, so a
// holder older than lockStale crashed there; a waiter outlasts that.
var (
	lockWait  = 15 * time.Second
	lockStale = 10 * time.Second
	lockPoll  = 5 * time.Millisecond
)

var lockSeq atomic.Int64

// commitLocked renames the staged temp file over s.path if the file is still the
// version this store last read or wrote, under the store's write lock
// (`<path>.lock`), so no other writer can replace it between the check and the
// rename. A changed, created or deleted file is refused with kind "kv" and left
// as it is. The caller holds s.mu and removes tmp on error.
func (s *store) commitLocked(tmp string) error {
	release, err := acquireLock(s.path + ".lock")
	if err != nil {
		return err
	}
	defer release()
	now, err := currentVersion(s.path)
	if err != nil {
		return err
	}
	if now != s.onDisk {
		what := "changed"
		switch {
		case !now.exists:
			what = "was deleted"
		case !s.onDisk.exists:
			what = "was created"
		}
		return dataerr.New("kv", "%s %s since this store last read or wrote it (another writer); not written, close every handle and reopen to continue from its content", s.path, what)
	}
	return os.Rename(tmp, s.path)
}

// acquireLock creates the lock file exclusively, holding "<id> <unix seconds>",
// and returns a release that removes it while it still holds that id. A lock
// older than lockStale is broken; one held past lockWait is a "timeout" error.
func acquireLock(lp string) (func(), error) {
	id := fmt.Sprintf("%d.%d.%d %d", os.Getpid(), time.Now().UnixNano(), lockSeq.Add(1), time.Now().Unix())
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := f.WriteString(id)
			cerr := f.Close()
			if werr == nil {
				werr = cerr
			}
			if werr != nil {
				_ = os.Remove(lp)
				return nil, werr
			}
			return func() {
				if b, rerr := os.ReadFile(lp); rerr == nil && string(b) == id {
					_ = os.Remove(lp)
				}
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		breakStale(lp)
		if time.Now().After(deadline) {
			return nil, dataerr.New("timeout", "%s: the write lock is still held after %s", lp, lockWait)
		}
		time.Sleep(lockPoll)
	}
}

// breakStale removes the lock at lp if its holder is older than lockStale. The
// lock is first renamed to a unique scratch name, so two breakers never both act
// on one file; if the moved lock turns out not to be the stale one observed (a
// new holder took it meanwhile), it is put back with an exclusive create.
func breakStale(lp string) {
	holder, err := os.ReadFile(lp)
	if err != nil || !lockIsStale(lp, string(holder)) {
		return
	}
	scratch := fmt.Sprintf("%s.stale.%d.%d", lp, os.Getpid(), lockSeq.Add(1))
	if os.Rename(lp, scratch) != nil {
		return
	}
	moved, _ := os.ReadFile(scratch)
	if string(moved) != string(holder) {
		if f, ferr := os.OpenFile(lp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); ferr == nil {
			_, _ = f.Write(moved)
			_ = f.Close()
		}
	}
	_ = os.Remove(scratch)
}

// lockIsStale reports whether the holder's stamp is older than lockStale. A
// holder without a readable stamp (a writer that crashed between creating the
// lock and writing its id) ages by the file's mtime.
func lockIsStale(lp, holder string) bool {
	var stamp time.Time
	if sp := strings.LastIndexByte(holder, ' '); sp >= 0 {
		if n, err := strconv.ParseInt(holder[sp+1:], 10, 64); err == nil {
			stamp = time.Unix(n, 0)
		}
	}
	if stamp.IsZero() {
		fi, err := os.Stat(lp)
		if err != nil {
			return false
		}
		stamp = fi.ModTime()
	}
	return time.Since(stamp) > lockStale
}
