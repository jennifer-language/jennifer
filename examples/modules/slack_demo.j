#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The slack module (modules/slack.j): post to a Slack Incoming Webhook. Renders
 * a plain message and a Block Kit message and, if a webhook URL is given (as the
 * first argument or the SLACK_WEBHOOK env var), actually posts them. Needs the
 * default `jennifer` binary (net). Never commit a real webhook URL.
 * Run: jennifer run examples/modules/slack_demo.j [webhookUrl]
 * @module slack_demo
 */
use io;
use os;
import "../../modules/http.j" as http;
import "../../modules/slack.j" as slack;

def url as string init os.getEnv("SLACK_WEBHOOK");
if (len(os.ARGS) > 1) { $url = os.ARGS[1]; }

# Build a Block Kit message.
def m as slack.Message init slack.message();
$m = slack.text($m, "Deploy finished");
$m = slack.header($m, "Deploy");
$m = slack.section($m, "*build 1234* is live in _production_");
$m = slack.divider($m);
$m = slack.section($m, "all checks passed :white_check_mark:");

io.printf("plain payload:  %s\n", slack.render(slack.text(slack.message(), "hello")));
io.printf("blocks payload: %s\n", slack.render($m));

if (len($url) == 0) {
    io.printf("\n(set SLACK_WEBHOOK or pass a URL to actually post)\n");
} else {
    io.printf("\nposting to Slack ...\n");
    def r as http.Response init slack.sendMessage($url, $m);
    io.printf("status %d: %s\n", $r.status, $r.body);
}
