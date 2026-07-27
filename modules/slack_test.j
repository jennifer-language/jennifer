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
    testing.assertEqual(
        render($m),
        "{\"blocks\":[" +
            "{\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":\"Deploy\"}}," +
            "{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"*build* live\"}}," +
            "{\"type\":\"divider\"}]}");
}

func testTextAndBlocks() {
    def m as Message init section(text(message(), "fallback"), "body");
    testing.assertEqual(
        render($m),
        "{\"text\":\"fallback\",\"blocks\":[{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"body\"}}]}");
}

func testContextBlock() {
    def m as Message init contextBlock(message(), "posted by ci");
    testing.assertEqual(
        render($m),
        "{\"blocks\":[{\"type\":\"context\",\"elements\":[" +
            "{\"type\":\"mrkdwn\",\"text\":\"posted by ci\"}]}]}");
}

func testFieldsSection() {
    def fields as list of string init ["*Env:*\nprod", "*Build:*\n1234"];
    def m as Message init fieldsSection(message(), $fields);
    testing.assertEqual(
        render($m),
        "{\"blocks\":[{\"type\":\"section\",\"fields\":[" +
            "{\"type\":\"mrkdwn\",\"text\":\"*Env:*\\nprod\"}," +
            "{\"type\":\"mrkdwn\",\"text\":\"*Build:*\\n1234\"}]}]}");
}

func testActionsBlock() {
    def buttons as list of string init [
        button("View build", "https://example.com/build"),
        button("Logs", "https://example.com/logs")
    ];
    def m as Message init actionsBlock(message(), $buttons);
    testing.assertEqual(
        render($m),
        "{\"blocks\":[{\"type\":\"actions\",\"elements\":[" +
            "{\"type\":\"button\",\"text\":{\"type\":\"plain_text\",\"text\":\"View build\"},\"url\":\"https://example.com/build\"}," +
            "{\"type\":\"button\",\"text\":{\"type\":\"plain_text\",\"text\":\"Logs\"},\"url\":\"https://example.com/logs\"}]}]}");
}

func testSectionEscaping() {
    def m as Message init section(message(), "a \"quote\" & <tag>\nnl");
    # json.encode is HTML-safe: `&` `<` `>` become & < > (still
    # valid JSON, decodes identically), so the Slack payload stays intact.
    testing.assertEqual(
        render($m),
        "{\"blocks\":[{\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"a \\\"quote\\\" \\u0026 \\u003ctag\\u003e\\nnl\"}}]}");
}
