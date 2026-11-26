// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTinyLocalHalves checks the local halves of the network modules (M29.17)
// on jennifer-tiny-dev (JENNIFER_TINY_DEV_BIN): kvstore's local stores,
// session and ratelimit over them, s3presign and webhook load and pass their
// overlays, and a program using them runs. The network halves (memcache,
// redis, s3) still refuse at load with the capability error.
func TestTinyLocalHalves(t *testing.T) {
	bin := tinyBinFromEnv(t, "JENNIFER_TINY_DEV_BIN")
	root, err := filepath.Abs(filepath.Join("..", "..", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "JENNIFER_SYSMODDIR="+root)

	for _, name := range []string{"kvstore", "session", "ratelimit", "s3presign", "webhook"} {
		cmd := exec.Command(bin, "test", filepath.Join(root, name+"_test.j"))
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("jennifer-tiny-dev test modules/%s_test.j: %v\n%s", name, err, out)
		}
	}

	dir := t.TempDir()
	prog := filepath.Join(dir, "local.j")
	if err := os.WriteFile(prog, []byte(`use io;
use json;
import "kvstore.j" as kvstore;
import "session.j" as session;
import "ratelimit.j" as ratelimit;
import "s3presign.j" as s3;
import "webhook.j" as webhook;
def st as kvstore.Store init kvstore.inProcessStore();
def id as string init session.create($st, 60);
session.save($st, $id, json.set(json.map(), "/user", "ada"), 60);
def lim as ratelimit.Limiter init ratelimit.fixedWindow($st, 1, 3600);
def a as ratelimit.Result init ratelimit.check($lim, "ip");
def b as ratelimit.Result init ratelimit.check($lim, "ip");
def c as s3.Client init s3.connect("https://s3.example.com", "us-east-1", "AK", "SK");
def u as string init s3.presign($c, "GET", "b", "k", 60);
def sig as string init webhook.sign("body", "secret");
io.printf("%s %t %t %t %t\n", json.asString(session.load($st, $id), "/user"), $a.allowed, $b.allowed,
    len($u) > 0, webhook.verify("body", $sig, "secret"));
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", prog)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jennifer-tiny-dev run local.j: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "ada true false true true"; got != want {
		t.Errorf("local.j printed %q, want %q", got, want)
	}

	for _, mod := range []string{"memcache", "redis", "s3"} {
		p := filepath.Join(dir, "net_"+mod+".j")
		if err := os.WriteFile(p, []byte(`import "`+mod+`.j" as m;`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "run", p)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("jennifer-tiny-dev loaded %s.j; want the capability refusal:\n%s", mod, out)
			continue
		}
		if !strings.Contains(string(out), `needs capability "net", unavailable in this build`) {
			t.Errorf("import %s.j on jennifer-tiny-dev: want the capability error, got:\n%s", mod, out)
		}
	}
}
