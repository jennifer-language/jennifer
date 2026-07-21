#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Parse a `.env` configuration string into a map, showing comments, `export`,
 * quoting, and inline comments. `dotenv.read(path)` does the same from a file;
 * `dotenv.load(path)` also sets each variable in the environment.
 * @module dotenv_demo
 */
use io;
import "../../modules/dotenv.j" as dotenv;

def sample as string init "# service config\n" +
    "PORT=8080\n" +
    "export NAME=\"ada lovelace\"\n" +
    "GREETING='hi # not a comment'\n" +
    "DEBUG=true            # inline comment\n";

def cfg as map of string to string init dotenv.parse($sample);
io.printf("PORT     = %s\n", $cfg["PORT"]);
io.printf("NAME     = %s\n", $cfg["NAME"]);
io.printf("GREETING = %s\n", $cfg["GREETING"]);
io.printf("DEBUG    = %s\n", $cfg["DEBUG"]);

# From a file instead of a string:
#   def cfg as map of string to string init dotenv.read(".env");   # parse only
#   dotenv.load(".env");                                           # parse + os.setEnv each
