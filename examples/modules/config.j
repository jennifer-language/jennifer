# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# config.j - a leaf module: declarations only (def const / def struct /
# func), no mutable module state. Its constants and functions become the
# module's surface, reached from an importer as `config.NAME` / `config.fn()`.
use convert;

def const MAXCONN as int init 16;
def const NAME as string init "jennifer-db";

# describe formats the configuration as a human-readable line.
func describe() {
    return NAME + " (max " + convert.toString(MAXCONN) + " connections)";
}
