#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The telegram module (modules/telegram.j): a Bot API client. With a bot token
 * (first argument or the TELEGRAM_TOKEN env var), it identifies the bot with
 * getMe and runs one getUpdates long-poll, echoing any text messages back to
 * their chat - the stateful receive-loop pattern (advance `offset` past each
 * processed update). Needs the default `jennifer` binary (net). Never commit a
 * token. Get one from @BotFather; message your bot, then run this.
 * Run: jennifer run examples/modules/telegram_demo.j [token]
 * @module telegram_demo
 */
use io;
use os;
import "../../modules/telegram.j" as telegram;

def token as string init os.getEnv("TELEGRAM_TOKEN");
if (len(os.ARGS) > 1) { $token = os.ARGS[1]; }

if (len($token) == 0) {
    io.printf("set TELEGRAM_TOKEN or pass a bot token to run the demo\n");
    exit 0;
}

def bot as telegram.Bot init telegram.bot($token);

def me as telegram.User init telegram.getMe($bot);
io.printf("bot: @%s (%s), id %d\n", $me.username, $me.firstName, $me.id);

# One long-poll cycle: fetch pending updates and echo each text message back.
io.printf("polling for updates (30s) ...\n");
def offset as int init 0;
def updates as list of telegram.Update init telegram.getUpdates($bot, $offset, 30);
io.printf("got %d update(s)\n", len($updates));
for (def u in $updates) {
    $offset = $u.updateId + 1;   # advance past this update
    if ($u.hasMessage and len($u.message.text) > 0) {
        io.printf("  chat %d: %s\n", $u.message.chatId, $u.message.text);
        telegram.sendMessage($bot, $u.message.chatId, "you said: " + $u.message.text);
    }
}
io.printf("next offset: %d\n", $offset);
