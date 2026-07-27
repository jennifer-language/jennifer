# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# discord_test.j - white-box tests for discord.j. Run with:
#
#     jennifer test modules/discord_test.j
#
# These exercise the pure embed payload rendering with no network; the live
# webhook POST is driven against a fake HTTP server in the Go suite
# (cmd/jennifer/discord_test.go). discord.j already `use`s json / lists /
# strings / convert, so the overlay only adds testing.
use testing;

func testRenderContentOnly() {
    def m as Message init content(message(), "deploy done");
    testing.assertEqual(render($m), "{\"content\":\"deploy done\"}");
}

func testRenderEmpty() {
    testing.assertEqual(render(message()), "{}");
}

func testEmbed() {
    def m as Message init embed(message(), "Deploy", "build 1234 live", 3066993);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"Deploy\",\"description\":\"build 1234 live\",\"color\":3066993}]}");
}

func testContentAndEmbed() {
    def m as Message init embed(content(message(), "heads up"), "T", "D", 255);
    testing.assertEqual(
        render($m),
        "{\"content\":\"heads up\",\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":255}]}");
}

func testEmbedEscaping() {
    def m as Message init embed(message(), "a \"q\"", "line\nnl", 0);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"a \\\"q\\\"\",\"description\":\"line\\nnl\",\"color\":0}]}");
}

func testMultipleEmbeds() {
    def m as Message init embed(embed(message(), "A", "a", 1), "B", "b", 2);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"A\",\"description\":\"a\",\"color\":1},{\"title\":\"B\",\"description\":\"b\",\"color\":2}]}");
}

func testEmbedField() {
    def m as Message init embedField(embed(message(), "T", "D", 1), "CPU", "80%", true);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1,\"fields\":[{\"name\":\"CPU\",\"value\":\"80%\",\"inline\":true}]}]}");
}

func testEmbedFieldMultiple() {
    def m as Message init embedField(
        embedField(embed(message(), "T", "D", 1), "a", "1", true),
        "b",
        "2",
        false);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1,\"fields\":[{\"name\":\"a\",\"value\":\"1\",\"inline\":true},{\"name\":\"b\",\"value\":\"2\",\"inline\":false}]}]}");
}

func testEmbedFooter() {
    def m as Message init embedFooter(embed(message(), "T", "D", 1), "ci bot");
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1,\"footer\":{\"text\":\"ci bot\"}}]}");
}

func testEmbedAuthor() {
    def m as Message init embedAuthor(embed(message(), "T", "D", 1), "deploybot");
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1,\"author\":{\"name\":\"deploybot\"}}]}");
}

func testEmbedFieldFooterAuthorOrder() {
    def m as Message init embed(message(), "T", "D", 1);
    $m = embedField($m, "k", "v", false);
    $m = embedFooter($m, "foot");
    $m = embedAuthor($m, "auth");
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1,\"fields\":[{\"name\":\"k\",\"value\":\"v\",\"inline\":false}],\"footer\":{\"text\":\"foot\"},\"author\":{\"name\":\"auth\"}}]}");
}

func testUsernameAvatar() {
    def m as Message init avatar(username(content(message(), "hi"), "bot"), "https://x/y.png");
    testing.assertEqual(
        render($m),
        "{\"username\":\"bot\",\"avatar_url\":\"https://x/y.png\",\"content\":\"hi\"}");
}

func testUsernameWithEmbed() {
    def m as Message init username(embed(message(), "T", "D", 1), "releaser");
    testing.assertEqual(
        render($m),
        "{\"username\":\"releaser\",\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":1}]}");
}

func testEmbedFieldEscaping() {
    def m as Message init embedField(embed(message(), "T", "D", 0), "a \"q\"", "line\nnl", true);
    testing.assertEqual(
        render($m),
        "{\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":0,\"fields\":[{\"name\":\"a \\\"q\\\"\",\"value\":\"line\\nnl\",\"inline\":true}]}]}");
}

# fieldNoEmbed drives embedField on a message with no embed, which must throw.
func fieldNoEmbed() {
    def m as Message init embedField(message(), "k", "v", true);
}

func testEmbedFieldNoEmbedThrows() {
    testing.assertThrows("fieldNoEmbed", "discord");
}
