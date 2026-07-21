#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Sign a webhook payload and verify it the way a receiver would (the GitHub
 * `X-Hub-Signature-256` convention), then show that a tampered payload or a
 * wrong secret is rejected.
 * @module webhook_demo
 */
use io;
import "../../modules/webhook.j" as webhook;

def secret as string init "topsecret";
def payload as string init "{\"event\":\"push\",\"ref\":\"main\"}";

# The sender computes the signature and sends it in the X-Hub-Signature-256 header.
def sig as string init webhook.sign($payload, $secret);
io.printf("X-Hub-Signature-256: %s\n", $sig);

# The receiver recomputes over the raw body it got and compares.
io.printf("valid delivery:      %t\n", webhook.verify($payload, $sig, $secret));
io.printf("tampered payload:    %t\n", webhook.verify("{\"event\":\"push\",\"ref\":\"evil\"}", $sig, $secret));
io.printf("wrong secret:        %t\n", webhook.verify($payload, $sig, "guessed"));

# To actually deliver it (needs the default `jennifer` binary, over `http`):
#   import "../../modules/http.j" as http;
#   def r as http.Response init webhook.send("https://example.com/hook", $payload, $secret);
#   io.printf("delivered: %d\n", $r.status);
