# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `yaml` library: decode a document (with an anchor and a `<<`
 * merge), read it by JSON pointer, build one from scratch with the non-mutating
 * write surface, and encode it in both flow and block styles. Every output is
 * deterministic, so it doubles as a golden test.
 * @module yaml
 */

use io;
use yaml;

# A small config with an anchor reused via a merge key.
def src as string init "defaults: &def\n"
    + "  retries: 3\n"
    + "  timeout: 30\n"
    + "service:\n"
    + "  <<: *def\n"
    + "  timeout: 60\n"
    + "  hosts:\n"
    + "    - a.example\n"
    + "    - b.example\n";

def doc as yaml.Value init yaml.decode($src);
io.printf("retries: %d\n", yaml.asInt(yaml.get($doc, "/service/retries")));
io.printf("timeout: %d\n", yaml.asInt(yaml.get($doc, "/service/timeout")));
io.printf("hosts: %d\n", yaml.length(yaml.get($doc, "/service/hosts")));
io.printf("host[0]: %s\n", yaml.asString(yaml.get($doc, "/service/hosts/0")));
io.printf("has hosts: %t / has proxy: %t\n",
    yaml.has($doc, "/service/hosts"), yaml.has($doc, "/service/proxy"));

# Build a document with the non-mutating write surface.
def cfg as yaml.Value init yaml.set(yaml.map(), "/name", "demo");
def withPorts as yaml.Value init yaml.set($cfg, "/ports", yaml.list());
def withOne as yaml.Value init yaml.append($withPorts, "/ports", 8080);
def built as yaml.Value init yaml.append($withOne, "/ports", 9090);

# Flow style is compact; block style is the readable form.
io.printf("flow: %s", yaml.encode($built));
io.printf("block:\n%s", yaml.encodePretty($built));
