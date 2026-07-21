# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A fixed-window rate limiter on memcached, the sharpest use of memcached's
 * distinctive strength: atomic `incr` plus a per-key TTL. Each key (an IP, a
 * user, an API token) counts hits in a time window; when the counter is first
 * created it is armed with the window's expiry, so it resets on its own when
 * the window ends - nothing to reap. Built on the `memcache` module, so it
 * needs the default `jennifer` binary. Fixed window: the count resets when the
 * window expires, so a burst can span a window boundary. A sliding window /
 * token bucket is a later refinement.
 * @module ratelimit
 * @example
 * def mc as memcache.Session init memcache.connect(memcache.Options{
 *     host: "127.0.0.1", port: 11211});
 * if (ratelimit.allow($mc, "ip:203.0.113.7", 100, 60)) {
 *     # ... serve the request (100 per 60 seconds) ...
 * }
 */

use convert;
import "./memcache.j" as memcache;

# --- pure helpers (private) ----------------------------------------

# withinLimit reports whether a post-increment hit count is within the limit.
func withinLimit(count as int, limit as int) {
    return $count <= $limit;
}

# remainingFrom computes the remaining budget from a stored counter value
# ("" means the window has no hits yet, so the full limit is available).
func remainingFrom(value as string, limit as int) {
    if (len($value) == 0) {
        return $limit;
    }
    def left as int init $limit - convert.toInt($value);
    if ($left < 0) {
        return 0;
    }
    return $left;
}

# --- rate limiting (exported) --------------------------------------

/**
 * Record one hit against `key` and report whether it is within `limit` for the
 * current `window` (seconds). The window starts at the first hit: an absent
 * counter is created with the window's TTL, and the counter expires on its own
 * when the window ends.
 * @param mc {memcache.Session} the memcached connection
 * @param key {string} the counter key (an IP, user, or API token)
 * @param limit {int} the maximum hits allowed in the window
 * @param window {int} the window length in seconds
 * @return {bool} true if this hit is within the limit, false if it exceeds it
 */
export func allow(mc as memcache.Session, key as string, limit as int, window as int) {
    def n as int init memcache.incr($mc, $key, 1);
    if ($n == -1) {
        # first hit this window: create the counter carrying the window TTL
        if (memcache.add($mc, $key, "1", $window)) {
            $n = 1;
        } else {
            # another caller created it in the same instant; count this hit
            $n = memcache.incr($mc, $key, 1);
        }
    }
    return withinLimit($n, $limit);
}

/**
 * Report how many hits are left for `key` in the current window (the full
 * `limit` when the window has no hits yet, 0 once it is exhausted).
 * @param mc {memcache.Session} the memcached connection
 * @param key {string} the counter key
 * @param limit {int} the maximum hits allowed in the window
 * @return {int} the remaining hit budget for the current window
 */
export func remaining(mc as memcache.Session, key as string, limit as int) {
    return remainingFrom(memcache.get($mc, $key), $limit);
}
