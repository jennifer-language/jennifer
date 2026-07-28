# `ratelimit` - rate limiter (fixed or sliding window)

Import with `import "ratelimit.j" as ratelimit;`. A rate limiter over a
**selectable** key/value backend ([`kvstore.Store`](kvstore.md) - `memcache`,
`redis`, or the in-process [`kv`](../libraries/kv.md)). Each key (an IP, a user,
an API token) is counted against a limit per time window; a `check` records a hit
and returns a `Result` with the decision **plus the metadata for a compliant
`429`** - remaining budget, reset time, and Retry-After.

```jennifer
import "ratelimit.j" as ratelimit;
import "kvstore.j" as kvstore;

def lim as ratelimit.Limiter init ratelimit.slidingWindow(kvstore.redisStore($rc), 100, 60);
def r as ratelimit.Result init ratelimit.check($lim, "ip:203.0.113.7");
if (not $r.allowed) {
    # respond 429; set Retry-After: $r.retryAfter, X-RateLimit-Remaining: $r.remaining
}
```

## Surface

| Call / type | Returns | |
| ----------- | ------- | - |
| `ratelimit.fixedWindow(store, limit, window)` | `Limiter` | a fixed-window limiter: `limit` hits per aligned `window` seconds |
| `ratelimit.slidingWindow(store, limit, window)` | `Limiter` | a sliding-window limiter (smooths the boundary burst) |
| `ratelimit.check(limiter, key)` | `Result` | **record** a hit and report the decision + metadata |
| `ratelimit.peek(limiter, key)` | `Result` | report the current state **without** recording a hit |
| `ratelimit.Result` | | `allowed` (bool), `remaining` (int), `retryAfter` (seconds, `0` when allowed), `resetSeconds` (seconds to the window roll) |

## Fixed vs sliding window

Both are driven by the wall clock and **window-aligned keys** (a counter per
aligned window), so they work identically on every backend, needing only atomic
increment.

- **Fixed window** - one counter per aligned window; simplest. A burst can
  straddle a window boundary: up to `2 * limit` requests across the seam (all of
  one window's budget at its end, then all of the next window's at its start).
- **Sliding window** - blends the current window's count with the previous
  window's, weighted by how far into the current window you are. At the very
  start of a new window the previous window still carries full weight, so a fresh
  burst is blocked; its influence decays linearly to zero by the window's end.
  This smooths the fixed-window boundary burst.

The counter is created and armed with the window TTL on the first hit, so it
resets on its own - nothing to reap.

## Building a 429

`check` returns everything a compliant response needs:

```jennifer
def r as ratelimit.Result init ratelimit.check($lim, $key);
if ($r.allowed) {
    # ... serve; optionally advertise X-RateLimit-Remaining: $r.remaining ...
} else {
    # HTTP 429 Too Many Requests
    #   Retry-After: $r.retryAfter
    #   X-RateLimit-Remaining: 0
    #   X-RateLimit-Reset: $r.resetSeconds
}
```

## See also

- [kvstore.md](kvstore.md) - the backend selector; [kv.md](../libraries/kv.md) -
  the in-process backend.
- [session.md](session.md) - the other module on the `kvstore` backend layer.
- [modules/index.md](index.md) - the module catalog and import rules.
