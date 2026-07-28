#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The transport module (modules/transport.j): the shared connection-security
 * mode every socket client uses. Runs offline (no server).
 * Run: jennifer run examples/modules/transport_demo.j
 * @module transport_demo
 */
use io;
import "../../modules/transport.j" as transport;
import "../../modules/smtp.j" as smtp;

# The three modes, and whether each encrypts the connection.
io.printf("=== security modes ===\n");
func label(s as transport.Security) {
    match ($s) {
        when None { return "none"; }
        when Tls { return "tls"; }
        when Starttls { return "starttls"; }
    }
    return "?";
}
def modes as list of transport.Security init [
    transport.Security.None,
    transport.Security.Tls,
    transport.Security.Starttls
];
for (def m in $modes) {
    io.printf("  %s encrypted=%t\n", label($m), transport.encrypted($m));
}

# The same Security value is used as a client Options field - one shared type
# across smtp / pop / imap / redis / amqp / mqtt.
io.printf("=== used as a client Options field ===\n");
def o as smtp.Options init smtp.Options{
    host: "smtp.example.com", port: 587,
    security: transport.Security.Starttls,
    clientName: "me.example.com", user: "me@example.com", pass: "secret",
    auth: "", allowInsecureAuth: false};
io.printf("  smtp to %s:%d over %s (credentials safe: %t)\n",
    $o.host, $o.port, label($o.security), transport.encrypted($o.security));
