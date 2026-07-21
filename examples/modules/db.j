# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Depends on config, reaching config's surface with the same `config.member` syntax an importer uses.
 * Each file states its own imports (`use` is not transitive); config is fully initialised before db loads, because imports initialise depth-first, post-order.
 * @module db
 */
import "./config.j" as config;

/**
 * Build a status line, calling into the config module (a module-to-module call).
 * @return {string} a "db up, ..." status summary
 */
export func status() {
    return "db up, " + config.describe();
}
