#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Leveled, structured logging in the three record formats (text / logfmt / json),
 * plus level filtering. A file sink and an RFC 5424 syslog sink are shown in
 * comments (syslog needs the default `jennifer` binary).
 * @module log_demo
 */
use io;
import "../../modules/log.j" as log;

def fields as map of string to string init {"user": "ada", "request": "GET /", "ms": "12"};

io.printf("=== text ===\n");
def t as log.Logger init log.new("debug", "text");
log.debug($t, "cache miss", $fields);
log.info($t, "request handled", $fields);
log.warn($t, "slow response", $fields);
log.error($t, "upstream failed", $fields);

io.printf("\n=== logfmt (level = info, so debug is dropped) ===\n");
def l as log.Logger init log.new("info", "logfmt");
def none as map of string to string init {};
def portField as map of string to string init {"port": "8080"};
log.debug($l, "this line is below the info level and never appears", $none);
log.info($l, "server started", $portField);

io.printf("\n=== json ===\n");
def j as log.Logger init log.new("info", "json");
def payment as map of string to string init {"order": "4711", "reason": "insufficient funds"};
log.error($j, "payment declined", $payment);

# Other sinks (not run here):
#   def f as log.Logger init log.toFile("info", "json", "/var/log/app.log");   # append to a file
#   def s as log.Logger init log.toSyslog("info", "localhost:514", "myapp");   # RFC 5424 syslog over UDP (default binary)
#   def e as log.Logger init log.toStderr("warn", "logfmt");                    # warnings + errors to stderr
