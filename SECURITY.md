# Security Policy

## Jennifer's security model in one paragraph

Jennifer is a **general-purpose programming language**, not a sandbox. A Jennifer
program has full access to the host - files, network, subprocesses - exactly like
a Python or Node program. That surface is a **language capability, not a
vulnerability**: there is no trust boundary between a script and the host, so
"a script can read any file / dial any address / run any program" is by design,
not a bug. **Do not run untrusted Jennifer code on a host you care about** -
isolate the whole interpreter at the OS level (a container, a VM, an
unprivileged restricted user). A capability-based in-process sandbox is planned
(`DRAFT#11`). The full model is in
[docs/technical/security-model.md](docs/technical/security-model.md).

## What counts as a vulnerability

A real bug is one that harms a program **using the stdlib correctly**, driven by
untrusted *data* rather than untrusted *code*:

- **Protocol / injection** - a module placing a caller value on a wire without
  sanitizing it (CRLF injection, JWS/JSON malformation, algorithm confusion).
- **DoS from untrusted input** - a fatal/uncatchable crash or unbounded
  memory/CPU from an attacker-shaped document, message, or response.
- **Cryptographic weakness** - predictable randomness, a timing side channel, a
  verification/downgrade bypass.

**Not** a vulnerability (won't be treated as one): the stdlib's `fs` / `net` /
`os` / `sql` / `meta` host access, an explicit opt-in like TLS `skipVerify`
(verification is on by default), or a program that itself passes untrusted input
to a dangerous call. See the security-model doc for the reasoning and
[docs/technical/rejected.md](docs/technical/rejected.md) for previously-declined
scanner findings.

## Reporting a vulnerability

Please report suspected vulnerabilities **privately** to `<security@jennifer-lang.dev>`
rather than opening a public issue. A useful report **describes** (rather than
patches):

- the affected file / module and a minimal reproducer,
- the untrusted-data path (who supplies the malicious input, and how it reaches
  the sink),
- the impact.

Jennifer is a pre-1.0, single-maintainer project: there is no security team,
response SLA, or formal embargo process. Reports are read at the address above
and handled as maintainer capacity allows; disclosure timing is coordinated case
by case, and fixes ship with a regression test.

Please send a **description**, not patch code. The project does not yet have a
contribution / inbound-licensing policy (the governance work is tracked as
`M29` in the [milestones](docs/milestones.md)), so unsolicited code cannot be
merged cleanly - the maintainer writes the fix from your report, which keeps the
tree's copyright unambiguous.

## Supported versions

Jennifer is pre-1.0 (`0.x`); only the latest `main` receives fixes.
