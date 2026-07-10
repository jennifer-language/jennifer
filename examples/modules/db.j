# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# db.j - depends on config. It reaches config's surface with the same
# `config.member` syntax an importer uses (each file states its own imports;
# `use` is not transitive). config is fully initialised before db loads,
# because imports initialise depth-first, post-order.
import "./config.j" as config;

# status builds a line from config's own describe(), a module-to-module call.
func status() {
    return "db up, " + config.describe();
}
