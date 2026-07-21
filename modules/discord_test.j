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
    testing.assertEqual(render($m),
        "{\"embeds\":[{\"title\":\"Deploy\",\"description\":\"build 1234 live\",\"color\":3066993}]}");
}

func testContentAndEmbed() {
    def m as Message init embed(content(message(), "heads up"), "T", "D", 255);
    testing.assertEqual(render($m),
        "{\"content\":\"heads up\",\"embeds\":[{\"title\":\"T\",\"description\":\"D\",\"color\":255}]}");
}

func testEmbedEscaping() {
    def m as Message init embed(message(), "a \"q\"", "line\nnl", 0);
    testing.assertEqual(render($m),
        "{\"embeds\":[{\"title\":\"a \\\"q\\\"\",\"description\":\"line\\nnl\",\"color\":0}]}");
}

func testMultipleEmbeds() {
    def m as Message init embed(embed(message(), "A", "a", 1), "B", "b", 2);
    testing.assertEqual(render($m),
        "{\"embeds\":[{\"title\":\"A\",\"description\":\"a\",\"color\":1},{\"title\":\"B\",\"description\":\"b\",\"color\":2}]}");
}
