# `acme` - ACME / Let's Encrypt TLS certificates

Import with `import "acme.j" as acme;`. An [ACME](https://www.rfc-editor.org/rfc/rfc8555)
(RFC 8555) client: obtain and renew TLS certificates from Let's Encrypt and
compatible CAs. It drives the whole flow - account registration, a new order,
an **HTTP-01** or **DNS-01** challenge, CSR finalize, and certificate download -
over [`http`](http.md) + [`json`](../libraries/json.md), with every request a JWS
signed by the account key (`RS256` for an RSA key, `ES256` for an EC key). Keys,
the CSR, and the JWK thumbprint come from [`crypto`](../libraries/crypto.md), so
`acme` needs the default `jennifer` binary.

> **Use a staging endpoint first.** Let's Encrypt's staging CA
> (`https://acme-staging-v02.api.letsencrypt.org/directory`) issues *untrusted*
> certificates with far higher rate limits - develop against it, then switch to
> production.

Runnable: [`examples/modules/acme_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/acme_demo.j).

## The flow

Proving you control a domain is **your** job, so the steps are separate calls,
not one `obtain()`. The order is: register an account, create an order, and for
each domain fetch its authorization, pick a challenge, **publish the response**,
`accept`, and poll until valid; then finalize with a CSR and download the cert.

```jennifer
use crypto;
import "acme.j" as acme;

def key as bytes init crypto.ecGenerateKey("p256");                     # account key
def client as acme.Client init acme.connect(DIRECTORY_URL, $key);
def client as acme.Client init acme.register($client, "you@example.com");

def order as acme.Order init acme.order($client, ["example.com"]);
def authz as acme.Authorization init acme.authorization($client, $order.authorizations[0]);
def ch as acme.Challenge init acme.challenge($authz, "http-01");

# --- publish the challenge response (see below), then: ---
acme.accept($client, $ch.url);
acme.pollAuthorization($client, $order.authorizations[0], 2000, 30);

def certKey as bytes init crypto.ecGenerateKey("p256");                 # certificate key
def csr as bytes init crypto.csr($certKey, ["example.com"]);
def issued as acme.Order init acme.finalize($client, $order, $csr, 2000, 30);
def pem as string init acme.downloadCertificate($client, $issued);     # the cert chain
```

## Publishing the challenge response

For a challenge `ch`, compute the response and publish it, then `accept`:

- **HTTP-01**: serve `acme.keyAuthorization($client, $ch.token)` as the body at
  `http://<domain>/.well-known/acme-challenge/<ch.token>` (pair with
  [`web`](web.md) / [`httpd`](../libraries/httpd.md)).
- **DNS-01**: publish `acme.dnsRecord($client, $ch.token)` as a TXT record at
  `_acme-challenge.<domain>` (via your DNS provider). DNS-01 is the only way to
  get a **wildcard** (`*.example.com`) certificate.

Both are pure (no network) - compute them, provision the response your own way,
then tell the CA it is ready with `accept`.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `acme.connect(directoryUrl, accountKey)` | `Client` | Fetch the CA directory; build a client around the account key (PEM `bytes`). |
| `acme.register(client, email)` | `Client` | Register the account (agrees to the ToS); returns a client with its `kid`. |
| `acme.order(client, domains)` | `Order` | Create an order for a `list of string` of domains. |
| `acme.authorization(client, authzUrl)` | `Authorization` | Fetch an authorization and its challenges. |
| `acme.challenge(authz, kind)` | `Challenge` | The `"http-01"` / `"dns-01"` challenge within an authorization. |
| `acme.keyAuthorization(client, token)` | `string` | The HTTP-01 response body (`token.thumbprint`). Pure. |
| `acme.dnsRecord(client, token)` | `string` | The DNS-01 TXT value. Pure. |
| `acme.accept(client, challengeUrl)` | `null` | Tell the CA the response is published. |
| `acme.pollAuthorization(client, authzUrl, intervalMs, maxTries)` | `Authorization` | Poll until the authorization settles. |
| `acme.finalize(client, order, csrDer, intervalMs, maxTries)` | `Order` | Submit the CSR and poll until the certificate is issued. |
| `acme.downloadCertificate(client, order)` | `string` | The issued PEM certificate chain. |
| `acme.fetchOrder(client, orderUrl)` | `Order` | Re-fetch an order's state. |

The structs (`Client`, `Order`, `Authorization`, `Challenge`) are value-semantic;
`register` returns a client with the account `kid` filled in. `Client` holds the
account key and the CA endpoints. Every signed request draws a fresh anti-replay
nonce from the CA before it is sent.

## Keys and algorithms

- **Account key**: `crypto.ecGenerateKey("p256")` (→ `ES256`) or
  `crypto.rsaGenerateKey(2048)` (→ `RS256`). `connect` derives the JWS algorithm
  from the key. Reuse the *same* account key across renewals.
- **Certificate key**: a **separate** key, only used for the CSR
  (`crypto.csr(certKey, domains)`). Generate a fresh one, or reuse per domain.

Errors from the CA (an RFC 7807 problem document) surface as catchable
`Error{kind: "acme"}`.

## Renewal

Renewing is just running the flow again before expiry (Let's Encrypt certs last
90 days; renew at ~60). Keep the account key; a new order re-validates the
domains and issues a fresh certificate.

## Platform

`acme` needs `net` (via `http`) and the `crypto` RSA / ECDSA / CSR / JWK surface,
so it runs on the **default `jennifer`** binary only; on `jennifer-tiny` the
crypto and network calls raise friendly errors.
