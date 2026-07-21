# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# telegram_test.j - white-box tests for telegram.j. Run with:
#
#     jennifer test modules/telegram_test.j
#
# These exercise the pure form encoding and response parsing with no network;
# the live getMe / sendMessage / getUpdates over http are driven against a fake
# Bot API server in the Go suite (cmd/jennifer/telegram_test.go). telegram.j
# already `use`s json / strings / convert / lists, so the overlay only adds
# testing.
use testing;

func testFormEncode() {
    def p as map of string to string init {};
    $p["chat_id"] = "12345";
    $p["text"] = "hi there & <ok>";
    testing.assertEqual(formEncode($p), "chat_id=12345&text=hi+there+%26+%3Cok%3E");
}

func testUrlEncodeUnicode() {
    testing.assertEqual(urlEncode("a.b-c_d~e"), "a.b-c_d~e");
    testing.assertEqual(urlEncode("=/?"), "%3D%2F%3F");
}

func testCheckResponseThrowsOnError() {
    def node as json.Value init json.decode("{\"ok\":false,\"error_code\":401,\"description\":\"Unauthorized\"}");
    def threw as bool init false;
    try {
        checkResponse($node);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "telegram");
    }
    testing.assertTrue($threw);
}

func testCheckResponseOkPasses() {
    checkResponse(json.decode("{\"ok\":true,\"result\":{}}"));
    testing.assertTrue(true);
}

func testParseMessage() {
    def node as json.Value init json.decode("{\"result\":{\"message_id\":42,\"chat\":{\"id\":-1001,\"type\":\"group\"},\"date\":1700000000,\"text\":\"hello\"}}");
    def m as Message init parseMessage($node, "/result");
    testing.assertEqual($m.messageId, 42);
    testing.assertEqual($m.chatId, -1001);
    testing.assertEqual($m.text, "hello");
    testing.assertEqual($m.date, 1700000000);
}

func testParseUser() {
    def node as json.Value init json.decode("{\"result\":{\"id\":777,\"is_bot\":true,\"first_name\":\"Botty\",\"username\":\"botty_bot\"}}");
    def u as User init parseUser($node, "/result");
    testing.assertEqual($u.id, 777);
    testing.assertTrue($u.isBot);
    testing.assertEqual($u.firstName, "Botty");
    testing.assertEqual($u.username, "botty_bot");
}

func testParseUpdates() {
    def body as string init "{\"ok\":true,\"result\":[" +
        "{\"update_id\":100,\"message\":{\"message_id\":1,\"chat\":{\"id\":5},\"date\":1,\"text\":\"first\"}}," +
        "{\"update_id\":101,\"edited_message\":{\"message_id\":2}}]}";
    def us as list of Update init parseUpdates(json.decode($body));
    testing.assertEqual(len($us), 2);
    testing.assertEqual($us[0].updateId, 100);
    testing.assertTrue($us[0].hasMessage);
    testing.assertEqual($us[0].message.text, "first");
    testing.assertEqual($us[1].updateId, 101);
    testing.assertTrue(not $us[1].hasMessage);
}

func testParseUpdatesEmpty() {
    def us as list of Update init parseUpdates(json.decode("{\"ok\":true,\"result\":[]}"));
    testing.assertEqual(len($us), 0);
}
