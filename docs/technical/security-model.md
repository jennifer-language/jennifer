# Security model

Jennifer is a **general-purpose programming language**, not a sandbox. This page
states what that means for security, so that a real bug is told apart from an
intended capability.

## The trust boundary: the script is trusted

A Jennifer program has **full access to the host**, like a Python, Ruby, or Node
program:

- `fs` reads, writes, and deletes any path the process can reach.
- `net` / `sql` / `http` dial any address or database.
- `os.run` / `os.spawn` execute any program.
- `meta.call` / `meta.callMain` invoke a function by runtime name.

**These are language capabilities, not vulnerabilities.** There is no trust
boundary between a script and the host: *the script is the trusted party.* "A
Jennifer program can read `/etc/passwd`" is the same statement as "a Python
program can call `open('/etc/passwd')`" - true, intended, and not a defect.
Security scanners often label this surface "path traversal", "SSRF" or
"RCE"; in a trusted-script model it is none of those.

The corollary: **do not run untrusted Jennifer code on a host you care about.**
If you need to run code you do not trust, isolate the *whole interpreter* at the
OS level (a container, a VM, a seccomp/AppArmor profile, an unprivileged user
with a restricted filesystem). A capability-based in-process sandbox - an
allow-listed `fs` root, a dial policy, an exec allow-list - is planned as
`DRAFT#11` in [horizon.md](../horizon.md); until it lands, OS-level isolation is
the answer.

## What *is* a security bug

A real vulnerability exists **even under the trusted-script model**: untrusted
*data*, not untrusted *code*, causes harm. The clear cases:

- **Protocol injection.** A module that puts a caller-supplied value onto a
  protocol wire without sanitizing it. A program using the module *correctly* -
  an email form calling `smtp.send`, a route parameter reaching a cookie - is
  exploitable through the data. So the network / protocol modules reject control
  characters at the wire boundary (`smtp` addresses / EHLO name, the `http`
  request method, the `websocket` handshake URL, `web` cookie `Path` / `Domain`),
  reject an unsupported JWT `crit` header, JSON-escape hand-built JWS, and cap a
  received message size.
- **Uncatchable crashes / DoS from untrusted input.** Parsing an attacker-shaped
  document must fail with a *catchable* error, not a fatal crash or an
  out-of-memory kill. The caps (default binary / `jennifer-tiny`):

  | Input                                       | Cap                                                                                               |
  | ------------------------------------------- | ------------------------------------------------------------------------------------------------- |
  | `json` / `toml` / `xml` / `yaml` / `asn1` decoders, the language parser | nesting depth `limits.MaxNestingDepth` (1000 / 64)                       |
  | `http` client response                      | body size                                                                                         |
  | `images.decode`                             | pixel count from the header, checked before allocating (a "decompression bomb")                   |
  | Jennifer recursion                          | `limits.MaxCallDepth` (10000 / 120), a catchable "call stack too deep"; the heap its per-frame value copies hold by `limits.MaxCopyChainBytes` (1 GiB / 64 MiB) |
  | Lexer                                       | `limits.MaxTokens` per source file (4,194,304 / 262,144), so a "token bomb" (a few MB of tiny tokens, ~100x larger as Token structs) is a lex error |

- **Cryptographic weakness.** Predictable "random", a timing side channel, an
  algorithm-confusion or downgrade bypass. `crypto` uses a crypto-grade random
  source, constant-time comparison, AEAD-only symmetric encryption, and
  length-validated keys; `jwt.verify` pins the expected algorithm, refuses an
  HMAC key shorter than its hash, and refuses a JWK not meant for that algorithm.

These are treated as bugs and fixed with regression tests.

## Guidance for programs that face untrusted input

Even with a trusted script, a program can expose a *trust boundary of its own* -
an HTTP handler, a job runner, a form. In that program:

- **Validate and bound untrusted data** before handing it to a module (sizes,
  formats, allowed values). The modules harden the wire, but a program should
  not pass arbitrary user input to `os.run`, `fs.*`, `meta.call`, or a
  `serveFile` path.
- **Do not echo raw interpreter or module error text to untrusted end-users.**
  Error messages carry source positions and internal names for debugging; that
  is intended for the operator, not for a remote user. Catch errors and return a
  generic message at the trust boundary.
- **Wrap network I/O in `try` / `catch`** and set timeouts; a remote peer is
  untrusted.
- **Keep secrets out of world-readable files.** On-disk tokens should use
  owner-only permissions.

## Reporting

See [SECURITY.md](https://github.com/jennifer-language/jennifer/blob/main/SECURITY.md)
for how to report a suspected vulnerability, and what is in and out of scope.
