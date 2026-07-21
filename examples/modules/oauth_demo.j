#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Acquire an OAuth2 access token with the Device Authorization grant (the CLI-friendly flow), then show how it feeds SASL XOAUTH2 for mail.
 * Needs a real OAuth2 provider and the default `jennifer` binary (the module uses `net` via `http`). Supply Google application credentials through the environment so nothing is committed. It prints a URL and code to approve in a browser, then waits for approval. With the variables unset it prints a hint instead of running. Not a golden test (it needs a live provider and a human).
 * @module oauth_demo
 */
use io;
use os;
import "../../modules/oauth.j" as oauth;
import "../../modules/sasl.j" as sasl;

def id as string init os.getEnv("GOOGLE_CLIENT_ID");
def secret as string init os.getEnv("GOOGLE_CLIENT_SECRET");

if (len($id) == 0 or len($secret) == 0) {
    io.printf("set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET to run the device flow\n");
} else {
    try {
        def cfg as oauth.Config init oauth.google($id, $secret, "https://mail.google.com/");

        # 1. Start the device flow and show the user what to do.
        def dev as oauth.DeviceAuth init oauth.deviceStart($cfg);
        io.printf("visit %s and enter code: %s\n", $dev.verificationUri, $dev.userCode);

        # 2. Poll until they approve.
        def tok as oauth.Token init oauth.deviceWait($cfg, $dev);
        io.printf("got an access token (expiresAt %d, refreshable: %t)\n",
            $tok.expiresAt, len($tok.refreshToken) > 0);

        # 3. Turn it into the SASL XOAUTH2 string an IMAP / SMTP server wants.
        io.printf("XOAUTH2 -> %s\n", sasl.bearer("you@gmail.com", $tok.accessToken));
    } catch (e) {
        io.printf("device flow failed (%s)\n", $e.message);
    }
}
