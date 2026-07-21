// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
)

// runServe implements `jennifer serve <file.j> [--watch] [-I dir] [--sysmoddir dir]`:
// the Hugo-style convenience for running a `web` (or any `httpd`) app. Without
// --watch it runs the app in-process, the same as `jennifer run`. With --watch
// it runs the app in a child process and restarts it whenever the entry file
// changes - a fast edit / reload loop for development.
func runServe(args []string) int {
	file, watch, passthrough, err := parseServeArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer serve: %v\n", err)
		fmt.Fprintln(os.Stderr, "usage: jennifer serve <file.j> [--watch] [-I dir] [--sysmoddir dir]")
		return 2
	}

	if !watch {
		sm, serr := setupSysmoddir(passthrough.sysmoddir)
		if serr != nil {
			fmt.Fprintf(os.Stderr, "jennifer serve: %v\n", serr)
			return 2
		}
		searchDirs := append([]string{sm.Dir}, passthrough.includes...)
		oslib.SetUserArgs([]string{file})
		// Print the banner only after a clean parse, so it never precedes a
		// syntax error.
		return runFileHook(file, searchDirs, "", func() {
			fmt.Fprintf(os.Stderr, "jennifer serve: running %s (pass --watch to reload on change)\n", file)
		})
	}
	return watchAndServe(file, passthrough)
}

// serveFlags holds the interpreter flags a server run forwards to a child.
type serveFlags struct {
	sysmoddir string
	includes  []string
}

// parseServeArgs pulls the entry file, the --watch toggle, and the passthrough
// interpreter flags out of `server`'s arguments.
func parseServeArgs(args []string) (string, bool, serveFlags, error) {
	var file string
	var watch bool
	var f serveFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--watch":
			watch = true
		case a == "--sysmoddir":
			if i+1 >= len(args) {
				return "", false, f, fmt.Errorf("--sysmoddir needs a directory")
			}
			i++
			f.sysmoddir = args[i]
		case strings.HasPrefix(a, "--sysmoddir="):
			f.sysmoddir = strings.TrimPrefix(a, "--sysmoddir=")
		case a == "-I":
			if i+1 >= len(args) {
				return "", false, f, fmt.Errorf("-I needs a directory")
			}
			i++
			f.includes = append(f.includes, args[i])
		case strings.HasPrefix(a, "-I"):
			f.includes = append(f.includes, strings.TrimPrefix(a, "-I"))
		case strings.HasPrefix(a, "-"):
			return "", false, f, fmt.Errorf("unknown flag %q", a)
		default:
			if file != "" {
				return "", false, f, fmt.Errorf("expected a single source file, got %q and %q", file, a)
			}
			file = a
		}
	}
	if file == "" {
		return "", false, f, fmt.Errorf("no source file given")
	}
	if !strings.HasSuffix(file, ".j") {
		return "", false, f, fmt.Errorf("source file %q must end in .j", file)
	}
	return file, watch, f, nil
}

// childRunArgs builds the `run` argument list for a reloaded child process.
func childRunArgs(file string, f serveFlags) []string {
	out := []string{"run"}
	if f.sysmoddir != "" {
		out = append(out, "--sysmoddir", f.sysmoddir)
	}
	for _, inc := range f.includes {
		out = append(out, "-I", inc)
	}
	return append(out, file)
}

// fileModTime returns the entry file's modification time in nanoseconds, or 0
// if it cannot be stat'd (e.g. mid-save).
func fileModTime(file string) int64 {
	info, err := os.Stat(file)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

// watchAndServe runs the app in a child process and restarts it on any change
// to the entry file. Ctrl-C stops the loop. Only the entry file is watched;
// changes to `include`d files or imported modules do not trigger a reload yet.
func watchAndServe(file string, f serveFlags) int {
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for {
		cmd := exec.Command(self, childRunArgs(file, f)...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		fmt.Fprintf(os.Stderr, "jennifer serve: starting %s (watching for changes, Ctrl-C to stop)\n", file)
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "jennifer serve: cannot start: %v\n", err)
			return 1
		}
		lastMod := fileModTime(file)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()

		alive := true
		restart := false
		// doneCh is nil-ed once the child exits so the (now always-ready) closed
		// channel stops starving the mtime-poll timer in the select below.
		doneCh := done
	poll:
		for {
			select {
			case <-sigCh:
				if alive {
					_ = cmd.Process.Kill()
					<-done
				}
				fmt.Fprintln(os.Stderr, "\njennifer serve: stopped")
				return 0
			case <-doneCh:
				alive = false
				doneCh = nil
				fmt.Fprintln(os.Stderr, "jennifer serve: app exited; waiting for a change to reload...")
			case <-time.After(400 * time.Millisecond):
				if m := fileModTime(file); m != 0 && m != lastMod {
					fmt.Fprintln(os.Stderr, "jennifer serve: change detected, reloading")
					if alive {
						_ = cmd.Process.Kill()
						<-done
					}
					restart = true
					break poll
				}
			}
		}
		if !restart {
			return 0
		}
	}
}
