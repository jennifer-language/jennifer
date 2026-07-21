# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * The smtp module (modules/smtp.j) with mime (modules/mime.j): build a message and send it.
 * By default it targets a local SMTP server on 127.0.0.1:2525 (e.g. a dev MailHog / aiosmtpd); with none running it prints the message and the connection error rather than failing. Needs the default jennifer binary (jennifer-tiny has no network stack).
 * @module smtp_demo
 */
use io;
import "../../modules/mime.j" as mime;
import "../../modules/smtp.j" as smtp;

# Build the message with mime.
def msg as mime.Part init mime.text("text/plain", "Hello from Claudine! Accents: café.");
$msg = mime.withHeader($msg, "From", mime.address("Claudine", "claudine@example.com"));
$msg = mime.withHeader($msg, "To", "claudette@example.com");
$msg = mime.withHeader($msg, "Subject", "Jennifer SMTP demo");
def wire as string init mime.encode($msg);

io.printf("=== message ===\n%s\n\n", $wire);

# Send it. security "none" | "starttls" | "tls"; set user / pass for AUTH.
def opts as smtp.Options init smtp.Options{host: "127.0.0.1", port: 2525,
    security: "none", clientName: "jennifer.demo", user: "", pass: "", auth: "",
    allowInsecureAuth: false};
def rcpts as list of string init ["you@example.com"];

try {
    smtp.send($opts, "claudine@example.com", $rcpts, $wire);
    io.printf("delivered to %s:%d\n", $opts.host, $opts.port);
} catch (e) {
    io.printf("no SMTP server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
