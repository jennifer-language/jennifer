# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# slack_test.j - white-box tests for slack.j. Run with:
#
#     jennifer test modules/slack_test.j
#
# These exercise the pure Block Kit payload rendering with no network; the live
# webhook POST is driven against a fake HTTP server in the Go suite
# (cmd/jennifer/slack_test.go). slack.j already `use`s json / lists / strings,
# so the overlay only adds testing.
use testing;

func testRenderTextOnly() {
    def m as Message init text(message(), "deploy done");
    testing.assertEqual(render($m), "{\"text\":\"deploy done\"}");
}

func testRenderEmpty() {
    testing.assertEqual(render(message()), "{}");
}

func testRenderBlocks() {
    def m as Message init message();
    $m = header($m, "Deploy");
    $m = section($m, "*build* live");
    $m = divider($m);
    testing.assertEqual(render($m),
        "{\"blocks\":[" +
        "{\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":\"Deploy\"}}," +
        "{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"*build* live\"}}," +
        "{\"type\":\"divider\"}]}");
}

func testTextAndBlocks() {
    def m as Message init section(text(message(), "fallback"), "body");
    testing.assertEqual(render($m),
        "{\"text\":\"fallback\",\"blocks\":[{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"body\"}}]}");
}

func testSectionEscaping() {
    def m as Message init section(message(), "a \"quote\" & <tag>\nnl");
    testing.assertEqual(render($m),
        "{\"blocks\":[{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"a \\\"quote\\\" & <tag>\\nnl\"}}]}");
}
