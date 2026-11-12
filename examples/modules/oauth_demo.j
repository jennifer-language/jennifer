#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Sign in to Gmail in the browser (the authorization-code flow with PKCE, the redirect caught on a loopback server), then show how the token feeds SASL XOAUTH2 for mail.
 * Needs a real OAuth2 provider and the default `jennifer` binary (the module uses `net` via `http`). Supply the credentials of a Google "Desktop app" OAuth client through the environment so nothing is committed. It prints a URL to open in a browser, then waits for the sign-in. With the variables unset it prints a hint instead of running. Not a golden test (it needs a live provider and a human). Google's device flow cannot ask for Gmail access, hence the browser.
 * @module oauth_demo
 */
use httpd;
use io;
use os;
import "../../modules/oauth.j" as oauth;
import "../../modules/sasl.j" as sasl;

def id as string init os.getEnv("GOOGLE_CLIENT_ID");
def secret as string init os.getEnv("GOOGLE_CLIENT_SECRET");

if (len($id) == 0 or len($secret) == 0) {
    io.printf("set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET to sign in\n");
} else {
    try {
        # Google's endpoints, from its metadata document
        def cfg as oauth.Config init oauth.discover(
            "https://accounts.google.com",
            $id,
            $secret,
            "https://mail.google.com/");

        # 1. Listen on this machine for the redirect, and send the user to sign in.
        def srv as httpd.Server init httpd.listen("127.0.0.1:0");
        def req as oauth.AuthRequest init oauth.authorize(
            $cfg,
            "http://" + httpd.address($srv) + "/callback");
        io.printf("open this URL in a browser and sign in:\n%s\n", $req.url);

        # 2. Catch the redirect and trade the code for a token.
        def code as string init oauth.awaitCode($req, $srv);
        httpd.shutdown($srv);
        def tok as oauth.Token init oauth.exchangeCode($cfg, $req, $code);
        io.printf("got an access token (expiresAt %d)\n", $tok.expiresAt);

        # 3. Turn it into the SASL XOAUTH2 string an IMAP / SMTP server wants.
        io.printf("XOAUTH2 -> %s\n", sasl.bearer("you@gmail.com", $tok.accessToken));
    } catch (e) {
        io.printf("sign-in failed (%s)\n", $e.message);
    }
}
