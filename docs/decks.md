# Decks

The deck roadmap: the decks around Jennifer (installed with `jvc`, mostly
`@deckyard/*`), apart from the [roadmap](roadmap.md), which is about the
language, its interpreter and its standard library. A deck needs no interpreter
release, and keeps its design notes in its own repository.

The IDs `M27.1` ... `M27.10` are stable. Other `M` numbers (`M30.9`,
`M30.14`, ...) are in the [roadmap](roadmap.md); `DRAFT#` handles are in
[horizon.md](horizon.md).

## M27 - candidate decks (deck ecosystem)

Service clients and format tooling that ship as decks through `jvc` (M26)
instead of core `modules/`. Core keeps general primitives (protocols, formats,
`http`, `graphql`, `csv`); a client for one vendor or service is a deck. Most
are thin clients over `http` / `rest` + `json`: a login or token step, a
generic `call`, and a few conveniences, never a fat typed wrapper.

### M27.1 - self-hosted infrastructure

LAN appliances and servers, usually behind a self-signed certificate.

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [docker](https://registry.jennifer-lang.dev/deck/deckyard/docker) | Done | Docker Engine REST API (Podman too): containers, images, networks, volumes, exec, system, with the API version negotiated. Takes `tcp://` (loopback unless allowed) or an `https://` socket proxy; `unix://` needs M30.15 plus `http` over a Unix socket, live streams wait for M30.9. |
| [frigate](https://registry.jennifer-lang.dev/deck/deckyard/frigate) | Done | Frigate NVR HTTP API: events, snapshots, clips, reviews, recordings, exports, config and stats; JWT login on port 8971 or the open port 5000. The live feed stays MQTT. |
| `homeassistant` | Planned | Home Assistant with a long-lived token. REST (`/api/...`, bearer) for states, services, history and events; WebSocket (`/api/websocket`, an `auth` message first) for subscriptions and the registries, replies matched by id. Gap: core `websocket` cannot trust a self-signed certificate on `wss://`. |
| `hoymiles` | Planned | Hoymiles microinverters through the DTU's local protocol (as `hoymiles-wifi` reverse-engineered it): TCP 10081, a 10-byte `HM` header with CRC-16/MODBUS, a protobuf body, AES-128-GCM on newer firmware. Live data, power limit, on / off, at least 2 s between requests. Requires M30.14; `net`. |
| [jellyfin](https://registry.jennifer-lang.dev/deck/deckyard/jellyfin) | Done | Jellyfin media server REST API (10.9+ routes): password or API key sign-in, libraries, items, played and favourite marks, sessions, scheduled tasks. |
| `midea` | Planned | Midea-family appliances on the LAN: UDP discovery (6445 / 20086), then the V3 "8370" protocol on TCP 6444 around the AES-128-ECB + MD5 frame. v1 types `0xAC` (air conditioner) and `0xA1` (dehumidifier); token and key come from one separate, fragile cloud call. `net`. |
| [onvif](https://registry.jennifer-lang.dev/deck/deckyard/onvif) | Done | IP cameras and NVRs over ONVIF SOAP 1.2 (profiles S, T, G) on the bundled `soap`: WS-Discovery, WS-Security digest and HTTP Digest, device, media, PTZ, imaging and pull-point events. Returns stream URLs, decodes no video. |
| [proxmox](https://registry.jennifer-lang.dev/deck/deckyard/proxmox) | Done | Proxmox VE REST / JSON API with `PVEAPIToken` header auth. |
| [routeros](https://registry.jennifer-lang.dev/deck/deckyard/routeros) | Done | A full MikroTik RouterOS abstraction layer, well beyond the `@deckyard/mikrotik` API client. |
| [synology](https://registry.jennifer-lang.dev/deck/deckyard/synology) | Done | Synology DSM Web API: `SYNO.API.Auth` login to a session `sid`, JSON replies. |
| [unraid](https://registry.jennifer-lang.dev/deck/deckyard/unraid) | Done | Unraid's GraphQL API (`/graphql`, 7.2+, `x-api-key`) over the `graphql` module: system, array, disks, shares, Docker, VMs, notifications. No subscriptions yet. |
| [virtualbox](https://registry.jennifer-lang.dev/deck/deckyard/virtualbox) | Done | VirtualBox 7.x through `VBoxManage` (`--machinereadable` parsed into structs), locally or over an `ssh` prefix. Capability `exec`, default binary only. |
| [virtualboxws](https://registry.jennifer-lang.dev/deck/deckyard/virtualboxws) | Done | The VirtualBox web service `vboxwebsrv`: SOAP to the Main API for a remote host without `exec`, every managed-object reference released. Capability `net`. |
| [vmware](https://registry.jennifer-lang.dev/deck/deckyard/vmware) | Done | vCenter / vSphere Automation REST API with session auth; a bare ESXi host (SOAP, partial REST on 8.x) is best effort. |

### M27.2 - public / SaaS APIs

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [anthropic](https://registry.jennifer-lang.dev/deck/deckyard/anthropic) | Done | Anthropic Messages API (`x-api-key`, `anthropic-version`): tools, thinking, prompt caching, token counting, batches. Same surface as `openai` and `gemini`; streaming waits for M30.9. |
| `brevo` | Planned | Brevo API v3 (`api-key` header): transactional mail with templates and attachments, plus contacts, lists, double opt-in and campaigns. Paging by `limit` / `offset`; no core gap. |
| `cloudstorage` | Planned | One file API (`put`, `get`, `list`, `stat`, `mkdirAll`, `remove`, `move`, `copy`) over koofr, seafile, onedrive, `webdav` and `s3`: a `Store` holding the backend's typed client and dispatching on its name. Ships after `onedrive` is released; `net`. |
| [codehost](https://registry.jennifer-lang.dev/deck/deckyard/codehost) | Done | One API over GitHub, GitLab and Forgejo / Gitea: repositories, issues, change requests, releases, branches, files, commit statuses, with normalised states. Built on the backend decks, reachable through `Forge`. |
| [gemini](https://registry.jennifer-lang.dev/deck/deckyard/gemini) | Done | Gemini API as AI Studio serves it (`x-goog-api-key`): `generateContent` with function calling, structured output and thinking, embeddings, the Files API, context caching. Vertex AI later. |
| [github](https://registry.jennifer-lang.dev/deck/deckyard/github) | Done | GitHub REST (`2022-11-28`) and GraphQL, github.com or Enterprise Server, token auth: repositories, issues, pull requests, releases, Actions, secrets sealed with `nacl-sealedbox`. |
| [gitlab](https://registry.jennifer-lang.dev/deck/deckyard/gitlab) | Done | GitLab REST v4 and GraphQL, token auth: projects, groups, issues, merge requests, releases, pipelines, CI variables. Follows redirects itself so the token never leaves its host. |
| [koofr](https://registry.jennifer-lang.dev/deck/deckyard/koofr) | Done | Koofr cloud storage API v2.1, app password or bearer token: mounts, files, search, links, trash. Sync stays with `webdav`; bodies are held in memory until M30.9. |
| `llm` | Planned | Provider-neutral facade over `openai`, `anthropic` and `gemini`, as `codehost` is over the forges: a neutral request builder, `chat`, `runTools`, one `LlmError`, `vendor(c)` for vendor features. A conversation stays in its vendor's format. Needs the three checked live once; streaming waits for M30.9. |
| `mailgun` | Planned | Mailgun sending API: Basic auth `api:<key>`, US or EU region, `multipart/form-data` messages, events, suppressions; webhooks checked by HMAC-SHA256 in constant time with a maximum age. No core gap. |
| `mailtrap` | Planned | Mailtrap Email Sending (bearer, JSON, templates) and the Email Testing sandbox, whose caught messages are read back to test other mail code in CI. No core gap. |
| [oauthext](https://registry.jennifer-lang.dev/deck/deckyard/oauthext) | Done | Vendor-specific OAuth2 over core `oauth`: the `google` / `microsoft` presets and `googleServiceAccount` through the RFC 7523 JWT-bearer grant. RS256, default binary only. |
| `onedrive` | Planned | OneDrive and SharePoint through the Microsoft Graph v1.0 drive API, OAuth2 only (device code through `oauthext`, `clientCredentials` for business). Downloads follow the redirect without `Authorization`; upload sessions above 250 MB in 320 KiB multiples; `delta`, sharing, versions. Bodies in memory until M30.9; `net`. |
| [openai](https://registry.jennifer-lang.dev/deck/deckyard/openai) | Done | OpenAI-compatible Chat Completions (Ollama, vLLM, OpenRouter and others by base URL; Azure as a config variant): tools, structured output, embeddings, models. Streaming waits for M30.9. |
| `resend` | Planned | Resend API (bearer): emails with `Idempotency-Key`, batches of 100, scheduling, domains, contacts and broadcasts; Svix-style HMAC-SHA256 webhook signatures; backs off on `429`. No core gap. |
| [seafile](https://registry.jennifer-lang.dev/deck/deckyard/seafile) | Done | Self-hosted Seafile Web API (`api2`, `api/v2.1`), account or per-library token: libraries including encrypted ones, files with chunked upload, history, shares, trash, search. |
| [steam](https://registry.jennifer-lang.dev/deck/deckyard/steam) | Done | Steam Web API, JSON with `?key=` auth; parts of it are community reverse-engineered. |
| [tmdb](https://registry.jennifer-lang.dev/deck/deckyard/tmdb) | Done | The Movie Database (TMDB) REST JSON API with bearer or key auth. |
| [vies](https://registry.jennifer-lang.dev/deck/deckyard/vies) | Done | EU VAT number checks against the Commission's VIES SOAP service (`checkVat`, `checkVatApprox` with a consultation number as proof); offline syntax check per country, transient faults retried. |

### M27.3 - daily helpers

Small everyday utilities.

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [clispinner](https://registry.jennifer-lang.dev/deck/mplx/clispinner) | Done | Terminal progress spinners in many styles, each colourable and at its own speed; silent when stdout is not a TTY. |
| `download` | Planned | HTTP(S) downloads over core `http` only: a `.part` file resumed by `Range` and checked by ETag / Last-Modified, retries, mirrors, sha256 or checksum-file verification, optional Ed25519 signature. Fixed 8 MiB range pieces until M30.9; `.torrent` web seeds (BEP 19) later. `net`. |

### M27.4 - bioinformatics

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [abif](https://registry.jennifer-lang.dev/deck/deckyard/abif) | Done | Reader for Applied Biosystems ABIF files (`.ab1`, `.fsa`, `.hid`): every element type, raw and analysed traces, base calls. `.hid` traces are encrypted and come back as raw bytes only. |
| [bioinformatics](https://registry.jennifer-lang.dev/deck/deckyard/bioinformatics) | Done | DNA / RNA / protein toolkit after the Sequence Manipulation Suite: transforms, composition and properties, pattern and ORF search, restriction digests, FASTA / FASTQ, pairwise alignment. Pure `.j`. |

### M27.5 - forensic / statistical genetics

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [forensicgenetics](https://registry.jennifer-lang.dev/deck/deckyard/forensicgenetics) | Done | STR profile statistics: Hardy-Weinberg match probabilities with theta correction, CPI / CPE, mtDNA / Y-STR haplotype counts with Clopper-Pearson bounds, kinship and paternity LRs. Pure `.j`, no I/O. |
| fragment analysis (no name yet) | Planned | Capillary-electrophoresis `.fsa` traces to called alleles over `abif`: baseline and pull-up, sizing against the lane standard, GeneMapper ID-X panels and bins, stutter filters, a `Profile` out. `.hid` traces are encrypted, so a 3500 lab needs `.fsa` or GeneMapper exports. Research and QC, not casework. |
| [strider](https://registry.jennifer-lang.dev/deck/deckyard/strider) | Done | Reads STRidER allele-frequency XML tables into a `forensicgenetics` `FrequencyDb`. |

### M27.6 - NGS / high-throughput sequencing

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [ngs](https://registry.jennifer-lang.dev/deck/deckyard/ngs) | Done | NGS glue and streaming I/O: FASTQ, SAM, VCF, BED, GFF / GTF as streams, QC and trimming, interval ops, and pipelines driving BWA, samtools and GATK. Capability `exec`; no alignment, assembly, variant calling or BAM / BGZF. |

### M27.7 - data-format tooling

Tooling on top of a core format, not a format of its own.

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [epub](https://registry.jennifer-lang.dev/deck/deckyard/epub) | Done | Reads and writes EPUB 2 and 3.x over `archive`, `xml` and `markdown`: deterministic EPUB 3.3 output that passes EPUBCheck; metadata, spine and table of contents on read, DRM an error. |
| `exif` | Done | Reads, changes and strips EXIF in JPEG, PNG and WebP over `binary`: typed tag values and names, GPS, date and thumbnail helpers, privacy stripping; writing replaces only the EXIF block. Strict parser with `"limit"` caps; XMP, IPTC, HEIC and raw files out of scope. |
| [jsonpath](https://registry.jennifer-lang.dev/deck/deckyard/jsonpath) | Done | RFC 9535 JSONPath over `json.Value`, matches also as JSON Pointers for the core writers; I-Regexp functions. |
| [jsonschema](https://registry.jennifer-lang.dev/deck/deckyard/jsonschema) | Done | JSON Schema validation (2020-12, draft-07): remote `$ref`s only from a registry the caller supplies, `pattern` on RE2 with unsupported syntax a compile error. |
| `pdftools` | Planned | Reading and changing existing PDFs by driving qpdf (`--json`) and optionally poppler: info, merge, split, rotate, encrypt, linearize, text, render. Passwords through `--password-file`; capability `exec`, default binary only. |
| `pdfread` (name open) | Idea | Reads PDFs in pure `.j`, both binaries: trailer, xref tables and xref streams, object streams, incremental updates; Flate, LZW, ASCII85, ASCIIHex and RunLength filters with PNG / TIFF predictors; document info and XMP metadata, page tree, outline; text extraction through ToUnicode CMaps and the standard encodings (WinAnsi, MacRoman, Standard, PDFDoc, the glyph list); encrypted files (RC4 and AES, R2 to R6) with a password. Image streams handed over undecoded. Strict: a broken xref or offset is an error; xref reconstruction, if ever, is a separate call the caller chooses. Caps on objects, nesting, stream and decoded sizes. Starts under one maintainer's scope (promotion rule). Unlike `pdftools`, no external binary and no editing. Requires M30.26 (RC4) and M30.27 (LZW). |
| `protobuf` | Planned | Protocol Buffers by schema over the M30.14 wire library: schemas from a `FileDescriptorSet`, messages as `json.Value` in canonical ProtoJSON, a `.proto` parser later. No gRPC. Requires M30.14. |
| [sealbox](https://registry.jennifer-lang.dev/deck/deckyard/sealbox) | Done | Encryption formats over core `crypto`: AES-256-GCM sealed box, AES-CBC + HMAC-SHA256, Fernet. One opaque `sealbox` error; pure `.j`, both binaries. |
| `torrent` | Planned | BitTorrent metadata only: strict bencode, v1 / v2 / hybrid `.torrent` read and create, info hashes, magnet links, piece verification, HTTP and UDP tracker announce and scrape. Never a peer client. Waits for a maintainer who uses BitTorrent. |
| torrent client control (no name yet) | Idea | Drives a running Transmission, qBittorrent or Deluge over its RPC or Web API (add, progress, limits, remove), with `torrent` supplying hashes and magnet links. A plain HTTP client deck. |

### M27.8 - money

| Deck | Status | Description |
| ---- | ------ | ----------- |
| `finance` | Planned | Financial maths: time value of money in Excel's conventions (`npv`, `irr`, `xirr`, ...), amortisation schedules in `money.Money`, day counts, bond pricing, depreciation, Black-Scholes, returns and risk, one Newton / Brent solver. Depends on `@deckyard/money`; no capability. |
| `marketdata` | Planned | FX rates, economic series and prices from providers with an official API (ECB, FRED, Alpha Vantage, Twelve Data) behind one `Series` / `Quote` / `FxRate` model, values kept as the published decimal string. Throttled to free-tier caps, optional `flatdb` cache; `net`. |
| [money](https://registry.jennifer-lang.dev/deck/deckyard/money) | Done | Exact currency amounts as whole minor units (`Money{amount, currency}`) over `decimal`: ISO 4217 table, explicit rounding modes, lossless allocation, fixed-rate exchange. No locale formatting. |

### M27.9 - statistical data files

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [statfiles](https://registry.jennifer-lang.dev/deck/deckyard/statfiles) | Done | Read-only SPSS (`.sav`, `.zsav`, `.por`), Stata `.dta` 104-119 and SAS (`.xpt`, `.sas7bdat`, `.sas7bcat` labels) readers into a column-stored `Dataset` with labels and missing codes. Pure `.j`, no capability. |

### M27.10 - healthcare interop

HL7 v2, FHIR and the ASTM lab-instrument protocol; the readers are strict.

| Deck | Status | Description |
| ---- | ------ | ----------- |
| [astm](https://registry.jennifer-lang.dev/deck/deckyard/astm) | Done | ASTM E1381 / E1394 (CLSI LIS01 / LIS02) analyzer protocol: framed transport with checksums and timers over TCP or `serial`, records, orders and queries. `net`; serial needs the default binary. |
| [fhir](https://registry.jennifer-lang.dev/deck/deckyard/fhir) | Done | HL7 FHIR R4 / R5 REST client: read, search with paging caps, create / update / patch / delete, history, transactions, operations; bearer or SMART backend-services auth. Resources stay `json.Value`. |
| [hl7](https://registry.jennifer-lang.dev/deck/deckyard/hl7) | Done | HL7 v2.x ER7 messages: strict parse and build with terser-style paths, ACK, and an MLLP client and server. `net`. |

---

## Planned deck work

Changes to existing decks that are planned but not done. Each item names the
deck; core gaps, if any, are M entries in the [roadmap](roadmap.md).

- **`@deckyard/discord`: bots, not only webhooks.** **Planned.** The deck posts
  to a channel through a webhook, so it can notify but not answer. Add what a
  bot needs, keeping the webhook API as it is:
  - **Interactions endpoint** (slash commands, buttons): verify each request's
    `X-Signature-Ed25519` over `X-Signature-Timestamp` + body with
    `crypto.verify("ed25519", ...)` against the application's public key and
    refuse anything else with 401, before parsing; answer `PING`; dispatch
    commands to handlers; immediate, deferred and follow-up replies. A handler
    plugs into a `web` app, so the bot runs behind any reverse proxy.
  - **REST with a bot token** (`Authorization: Bot`): send and edit messages in
    a channel, register commands (global and per guild), the per-route rate
    limit buckets and `429` `retry_after`, errors as one `DiscordError` struct
    (status, Discord's error code, message).
  - **Gateway** (second step): the live connection over the `websocket`
    module: identify with intents, heartbeat, resume after a drop, a bounded
    event queue. Needed for bots that read messages instead of commands.
  - **Tests offline**: a fake API and gateway server, signature test vectors.
  - No core gap.
- **`@deckyard/telegram`: the rest of an interactive bot.** **Planned.** Found
  writing the academy's Telegram bot talk, which works around each of these
  through the raw update JSON:
  - `getUpdates` keeps only text messages; return every update kind (or a
    tagged `Update`), so a bot needs no `getUpdatesRaw` for button presses.
  - `CallbackQuery` lacks the chat id and the message id of the pressed
    button; `Message` lacks its sender (`from`).
  - `editMessageText` and `editMessageReplyMarkup`, so a button press can
    update its own message.
  - `setMyCommands` / `deleteMyCommands`, the command menu.
  - Webhooks: `setWebhook` / `deleteWebhook` / `getWebhookInfo`, and a
    handler for a `web` app that checks the
    `X-Telegram-Bot-Api-Secret-Token` header before parsing.
  - Optional: a helper that waits out a `429` for its `retryAfter`, bounded
    by a deadline (the deck leaves waiting to the caller today).
  No core gap.
- **`@deckyard/routeros`: infrastructure as code.** **Planned.** Found writing
  the academy's MikroTik IaC talk, whose plan, dry run and undo are its own
  program on top of `getAll`, `add`, `set` and `remove`:
  - a plan step: compare a desired state (rows per menu path, matched by a
    key such as `name`) with `getAll` and return the adds, sets (changed
    attributes only) and removes, limited to rows the caller owns (e.g. by a
    comment prefix); `apply(plan)` with an undo list on failure.
  - a setter for single-row menus (`/ip/dns`, `/system/identity`,
    `/system/clock`), which have no `.id` for `set`.
  - drift detection: the plan of a device already in sync is empty.
  - Restoring a backup over the API is out of scope (it reboots the router);
    the deck documents that.
  Lands after the move to `@deckyard/mikrotik` (before 0.28.0). No core gap.

## Scopes

| Scope | What it holds |
| --- | --- |
| `@deckyard` | community decks, and every deck that was `@jennifer` (moved in 0.28.0) |
| `@jennifer` | reserved for the project, no decks; its old names answer with a moved record |
| `@<user>` | a single maintainer's decks (`@mplx/...`); promoted to `@deckyard` under deckyard's CONTRIBUTING rule |

## Ideas (not planned yet)

Candidates that are not planned yet. A deck needs no interpreter release, so
one moves up into an M27 section whenever it earns a slot.

- **`grimoire-plugin`** - Grimoire's plugin kit (`src/pluginkit.j` in Grimoire:
  the request and reply contract, typed settings, `chapter:line` errors,
  CommonMark fences, `_generated` links, content-hash names, running a program,
  bounded concurrent work) as a deck of its own, so a plugin written in
  Jennifer declares it under `[decks]` instead of carrying a copy of the file.
  Depending on the `@jennifer/grimoire` deck for it would vendor all of Grimoire
  (about 900 KB) into every plugin. Until then a plugin copies the one file from
  the Grimoire release it needs (0.5.0 or later), as `grimoire-mermaid` does.
  Grimoire itself would vendor the deck too, so every packaging path (image,
  .deb, Arch, Pages) needs a vendoring step first. No core gap.
- **`sendgrid`** - the SendGrid v3 mail API (no free plan since 2025; M27.2
  has `mailgun`, `mailtrap`, `brevo` and `resend` instead): bearer auth, JSON
  (`/v3/mail/send` with personalizations, attachments, dynamic templates), plus
  Event Webhook verification. That signature is ECDSA P-256 in **DER** form over
  a base64 DER public key, while `crypto.verify("ecdsa-sha256", ...)` takes a PEM
  key and the JOSE `R||S` form, so the deck wraps the key as PEM and converts the
  signature with `asn1`. Testable offline through the API's sandbox mode. No core
  gap. A separate deck from `twilio` despite the common owner: different API,
  different auth.
- **`twilio`** - the Twilio REST API: Basic auth (account SID + auth token),
  form-encoded requests (`uri.encodeForm`), JSON responses paged through
  `next_page_uri`, conveniences for messages, calls, and Verify. Webhook
  verification is HMAC-SHA1 over the full URL plus the sorted POST parameters,
  base64 (`hash.hmac` with `"sha1"`). Twilio's test credentials and magic numbers
  make it testable without charges. No core gap.
- **`stripe`** - the Stripe API: bearer auth, a pinned `Stripe-Version` header,
  an `Idempotency-Key` per mutating call (`uuid.v4`), list paging through
  `has_more` / `starting_after`, and Stripe's error object mapped to catchable
  `Error` kinds. Requests are form-encoded with **nested bracket keys**
  (`line_items[0][price]=...`), so the deck carries a small `json.Value` ->
  bracket-form encoder. Webhook verification already ships in core
  (`webhook.stripeVerify`). Testable in CI against `stripe-mock`. No core gap.
- **`paypal`** - the PayPal REST API, beside `stripe` as the other payment
  provider: OAuth2 client credentials (client id and secret, through core
  `oauth.clientCredentials`) for a bearer token kept until it expires; the
  live and sandbox hosts (`api-m.paypal.com`, `api-m.sandbox.paypal.com`);
  Orders v2 (create, authorize, capture), Payments v2 (captures, refunds,
  authorizations), Subscriptions and Payouts; a `PayPal-Request-Id` per
  mutating call for idempotency; PayPal's error object (`name`,
  `details[]`, `debug_id`) mapped to a typed error. Webhooks verified
  through the `verify-webhook-signature` call. Sandbox accounts make it
  testable without charges. No core gap.
- **`aws`** - a **generic** AWS client, not per-service wrappers (300+ services):
  one entry point per wire protocol - JSON 1.0 / 1.1 (`X-Amz-Target`: DynamoDB,
  SQS, KMS, Secrets Manager, CloudWatch Logs), Query (form `Action=`, XML
  responses through `xml`: STS, SNS, SES v1), and REST-JSON / REST-XML (Lambda,
  SES v2, Route 53) - plus region / endpoint resolution and thin conveniences
  for the common services (DynamoDB attribute-value encoding among them). The
  credential chain covers static keys, the `AWS_*` environment variables, the
  shared `~/.aws/credentials` file (a small INI reader in the deck), session
  tokens, STS `AssumeRole`, and EC2 IMDSv2; IAM Identity Center (SSO) stays out
  of v1. **Requires a core change first:** SigV4 signing is private to
  `modules/s3_sigv4.inc.j`, which `s3` and `s3presign` splice in (M29.17), and
  pinned to the `s3` service, so `s3` exports it: an
  `s3.sign` that takes the service as a parameter beside the existing
  `s3.presign`, returning the `Authorization` / `x-amz-date` headers. Too few
  names for a module of its own, and `s3` is already the core home of SigV4 for
  every S3-compatible store; the generalized signer also serves OpenSearch and
  other SigV4 services. Tested offline against the published SigV4 test-suite vectors and in CI against
  LocalStack. Roughly 2-3x the size of the `twilio` or `stripe` deck.
- **`firebase`** - Firebase over its REST surfaces: Auth ID-token verification
  (`jwt.verifyJwks` against the securetoken JWKS, plus the `iss` / `aud` / `sub`
  / `auth_time` checks), Admin Auth user management (Identity Toolkit REST), FCM
  HTTP v1 push, Firestore REST (with a converter between Jennifer values and
  Firestore's typed value encoding - `stringValue`, `integerValue` as a string,
  `mapValue`, `arrayValue`, `timestampValue`, ...; `runQuery` for structured
  queries), and Realtime Database REST. Server access authenticates with a
  Google service account through `oauthext.googleServiceAccount` (the RFC 7523
  grant over core `oauth`, M27.2 above). **Out of v1:** realtime listeners - Firestore's need gRPC (no HTTP/2 / gRPC client),
  the Realtime Database's need a streaming (SSE) response, which `http` cannot
  read yet (planned as M30.9, the streaming `http` + `sse` addition). Tested in
  CI against the Firebase Emulator Suite (Java + Node in the runner). Larger
  than any M27.2 deck.
