#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Parse a `.env` configuration string into a map, showing comments, `export`,
 * quoting, inline comments, `${VAR}` interpolation, and multi-line double-quoted
 * values. From files instead, `dotenv.read(path)` parses one file and the cascade
 * loaders (`readCascade` / `resolve` / `loadCascade` / `autoload`) merge the
 * layered `.env` / `.env.local` / `.env.<profile>` files with a real OS env var
 * always winning.
 * @module dotenv_demo
 */
use io;
import "../../modules/dotenv.j" as dotenv;

def sample as string init "# service config\n" +
    "PORT=8080\n" +
    "export NAME=\"ada lovelace\"\n" +
    "GREETING='hi # not a comment'\n" +
    "DEBUG=true            # inline comment\n" +
    "URL=\"http://localhost:$\{PORT\}/api\"\n" + # ${PORT} interpolates the earlier key
"BANNER=\"line one\nline two\"\n"; # a multi-line double-quoted value

def cfg as map of string to string init dotenv.parse($sample);
io.printf("PORT     = %s\n", $cfg["PORT"]);
io.printf("NAME     = %s\n", $cfg["NAME"]);
io.printf("GREETING = %s\n", $cfg["GREETING"]);
io.printf("DEBUG    = %s\n", $cfg["DEBUG"]);
io.printf("URL      = %s\n", $cfg["URL"]);
io.printf("BANNER   = %s\n", $cfg["BANNER"]);

# Layered load from a directory (later file wins; a real env var beats a file):
#   dotenv.autoload(os.cwd());                          # profile from JENNIFER_ENV
#   def eff as map of string to string init dotenv.resolve(os.cwd(), "production");
# Single file:
#   def cfg as map of string to string init dotenv.read(".env");   # parse only
#   dotenv.load(".env");                                           # parse + os.setEnv each
