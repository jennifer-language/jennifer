# Decks

The deck roadmap: what was M27 in [milestones.md](milestones.md), moved here on
2026-11-15 so the milestones stay about the language, its interpreter and its
standard library. Decks are the ecosystem around it (installed with `jvc`,
mostly `@deckyard/*`), they need no interpreter release, and they carry their
own history in their own repositories.

The section and entry IDs (`M27.1` ... `M27.10`) are unchanged and stay stable,
so references to them elsewhere still resolve here. Other `M` numbers (`M28.29`,
`M28.61`, ...) are in [milestones.md](milestones.md); `DRAFT#` handles are in
[horizon.md](horizon.md).

**Pruning.** When a deck is done and its repository carries its own design
notes (a "Design" section in its docs), its entry here may be deleted; git
history keeps it. The file is meant to shrink to planned work, and to go away
once nothing is planned.

## M27 - candidate decks (deck ecosystem)

**Planned.** A parking lot of deck ideas - third-party service and integration
clients that ship as **decks** (installed with `jvc`, M26) rather than core
`modules/`. The rule: core stays *general primitives* (protocols, formats,
infrastructure - `http`, `graphql`, `csv`); a client for one *specific vendor or
service* lives in the ecosystem as a deck (independently versioned,
community-maintainable, so vendor API churn never touches the core). This list is
demand-driven and open-ended, a collection not a commitment.

Most are thin clients over `http` / `rest` + `json` (plus `xml` where a vendor
returns XML, and the `graphql` module where the API is GraphQL). Each is a
login / token step, a generic `call(path, params) -> json.Value`, and a handful of
conveniences; a fat typed wrapper is explicitly **not** the plan - these APIs are
enormous and firmware-versioned, so a thin client ages far better.

### M27.1 - self-hosted infrastructure

A LAN appliance, usually a **self-signed** cert. Per-vendor maturity differs and
should set the order, not the vendor:

