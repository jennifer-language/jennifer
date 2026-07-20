# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

# acme_demo.j - obtain a TLS certificate from an ACME CA (Let's Encrypt). The
# networked flow needs a real CA account and a domain you control, so this demo
# runs the offline parts (generate a key, compute the HTTP-01 / DNS-01 challenge
# responses) and defines `getCertificate` - the full HTTP-01 orchestration - for
# you to run against a CA's *staging* endpoint.
#
#     jennifer run examples/modules/acme_demo.j

use io;
use crypto;
use convert;
import "../../modules/acme.j" as acme;

# An account key (EC P-256 -> ES256, or crypto.rsaGenerateKey(2048) -> RS256).
def key as bytes init crypto.ecGenerateKey("p256");
io.printf("account key (PEM, %d bytes) generated\n", len($key));

# Build a client by hand to show the offline challenge math (real code uses
# acme.connect, which fetches the CA directory over the network).
def client as acme.Client init acme.Client{directory: "", newNonce: "", newAccount: "",
    newOrder: "", accountKey: $key, alg: "ES256", kid: ""};

# For a challenge token TOKEN the CA hands you:
def token as string init "sample-challenge-token";
io.printf("\nHTTP-01: serve this at /.well-known/acme-challenge/%s\n  %s\n",
    $token, acme.keyAuthorization($client, $token));
io.printf("\nDNS-01: set this TXT at _acme-challenge.<domain>\n  %s\n",
    acme.dnsRecord($client, $token));

# getCertificate runs the whole HTTP-01 flow against a CA. `serveChallenge` is
# your job: publish the key authorization at the well-known path (e.g. with the
# `web` / `httpd` server) before this returns from `accept`.
func getCertificate(directoryUrl as string, email as string, domains as list of string) {
    def accountKey as bytes init crypto.ecGenerateKey("p256");
    def session as acme.Client init acme.connect($directoryUrl, $accountKey);
    $session = acme.register($session, $email);

    def order as acme.Order init acme.order($session, $domains);
    for (def i as int init 0; $i < len($order.authorizations); $i = $i + 1) {
        def authz as acme.Authorization init acme.authorization($session, $order.authorizations[$i]);
        def ch as acme.Challenge init acme.challenge($authz, "http-01");
        # Publish acme.keyAuthorization($session, $ch.token) at
        # /.well-known/acme-challenge/$ch.token on http://$authz.domain, then:
        acme.accept($session, $ch.url);
        acme.pollAuthorization($session, $order.authorizations[$i], 2000, 30);
    }

    # A separate certificate key, and a CSR for the domains.
    def certKey as bytes init crypto.ecGenerateKey("p256");
    def request as bytes init crypto.csr($certKey, $domains);
    def issued as acme.Order init acme.finalize($session, $order, $request, 2000, 30);
    return acme.downloadCertificate($session, $issued);
}

io.printf("\n(getCertificate is defined; run it against a CA staging endpoint)\n");
