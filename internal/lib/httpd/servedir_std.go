// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"errors"
	iofs "io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// answerWithDirFile answers a serveDir / serveDirEtag request with the file at
// full under root. Unless the server set httpd.Options.followSymlinks, the file
// is opened contained in root (containedFS): a symlink whose target lies outside
// root is not found.
func answerWithDirFile(rs *reqState, root, full string, etag bool) (Value, error) {
	if rs.srv != nil && rs.srv.followSymlinks {
		return answerWithFileEtag(rs, full, etag)
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return answerWithStatus(rs, http.StatusNotFound, "not found")
	}
	rs.mu.Lock()
	if rs.responded {
		err := answeredErr("", rs)
		rs.mu.Unlock()
		return interpreter.Null(), err
	}
	rs.responded = true
	rs.useServeFile = true
	rs.serveRoot = root
	rs.serveFile = filepath.ToSlash(rel)
	rs.serveEtag = etag
	rs.mu.Unlock()
	close(rs.done)
	return interpreter.Null(), nil
}

// containedFS opens files beneath one directory and never outside it. Every open
// goes through an os.Root, which resolves each path component itself and refuses
// one that leaves the root, so swapping a component for a symlink between a
// check and the open cannot escape (no TOCTOU). A link the os.Root refuses (an
// absolute target, or one through `..` above where it stands) is resolved once
// with EvalSymlinks: a target inside the root is opened again through the os.Root
// by its symlink-free path; one outside is not found.
type containedFS struct {
	root *os.Root
	real string // the root directory, absolute and symlink-free
}

// openContained opens dir (itself may be a symlink: the operator chose it) as a
// containedFS.
func openContained(dir string) (*containedFS, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(real)
	if err != nil {
		return nil, err
	}
	return &containedFS{root: r, real: real}, nil
}

func (c *containedFS) Close() error { return c.root.Close() }

// Open implements fs.FS for http.ServeFileFS. Names come from net/http (".",
// "sub", "./index.html", "sub/index.html").
func (c *containedFS) Open(name string) (iofs.File, error) {
	f, err := c.open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (c *containedFS) open(name string) (*os.File, error) {
	f, err := c.root.Open(name)
	if err == nil || errors.Is(err, iofs.ErrNotExist) || errors.Is(err, iofs.ErrPermission) {
		return f, err
	}
	real, rerr := filepath.EvalSymlinks(filepath.Join(c.real, filepath.FromSlash(name)))
	if rerr != nil {
		return nil, rerr
	}
	rel, rerr := filepath.Rel(c.real, real)
	if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrNotExist}
	}
	return c.root.Open(rel)
}

// etag is contentEtag for the contained file name.
func (c *containedFS) etag(name string) string {
	f, err := c.open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	return fileEtag(f, filepath.Join(c.real, filepath.FromSlash(name)))
}

// serve answers r with the contained file or directory name, as http.ServeFile
// would. A directory without an index.html (or whose index.html is not inside
// the root) is listed only with httpd.Options.dirListing, leaving out dot
// entries; otherwise it is not found.
func (c *containedFS) serve(w http.ResponseWriter, r *http.Request, name string, dirListing bool) {
	f, err := c.open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fi, err := f.Stat()
	_ = f.Close()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi.IsDir() {
		if dirListing {
			http.ServeFileFS(w, r, hideDotContained{c}, name)
			return
		}
		idx, err := c.open(path.Join(name, "index.html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_ = idx.Close()
	}
	http.ServeFileFS(w, r, c, name)
}

// hideDotContained is a containedFS whose directory listings leave out entries
// starting with `.`, as serveDir refuses to serve them.
type hideDotContained struct{ c *containedFS }

func (h hideDotContained) Open(name string) (iofs.File, error) {
	f, err := h.c.open(name)
	if err != nil {
		return nil, err
	}
	return hideDotFile{f}, nil
}