- **`routeros`** (**Done**,
  [deckyard/deck-routeros](https://github.com/deckyard/deck-routeros)) - a
  *full* RouterOS abstraction layer (MikroTik), well beyond the small bundled
  `mikrotik.j` API client.
- **`proxmox`** (**Done**,
  [deckyard/deck-proxmox](https://github.com/deckyard/deck-proxmox)) - Proxmox
  VE: documented REST / JSON, API-token header auth (`PVEAPIToken=...`). Clean.
- **`vmware`** (**Done**,
  [deckyard/deck-vmware](https://github.com/deckyard/deck-vmware)) - vCenter /
  vSphere (vCSA): the vSphere Automation **REST** API (JSON, session auth),
  clean and tractable like Proxmox. A standalone **ESXi**
  host is the messier case - historically only the SOAP / VMOMI Web Services API
  (XML / SOAP, heavier), with just partial REST on 8.x - so vCenter is the target,
  a bare ESXi host best-effort.
- **`virtualbox`** (**Done**,
  [deckyard/deck-virtualbox](https://github.com/deckyard/deck-virtualbox)) - Oracle VirtualBox 7.x
  through the `VBoxManage` CLI, installed with every VirtualBox. Drives it
  through `os.run` with an argument list (no shell), so capability `exec`,
  default binary only. The SOAP web service is a separate deck
  (`virtualboxws` below): capabilities are per deck, and one deck with both
  backends would make every consumer grant `exec` and `net`.
  - **Remote hosts:** an optional command prefix (`["ssh", "user@host"]`)
    runs `VBoxManage` on another machine. `ssh` joins its arguments into one
    remote shell command, so with a prefix every argument is quoted for the
    remote `sh`. The guest-password verbs are refused over a prefix in v1:
    the password file would have to exist on the remote host.
  - **Parsing:** `--machinereadable` output (`key="value"` lines, indexed keys
    such as `"SATA-0-0"`, `natnet1`, `Forwarding(0)`) into typed structs;
    `list -l` blocks for the inventories. The binary path is configurable;
    `VBoxManage --version` is checked once and older majors refused.
  - **v1 verbs:** inventories (`vms`, `runningvms`, `ostypes`, `hdds`,
    `hostonlyifs`, `natnets`, host info); `info` (`showvminfo`); create /
    register / unregister / delete (`createvm`, `modifyvm` for CPU, memory,
    firmware, NICs, `storagectl` / `storageattach`, `createmedium`); power
    (`startvm` headless / gui / separate, `controlvm` poweroff,
    acpipowerbutton, pause, resume, reset, savestate); NAT port forwards;
    snapshots (take, list, restore, delete); `clonevm` (full or linked);
    OVA / OVF `import` / `export`; guest properties (the guest's IP from
    `/VirtualBox/GuestInfo/Net/N/V4/IP`, wait for one); `guestcontrol` run,
    copyto, copyfrom (needs Guest Additions); shared folders. `run(args)`
    reaches any other subcommand.
  - **Security:** VM names, UUIDs, paths and options are validated before
    they reach the argument list; a guest password goes through
    `--passwordfile` pointed at a named pipe (mode 0600, in a private
    directory) instead of `--password`, which `ps` would show, so it never
    touches the disk; stderr is cut and cleaned in error messages. Over a
    prefix the remote login shell must be POSIX sh; `\` and `!` are refused.
  - **Errors:** one kind `virtualbox`, with `VBoxManage`'s exit code and its
    `VBOX_E_*` / `E_*` code parsed from stderr where present. Long
    operations (import, clone, export) get a deadline.
- **`virtualboxws`** (**Done**,
  [deckyard/deck-virtualboxws](https://github.com/deckyard/deck-virtualboxws)) - the
  VirtualBox web service `vboxwebsrv`: SOAP over HTTP(S) to the full Main
  API, for a remote host without `exec`. Capability `net` only, over `http`
  and `xml`. Off by default on a VirtualBox install and rarely deployed, so
  it waits for a user; `virtualbox` over `ssh` covers most remote setups.
  - **Session:** `IWebsessionManager.logon` returns a managed-object
    reference for `IVirtualBox`; every reference the deck obtains is
    released (`IManagedObjectRef.release`) and the session logged off, also
    on error. TLS options and a timeout as in the other HTTP clients.
  - **Calls:** request bodies built from the WSDL (`vboxweb.wsdl` of the
    7.x SDK), one function per used method, `call(method, args)` for the
    rest. Long operations return `IProgress`, polled to completion with a
    deadline.
  - **Scope:** the `virtualbox` deck's verbs where the API has them (VM
    lifecycle, power, snapshots, clone, import / export, guest properties);
    shares its domain shapes by convention, not by code (a facade over both
    decks, as `cloudstorage` is for storage, only on demand).
  - Errors: one kind `virtualboxws` with the SOAP fault's result code
    (`VBOX_E_*`). Plain http only to this machine unless allowed; replies
    capped at 1 MiB. Floor `>=0.26.0`, `net`.
- **`synology`** (**Done**,
  [deckyard/deck-synology](https://github.com/deckyard/deck-synology)) - DSM:
  the cleanest NAS API - a documented Web API (`SYNO.API.Auth` login ->
  session `sid`, JSON responses).
- **`docker`** (**Done**,
  [deckyard/deck-docker](https://github.com/deckyard/deck-docker)) - the
  Docker Engine API (Podman serves the same): containers
  (create, start, stop, kill, remove, wait, logs, one-shot stats), images
  (pull, tag, remove, build), networks, volumes, exec and system (info,
  version, ping, prune). Documented, versioned REST / JSON (`/v1.NN/...`),
  so a thin client is clean; transport is the blocker. The daemon's home is
  the Unix socket `/var/run/docker.sock`, which needs M28.33 plus `http` over
  a Unix socket (out of scope there, no milestone yet). Its remote TLS port
  (2376, `--tlsverify`) wants a client certificate, which `http.TlsOptions` /
  `net.TLSOptions` cannot present (no milestone yet either). Until then the
  deck takes an address: `tcp://` (2375; plain and root-equivalent, so
  localhost or an SSH tunnel only) or `https://` through a socket proxy with
  a token, and `unix://` slots in later without an API change. `http` reads
  a whole response, so live streams (`logs?follow`, `events`,
  `stats?stream`) wait for M28.29 (streaming responses); the one-shot
  forms and the multiplexed log framing (8-byte headers) work over `bytes`,
  and `build` sends its tar context from `archive`. Exec output needs the
  hijacked connection of `exec/start`; detached exec works over plain
  `http`. Compose is client-side, not in the Engine API.
  - **Built:** the API version negotiated per client from `/version`
    (1.41 to 1.56; Podman's compatible service reports less, 1.44 in 6.2),
    `withApiVersion` to pin it; `tcp://` refused off loopback unless
    `withPlainHttp`, `https://` proxies with a token or Basic, `unix://` a
    clear error until the transport exists. `DockerError{status, detail}`,
    generic kinds passed on; 16 MiB JSON and 64 MiB stream caps. Validation
    refuses only what the daemon or kernel would: label keys and values up
    to 4096 characters (multi-line values allowed), env names without `=`,
    signals 1 to 64. `stop` / `restart` with -1 read the container's own
    `StopTimeout`. A Podman pull, which reports no digest, is followed by
    one image inspect.
  - **Validated** (2026-11-14) live against Docker 29.9.3 and Podman 6.2.0
    with every verb, failures included (409s, missing tags, failed build
    steps, a 3.7 MB log); fixed there: `waitExec` hanging on Podman, an
    unencoded `SIGRTMIN+3`, IPv6 registry hosts refused. A 3.7 MB log
    takes about 8 s, 6.5 s of it in `http`'s chunked decoding (about
    55 us per chunk), the place to speed it up.
- **`unraid`** (**Done**,
  [deckyard/deck-unraid](https://github.com/deckyard/deck-unraid)) - the
  official API is **GraphQL** at `/graphql` (Unraid 7.2+, earlier with the
  Connect plugin), `x-api-key` auth. Over the `graphql` module: system and
  metrics, the array, disks, shares and parity checks, Docker containers,
  VMs, notifications, logs, API keys, flash backup and rclone remotes.
  Live updates (GraphQL subscriptions over `graphql-ws`) wait for the
  subprotocol to be confirmed on a server; `websocket.connectOptions` can
  already offer it.
- **`jellyfin`** (**Done**,
  [deckyard/deck-jellyfin](https://github.com/deckyard/deck-jellyfin)) - the
  self-hosted media server's REST API, 10.9-and-later routes (checked
  against the 12.4.0 OpenAPI description): sign-in with a password or an
  API key (`withUser` for the per-user calls) and a stable device id;
  libraries, items with paging, search, latest, resume, next up, episodes,
  played and favourite marks, images; server info, users, library and item
  refresh, scheduled tasks, sessions with messages and playback commands;
  `call` for the rest. Tested offline against a fake over `httpd` and live
  against Jellyfin 12.4.0 in a container. Floor `>=0.26.0`, `net`.
- **`frigate`** (**Done**,
  [deckyard/deck-frigate](https://github.com/deckyard/deck-frigate)) - the
  Frigate NVR HTTP API, checked against 0.18.2 and its OpenAPI description:
  sign-in on port 8971 (the JWT from the login cookie, sent as a bearer
  token) or the unauthenticated port 5000; events with filters, snapshots,
  thumbnails and clips, manual events, sub labels and retention; review
  items and reviewed marks; recordings, hourly summary, clips and frames,
  storage; exports (start, wait, download, delete); config, stats, the
  latest frame, labels, camera feature switches, restart; `call` / `fetch`
  for the rest. The live feed stays MQTT (the `mqtt` module). Tested
  offline against a fake over `httpd` and live against Frigate 0.18.2 in a
  container. Floor `>=0.26.0`, `net`.
- **`onvif`** (**Done**,
  [deckyard/deck-onvif](https://github.com/deckyard/deck-onvif)) - IP
  cameras and NVRs over ONVIF:
  SOAP 1.2 web services, one client for every vendor that implements the
  profiles (S streaming, T advanced streaming, G recording). Over the bundled
  `soap` module plus `http`, `xml`, `hash` and `net`. Capability `net`; floor
  `>=0.26.0` (`soap` and HTTP Digest in `http` ship there).
  - **Discovery:** WS-Discovery `Probe` to `239.255.255.250:3702` over a plain
    `net.listenUDP` socket; cameras answer by unicast with `ProbeMatches`
    (device service address, scopes such as name and hardware). No multicast
    join is needed; `net.bindToDevice` picks the interface on a multi-homed
    host.
  - **Auth:** WS-Security `UsernameToken` with `PasswordDigest`
    (`base64(sha1(nonce + created + password))`), a fresh nonce and `Created`
    per request, so the header block is built per call rather than once on
    the `soap.Client`. The camera's clock decides whether a digest is fresh:
    `GetSystemDateAndTime` (unauthenticated) gives the offset applied to
    `Created`. HTTP Digest (RFC 7616, in `http`) for cameras that also want
    it: on the snapshot URL, a byte download, the deck answers the `401`
    with `http.digestAuthorization` itself; on the SOAP endpoint
    `soap.withDigestAuth(c, user, password)` answers it (through
    `http.sendDigest`).
  - **Services:** Device (`GetDeviceInformation`, `GetServices` /
    `GetCapabilities` to find the other endpoints, time, reboot); Media and
    Media2 (`GetProfiles`, `GetStreamUri` for the RTSP URL, `GetSnapshotUri`
    and a snapshot fetch as `bytes`); PTZ (continuous, absolute and relative
    moves, stop, presets); Imaging (brightness, focus); Events through a
    pull-point subscription (`CreatePullPointSubscription`, `PullMessages`,
    renew and unsubscribe), so no callback server is needed. `call(service,
    action, body)` reaches any other operation.
  - **Not in scope:** RTSP or video decoding (the deck returns stream URLs;
    `frigate`, `ffmpeg` or a player consume them), base-notification push
    events (need a listening HTTP endpoint), Profile M analytics.
  - Errors: one kind `onvif`, carrying the SOAP fault's code and subcode
    (`ter:NotAuthorized`, `ter:InvalidArgVal`, ...). Tested against a
    loopback fake replaying recorded exchanges from two vendors (vendors
    differ in namespaces, optional fields and which auth they demand); a
    real camera only for an opt-in integration test.
  - **Validated** (2026-11-14) against an ONVIF simulator in a container
    (every verb, a NAT setup) and raw captures from Axis, Bosch and Reolink
    cameras; the PasswordDigest matches the ONVIF Application Programmer's
    Guide vector. Fixed there: camera text returned as data was cut at 120
    characters and lost every non-ASCII character. No camera answered a
    multicast probe on this LAN; unicast probes work. Decided after it:
    addresses are rebased only when the reported host differs (a same-host
    port is kept), `streamUri` returns `Stream{address, rebased}`, a
    snapshot URL's user info is dropped for the client's own credentials,
    PTZ moves name the generic spaces, the auth method (`Device.auth`) is
    settled by `connect` and Digest tried only on a Digest challenge (one
    failed login per call), `pull` returns `Pulled` with the renewed
    termination time, no `mustUnderstand` (non-WS-Security cameras must
    ignore the header to reach Digest). Requests go through `http` with
    `soap`'s envelope helpers, because `soap.send` drops reply headers.
- **`homeassistant`** - Home Assistant, the largest self-hosted automation
  platform; documented APIs, a long-lived access token for auth. Two halves:
  - **REST** (`/api/...`, `Authorization: Bearer`): states (read, set),
    services (`/api/services/<domain>/<service>`, the way anything is
    switched or triggered), history and logbook, events (fire), template
    rendering and config checks. Plain `http` + `json`, works today.
  - **WebSocket** (`/api/websocket`): authenticates with a first `auth`
    message, not a header, then `subscribe_events` / `subscribe_trigger` for
    live changes, `call_service`, `get_states` and the registries (areas,
    devices, entities) the REST API lacks. Over core `websocket`; messages
    carry ids, so the deck matches replies to requests while events
    interleave. Core `websocket` has no way to trust a self-signed
    certificate on `wss://`, which a local install often uses (plain
    `ws://` on port 8123, or a public certificate, work).
  - Publishing entities is MQTT discovery (the `mqtt` module), a pattern for
    the docs rather than deck code.

- **`hoymiles`** (planned) - Hoymiles microinverters through the
  manufacturer's DTU gateway (DTU-WLite, DTU-Lite-S, DTU-Pro and their
  successors) over its local protocol, no cloud and no account. The protocol
  is the one [`hoymiles-wifi`](https://github.com/suaveolent/hoymiles-wifi)
  (MIT) reverse-engineered, which the maintainer's own
  [hoymetrics](https://github.com/mplx/hoymetrics) exporter already runs
  around the clock against an HM-800: TCP port 10081, one request per
  connection, a 10-byte header (`HM`, a two-byte command tag, a sequence
  number, a CRC-16/MODBUS over the body, the length) and a protobuf body;
  newer DTU firmware encrypts the body with AES-128-GCM, key and nonce derived
  by triple SHA-256 from the DTU's `enc_rand` and the tag and sequence (core
  `crypto` and `hash` cover both). The body is built and read with the
  M28.61 wire library from the field numbers in `hoymiles-wifi`'s `.proto`
  files; no `protobuf` schema deck needed.
  - **Read:** live data (DTU and per-inverter power, voltage, frequency,
    temperature, today's and total energy; per panel input power, voltage,
    current and energy), DTU and inverter information and firmware, network
    info, alarms, and the day's power history. A `Reading` struct with a
    list of `Port`s, so a program like hoymetrics (CSV, Prometheus through
    the `prometheus` deck) is a few lines.
  - **Control:** the inverter power limit (permanent and temporary, the
    zero-export case), inverter on / off and reboot, DTU reboot. Writes are
    explicit calls, never retried.
  - **Pacing:** the DTU misbehaves when polled too fast; `hoymiles-wifi`
    keeps at least 2 s between requests and hoymetrics' minimum interval is
    10 s. The client enforces a minimum gap between requests (default 2 s,
    settable upward, never below it), and documents a sensible poll
    interval for a 24-hour poller.
  - **Robustness:** a reply is capped (scaled with the inverter count, and
    measured against the maintainer's DTU before it is fixed) and must carry
    the request's tag, sequence and a matching CRC; an inverter that is
    offline at night is data (`online` false, zero power), not an error.
    Errors are one `HoymilesError{kind: "hoymiles", message, ...}`, generic
    kinds passed on. Plain TCP on the LAN is the device's only interface,
    so no plain-network opt-in is needed beyond refusing a public address
    unless allowed.
  - Tested in CI against exchanges recorded from the maintainer's DTU and
    HM-800 (encrypted and plain where available), and live on the LAN.
    Floor at the release that ships M28.61, capability `net`. **Requires:**
    M28.61.
- **`midea`** (planned) - Midea-family appliances (Midea, Comfee and the
  other brands built on Midea's Wi-Fi module) over the local LAN protocol, starting
  with the two device types the maintainer owns: the air conditioner type
  `0xAC` (a Midea PortaSplit) and the dehumidifier type `0xA1` (a Comfee
  MDDF-16DEN7-WF). The protocol is the one the community clients
  (`msmart-ng`, `midea-beautiful-air`, `midea_ac_lan`) implement: UDP
  broadcast discovery on ports 6445 / 20086, then the V3 "8370" protocol on
  TCP 6444 (a handshake with the device token, then AES-framed packets with a
  SHA-256 signature) around the classic AES-128-ECB + MD5 inner frame with
  the device-type command sets. Core `crypto` (`aes-ecb`, `aes-cbc`) and
  `hash` cover it.
  - **Local first, cloud once.** Day-to-day control needs only the device's
    token and key, which stay valid until the device is paired again; the
    deck takes them as settings and works offline. Getting them needs one
    Midea cloud login, which Midea changes and locks down from time to
    time, so it is a separate, explicitly fragile call
    (`fetchKeys(account, password, deviceId)`) that the docs say may break,
    and that nothing else depends on.
  - **Air conditioner (`0xAC`):** power, mode, target temperature, fan
    speed, swing, eco / turbo where the model reports them, indoor and
    outdoor temperature. The PortaSplit's exact feature set is checked
    against the community code and the device's capability reply, and
    unknown capabilities are refused rather than guessed.
  - **Dehumidifier (`0xA1`):** power, mode, target humidity, fan speed,
    current humidity, tank-full and error flags.
  - **Other types** are out of v1; the frame layer is shared, so each new
    type is a command set and its parser.
  - Errors: one `MideaError` struct; a wrong token or key is its own reason,
    so a program can tell "re-fetch the keys" from "device offline". Tested
    against exchanges recorded from the maintainer's two devices and live on
    the LAN; the cloud fetch is checked by hand only. Floor `>=0.26.0`,
    capability `net`.

### M27.2 - public / SaaS APIs

- **`github`** (**Done**,
  [deckyard/deck-github](https://github.com/deckyard/deck-github)) - the
  GitHub REST API (version `2022-11-28`) and GraphQL, github.com or GitHub
  Enterprise Server, token auth, in the forgejo deck's design: repositories,
  collaborators and webhooks; issues, comments, labels, milestones; pull
  requests, reviews, merging; releases and assets; branches, tags, files;
  commits, statuses, check runs; Actions dispatch and runs, variables and
  secrets (`setSecret` seals with `crypto.wrap("nacl-sealedbox")`,
  M28.58); `call` / `getAll` / `graphql` for the rest. Tested offline
  against a fake over `httpd`, live with anonymous reads. Floor
  `>=0.26.0`, `net`.
- **`gitlab`** (**Done**,
  [deckyard/deck-gitlab](https://github.com/deckyard/deck-gitlab)) - the
  GitLab REST API v4 and GraphQL, gitlab.com or self-managed, token auth, in
  the forgejo deck's design: projects, groups, members, hooks, forks;
  issues, notes, labels, milestones; merge requests, reviewers and
  approvals; releases with links and uploads to the generic package
  registry; branches, tags, files; commit statuses; pipelines (run, retry,
  cancel, trigger) and CI variables; `call` / `getAll` / `graphql` for the
  rest. Follows redirects itself, so its token never reaches another host
  (M28.59). Tested offline and live against GitLab CE 19.5.5 in a
  container. Floor `>=0.26.0`, `net`.
- **`codehost`** (**Done**,
  [deckyard/deck-codehost](https://github.com/deckyard/deck-codehost)) -
  one API over the code forges, so a release script or a triage bot runs
  against GitHub, GitLab and Forgejo (and Gitea) alike. Common denominator
  only, as `cloudstorage` is for storage: what one forge alone has (GitHub
  Actions and secrets, GitLab pipelines, groups and approvals, webhooks,
  GraphQL) stays in the backend decks, reachable through the `Forge`.
  - **Shape:** a `Forge` holds the backend name and that backend's own
    typed client in a field of its type. `connect(backend, host, token)`
    builds the client inside the deck, so a program imports only codehost
    (jvc installs the backends transitively) and can take the backend from
    a configuration file; `fromGithub(c)`, `fromGitlab(c)`,
    `fromForgejo(c)` wrap a client built with its own settings (CA,
    timeouts, retries, plain http). Every verb switches on the backend name.
  - **Verbs:** repositories (`repository`, `myRepositories`); issues (list,
    get, create, close / reopen, comments, labels: set, add, remove);
    change requests, one `ChangeRequest` for a pull or merge request (list,
    get, create with drafts, close / reopen, merge, comments); releases
    (list, get, latest, create, upload and download an asset); branches and
    tags (list, create, delete); files (`readFile`, `writeFile`,
    `deleteFile`, `listDirectory`); commit statuses (set, list, newest
    first). `repo` is `owner/name`, or GitLab's full path with subgroups;
    `number` is the issue or change request number (GitLab's `iid`).
  - **Normalised behaviour:** states (`open` / `closed` / `merged`; GitLab
    `opened` / `locked` are `open`); commit-status states both ways
    (GitLab `running` -> `pending`, `failed` -> `failure`, `canceled` ->
    `error`; Forgejo `warning` -> `success`, kept in `forgeState`); drafts
    by each forge's own title prefix (GitLab `Draft:`, Forgejo / Gitea
    `WIP:`), a title the forge reads as a draft refused with `isDraft`
    false; `mergeable` only when open and not a draft; labels created where
    missing (Forgejo organisation labels respected); release assets
    (GitLab's generic-package upload plus a release link; the token only to
    the forge's own address); directory entry types (`file`, `dir`,
    `symlink`, `submodule`; GitHub from the directory's git tree, since its
    contents listing calls symlinks and submodules files).
  - **Errors:** one `CodehostError{kind: "codehost", message, status,
    backend}` rebuilt from `GithubError`, `GitlabError` and
    `ForgejoError`; `"limit"`, `"timeout"`, `"closed"` and `"cancelled"`
    pass through.
  - **Depends on** `@deckyard/github` and `@deckyard/gitlab` `>=0.1.0` and
    `@deckyard/forgejo` `>=0.2.1` (`[decks]`, vendored from the registry).
    One conformance suite runs every verb against a loopback fake of each
    backend; checked live read-only on github.com, gitlab.com and
    codeberg.org, and every write verb against GitLab CE, Forgejo 13 and
    Gitea 1.24 containers.
- **`steam`** (**Done**,
  [deckyard/deck-steam](https://github.com/deckyard/deck-steam)) - the Steam
  Web API (JSON, `?key=` auth); parts of the surface are
  community-reverse-engineered, which is why it belongs in a deck, not core.
- **`themoviedb`** (**Done**,
  [deckyard/deck-tmdb](https://github.com/deckyard/deck-tmdb)) - TMDB's clean,
  well-documented REST JSON API (bearer / key auth).
- **`mailgun`** - the Mailgun sending API (free plan: 100 messages a day; it
  replaces SendGrid here, whose free plan ended in 2025): Basic auth
  (`api:<key>`), the US or EU region (`api.mailgun.net` /
  `api.eu.mailgun.net`), `POST /v3/<domain>/messages` as
  `multipart/form-data` (the bundled `multipart` module) with attachments,
  inline images, tags, custom variables and stored templates; the events
  log paged by its `next` URL, suppressions (bounces, complaints,
  unsubscribes), domains. Webhook verification is HMAC-SHA256 of
  `timestamp + token` under the webhook signing key, compared in constant
  time (`hash.hmac`, `hash.equal`), with a maximum age against replays. The
  free plan's sandbox domain (only authorised recipients) makes it testable
  without a sending domain. No core gap.
- **`mailtrap`** - Mailtrap's two APIs, both on its free plan: **Email
  Sending** (`send.api.mailtrap.io/api/send`, bulk stream on
  `bulk.api.mailtrap.io`; bearer token, JSON with base64 attachments,
  categories, templates by UUID with variables) and **Email Testing**, a
  sandbox that catches mail instead of delivering it
  (`sandbox.api.mailtrap.io/api/send/<inbox>`, also reachable by SMTP).
  The testing half also reads what was caught (inboxes, messages, their
  HTML, text, raw source and headers), so other mail code (the bundled
  `smtp` module, `mailgun`) can be checked end to end in CI. No core gap.
- **`brevo`** - Brevo's API v3 (`api.brevo.com/v3`, `api-key` header; free
  plan: 300 messages a day). Two halves: transactional mail (`POST
  /smtp/email` with HTML / text, base64 or URL attachments, stored templates
  with `params`, tags, scheduling; events and statistics) and the marketing
  side a Mailchimp user needs: contacts with attributes, lists and folders,
  double opt-in, and email campaigns (create, send now, schedule, report).
  Paging by `limit` / `offset`. No core gap.
- **`resend`** - the Resend API (`api.resend.com`, bearer key; free plan:
  3,000 messages a month, 100 a day): `POST /emails` (HTML / text, base64
  or URL attachments, tags, headers, `scheduled_at`) with an
  `Idempotency-Key`, batches of up to 100, reading, rescheduling and
  cancelling a scheduled message; domains with their DNS records and
  verification; API keys; contacts and broadcasts. Webhooks are signed the
  Svix way: HMAC-SHA256 over `<id>.<timestamp>.<body>` with the base64
  secret after `whsec_`, compared in constant time against each signature
  in `svix-signature`, with a maximum age. Backs off on `429`. No core gap.
- **`vies`** (**Done**,
  [deckyard/deck-vies](https://github.com/deckyard/deck-vies)) - EU VAT
  number validation through the European Commission's VIES service, the
  check a seller makes before invoicing a business customer in another
  member state without VAT (reverse charge). Over the bundled `soap` module
  (VIES is SOAP 1.1, `checkVatService`; `soap.send`, so a fault comes back
  as data). Capability `net`; floor `>=0.26.0`.
  - **Calls:** `check(c, country, number)` -> `Result{valid, name,
    address, requestDate}` (`checkVat`; VIES's `---` for an undisclosed
    name or address becomes `""`), and `checkApprox(c, country, number,
    requesterCountry, requesterNumber)` (`checkVatApprox`) -> `ApproxResult`
    with the consultation number (`requestIdentifier`) to keep as proof.
    `client()`, `testClient()` (the Commission's test service; syntax
    checks and retries off), `withEndpoint` (plain http only on loopback),
    `withTimeout`, `withRetries`.
  - **Input:** `normalize` / `parse` (spaces, dots, hyphens and a leading
    country prefix removed, upper case, `GR` taken as `EL`) and a syntax
    check per country (`isWellFormed`, 27 member states plus `XI`) before
    any request, so an obviously wrong number fails offline. No check-digit
    algorithms; VIES's answer is the authority.
  - **Faults:** a `ViesError{code, transient, status}` (kind `vies`, a
    struct per the error rules) with VIES's `faultstring` code; the
    `*_UNAVAILABLE`, `TIMEOUT` and concurrency codes are `transient` and
    retried with doubling backoff first, each wait at most 30 s. A number VIES does not know is
    `valid = false`, not an error. No caching (a check is proof only for
    its date). VIES sends its faults with HTTP 200.
  - Tested against a loopback fake with replies recorded from the live
    service and the test service; checked live against both. Timeouts and
    closed connections pass through as `"timeout"` / `"closed"` since the
    bundled `soap` passes them on (M28.60).
- **`oauthext`** (**Done**,
  [jennifer-language/deck-oauthext](https://github.com/jennifer-language/deck-oauthext)) -
  vendor-specific OAuth2 on top of core `oauth`. Core `oauth` keeps only the
  standard mechanics (the grants and token handling); anything
  that encodes one provider's endpoints, key formats, or claim conventions lives
  here, so a new provider (Oracle, Salesforce, Okta, ...) is a deck change, never
  a core one. Three parts:
  - **Move the provider presets out of core.** `oauth.google` and
    `oauth.microsoft` (endpoint-URL presets returning a `Config`) move here as
    `oauthext.google` / `oauthext.microsoft`, and core `oauth` drops them. A
    **breaking change** for programs that call the presets (in the repo:
    `examples/modules/oauth_demo.j` and `docs/modules/oauth.md`), migrated by
    importing the deck and renaming the prefix, or by writing the two URLs into
    a `Config` directly.
  - **Builds on the generic RFC 7523 JWT-bearer grant in core `oauth`**:
    `oauth.jwtBearer(config, assertion) -> Token` (the grant, a sibling of
    `clientCredentials`) and `oauth.jwtAssertion(issuer, subject, audience,
    lifetime, extra, key, alg, kid) -> string` (the standard claim set - `iss` /
    `sub` / `aud` / `iat` / `exp` - plus a provider's `extra` claims, signed
    through `jwt.signWithKid`). Both are provider-neutral, so they serve any RFC
    7523 provider without a preset.
  - **`oauthext.googleServiceAccount(keyJson, scope) -> Token`** - reads a Google
    service-account key file (`client_email`, `private_key`, `private_key_id`,
    `token_uri`), builds the assertion with Google's conventions (`kid` = the key
    id, `aud` = the token URI, `scope` in the `extra` claims, one-hour
    lifetime) through `oauth.jwtAssertion`, and redeems it with
    `oauth.jwtBearer`. Opens every Google Cloud API (Cloud
    Storage, BigQuery, Pub/Sub, Workspace) to `.j`, and is what a `firebase`
    deck would authenticate with (an idea in [horizon.md](horizon.md)).
    RS256 signing is default-binary only, which costs nothing here: `oauth`
    already needs the default binary through `http`.
- **`openai`** / **`anthropic`** / **`gemini`** (**Done**,
  [deckyard/deck-openai](https://github.com/deckyard/deck-openai),
  [deckyard/deck-anthropic](https://github.com/deckyard/deck-anthropic),
  [deckyard/deck-gemini](https://github.com/deckyard/deck-gemini)) - LLM clients, **one deck per
  vendor API**, not one generic `llm` deck. The wire formats differ in every
  part a client touches: OpenAI's Chat Completions takes a `system` message in the list and
  returns `choices[].message` with `tool_calls` whose arguments are a JSON
  *string*; Anthropic's Messages API takes `system` as a top-level field,
  requires `max_tokens`, and returns typed content blocks (`text`, `tool_use`,
  `thinking`) with tool input as an object; Gemini takes `contents` of
  `parts` with roles `user` / `model`, a top-level `systemInstruction`, and
  returns `candidates[].content.parts` with `functionCall` arguments as an
  object. Auth, headers, error shapes and the streaming event vocabularies
  differ too. A common facade would expose
  only what both share and hide what each does well (Anthropic's prompt
  caching, extended thinking and citations; OpenAI's structured outputs;
  Gemini's long-context media and context caching), and it would break
  whenever any vendor moves. Thin clients follow the M27 rule; a
  provider-neutral `llm` facade over them is planned as its own deck
  (below), after the three are released.
  - **`openai`** targets the **OpenAI-compatible** API, so a configurable base
    URL covers the many servers that implement it: Ollama, vLLM, llama.cpp
    server, LM Studio, OpenRouter, Groq, Mistral, Together. Bearer auth
    (optional `OpenAI-Organization` / `OpenAI-Project`); Chat Completions with
    tools and `response_format` structured output; embeddings; model listing.
    Azure OpenAI is a config variant (`api-key` header, deployment in the
    path, `api-version` query). The newer Responses API is a later addition.
  - **`anthropic`** - the Messages API: `x-api-key` + `anthropic-version`
    headers, text and base64 image content blocks, tool use (`tool_use` /
    `tool_result` blocks, tools declared with an `input_schema`), extended
    thinking, prompt caching (`cache_control`), token counting, and the
    Message Batches API. Retries back off on `429` and `529` (overloaded),
    honouring `retry-after`.
  - **`gemini`** - the Gemini API as Google AI Studio serves it
    (`generativelanguage.googleapis.com/v1beta`), with an AI Studio API key
    in the `x-goog-api-key` header; the free tier needs no billing account
    and answers over its rate limits with `429 RESOURCE_EXHAUSTED`, so
    retries back off. `models/<model>:generateContent` with text and inline
    base64 media parts, `systemInstruction`, function calling
    (`functionDeclarations`, `functionCall` / `functionResponse` parts),
    structured output (`responseMimeType` + `responseSchema`), thinking
    (`thinkingConfig`) and safety settings; `countTokens`; embeddings
    (`embedContent`, `batchEmbedContents`); model listing; the Files API for
    media too large to inline (a resumable upload, then a `fileData` part);
    context caching (`cachedContents`). Errors are Google's
    `{"error": {"code", "message", "status"}}`. Google also serves an
    OpenAI-compatible endpoint (`.../v1beta/openai/`), which the `openai`
    deck reaches by base URL; this deck is for the native features that
    endpoint leaves out. Vertex AI (OAuth through
    `oauthext.googleServiceAccount`, regional endpoints) is a later
    config variant.

  The decks share a shape: `chat(client, request) -> json.Value` over a
  request built with `json`, a typed error per vendor error object, and a
  small **tool loop** helper (send, run each requested tool through a `func`
  value, append the results, repeat until the model stops). Tool and
  structured-output definitions are JSON Schemas, so the `jsonschema` deck
  (M27.7) can validate the model's arguments before a tool runs. The `mcp`
  module's tool definitions use the same schemas, so its tools can be offered
  to a model. Long generations need a generous `http.requestWith` idle
  timeout. Tested offline against a fake server written in `.j` (over
  `httpd`) replaying recorded responses, so CI needs no API key. **Out of
  v1:** streaming. All three APIs stream over server-sent events, which `http`
  cannot read incrementally yet; M28.29 adds streaming `http` and an `sse`
  module, after which streaming is a deck addition.
  - **Built and validated** (2026-11-14 / 15). The three share one surface,
    aligned in three rounds: `newRequest`, `withSystem`, `addUserText`,
    `addReply`; `tool` / `addTool` / `addTools`, `toolError`; `runTools` /
    `runToolsWith` returning `ToolRun{reply, request, turns, calls, usage,
    stopped}` (a `maxTurns` stop returns the conversation instead of
    throwing); `ToolCall{id, name, input}`; `Usage{inputTokens` (cache
    included), `outputTokens, totalTokens, cachedTokens, reasoningTokens}`;
    `call(c, method, path, body)`; `withMaxToolInput` (1 MiB); errors with
    `status`, `retryAfterMs` and the vendor's rate-limit headers in
    `limits`. One retry rule: 408, 409, 429, 5xx and refused connections,
    `retry-after-ms` / `retry-after` (seconds or HTTP-date), no wait over
    60 s taken, and an answer that broke off mid-way never resent.
    **openai** was tested live against Ollama (chat, parallel tools, strict
    JSON schema, embeddings) and against the openai-openapi spec; Azure v1
    and dated paths offline. **anthropic** and **gemini** were tested
    offline only (no API key): fixtures from the current API references,
    the Anthropic docs for adaptive thinking, effort, refusals and
    fallbacks, and Gemini's thought signatures and resumable upload. Each
    was validated by a separate pass (bugs, key leaks into errors,
    hostile replies: oversized and endless bodies, slow drips, huge
    `retry-after`, endless paging) and profiled. One live call per vendor
    remains to be made when a key is available. The core gaps the
    validation found are M28.65 and M28.66 to M28.71.

- **`llm`** (planned) - a thin provider-neutral facade over the three LLM
  decks above, the way `codehost` sits over `github` / `gitlab` / `forgejo`.
  Possible because the three were aligned on purpose (2026-11-15): the same
  `newRequest` / `withSystem` / `addUserText` / `addReply`, `tool` /
  `addTool` / `addTools`, `toolError`, `runTools` / `runToolsWith`,
  `ToolCall{id, name, input}`, `ToolRun{reply, request, turns, calls,
  usage, stopped}`, `Usage{inputTokens, outputTokens, totalTokens,
  cachedTokens, reasoningTokens}`, one retry rule, `withMaxToolInput`, and
  errors with `status`, `retryAfterMs` and `limits`. So the facade is mostly
  routing.
  - **Surface:** `client(provider, apiKey)` (`"openai"`, `"anthropic"`,
    `"gemini"`, plus `connect(provider, baseUrl, apiKey)` for an
    OpenAI-compatible local server), a neutral request builder, `chat`,
    `text`, `toolCalls`, `usage`, `runTools` / `runToolsWith`, and
    `vendor(c)` / `vendorRequest(req)` to drop down to the vendor deck for
    its own features. One `LlmError` that carries the vendor error's
    fields and the provider name; generic kinds passed on.
  - **The conversation stays in its vendor's format.** Thinking blocks and
    Gemini's thought signatures are only valid for the model that wrote
    them, so the facade never translates a conversation between providers
    and never switches provider mid-conversation; it reads a conversation in
    neutral form (`messages(req) -> list of Message`) but does not rewrite
    one. Starting a fresh conversation on another provider is the caller's
    choice.
  - **Not hidden, not flattened:** prompt caching, batches, effort and
    thinking settings, safety settings, structured-output variants, Azure,
    files and context caching stay in the vendor decks, reached through
    `vendor(c)`. The facade offers only what all three do the same way.
  - **Not an agent framework:** no memory, planning, multi-agent
    orchestration or prompt templating; those are application design and
    move faster than a deck can. The tool loop is the shared core.
  - Tested offline against the vendor decks' own fakes, with one scenario
    (a two-tool loop, a refusal, a rate limit, a cap) run through all three
    providers to the same neutral result.
  - **Requires:** `openai`, `anthropic` and `gemini` released (a facade sees
    only released versions through `[decks]`, as `codehost` did) and checked
    live against each vendor at least once; streaming waits for M28.29 and
    the vendor decks.

- **`koofr`** (**Done**,
  [deckyard/deck-koofr](https://github.com/deckyard/deck-koofr)) - the Koofr
  cloud-storage API v2.1 (OpenAPI spec at
  `https://app.koofr.net/api/v2/swagger.json`); base URL settable, other
  providers run the same API. Auth by app password (Basic) or bearer token.
  File sync stays with `webdav` (`/dav/Koofr`). Covers user, limits, mounts;
  files (`info`, `exists`, `listFolder`, NDJSON `listRecursive`, `mkdir` /
  `mkdirAll`, `get`, `put` with overwrite / autorename / `overwriteIfHash`,
  `remove`, `rename`, cross-mount `copy` / `move`); search; public links and
  receivers; trash; `call` for the rest. **Later:** versions, tags, ranged and
  ZIP downloads, async jobs, mount users, OAuth2 device-code login. Bodies are
  held in memory until `http` streams (M28.29).
- **`seafile`** (**Done**,
  [deckyard/deck-seafile](https://github.com/deckyard/deck-seafile)) - the
  Web API (`api2`, `api/v2.1`) of a self-hosted Seafile server (reference `https://seafile-api.readme.io/`, v13.0). Shape as `koofr`; file
  sync stays with `webdav` (`/seafdav`). Account token (`login`, optional
  2FA; `Bearer`, `Token` before 11.0) or a per-library API token
  (`via-repo-token/*`). Transfers go through `/seafhttp` links that carry
  their own token: no `Authorization` there, and a link on another origin is
  refused unless `withFileServer` allows it. Covers server and account info;
  libraries (incl. encrypted, `decrypt`); `listDir`, detail, `exists`,
  `mkdir`, `get`, `put`, resumable chunked upload, update, `rename`,
  `copy` / `move` across libraries (sync and async batch), `remove`, lock;
  file and folder history and revert; user and group shares, shared-with-me,
  starred items; share and upload links; trash (list, restore, clean);
  library API tokens; search (Pro) and name search; `call` for the rest.
  Seafile auto-renames instead of refusing, so verbs return the stored name.
  Tested live against 13.0 Community Edition in Docker. Out of scope: admin
  API, wiki, notifications, OCM.
- **`onedrive`** (**Planned**, `deck-onedrive`) - OneDrive (personal and
  business) and SharePoint document libraries through the Microsoft Graph
  v1.0 drive API (`/me/drive`, `/drives/{id}`, `/sites/{id}/drive`). Shape as
  `koofr`; items by path (`root:/a/b.txt:`) or id. **Auth:** OAuth2 only, an
  Entra ID app registration; device-code login and refresh through the
  bundled `oauth` module and the `@jennifer/oauthext` deck (`oauthext.microsoft`
  preset, `deviceStart`/`deviceWait`,
  `refresh`, `save`/`load`), app-only `clientCredentials` for business
  tenants; the deck takes a token and refreshes it on 401. **Transfers:**
  `get` follows the 302 of `/content` to the pre-authenticated download URL
  itself, without `Authorization`, and refuses a non-https target; uploads up
  to 250 MB are one `PUT .../content`, larger ones an upload session
  (fragments in multiples of 320 KiB, under 60 MiB, no `Authorization` on the
  upload URL, resume from `nextExpectedRanges`). Name clashes follow
  `conflictBehavior` (`fail` default, `replace`, `rename`). **v1:** client,
  `call`; drive and quota; `listChildren` over `@odata.nextLink` with a page
  cap; item detail, `exists`, `mkdir`/`mkdirAll`, `get` (with `maxBytes`,
  ranged), `put`, `putSession`, rename/move (`PATCH`), async `copy` with the
  monitor URL, `remove`; `delta` with `deltaLink` and the 410 resync codes;
  sharing links (`view`/`edit`/`embed`, scope, expiry, password where the
  account type allows), permissions and invites; search; versions (list,
  restore); special folders (`approot`). Errors: `{"error":{"code",
  "message"}}`; 429/503 honour `Retry-After`. **Unverified:** a v1.0 recycle
  bin API for OneDrive items, the `Files.ReadWrite.AppFolder` scope, the
  device-code tenant for personal accounts (`consumers`). Out of scope: the
  rest of Graph (mail, calendar, Teams), thumbnails, workbook API. Bodies
  held in memory until `http` streams (M28.29). Floor `>=0.26.0`, `net`;
  tested offline against a fake over `httpd`; live tests need a Microsoft
  account and an app registration.
- **`cloudstorage`** (**Planned**, `deck-cloudstorage`) - one file API over
  the storage backends, so an app reads and writes files without binding to
  a provider. Common denominator only: links, shares, trash, versions and
  search stay in the backend decks, which an app imports directly when it
  needs them (the precedent of the deferred LLM facade above).
  - **Shape:** a `Store` holds the backend name and that backend's own typed
    client in a field of its type, set by one constructor per backend:
    `fromKoofr(c, mountId, root)`, `fromSeafile(c, repoId, root)`,
    `fromOnedrive(c, driveId, root)`, `fromWebdav(c, root)`,
    `fromS3(c, bucket, prefix)`. Every verb switches on the backend name.
    Jennifer has no interfaces, unions or closures, so dispatch over typed
    fields is the type-safe form; the cost is that the deck depends on every
    backend deck (`[decks]`: koofr, seafile, onedrive) and consumers vendor
    them all.
  - **Verbs:** `put` (with `onExists`: `fail`, `replace`, `rename`), `get`
    (`maxBytes`), `list` (one folder, `recursive` optional), `stat`,
    `exists`, `mkdirAll`, `remove`, `move`, `copy`. Paths are `/`-separated
    strings below the store's root; one `Entry` (path, name, isDir, size,
    modified, hash or etag when the backend has one); one error kind
    `cloudstorage` with a reason (`notFound`, `exists`, `denied`,
    `tooLarge`, `backend`) and the backend's message.
  - **Normalised behaviour:** name clashes differ (Seafile auto-renames,
    Koofr refuses, OneDrive and S3 overwrite), so `onExists` is mapped to each
    backend's own mode, and `fail` is a check before the upload where the
    backend has none (racy; documented). S3 has no folders: `mkdirAll` is a
    no-op and folders exist through their keys. Size caps pass through as
    `maxBytes`.
  - **Backends:** the koofr, seafile and onedrive decks, plus the bundled
    `webdav` module (Nextcloud, ownCloud, Box and others) and `s3` module
    (MinIO, Cloudflare R2, Backblaze B2).
  - One conformance suite runs against a loopback fake of each backend.
    Ships after onedrive's first release, since its `[decks]` dependencies
    must be released tags. Floor `>=0.26.0`, `net`.

### M27.3 - daily helpers

A grab-bag of small, everyday helper decks - the little utilities a script reaches
for often, each a tiny pure-`.j` deck.

- **cli spinners** (**Done**,
  [mplx/clispinner](https://registry.jennifer-lang.dev/deck/mplx/clispinner)) -
  a small deck of terminal progress spinners for a long-running task: a
  catalogue of spinner styles (dots / line / bar / braille / arc / ...), each
  individually **colourable** (through `ansi`) and driven at its **own speed**
  (frame interval). Self-suppresses when stdout is not a TTY (`os.isTerminal`), so
  piped output stays clean.
- **`download`** (planned) - fetching a file over HTTP(S) the way a script
  or installer should: to a file, resumable, verified, bounded. Native only,
  over core `http`: no `wget` / `curl` / `aria2c` backends, which would force
  the `exec` capability on every user and give a lowest common denominator
  (an `aria2` RPC client would be a deck of its own).
  - `fetch(url, path, options) -> Download{path, size, sha256, resumed,
    source}`: writes to `path.part` and renames when complete; resumes a
    `.part` with a `Range` request when the server supports it
    (`Accept-Ranges`, an `ETag` / `Last-Modified` check so a changed file
    restarts instead of corrupting); retries with backoff on network
    errors and 5xx; mirrors in order; a size cap and a deadline per
    request.
  - **Verification:** an expected `sha256` (or sha512), or a checksum file
    (`SHA256SUMS`, `.sha256`) fetched alongside; optionally a detached
    signature check through core `crypto` where a project signs with
    minisign / signify-style Ed25519 keys. A mismatch deletes the file.
  - **Memory:** `http` reads a whole response until M28.29, so v1 downloads
    in fixed-size `Range` pieces (default 8 MiB) appended to the `.part`
    file, bounding memory and giving resume for free; a server without
    range support is limited to the cap. After M28.29 the body streams to
    disk and the piece mode stays for parallel or resumable downloads.
  - Progress through a callback `func` value (no closures, so state goes
    through `runWith`-style context or top-level variables), and a
    `clispinner`-friendly progress struct. Web seeds of a `.torrent`
    (BEP 19) are a later companion with the planned `torrent` deck
    (M27.7).
  - Errors: one `DownloadError{kind: "download", message, status, url}`,
    generic kinds passed on; credentials only to the origin that was
    asked, never across a redirect to another host. Tested against a fake
    over `httpd` (ranges, missing ranges, changed ETags, truncation,
    lying lengths, slow drips). Capability `net` + `fs`.

### M27.4 - bioinformatics

**Done** ([deckyard/deck-bioinformatics](https://github.com/deckyard/deck-bioinformatics)).

A sequence-manipulation deck for DNA / RNA / protein, modelled on the
[Sequence Manipulation Suite](https://stothardresearch.ca/sequence-manipulation-suite/)
(SMS) tool catalogue - the classic, comprehensive reference for this surface.
Almost all of it is pure string / list / map work, so it is a natural
**pure-`.j` deck** (leaning on `strings`, `regex`, `lists`, `maps`, `math`, and
`stats`, no Go), dogfooding the language and community-maintainable as tables and
algorithms are added. Grouped SMS-style:

- **Transforms** - `complement`, `reverseComplement`, `reverse`, `transcribe`
  (DNA <-> RNA), `translate` (codon table), six-frame translation, `splitCodons`,
  amino-acid `oneToThree` / `threeToOne`.
- **Composition & properties** - `gcContent`, base / amino-acid composition,
  `dnaStats` / `proteinStats` summaries, `molecularWeight` (DNA / RNA / protein),
  `meltingTemp` (Wallace + nearest-neighbour), `isoelectricPoint` (pI), `gravy`
  (hydropathy), `codonUsage`. The thermodynamic / pI / nearest-neighbour formulas
  want `exp` / `ln` / `log`, so they pull in `M24.10` (`math` foundations).
- **Search** - exact and IUPAC-ambiguity pattern find (via `regex` character
  classes), fuzzy search (n mismatches), ORF finder (six frames), CpG islands,
  restriction-site search and `restrictionDigest` (fragments) over a bundled
  enzyme table.
- **Formats** - FASTA parse / write (a `Record{id, description, sequence}` list),
  FASTQ reads (sequence + quality), format conversion, `filterDna` /
  `filterProtein` (strip non-sequence characters).
- **Manipulation** - `randomDna` / `randomProtein` (length + optional
  composition), composition-preserving `shuffle`, point `mutate` (rate), range /
  sliding-window extraction.
- **Comparison** - pairwise `alignGlobal` (Needleman-Wunsch) / `alignLocal`
  (Smith-Waterman) with percent `identity` / `similarity`, plus `hammingDistance`
  / `editDistance`. Alignment is the one `O(nm)` hot loop - fine in `.j` for the
  small sequences a deck user handles; a candidate Go primitive only if
  whole-genome throughput is ever needed.
- **Reference data** - the standard genetic code + alternative codon tables, IUPAC
  ambiguity codes, per-residue property tables (MW / pKa / hydropathy), and a
  common-enzyme table - all plain `.j` maps a community can extend.

FASTA / FASTQ I/O and alignment cover the "molecule structures" the original note
gestured at; PDB / 3-D structure parsing stays out of v1 (a much larger, separate
effort).

- **`abif`** (**Done**, [deckyard/deck-abif](https://github.com/deckyard/deck-abif);
  0.1.0 tagged, the registry publish to retry: it failed on 2026-11-14 with
  Cloudflare's own 502, the registry not answering) - a reader for Applied
  Biosystems'
  ABIF container: Sanger traces (`.ab1`), fragment-analysis runs (`.fsa`) and
  3500-series HID runs (`.hid`). A separate deck because both this deck and
  forensicgenetics (M27.5) consume it. Spec: ABI's published "ABIF File
  Format Specification" (2009). Pure `.j` over the `binary` number readers.
  - **Container:** 128-byte header (`ABIF`, version, directory entry), the
    directory of 28-byte entries (tag, number, element type and size, count,
    data size, offset or inline data), all big-endian; every ABIF element
    type decoded (byte, char, word, short, long, float, double, date, time,
    pString, cString, thumb, bool, user). `tags` / `entries` list,
    `tag(f, name, number)` returns a `Value` enum, `text` / `ints` / `floats`
    read a known type; other types pass through as bytes (`raw`). Offsets,
    counts and sizes are checked against the file before any read; text is
    strict UTF-8. `File` keeps the directory as plain lists, so it crosses
    the module boundary without a rebuild.
  - **Accessors:** raw traces (`DATA 1-4`, `DATA 105+` for a fifth and later
    dye) and analysed traces (`DATA 9-12`) as `list of int` via
    `binary.readInts`; `dyes` (`Dye#`, `DyeN`, `DyeW`); `info` (sample,
    well, instrument, model, run times, dye set, polymer, `StdF`); for `.ab1`
    `calls` / `editedCalls` (`PBAS`, `PCON`, `PLOC` 2 / 1).
  - `.hid` checked against an OSIRIS GlobalFiler sample: ABIF version 300,
    and the traces, peak tables, size map and fit, most dye names and the
    model are element types 28 to 34, undefined in the spec and encrypted
    (8-byte blocks, deterministic). The deck reads them as raw bytes and its
    trace readers refuse them; sample and run metadata read as usual.
  - Fixtures: Biopython's `.ab1` / `.fsa` samples (Biopython License) and
    one OSIRIS `.hid` (public domain), with Biopython's expected values as
    the oracle. Floor `>=0.26.0` (the `binary` readers), no capabilities.
  - Validated against Biopython 1.88 on nine files (1025 tags): the only
    differences are Biopython's (signed `byte` and thumb bytes, where the
    spec and seqinr say unsigned; a two-element bool collapsed to one) and
    the deck's refusal of a non-UTF-8 comment. An independent review's
    findings (empty cString, lookup-key collision, `info` / `dyes` on
    non-integer fields) are fixed with tests.

### M27.5 - forensic / statistical genetics

**Done** ([deckyard/deck-forensicgenetics](https://github.com/deckyard/deck-forensicgenetics)).

The statistical-genetics sibling of the sequence deck (M27.4): it works on
**profiles** (an unordered allele pair per autosomal STR locus, plus uniparental
**haplotypes** for mtDNA / Y-STR) and reference frequency / count data, not on
sequences, so it shares no code and no audience with SMS. Pure `.j` (probability
arithmetic over allele-frequency maps, no Go, TinyGo-clean), leaning on `math`
and on `xml` / text parsing to ingest a published frequency table.

- **Match probabilities** - Hardy-Weinberg single-locus genotype frequencies
  (homozygote `p^2`, heterozygote `2 pa pb`), with the NRC II theta / Fst
  sub-population correction (Balding-Nichols), multiplied across loci:
  `randomMatchProbability` (RMP), the single-source match `LR = 1 / RMP`, and
  `cpi` / `cpe` (combined probability of inclusion / exclusion) for mixtures. A
  minimum-allele-frequency floor (`5 / 2N`, so the table keeps each locus's `N`)
  handles rare or unobserved alleles.
- **Lineage markers (mtDNA / Y-STR)** - mitochondrial and Y-chromosome markers are
  haploid, non-recombining, and uniparentally inherited, so match probability is
  **not** Hardy-Weinberg but a direct **haplotype count**: frequency `k / N` in a
  reference database, with a **Clopper-Pearson** exact-binomial upper bound as the
  conservative estimate (which pulls in the `beta` distribution `M24.11` adds to `stats`).
  Deliberately **database-independent**, so the deck supplies the estimator
  (`haplotypeFrequency(k, N)` / `lineageMatchProbability`) and the caller feeds the
  count from whatever database they queried. mtDNA adds a haplotype coded as
  differences from the **rCRS** (e.g. `263G 315.1C`) plus the substantive `align`
  step that renders a raw sequence into that standard nomenclature; Y-STR is a
  per-locus repeat-count vector with exact / single-step comparison.
- **Kinship likelihood ratios** - relationships encoded as IBD coefficients
  (kappa0 / kappa1 / kappa2: parent-child `(0, 1, 0)`, full sibs
  `(1/4, 1/2, 1/4)`, half-sib / avuncular / grandparent `(1/2, 1/2, 0)`, first
  cousins `(3/4, 1/4, 0)`, ...). `kinshipLR(a, b, relationship, db)` tests one
  relationship against another; `paternityIndex(mother, child, allegedFather,
  db)` gives a combined paternity index (CPI) and `probabilityOfPaternity`
  (`W = CPI / (CPI + 1)`), with a stepwise STR mutation model to survive a lone
  inconsistency. General pedigrees (beyond pairs / trios) need a peeling engine -
  **Elston-Stewart** - the one substantial algorithm, the Familias / `forrel`-style
  capability.

Mixture deconvolution (multi-contributor, drop-in / drop-out - the EuroForMix
space) is a larger, later effort layered on this base.

**Fragment analysis** (**Planned**) - from a capillary-electrophoresis run to
a profile, over the `abif` deck (M27.4; a `[decks]` dependency): `.fsa`
traces in, called alleles per locus out, ready for the statistics above.
A 3500 HID run's `.hid` keeps its traces encrypted (M27.4), so `.hid` input
gives metadata only; for a lab on 3500 + GeneMapper ID-X 1.6, find out
first whether its runs can be had as `.fsa`, or whether the input is
GeneMapper ID-X's own exports.
- **Signal:** baseline correction, pull-up (spectral cross-talk) detection,
  saturation flags.
- **Peaks and sizing:** peak detection with height and area; sizing against
  the internal lane standard (local Southern; the size-standard definition
  supplied or read from GeneMapper's export); size-standard quality checks.
- **Allele calling:** panel and bin definitions (loci, dye, size range,
  allele bins), read from GeneMapper ID-X panel / bin exports, so a lab keeps
  its kit definitions; analytical and stochastic thresholds, stutter filters
  (back / forward stutter ratios per locus), off-ladder and
  out-of-bin flags.
- **Output:** a `Profile` the match-probability, kinship and mixture
  functions take, plus a per-peak table (size, height, area, call, flags).
- Validation: against OSIRIS (NCBI's free analysis software) and published
  GeneMapper results on the same files. The docs state the scope: research,
  teaching and QC cross-checks; casework stays with validated software.

A concrete frequency source to wire in first, as a sample: the ENFSI STR reference
database **[STRidER](https://strider.online/frequencies)** - a
`loadFrequencies(xml, "strider")` over the `xml` library, with its
[formulae page](https://strider.online/formulae) and online calculator as the
exact-conventions spec and a validation target.

### M27.6 - NGS / high-throughput sequencing

**Done** ([deckyard/deck-ngs](https://github.com/deckyard/deck-ngs)).

Unlike the pure-`.j` decks above, NGS breaks the model on two axes - **scale**
(FASTQ.gz files run 10s of GB, BAM 100s, so everything **streams**, never
load-into-memory) and **compute** (alignment / assembly / variant calling are
heavily SIMD-optimised C a tree-walker is ~100-1000x too slow to replace). So the
deck is deliberately the **glue and light-I/O layer**, not the heavy engine, with
a different posture from the pure-`.j` decks: **Go-backed streaming parsers** (the
decompress-and-parse hot loop in Go, the `net.readAll` / `binary` pattern) with
the per-record logic in `.j`. In scope:

- **Streaming format I/O** - FASTQ (gzipped, over `compress` + `fs` handles) and
  the tab-delimited **SAM** / **VCF** / **BED** / **GFF** / **GTF** text formats:
  parse / filter / convert as a stream, so a 50 GB file never lands in memory.
- **QC + preprocessing** (FastQC / fastp-lite) - per-base quality and read-length
  / GC distributions, adapter detection, quality / adapter trimming, length
  filtering, subsampling: one streaming pass over FASTQ.
- **Interval ops** (bedtools-lite) - overlap / intersect / merge over BED / GFF
  features.
- **Pipeline orchestration** - the real sweet spot: NGS is fundamentally
  `bwa | samtools | gatk` glued together, and Jennifer already has `os.run` /
  `os.spawn` + `spawn` concurrency, so it can drive the standard tools, parse
  their output, manage intermediate files, fan out over samples in parallel, and
  handle errors - a Snakemake-lite in a real language.

Out of scope (wrap and pipe the native tool, do not reimplement): read
**alignment** (BWA / Bowtie2 / minimap2), de-novo **assembly** (SPAdes), **variant
calling** (GATK), and heavy **BAM / CRAM** handling - which additionally needs
**BGZF** (blocked gzip for random access), a `compress` gap and a Go addition
whose standalone value is limited without the downstream compute the deck omits.

### M27.7 - data-format tooling

**Partly done:** `jsonschema`, `jsonpath`, `sealbox` and `epub` are done,
`pdftools`, `protobuf` and `torrent` are planned.

Tooling **on top of** a core format, not a format itself. Core keeps the format
(`json` decode / encode, the `(node, pointer)` accessors and writers, JSON
Pointer as the one addressing scheme) and these decks build on it. Both are pure
`.j` over the `json.Value` accessors, so neither needs a core change: `typeOf(v,
ptr)` reports `"missing"` for an absent member, `==` on two `json.Value`s is
structural (key order ignored, `1 == 1.0`), and `len` counts code points.

- **`jsonschema`** (**Done**,
  [deckyard/deck-jsonschema](https://github.com/deckyard/deck-jsonschema)) -
  JSON Schema validation. A deck rather than core because the spec is several
  live drafts (draft-07, 2019-09, 2020-12, with a successor in
  progress) whose churn would force breaking core releases, because remote
  `$ref`s mean I/O and trust decisions a pure-value library should not make, and
  because `pattern` is ECMA-262 regex while `regex` is RE2. Draft 2020-12 first
  (what MCP tool schemas and OpenAI structured outputs use), draft-07 second.
  - **Surface:** `compile(schema) -> Schema` (resolves `$ref` / `$defs` /
    `$anchor` once and rejects a malformed schema up front) and `validate(schema,
    instance) -> list of Error`, each error carrying the instance location and
    the keyword location as JSON Pointers and a message, after the spec's
    "basic" output format; `isValid` for the yes / no case.
  - **References:** local ones resolve; a remote `$ref` resolves only through a
    registry the caller supplies (a map of URI to `json.Value`), so the deck
    never fetches anything.
  - **`format`** is an annotation by default, as the spec says. Opt in to
    asserting `date-time` / `date` (`time.fromIso`), `email`, `uri` (`uri`),
    `uuid` (`uuid.isValid`) and `ipv4` / `ipv6` (`ipnet`).
  - **`pattern`** runs on RE2: a pattern using lookaround or a backreference is
    a `compile` error naming the keyword, never a silent mismatch.
  - Tested against the official JSON-Schema-Test-Suite (vendored fixtures).
- **`jsonpath`** (**Done**,
  [deckyard/deck-jsonpath](https://github.com/deckyard/deck-jsonpath)) - RFC
  9535 JSONPath queries. A deck because JSON Pointer already is the single
  addressing scheme of the document-tree libraries, and a second syntax in core
  would blur that; JSONPath is a query language on top, returning many nodes
  where a pointer names one.
  - **Surface:** `query(doc, path) -> list of json.Value` and `paths(doc, path) ->
    list of string`, the matches as JSON Pointers so they feed the core writers
    (`json.set` / `json.remove`) directly; `compile(path)` parses once for reuse.
  - **Coverage:** the full RFC: name, wildcard, index, slice and filter
    selectors, descendant segments (`..`), the comparison and logical operators,
    and the functions `length`, `count`, `match`, `search` and `value`. The
    regex functions use I-Regexp (RFC 9485), a subset that maps onto RE2.
  - `json.Value` only in v1. `toml` / `yaml` share the accessor surface, so a
    later version can take the library as a parameter.
  - Tested against the jsonpath-compliance-test-suite (the RFC's CTS).
  - **Possible core follow-up, only if profiling the deck shows it:** a native
    descendant walk returning every pointer under a node, which turns `..`
    from a recursive `.j` walk into one Go call and serves all three
    document-tree libraries.
- **`epub`** (**Done**,
  [deckyard/deck-epub](https://github.com/deckyard/deck-epub)) - reading
  and writing EPUB books. A container of core
  formats rather than a format of its own: a zip of XHTML plus two XML files
  (`META-INF/container.xml`, the OPF package document), so it is pure `.j`
  over `archive`, `xml` and `markdown`. A deck because nothing in core needs
  it and the spec moves (EPUB 2, 3.0, 3.3).
  - **Writing** reuses Grimoire's EPUB renderer (`app-grimoire`,
    `plugins/epub.j`): its own zip writer, since the `mimetype` entry must
    come first and stored, which `archive.pack` cannot express; the OPF
    metadata, manifest and spine, and `nav.xhtml` built as `xml` trees, never
    as text; chapters from `markdown.toXhtml` parsed back to prove them well
    formed; deterministic bytes (a fixed timestamp, an identifier derived from
    the title). Surface: `book(title, lang)`, then authors, a cover, chapters
    (XHTML or Markdown), images and CSS, and `build(book) -> bytes`, EPUB 3.3
    with an optional EPUB 2 `toc.ncx` for older readers. Grimoire's plugin can
    then use the deck instead of its own copy.
  - **Reading** is new: `read(bytes) -> Book` unpacks with
    `archive.unpackWith` (its caps and zip-slip checks), follows
    `container.xml` to the OPF, and returns the metadata (title, authors,
    language, identifier, cover), the chapters in spine order, the table of
    contents (EPUB 3 `nav.xhtml` or EPUB 2 `toc.ncx`) and the other
    resources; `text(chapter)` gives plain text. Real-world chapters are
    often not well-formed XML or use HTML entities (`&nbsp;`), so a chapter
    the `xml` parser rejects falls back to `html`. A DRM-protected book
    (Adobe ADEPT, Readium LCP, seen in `META-INF/encryption.xml`) is a
    catchable error; IDPF font obfuscation may be undone, optionally.
  - Tested by round trips (`read(build(book))`) and fixture books, with
    `epubcheck` (the reference validator, Java) as an optional container-run
    check in the deck's `check-extra.sh`.
  - **Built and checked** (2026-11-14): Project Gutenberg #1080 (EPUB 3 and
    2) as fixtures, all 45 IDPF epub3-samples read by hand, EPUBCheck 5.4.0
    with 0 errors and 0 warnings on built books; DRM (ADEPT, LCP,
    FairPlay) is an `EpubError` with reason `drm`, IDPF and Adobe font
    obfuscation are undone. Its writer fixed Grimoire's `<body xmlns="">`
    (every chapter failed EPUBCheck); Grimoire's plugin has the fix too and
    keeps its own code until the deck is released. Validated against 73
    real books (Gutenberg incl. Chinese and Japanese, Standard Ebooks, the
    IDPF samples) compared with lxml, plus constructed hostile archives;
    added after it: batch builders (`chapters`, `images`, `tocEntries`;
    300 chapters + 60 pictures 3.3 s -> 0.05 s), `check(b)` for in-book
    links, `Book.direction` (RTL), generated landmarks, `Book.compress`
    (deflate, `mimetype` stored). UTF-16 documents are read (`convert`
    has no UTF-16 codec, decoded in `.j`). The reader tolerates, each
    with a line in `Book.notes`: a declared ISO-8859 / Windows charset, a
    manifest file missing from the archive, a name differing only in case,
    an imperfect `mimetype` entry. The writer refuses a non-W3C `dc:date`,
    percent-encodes hrefs with spaces or Unicode (a space still draws
    EPUBCheck's PKG-010 warning) and converts a read book's nav or images
    in the spine when rebuilding it.
- **`pdftools`** (planned) - reading and changing existing PDFs. Core's `pdf`
  is a writer, not a reader; this deck drives installed tools as
  `@deckyard/magick` drives ImageMagick (`os.start`, killed after a timeout),
  so it needs the default `jennifer` binary and the `exec` capability. A deck
  because it depends on external programs and their versions.
  - **Backends:** one per capability, no common-denominator layer. **qpdf** is
    the base: `qpdf --json` (JSON v2, qpdf 10.6+) gives pages, objects,
    encryption and outlines as structured data instead of scraped text.
    **poppler** (`pdftotext`, `pdftoppm`) is optional, for text extraction
    and page rendering. pdfcpu only if stamping or form filling is wanted
    later; not pdftk.
  - **Surface:** `info` / `pages`, `merge`, `split`, `selectPages`, `rotate`,
    `encrypt` / `decrypt`, `linearize`, `repair`, `attachments`; `text` and
    `render` when poppler is present. Each verb names the tool it needs, and a
    missing tool is a catchable error before anything runs.
  - Passwords go through qpdf's `--password-file`, never the command line
    (`ps` shows it). Output is written to a temporary file and moved into
    place, so a failed run leaves no partial PDF.
  - Tested against fixture PDFs generated by core `pdf` plus a few external
    ones (encrypted, linearized, damaged); the deck's CI installs qpdf and
    poppler.
- **`sealbox`** (**Done**,
  [deckyard/deck-sealbox](https://github.com/deckyard/deck-sealbox)) - one-call encryption formats
  on top of core `crypto`. After M28.40, `crypto.encrypt` takes every
  parameter (algorithm, nonce, AAD) and `blockEncrypt` gives the raw block
  modes; this deck is the convenience layer that packages them into formats,
  so the caller never handles a nonce, an IV or a MAC. A deck because the
  core keeps one way to reach each primitive (stance #1), and formats are
  layouts built from them, of which there are many. Pure `.j` over `crypto`,
  `hash` and `encoding`, so it runs on both binaries.
  - **Sealed box:** `seal(plaintext, key) -> bytes` / `open(box, key) ->
    bytes`, AES-256-GCM with a fresh random 12-byte nonce, the box `nonce ||
    ciphertext || tag`: the same bytes the pre-M28.40 `crypto.encrypt` made,
    so existing boxes open unchanged. `sealWith` / `openWith` add
    associated data. `newKey()` gives 32 random bytes. The docs say to
    rotate the key well before 2^32 messages (the random-nonce limit of NIST
    SP 800-38D).
  - **Encrypt-then-MAC:** `cbcHmacSeal(plaintext, encKey, macKey) ->
    CbcHmac{iv, ciphertext, mac}` / `cbcHmacOpen(box, encKey, macKey)`,
    AES-CBC with PKCS#7 and HMAC-SHA256 over `iv || ciphertext`, the MAC
    checked in constant time before decrypting. A wrong MAC and a bad
    padding are the same error.
  - **Fernet:** `fernetEncrypt(plaintext, key) -> string` /
    `fernetDecrypt(token, key, ttlSeconds) -> bytes`, the token format of
    Python's `cryptography.fernet` (version byte, timestamp, IV, AES-128-CBC,
    HMAC-SHA256, base64url; the key is 32 bytes base64url, half signing,
    half encryption). An expired token, a token from the future beyond a
    small clock skew, and a bad version byte are catchable errors.
  - Every failure is one catchable `Error{kind: "sealbox"}` that does not say
    which check failed.
  - Tested with a box made by the pre-M28.40 `crypto.encrypt` (jennifer
    0.25), the Fernet spec's `generate.json` / `verify.json` / `invalid.json`
    vectors, and a CBC+HMAC box made with `openssl enc` / `openssl dgst -mac
    HMAC`; a flipped bit in every byte of every format fails. `cbcHmacOpen`
    takes the `CbcHmac` struct; `macKey` is at least 16 bytes; a Fernet TTL
    of 0 accepts any age but still refuses a token more than 60 s in the
    future (Python skips that check without a TTL); only the canonical
    base64url spelling of a token or key is accepted.
  - Argument order follows M28.41 (subject first, then keys).
  - **Requires:** M28.40 and M28.41.

- **`protobuf`** (planned) - Protocol Buffers messages by schema, on top of
  the core wire library (M28.61), which reads and writes fields by number.
  A deck because schemas, the JSON mapping and the `.proto` language move
  (proto2, proto3, editions) while the wire format does not; pure `.j`, both
  binaries.
  - **Schemas from a descriptor set.** `schema(descriptorSet as bytes) ->
    Schema` reads a `FileDescriptorSet` (what `protoc --descriptor_set_out
    --include_imports` writes, itself protobuf, so it needs only M28.61):
    messages, fields with number, type, label and `json_name`, enums,
    nested types, `oneof`, maps (as their synthetic entry messages), packed
    defaults by syntax, proto3 `optional` presence, and the editions
    features that change the wire (`field_presence`, `repeated_field_encoding`,
    `enum_type`). Imports resolve within the set; a missing type is an error.
  - **Messages as `json.Value`.** `decode(schema, typeName, data) ->
    json.Value` and `encode(schema, typeName, value) -> bytes`, following
    protobuf's canonical **ProtoJSON** mapping (lowerCamelCase `json_name`
    keys, 64-bit integers as strings, `bytes` as base64, enums by name,
    `NaN` / `Infinity` as strings, the well-known types `Timestamp`,
    `Duration`, `Struct`, `Value`, `Any` (with a resolver), the wrappers and
    `FieldMask` in their special forms), so a message reads like any JSON
    the program already handles. Unknown fields on decode are kept in a
    reserved member or refused (`DecodeOptions{unknown: "keep" | "refuse"}`,
    strict default `refuse`); on encode an unknown key is an error.
    `toJson(schema, typeName, data) -> string` / `fromJson` for the text form.
  - **A `.proto` parser** (second step): `parse(files as map of string to
    string) -> Schema` for proto3, proto2 and the common editions options,
    so a program needs no `protoc`. Its result must match `protoc`'s
    descriptor for the same files, which is how it is tested.
  - **Not in scope:** generated `.j` structs (a `protoc` plugin could come
    later), gRPC (HTTP/2; `horizon.md`), text format beyond the debug
    output.
  - Errors: one `ProtobufError` struct of the deck's own kind
    (`"protobuf.schema"` for schema problems, the core library's
    `"protobuf"` passed through), generic kinds passed on; depth and size
    limits on nested messages, since the deck recurses where the core codec
    does not.
  - Tested against `protoc`-made descriptor sets and messages for the
    `protobuf` conformance suite's test protos (`test_messages_proto3.proto`,
    `..._proto2.proto`), with the ProtoJSON expectations cross-checked
    against Go's `protojson` in a scratch run. **Requires:** M28.61.

- **`torrent`** (planned) - BitTorrent metadata, not BitTorrent transfer.
  Pure `.j` over `hash` (SHA-1, SHA-256) and `encoding`, both binaries; no
  capability for the metadata itself, `net` only for the tracker calls.
  - **Bencode:** `bdecode(data) -> json.Value` / `bencode(v) -> bytes`,
    strict as BEP 3 defines it: canonical integers (no leading zeros, no
    `-0`), dictionary keys sorted and unique, byte strings that are not
    UTF-8 kept as base64 under a marker key rather than guessed, and a
    nesting and size budget (kind `"limit"`), because a `.torrent` is
    untrusted input.
  - **Torrent files:** `read(data) -> Torrent` (name, files with sizes and
    paths, piece length, piece hashes, trackers and tiers, web seeds
    (BEP 19 `url-list`), private flag, creation date, comment) for v1
    (BEP 3), v2 and hybrid torrents (BEP 52, per-file Merkle roots);
    `infoHash` (v1 SHA-1, v2 SHA-256, taken from the raw `info` bytes, never
    re-encoded); file paths checked like an archive's (no `..`, no absolute
    paths, no empty segments). `create(files, pieceLength, options) ->
    bytes` builds a torrent from files on disk or bytes in memory,
    deterministically, so a release pipeline can publish one beside its
    tarball.
  - **Magnet links:** `magnet(torrent) -> string` and `parseMagnet(uri) ->
    Magnet` (`xt` v1 and v2 hashes, `dn`, `tr`, `ws`, `xl`), strict about
    the hash encodings (hex or base32).
  - **Verification:** `verify(torrent, root) -> list of PieceCheck` reads
    the files under a directory piece by piece and reports which pieces
    match, so data fetched any other way (HTTP mirror, rsync, a copy) can
    be checked against the torrent; v2 checks per file.
  - **Trackers:** HTTP announce and scrape (BEP 3, compact peers BEP 23,
    scrape BEP 48) and UDP trackers (BEP 15), for monitoring seed and peer
    counts of torrents one publishes; replies capped and bencode-checked.
  - **Never a peer-protocol client.** The deck will not implement the peer
    wire protocol, piece exchange, choking, the DHT, PEX or uTP. A
    downloading client is a large, long-lived attack surface of hostile
    peers, does better in a dedicated program, and is outside what a
    Deckyard deck can maintain to its bar. Downloading belongs to the two
    future companions below.
  - **Future ideas, separate decks:**
    - **A client-control deck** (`transmission` / `qbittorrent`, Deluge):
      drives a client that is already running over its RPC or Web API (add
      a torrent or magnet, progress, limits, move and remove finished
      downloads), with this deck supplying info hashes, file lists and
      magnet links. A plain HTTP client deck, testable against the client
      in a container.
    - **`download` with web seeds:** a torrent's BEP 19 `url-list` lets the
      planned `download` deck fetch pieces over plain HTTP ranges and check
      each against the piece hashes: resumable, verified downloads with no
      peer protocol. Best after M28.29 (streaming responses).
  - Errors: one `TorrentError{kind: "torrent", message, ...}` struct, generic
    kinds passed on. Tested with torrents made by `mktorrent`, `transmission-create`
    and libtorrent's `create_torrent` (v1, v2, hybrid), the BEP 52 test
    vectors, and hostile bencode (deep nesting, huge declared lengths,
    unsorted keys, duplicate keys). **Note:** no maintainer uses BitTorrent
    yet, which Deckyard's bar asks for; it waits for one.

### M27.8 - money

**Partly done:** `money` is done
([deckyard/deck-money](https://github.com/deckyard/deck-money)), `finance` and
`marketdata` are planned.

- **`money`** (**Done**, `@deckyard/money`) - currency amounts that are
  always exact, on the `decimal` system library. A deck rather than core
  because the currency table is data that changes (ISO 4217 amendments) and
  because money rules (allocation, rounding policy, display) vary by
  application, while core keeps the general primitive: exact decimal
  arithmetic.
  - **Model:** `Money{amount, currency}`, the amount stored as whole **minor
    units** (cents) in an `int`, as moneyphp does: 12.34 EUR is `1234`. Every
    amount is exact by construction, compares and sorts as an `int`, `==` is
    already right, and an `int` covers about 9.2 x 10^16 minor units, ample
    for any single amount. A result past the `int` range is a `money` error,
    not the interpreter's overflow. `decimal` works inside the deck only, for
    factors, rates and rounding; `toDecimal` / `fromDecimal` bridge to it.
  - **Currencies:** an ISO 4217 table of each currency's alphabetic code and
    number of decimal places (JPY 0, EUR 2, KWD 3), **generated into the source**
    from the ISO 4217 list (SIX `list-one.xml`) by a Jennifer script in the
    deck over the `xml` library, so a table update is a regenerate-and-release
    and the deck needs no data file at run time. Codes without minor units
    (XAU, XXX, XTS) are left out. An unknown code is an error, never a guess.
    `isCurrency`, `digits` and `currencies` read the table. Custom currencies
    (crypto, loyalty points) stay out: they would put the number of places on
    each `Money` and change equality and storage.
  - **Arithmetic:** `add` / `subtract` / `sum` refuse mixed currencies;
    `negate`, `absolute`. `multiply` / `divide` take the factor as a string
    such as `"1.19"`, turned into an exact fraction through `decimal`, so no
    float ever touches an amount (Jennifer has no union parameter type, and a
    `Decimal` becomes exact text with `decimal.toString`), and take an
    **explicit rounding mode**, any of `decimal`'s seven, for the result back
    to whole minor units. There is no default mode.
  - **Allocation:** `allocate(m, ratios)` splits an amount by ratios (strings,
    not negative, not all zero) by largest remainder, ties to the earlier part,
    and `allocateTo(m, n)` into `n` equal parts, the leftover minor units on the
    first parts, so the parts always sum to the original (no cent lost or
    created). A negative amount splits as its mirror.
  - **Comparisons:** `compare` / `lessThan` / `greaterThan` (same currency
    only), `isZero`, `isNegative`, `isPositive`. No `equals`: `==` compares by
    value.
  - **Text:** `parse("12.34", "EUR")` and `format(m)`, plain with a dot only:
    `"12.34"`, `"1234"` for JPY. `parse` takes `-?digits(.digits)?` only (no
    exponent, spaces or digit separators) and refuses more places than the
    currency has unless they are zeros (`"12.340"` is fine, `"12.345"` is not).
    Locale-aware formatting (grouping, the decimal comma, symbol placement)
    stays out: it is the open `printf` question in `rejected.md`
    ("Locale-aware *value* formatting"), and a deck should not settle it first.
    No JSON shape either: `Money` is two plain fields.
  - **Exchange:** `exchange(m, target, rate, mode)` with a fixed rate the
    caller supplies (a string, like the multiply factor); named `exchange`
    because the deck itself uses the `convert` library. No exchange-rate
    fetching: it would need network access (the `net` capability) and tie the
    deck to one rates provider; a caller fetches rates from whatever source it
    trusts and passes them in.
  - **Requires:** the `decimal` system library, so the deck's floor is
    `>=0.26.0`, the first Jennifer release that ships it; until then its CI
    runs on the `:dev` image.
- **`finance`** (**Planned**, `deck-finance`) - financial maths: time value of
  money, loans, day counts, fixed income, depreciation, options, returns and
  risk. Nothing bundled covers it (`stats` / `math` have no NPV or pricing, the
  `sheets` evaluator implements no financial function), and `money` stops at
  exact amounts. Rates, yields and prices are `float` (IRR is a root-find,
  pricing needs `exp` / `erf`); every amount that is booked - a schedule's
  interest, principal and balance, a depreciation charge - is a `money.Money`,
  rounded per period with an explicit mode and closed so the rows sum exactly
  to the principal. Depends on `@deckyard/money` (`[decks]`), so its CI vendors
  the released money tag.
  - **Time value of money:** `pv`, `fv`, `pmt`, `nper`, `rate`, `ipmt`,
    `ppmt`, `npv`, `irr`, `mirr`, `xnpv` / `xirr` (dated flows),
    `effective` / `nominal`; Excel's argument order, sign convention and `type`
    (payments at period end or start), so results check against a
    spreadsheet and the `sheets` evaluator can call the deck for its financial
    functions later.
  - **Loans:** amortisation schedules (annuity, linear, interest-only,
    balloon), per row date, payment, interest, principal and balance as
    `Money`; the last row absorbs the rounding residue.
  - **Day counts and calendars:** 30/360 (US, EU), ACT/360, ACT/365F,
    ACT/ACT (ISDA, ICMA), year fractions; business-day adjustment (following,
    modified following, preceding) over a holiday list the caller supplies; no
    bundled holiday data.
  - **Fixed income:** bond price from yield and yield from price, accrued
    interest, clean / dirty price, Macaulay and modified duration, convexity;
    discount factors and zero-curve bootstrapping from par rates.
  - **Depreciation:** `sln`, `db`, `ddb`, `syd`, charges as `Money`.
  - **Options:** Black-Scholes-Merton price and Greeks, implied volatility,
    Cox-Ross-Rubinstein binomial trees for American exercise. Phase 2.
  - **Returns and risk** over `stats`: simple and log returns, CAGR,
    annualised volatility, Sharpe and Sortino ratios, maximum drawdown,
    historical VaR.
  - **Solver:** one root-finder for `irr`, `xirr`, `rate`, bond yield and
    implied volatility: Newton from a guess, a bracketing (Brent) fallback, an
    iteration cap; an error, not a guess, when flows have no sign change or
    the search does not converge. Flows that change sign more than once (more
    than one IRR) are reported, not resolved silently.
  - Tested against published vectors: Excel / LibreOffice results (a
    LibreOffice oracle in `check-extra.sh`, as `deck-sheets` has), the ISDA
    day-count examples, Hull's option examples. Out of scope: market data
    (vendor APIs are client decks of their own), trading, portfolio
    optimisation, exotic derivatives. Floor `>=0.26.0`, no capabilities.
- **`marketdata`** (**Planned**, `deck-marketdata`) - exchange rates, economic
  series and prices from providers with an official API, behind one model;
  the data source for `money.exchange` and for `finance`'s curves and
  returns. One deck for several providers, a deliberate exception to the M27
  one-vendor rule: each provider is a few GET endpoints, and callers want
  the same shapes from all of them. The cost: a breaking change at one
  provider means a release of the whole deck.
  - **Model:** `Series{provider, id, unit, frequency, points}` of
    `Point{date, value}`, `Quote{symbol, price, currency, time}`,
    `FxRate{base, quote, date, rate}`. Values are kept as the decimal
    string the provider published, next to a `float`, so `money.exchange`
    gets the exact rate without a float round trip.
  - **Clients:** one constructor and config per provider (API key where one
    is needed); shared verbs where they fit - `series(c, id, from, to)`,
    `latest(c, id)`, `fx(c, base, quote, date)` - plus each provider's own
    calls (FRED search, the ECB history file).
  - **Providers in v1:** ECB reference rates (no key; daily EUR rates from
    1999), FRED (free key; interest rates, yields, macro series), Alpha
    Vantage and Twelve Data (keyed, free tiers; prices, FX, history). Others
    are later additions to the same deck.
  - **Behaviour:** 429 / `Retry-After` honoured, plus a client-side throttle
    at each free tier's documented per-minute cap; an optional on-disk cache
    over `flatdb` so history is not refetched; API keys redacted from error
    messages.
  - Out of scope: Yahoo Finance, Google Finance and any scraped source (no
    official API, terms forbid automated access), streaming / websocket
    feeds, order execution. Tested offline against a loopback fake replaying
    recorded responses. Floor `>=0.26.0`, `net`.

### M27.9 - statistical data files

- **`statfiles`** (**Done**,
  [deckyard/deck-statfiles](https://github.com/deckyard/deck-statfiles)) -
  read-only readers for the data files of SPSS, Stata and SAS, the formats
  study and clinical data arrives in for the M27.4 / M27.5 decks. Pure `.j`
  over the `binary` number readers, `compress` (zlib) and `encoding`
  (Windows-* / ISO-8859-* codepages); no capability, the readers take
  `bytes`. One result type, `Dataset`, stored by column rather than as rows
  (a deck's own structs are rebuilt on every call into it, so rows of
  structs would be slow): per variable its name, label, type, storage,
  width, display format, value labels and declared missing values, and its
  cells in `numbers` / `texts` / `missing`. No NaN in Jennifer, so a missing
  cell carries a code: `.`, `.a`-`.z`, `._`, or `user` for a value SPSS
  declares missing (kept in `numbers`). `column` (strict: a missing cell is
  an error) and `present` hand a variable to `stats` / `linalg`; `toCsv`,
  `labelOf`, `detect`, `read` / `readWith` with `Options` (`encoding`,
  `maxCells`, `maxBytes`). Formats:
  - **Stata `.dta`** 104-119 (StataCorp's spec; 117+ tagged sections,
    `strL` in the 117 and 118+ layouts, value labels, both byte orders,
    `.a`-`.z`).
  - **SPSS `.sav` / `.zsav`** (PSPP's system-file documentation: header,
    variable, value-label and extension records 3, 4, 11, 13, 14, 16, 20,
    21, 22; very long strings; bytecode compression; `.zsav` zlib blocks
    under `maxBytes`; SYSMIS, declared missing values and ranges) and
    **`.por`** (translation table, base-30 numbers).
  - **SAS XPORT `.xpt`** v5 / v8 (TS-140 and the v8/v9 layout; IBM hex
    floats converted in `.j`, missing `.` / `.A`-`.Z` / `._`, several
    members through `readXptMembers`).

  Fixtures written by pyreadstat 1.3.8, pandas, PSPP 2.0.1 and hand-made
  writers, plus one SAS 9.3 file (phuse-scripts, MIT), cross-checked value
  for value against pyreadstat / pandas; truncation and corruption sweeps
  give only `statfiles` / `limit`; validated against about 4,000 fresh files
  (some 1,000,000 cells) from pyreadstat, pandas and PSPP. 10k rows x 20
  variables read in 0.5 s (dta), 0.6 s (sav), 1.2 s (compressed sav, zsav,
  xpt) and about 8 s (por). Out of
  scope: writing any format, SPSS multiple-response sets, attributes and
  documents, Stata notes and characteristics. Floor `>=0.26.0`.
  - **Phase 2 (**Done**): SAS `.sas7bdat`** (undocumented; layout from
    ReadStat, parso and pandas): 32/64-bit, both byte orders, meta, data,
    mix and AMD pages, RLE and RDC compression, moved and deleted rows,
    numbers in 3 to 8 bytes, missing values from NaN tags read through
    `readUint`; value labels from a `.sas7bcat` catalog (`readSas7bcat`,
    `withSasLabels`; SAS 9.4's fixed label records read; numeric and
    character ranges and OTHER left out). Validated against 352 real data
    sets from 13 projects plus the GSS 2021 release (4,032 x 943) and 590
    generated files, all agreeing with pyreadstat; 9 sample catalogs and
    the 146 MB GSS catalog; about 38,000 corrupted files give only
    `statfiles` / `limit`. 10k rows x 20 variables in 0.7 s (uncompressed)
    to 3.3 s (RDC); numbers stored in 3 to 7 bytes are converted per cell
    and cost the most.

### M27.10 - healthcare interop

HL7's two standards and the ASTM instrument protocol, as three decks: HL7 v2
for hospital message feeds, FHIR for the REST API newer systems expose, ASTM
for the lab analyzers that send "HL7-alike" frames. Instrument interfaces
deviate from their specs often, so the readers are lenient by default and
report what they tolerated.

- **`hl7`** (**Done**,
  [deckyard/deck-hl7](https://github.com/deckyard/deck-hl7)) - HL7 v2.x
  messages (ER7, the pipe-delimited encoding): ADT, ORU, ORM / OML, SIU and
  any other type. Pure `.j`, no core change.
  - **Parse / build:** message -> segments -> fields -> repetitions ->
    components -> subcomponents, delimiters read from MSH-1 / MSH-2 (with
    the v2.7 truncation character), escape sequences (`\F\ \S\ \T\ \R\
    \E\ \P\ \Xhh\`, highlighting and formatting) decoded and encoded,
    segment terminator CR. 1-based terser-style paths (`PID-5-1`,
    `OBX(2)-5`, `PID-3(2)-1`) for `get` / `set`; an unchanged message
    re-encodes byte-identical. MSH-18 character sets through `encoding`
    (ASCII, 8859/n, UNICODE UTF-8). `parse` caps at 4096 segments and 65536
    fields, repetitions, components and subcomponents.
  - **Strict, not lenient** as first planned here: CR is the only segment
    terminator, malformed input throws, no `warnings` / `parseStrict`.
  - **ACK:** `ack(msg, code, text, controlId)` builds the AA / AE / AR (or
    CA / CE / CR) reply with MSA and swapped sender / receiver.
  - **MLLP:** client `connect` / `send` (ACK timeout, resend, MSA-2 check),
    `listen` / `accept` / `receive` / `reply`, and `serve` with a handler,
    one task per connection; a size cap per message.
  - Out of scope: validation against the per-version message structures and
    data-type tables (large, licence-restricted), conformance profiles,
    batch / file headers (FHS / BHS), the v2 XML encoding. Tested offline
    with sample messages and a loopback MLLP peer. Floor `>=0.26.0`, `net`.
- **`fhir`** (**Done**,
  [deckyard/deck-fhir](https://github.com/deckyard/deck-fhir)) - an HL7
  FHIR R4 / R5 REST client over `http` + `json`: `client(baseUrl)` with
  `with*` settings, `capabilities`, `read` / `vread`, `search` following
  `link[next]` (page cap and a byte cap over all pages; `searchBundle` /
  `nextBundle` to page by hand), `create` / `update` (`If-Match` from
  `meta.versionId`) / `patch` (JSON Patch) / `delete`, `history`,
  `transaction` / `batch`, `operation` (`$name`), `call` for the rest.
  Resources stay `json.Value`; helpers for references, identifiers,
  codings, bundles and `OperationOutcome` issues; errors carry the
  diagnostics and `(HTTP nnn)` (`statusOf`). Auth: bearer token, SMART on
  FHIR backend services (RS384 / ES384 client assertion through `oauth`).
  Plain http only to loopback unless allowed; next links and Locations are
  followed only below the base URL. Out of scope: profile validation and
  FHIRPath, terminology services, subscriptions, a device-code launch (the
  `oauth` module's flows plus `withToken` cover user sign-in). Tested
  offline against a fake over `httpd`, and live against a local HAPI FHIR
  4.0.1 container. Floor `>=0.26.0`, `net`.
- **`astm`** (**Done**,
  [deckyard/deck-astm](https://github.com/deckyard/deck-astm)) - the
  lab-instrument protocol of ASTM E1381 / E1394 (now CLSI LIS01-A2 /
  LIS02-A2) that most clinical analyzers speak to a LIS. Pure `.j`.
  - **Transport (LIS01):** ENQ / ACK / NAK / EOT establishment, frames
    `STX fn text ETB|ETX checksum CR LF` with frame numbers 1-7, 0, the
    modulo-256 checksum, records split at 240 text bytes, NAK retry (6), busy
    NAK waits, the 15 s / 30 s timers, contention resolution (the computer
    side receives first and keeps it for the next `receive`). Over `net`
    (TCP client `connect`, server `listen` / `accept`) or `serial`, one
    `Link` for both.
  - **Records (LIS02):** any record type, delimiters read from each H record,
    escapes (`F R S E X`) decoded and encoded, field / repeat / component
    access numbered as LIS02 numbers them; `results` (P / O / R / C),
    `orders`, `queries`, and `orderMessage` / `queryMessage` to answer host
    queries and send orders. `parse` caps at 4096 records and 65536 fields,
    repeats and components (uncapped, 1 MiB of empty fields took 1.1 GB).
  - **Device profiles:** a `Profile` struct (encoding, frame size, retries,
    timers, contention role, largest transmission); the deck ships none for
    named products.
  - **Strict, not lenient** as first planned here: a damaged frame (checksum,
    number, end, size, control character) gets NAK as LIS01 asks; records
    that break LIS02 are an error. No `warnings`, no tolerated deviations.
  - Out of scope: the obsolete ASTM E1238, vendor-specific record meaning
    beyond pass-through, unframed record streams over TCP (parse their text
    with `parse`). Tested offline against a scripted peer over a channel
    pipe and loopback TCP; checksums in the fixtures computed apart from the
    deck. Floor `>=0.26.0`, `net` (serial needs the default binary).

---

## Ideas (not planned yet)

Candidates that are not planned yet. A deck needs no interpreter release, so
one moves up into an M27 section whenever it earns a slot.

- **`sendgrid`** - the SendGrid v3 mail API (no free plan since 2025; M27.2
  has `mailgun`, `mailtrap`, `brevo` and `resend` instead): bearer auth, JSON
  (`/v3/mail/send` with personalizations, attachments, dynamic templates), plus
  Event Webhook verification. That signature is ECDSA P-256 in **DER** form over
  a base64 DER public key, while `crypto.ecdsaVerify` takes a PEM key and the
  JOSE `R||S` form, so the deck wraps the key as PEM and converts the signature
  with `asn1`. Testable offline through the API's sandbox mode. No core gap. A separate deck from `twilio` despite the common
  owner: different API, different auth.
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
  `modules/s3.j` (and pinned to the `s3` service), so `s3` exports it: an
  `s3.sign` that takes the service as a parameter beside the existing
  `s3.presign`, returning the `Authorization` / `x-amz-date` headers. Too few
  names for a module of its own, and `s3` is already the core home of SigV4 for
  every S3-compatible store; the generalized signer also serves OpenSearch and
  other SigV4 services. Tested
  offline against the published SigV4 test-suite vectors and in CI against
  LocalStack. Roughly 2-3x the size of the `twilio` or `stripe` deck.
- **`firebase`** - Firebase over its REST surfaces: Auth ID-token verification
  (`jwt.verifyJwks` against the securetoken JWKS, plus the `iss` / `aud` / `sub`
  / `auth_time` checks), Admin Auth user management (Identity Toolkit REST), FCM
  HTTP v1 push, Firestore REST (with a converter between Jennifer values and
  Firestore's typed value encoding - `stringValue`, `integerValue` as a string,
  `mapValue`, `arrayValue`, `timestampValue`, ...; `runQuery` for structured
  queries), and Realtime Database REST. **Requires a core change first:** server
  access authenticates with a Google service account, which `oauthext` provides
  (`oauthext.googleServiceAccount`, over the RFC 7523 grant in core `oauth`;
  the `oauthext` deck, M27.2 above). **Out of
  v1:** realtime listeners - Firestore's need gRPC (no HTTP/2 / gRPC client),
  the Realtime Database's need a streaming (SSE) response, which `http` cannot
  read yet (planned as M28.29, the streaming `http` + `sse` addition). Tested in
  CI against the Firebase Emulator Suite (Java + Node in the runner). Larger
  than any M27.2 deck.
