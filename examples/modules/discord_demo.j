#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The discord module (modules/discord.j): post to a Discord channel Webhook.
 * Renders a plain message and an embed message and, if a webhook URL is given
 * (as the first argument or the DISCORD_WEBHOOK env var), actually posts them.
 * Needs the default `jennifer` binary (net). Never commit a real webhook URL.
 * Run: jennifer run examples/modules/discord_demo.j [webhookUrl]
 * @module discord_demo
 */
use io;
use os;
import "../../modules/http.j" as http;
import "../../modules/discord.j" as discord;

def url as string init os.getEnv("DISCORD_WEBHOOK");
if (len(os.ARGS) > 1) { $url = os.ARGS[1]; }

# Build a message with content plus two coloured embeds.
def m as discord.Message init discord.content(discord.message(), "Deploy finished");
$m = discord.embed($m, "Deploy", "build 1234 is live in production", 3066993);   # green
$m = discord.embed($m, "Checks", "all checks passed", 3447003);                  # blue

io.printf("plain payload: %s\n", discord.render(discord.content(discord.message(), "hello")));
io.printf("embed payload: %s\n", discord.render($m));

if (len($url) == 0) {
    io.printf("\n(set DISCORD_WEBHOOK or pass a URL to actually post)\n");
} else {
    io.printf("\nposting to Discord ...\n");
    def r as http.Response init discord.sendMessage($url, $m);
    io.printf("status %d\n", $r.status);
}
