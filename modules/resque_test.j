# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# resque_test.j - white-box tests for resque.j's pure envelope helpers. Run with:
#
#     jennifer test modules/resque_test.j
#
# The overlay splices resque.j in front of this file, so the tests reach its
# private key builders and JSON envelope encode / decode by bare identifier. The
# networked enqueue / reserve round-trip is verified against an in-process RESP
# server in the Go suite (TestResqueJobs).
use testing;

func testKeys() {
    testing.assertEqual(queuesKey(), "resque:queues");
    testing.assertEqual(queueKey("email"), "resque:queue:email");
    testing.assertEqual(failedKey(), "resque:failed");
}

func testEncodePayload() {
    testing.assertEqual(encodePayload("SendWelcome", ["a@b.c", "en"]),
        "{\"class\":\"SendWelcome\",\"args\":[\"a@b.c\",\"en\"]}");
}

func testEncodePayloadEmptyArgs() {
    testing.assertEqual(encodePayload("Ping", []), "{\"class\":\"Ping\",\"args\":[]}");
}

func testDecodeJob() {
    def job as Job init decodeJob("email", "{\"class\":\"X\",\"args\":[\"p\",\"q\"]}");
    testing.assertEqual($job.queue, "email");
    testing.assertEqual($job.class, "X");
    testing.assertEqual(len($job.args), 2);
    testing.assertEqual($job.args[0], "p");
    testing.assertEqual($job.args[1], "q");
}

func testDecodeJobEmptyArgs() {
    def job as Job init decodeJob("high", "{\"class\":\"Ping\",\"args\":[]}");
    testing.assertEqual($job.class, "Ping");
    testing.assertEqual(len($job.args), 0);
}

func testDecodeJobNonStringArgs() {
    # a job enqueued elsewhere with int / bool / float args still reserves as strings
    def job as Job init decodeJob("q", "{\"class\":\"C\",\"args\":[1,true,2.5]}");
    testing.assertEqual($job.args[0], "1");
    testing.assertEqual($job.args[1], "true");
    testing.assertEqual($job.args[2], "2.5");
}

func testRoundTrip() {
    def job as Job init decodeJob("work", encodePayload("Resize", ["img.png", "800"]));
    testing.assertEqual($job.class, "Resize");
    testing.assertEqual($job.args[0], "img.png");
    testing.assertEqual($job.args[1], "800");
}
