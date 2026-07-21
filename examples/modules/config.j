# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A leaf module: declarations only (def const / def struct / func), no mutable module state.
 * Its constants and functions become the module's surface, reached from an importer as `config.NAME` / `config.fn()`.
 * @module config
 */
use convert;

export def const MAXCONN as int init 16;
def const NAME as string init "jennifer-db";

# NAME stays private (unmarked) - only the `export`-ed names cross the
# module boundary.
/**
 * Format the configuration as a human-readable line.
 * @return {string} a "NAME (max N connections)" summary
 */
export func describe() {
    return NAME + " (max " + convert.toString(MAXCONN) + " connections)";
}
