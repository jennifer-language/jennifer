# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# main.j - the entry program. It imports db (which imports config) and also
# imports config directly, then reaches each module's surface with the
# `alias.member` syntax. Run it with:
#
#     jennifer run examples/modules/main.j
#
# Expected output:
#
#     db up, jennifer-db (max 16 connections)
#     config: jennifer-db (max 16 connections)
#     max connections: 16
#
# `db.status()` and `config.describe()` are qualified calls into the loaded
# modules; `config.MAXCONN` is a qualified constant. config is loaded once
# even though both main and db import it (run-once cache), so both see the
# same values.
use io;
import "./db.j" as db;
import "./config.j" as config;

io.printf("%s\n", db.status());
io.printf("config: %s\n", config.describe());
io.printf("max connections: %d\n", config.MAXCONN);
