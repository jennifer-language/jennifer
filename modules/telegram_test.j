# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

func testRenderInlineKeyboard() {
    def rows as list of list of Button init [
        [urlButton("Open", "https://example.org"), inlineButton("Ping", "ping")],
        [inlineButton("Close", "close")]
    ];
    testing.assertEqual(
        renderInlineKeyboard($rows),
        "{\"inline_keyboard\":[[{\"text\":\"Open\",\"url\":\"https://example.org\"}," +
            "{\"text\":\"Ping\",\"callback_data\":\"ping\"}]," +
            "[{\"text\":\"Close\",\"callback_data\":\"close\"}]]}");
}

func testRenderInlineKeyboardEmpty() {
    def rows as list of list of Button init [];
    testing.assertEqual(renderInlineKeyboard($rows), "{\"inline_keyboard\":[]}");
}

func testParseCallbackQuery() {
    def body as string init "{\"update_id\":200,\"callback_query\":{" +
        "\"id\":\"cbq-99\"," +
        "\"from\":{\"id\":42,\"is_bot\":false,\"first_name\":\"Ada\",\"username\":\"ada\"}," +
        "\"message\":{\"message_id\":7,\"chat\":{\"id\":5},\"date\":1,\"text\":\"pick\"}," +
        "\"data\":\"ping\"}}";
    def cq as CallbackQuery init parseCallbackQuery(json.decode($body));
    testing.assertEqual($cq.id, "cbq-99");
    testing.assertEqual($cq.data, "ping");
    testing.assertEqual($cq.from.id, 42);
    testing.assertEqual($cq.from.username, "ada");
    testing.assertEqual($cq.messageId, 7);
}

func testParseCallbackQueryAbsent() {
    def cq as CallbackQuery init parseCallbackQuery(json.decode("{\"update_id\":201}"));
    testing.assertEqual($cq.id, "");
    testing.assertEqual($cq.data, "");
    testing.assertEqual($cq.messageId, 0);
}

func testBuildUpload() {
    def data as bytes init convert.bytesFromString("PNGDATA", "utf-8");
    def form as multipart.Built init buildUpload("photo", -1001, "pic.png", "image/png", $data);
    testing.assertTrue(strings.contains($form.contentType, "multipart/form-data; boundary="));
    # Round-trip the built body back through multipart.parse to assert its shape.
    def parts as list of multipart.Part init multipart.parse($form.contentType, $form.body);
    testing.assertEqual(len($parts), 2);
    testing.assertEqual($parts[0].name, "chat_id");
    testing.assertEqual(multipart.text($parts[0]), "-1001");
    testing.assertEqual($parts[1].name, "photo");
    testing.assertEqual($parts[1].filename, "pic.png");
    testing.assertEqual($parts[1].contentType, "image/png");
    testing.assertEqual(multipart.text($parts[1]), "PNGDATA");
}

func testRedactToken() {
    def token as string init "123456:ABC-secretpart";
    def msg as string init "http: POST https://api.telegram.org/bot" + $token +
        "/sendMessage read timed out";
    def red as string init redactToken($msg, $token);
    testing.assertTrue(not strings.contains($red, $token));
    testing.assertTrue(not strings.contains($red, "secretpart"));
    testing.assertTrue(strings.contains($red, "bot<redacted>"));
}
