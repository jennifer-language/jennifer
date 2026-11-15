# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use net;
use task;

func testFormEncode() {
    def p as map of string to string init {};
    $p["chat_id"] = "12345";
    $p["text"] = "hi there & <ok>";
    testing.assertEqual(formEncode($p), "chat_id=12345&text=hi+there+%26+%3Cok%3E");
}

func testCheckResponseThrowsOnError() {
    def node as json.Value init json.decode('{"ok":false,"error_code":401,"description":"Unauthorized"}');
    def threw as bool init false;
    try {
        checkResponse($node, 401);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "telegram");
        testing.assertEqual($e.errorCode, 401);
        testing.assertEqual($e.status, 401);
        testing.assertEqual($e.message, "telegram: Unauthorized");
    }
    testing.assertTrue($threw);
}

# Flood control carries retry_after into the error.
func testFloodControlCarriesRetryAfter() {
    try {
        checkResponse(
            json.decode('{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":35}}'),
            429);
        throw Error{kind: "missed", message: "a 429"};
    } catch (e) {
        testing.assertEqual($e.errorCode, 429);
        testing.assertEqual($e.retryAfter, 35);
        testing.assertEqual($e.migrateToChatId, 0);
    }
}

func testCheckResponseOkPasses() {
    checkResponse(json.decode('{"ok":true,"result":{}}'), 200);
    testing.assertTrue(true);
}

func testParseMessage() {
    def node as json.Value init json.decode('{"result":{"message_id":42,"chat":{"id":-1001,"type":"group"},"date":1700000000,"text":"hello"}}');
    def m as Message init parseMessage($node, "/result");
    testing.assertEqual($m.messageId, 42);
    testing.assertEqual($m.chatId, -1001);
    testing.assertEqual($m.text, "hello");
    testing.assertEqual($m.date, 1700000000);
}

func testParseUser() {
    def node as json.Value init json.decode('{"result":{"id":777,"is_bot":true,"first_name":"Botty","username":"botty_bot"}}');
    def u as User init parseUser($node, "/result");
    testing.assertEqual($u.id, 777);
    testing.assertTrue($u.isBot);
    testing.assertEqual($u.firstName, "Botty");
    testing.assertEqual($u.username, "botty_bot");
}

func testParseUpdates() {
    def body as string init '{"ok":true,"result":[' +
        '{"update_id":100,"message":{"message_id":1,"chat":{"id":5},"date":1,"text":"first"}},' +
        '{"update_id":101,"edited_message":{"message_id":2}}]}';
    def us as list of Update init parseUpdates(json.decode($body));
    testing.assertEqual(len($us), 2);
    testing.assertEqual($us[0].updateId, 100);
    testing.assertTrue($us[0].hasMessage);
    testing.assertEqual($us[0].message.text, "first");
    testing.assertEqual($us[1].updateId, 101);
    testing.assertTrue(not $us[1].hasMessage);
}

func testParseUpdatesEmpty() {
    def us as list of Update init parseUpdates(json.decode('{"ok":true,"result":[]}'));
    testing.assertEqual(len($us), 0);
}

func testRenderInlineKeyboard() {
    def rows as list of list of Button init [
        [urlButton("Open", "https://example.org"), inlineButton("Ping", "ping")],
        [inlineButton("Close", "close")]
    ];
    testing.assertEqual(
        renderInlineKeyboard($rows),
        '{"inline_keyboard":[[{"text":"Open","url":"https://example.org"},' +
            '{"text":"Ping","callback_data":"ping"}],' +
            '[{"text":"Close","callback_data":"close"}]]}');
}

func testRenderInlineKeyboardEmpty() {
    def rows as list of list of Button init [];
    testing.assertEqual(renderInlineKeyboard($rows), '{"inline_keyboard":[]}');
}

func testParseCallbackQuery() {
    def body as string init '{"update_id":200,"callback_query":{' +
        "\"id\":\"cbq-99\"," +
        '"from":{"id":42,"is_bot":false,"first_name":"Ada","username":"ada"},' +
        '"message":{"message_id":7,"chat":{"id":5},"date":1,"text":"pick"},' +
        '"data":"ping"}}';
    def cq as CallbackQuery init parseCallbackQuery(json.decode($body));
    testing.assertEqual($cq.id, "cbq-99");
    testing.assertEqual($cq.data, "ping");
    testing.assertEqual($cq.from.id, 42);
    testing.assertEqual($cq.from.username, "ada");
    testing.assertEqual($cq.messageId, 7);
}

func testParseCallbackQueryAbsent() {
    def cq as CallbackQuery init parseCallbackQuery(json.decode('{"update_id":201}'));
    testing.assertEqual($cq.id, "");
    testing.assertEqual($cq.data, "");
    testing.assertEqual($cq.messageId, 0);
}

func testBuildUpload() {
    def data as bytes init convert.bytesFromString("utf-8", "PNGDATA");
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

# serveCanned answers one request on `listener` with a 200 carrying `body`.
func serveCanned(listener as net.Listener, body as string) {
    def c as net.Conn init net.accept($listener);
    net.readBytes($c, 65536);
    net.writeBytes(
        $c,
        convert.bytesFromString(
            "utf-8",
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " +
                convert.toString(len($body)) + "\r\nConnection: close\r\n\r\n" + $body));
    net.close($c);
}

# A refused connection is http's net.NetError (kind "net"), passed on as it
# is (its message names no token); a reply past json's nesting cap is kind
# "limit", not a TelegramError.
func testGenericKindsPassThrough() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def b as Bot init botWith("123:SECRET", "http://" + net.address($listener));
    def deep as string init strings.repeat("[", 1001);
    def server as task of null init spawn {
        serveCanned($listener, $deep);
    };
    try {
        call($b, "getMe", {}, 2000);
        throw Error{kind: "missed", message: "a decoding cap"};
    } catch (e) {
        testing.assertEqual($e.kind, "limit");
    }
    task.wait($server);
    net.close($listener);
    try {
        call($b, "getMe", {}, 2000);
        throw Error{kind: "missed", message: "a refused connection"};
    } catch (e) {
        testing.assertEqual($e.kind, "net");
        testing.assertEqual($e.reason, "refused");
        testing.assertFalse(strings.contains($e.message, "SECRET"));
    }
}

# transportFailed keeps a generic kind and redacts the token from its message;
# a cancelled task goes on as it is.
func testTransportFailedKeepsGenericKinds() {
    def token as string init "123456:ABCdefGHI";
    for (def k in ["timeout", "closed", "limit"]) {
        try {
            transportFailed(
                Error{kind: $k, message: "POST /bot" + $token + "/getMe failed"},
                $token);
            throw Error{kind: "missed", message: $k};
        } catch (e) {
            testing.assertEqual($e.kind, $k);
            testing.assertEqual($e.message, "telegram: POST /bot<redacted>/getMe failed");
        }
    }
    try {
        transportFailed(Error{kind: "cancelled", message: "task cancelled"}, $token);
        throw Error{kind: "missed", message: "cancelled"};
    } catch (e) {
        testing.assertEqual($e.kind, "cancelled");
        testing.assertEqual($e.message, "task cancelled");
    }
}
