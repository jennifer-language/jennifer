# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The shared connection-transport security mode, used by every network-client
 * module that opens a socket (`smtp`, `pop`, `imap`, `redis`, `amqp`, `mqtt`).
 * One closed enum instead of a stringly-typed `security` field per module, so a
 * value is typed and a typo is a compile-time error, not a silent plaintext
 * downgrade.
 *
 * `Security.None` is plaintext, `Security.Tls` is implicit TLS on connect, and
 * `Security.Starttls` is an in-band upgrade after connecting (SMTP STARTTLS,
 * POP3 STLS, IMAP STARTTLS). A module that has no in-band upgrade (`redis`,
 * `amqp`, `mqtt`) rejects `Security.Starttls` with a clear error rather than
 * treating it as anything else.
 *
 * Build a connection option with a variant directly:
 * `smtp.Options{security: transport.Security.Starttls, ...}` - which needs
 * `import "transport.j" as transport;` alongside the client module.
 * @module transport
 * @example
 * import "transport.j" as transport;
 * import "smtp.j" as smtp;
 * def o as smtp.Options init smtp.Options{
 *     host: "smtp.example.com", port: 587, security: transport.Security.Starttls,
 *     clientName: "me", user: "u", pass: "p", auth: "", allowInsecureAuth: false};
 */

/**
 * A connection's transport security mode.
 * - `None` - plaintext (no encryption).
 * - `Tls` - implicit TLS negotiated on connect (SMTPS / POP3S / IMAPS / rediss / ...).
 * - `Starttls` - connect in plaintext, then upgrade in-band (SMTP STARTTLS, POP3
 *   STLS, IMAP STARTTLS). Only the protocols with an upgrade command accept it.
 */
export def enum Security { None, Tls, Starttls };

/**
 * Whether a security mode encrypts the connection (`Tls` or `Starttls`). `None`
 * is the only unencrypted mode. Handy for a "refuse to send credentials over
 * plaintext" guard.
 * @param s {Security} the security mode
 * @return {bool} true if the connection is (or becomes) encrypted
 */
export func encrypted(s as Security) {
    match ($s) {
        when None { return false; }
        when Tls { return true; }
        when Starttls { return true; }
    }
}
