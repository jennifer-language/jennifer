# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A rate limiter over a selectable key/value backend (`kvstore.Store` -
 * `memcache`, `redis`, or the in-process `kv`). Each key (an IP, a user, an API
 * token) is counted against a limit per time window; a `check` records a hit and
 * returns a `Result` with the decision plus the metadata for a compliant `429`
 * (remaining budget, reset time, and Retry-After). Two algorithms, both driven by
 * the wall clock and window-aligned keys (so they work identically on every
 * backend, needing only atomic increment):
 * - **fixed window** - a counter per aligned window; simplest, but a burst can
 *   straddle a window boundary (up to 2x the limit across the seam).
 * - **sliding window** - a weighted blend of the current and previous window
 *   counts, which smooths that boundary burst.
 * @module ratelimit
 * @example
 * def store as kvstore.Store init kvstore.redisStore($rc);
 * def lim as ratelimit.Limiter init ratelimit.slidingWindow($store, 100, 60);
 * def r as ratelimit.Result init ratelimit.check($lim, "ip:203.0.113.7");
 * if (not $r.allowed) {
 *     # respond 429 with Retry-After: $r.retryAfter
 * }
 */
use convert;
use math;
use time;
import "./kvstore.j" as kvstore;

/**
 * A configured limiter: a backend store, a per-window `limit`, a `window` in
 * seconds, and the algorithm. Value-semantic; build with `fixedWindow` /
 * `slidingWindow`.
 * @field store {kvstore.Store} the backend store
 * @field limit {int} the maximum hits allowed per window
 * @field window {int} the window length in seconds
 * @field algorithm {string} "fixed" or "sliding"
 */
export def struct Limiter {
    store as kvstore.Store,
    limit as int,
    window as int,
    algorithm as string
};

/**
 * The outcome of a `check` / `peek`.
 * @field allowed {bool} whether this hit is within the limit
 * @field remaining {int} hits left in the window before the limit (0 once exhausted)
 * @field retryAfter {int} seconds to wait before retrying (0 when allowed) - the `Retry-After` value
 * @field resetSeconds {int} seconds until the current window rolls over
 */
export def struct Result {
    allowed as bool,
    remaining as int,
    retryAfter as int,
    resetSeconds as int
};

/**
 * A fixed-window limiter: `limit` hits per aligned `window` seconds.
 * @param store {kvstore.Store} the backend store
 * @param limit {int} the maximum hits per window
 * @param window {int} the window length in seconds
 * @return {Limiter} the configured limiter
 */
export func fixedWindow(store as kvstore.Store, limit as int, window as int) {
    requirePositive($limit, $window);
    return Limiter{store: $store, limit: $limit, window: $window, algorithm: "fixed"};
}

# requirePositive rejects a non-positive limit / window up front, with a clear
# error - rather than a later, cryptic division-by-zero at `now // window`.
# MAX_WINDOW bounds the window (seconds) so the aligned-index arithmetic
# ((idx+1)*window and the 2*window TTL) cannot overflow int64 into a wrong reset
# deadline / TTL.
def const MAX_WINDOW as int init 2147483647;

func requirePositive(limit as int, window as int) {
    if ($limit < 1 or $window < 1) {
        throw Error{
            kind: "ratelimit",
            message: "ratelimit: limit and window must both be >= 1",
            file: "",
            line: 0,
            col: 0
        };
    }
    if ($window > MAX_WINDOW) {
        throw Error{
            kind: "ratelimit",
            message: "ratelimit: window exceeds the maximum of " + convert.toString(MAX_WINDOW),
            file: "",
            line: 0,
            col: 0
        };
    }
}

/**
 * A sliding-window limiter: like `fixedWindow`, but blends the current and
 * previous window counts so a burst cannot straddle a window boundary.
 * @param store {kvstore.Store} the backend store
 * @param limit {int} the maximum hits per window
 * @param window {int} the window length in seconds
 * @return {Limiter} the configured limiter
 */
export func slidingWindow(store as kvstore.Store, limit as int, window as int) {
    requirePositive($limit, $window);
    return Limiter{store: $store, limit: $limit, window: $window, algorithm: "sliding"};
}

# --- helpers (private) ---------------------------------------------

func nowSeconds() {
    return time.unix(time.now());
}

# winKey names a window-aligned counter key.
func winKey(key as string, idx as int) {
    return $key + ":" + convert.toString($idx);
}

# countAt reads a window counter (0 when absent / expired).
func countAt(store as kvstore.Store, wk as string) {
    def v as string init kvstore.get($store, $wk);
    if (len($v) == 0) {
        return 0;
    }
    return convert.toInt($v);
}

func clampLow(n as int, lo as int) {
    if ($n < $lo) {
        return $lo;
    }
    return $n;
}

# --- fixed window --------------------------------------------------

# fixedAt evaluates the fixed-window algorithm at an explicit `now` (seconds); it
# records a hit when `record` is true. Split out so the overlay can drive window
# boundaries deterministically without waiting on the clock.
func fixedAt(lim as Limiter, key as string, now as int, record as bool) {
    def w as int init $lim.window;
    def idx as int init $now // $w;
    def resetIn as int init ($idx + 1) * $w - $now;
    def wk as string init winKey($key, $idx);
    def allowed as bool init true;
    def rem as int init 0;
    if ($record) {
        def count as int init kvstore.incrWindow($lim.store, $wk, 2 * $w);
        $allowed = $count <= $lim.limit;
        $rem = $lim.limit - $count;
    } else {
        def stored as int init countAt($lim.store, $wk);
        $allowed = $stored < $lim.limit;
        $rem = $lim.limit - $stored;
    }
    def retry as int init 0;
    if (not $allowed) {
        $retry = $resetIn;
    }
    return Result{
        allowed: $allowed,
        remaining: clampLow($rem, 0),
        retryAfter: $retry,
        resetSeconds: $resetIn
    };
}

# --- sliding window ------------------------------------------------

# slidingRetry estimates the seconds until a denied request would fit: if the
# current window alone is full, wait for it to roll; otherwise wait for the
# previous window to age out enough. Capped at the window roll.
func slidingRetry(lim as Limiter, curCount as int, prevCount as int, elapsed as int, resetIn as int) {
    def w as int init $lim.window;
    if ($curCount >= $lim.limit or $prevCount == 0) {
        return $resetIn;
    }
    def room as int init $lim.limit - 1 - $curCount;
    if ($room < 0) {
        return $resetIn;
    }
    # need prevCount*(w-e')/w <= room  ->  e' >= w - w*room/prevCount
    def frac as float init $room / $prevCount;
    def eNeeded as int init $w - math.floor($w * $frac);
    def wait as int init $eNeeded - $elapsed;
    if ($wait < 1) {
        return 1;
    }
    if ($wait > $resetIn) {
        return $resetIn;
    }
    return $wait;
}

# slidingAt evaluates the sliding-window algorithm at an explicit `now`.
func slidingAt(lim as Limiter, key as string, now as int, record as bool) {
    def w as int init $lim.window;
    def idx as int init $now // $w;
    def elapsed as int init $now - $idx * $w;
    def resetIn as int init ($idx + 1) * $w - $now;
    def curK as string init winKey($key, $idx);
    def prevCount as int init countAt($lim.store, winKey($key, $idx - 1));
    # weight of the previous window: 1.0 at the window start, 0 at its end.
    def weight as float init ($w - $elapsed) / $w;
    def prevWeighted as float init $prevCount * $weight;
    def curCount as int init 0;
    def load as float init 0.0;
    def allowed as bool init true;
    if ($record) {
        $curCount = kvstore.incrWindow($lim.store, $curK, 2 * $w);
        $load = $curCount + $prevWeighted;
        $allowed = $load <= $lim.limit;
    } else {
        $curCount = countAt($lim.store, $curK);
        $load = $curCount + $prevWeighted;
        $allowed = ($load + 1.0) <= $lim.limit; # would a new hit fit
    }
    def remF as float init $lim.limit - $load;
    def rem as int init 0;
    if ($remF > 0.0) {
        $rem = math.floor($remF);
    }
    def retry as int init 0;
    if (not $allowed) {
        $retry = slidingRetry($lim, $curCount, $prevCount, $elapsed, $resetIn);
    }
    return Result{allowed: $allowed, remaining: $rem, retryAfter: $retry, resetSeconds: $resetIn};
}

# --- dispatch (exported) -------------------------------------------

func evalAt(lim as Limiter, key as string, now as int, record as bool) {
    if ($lim.algorithm == "sliding") {
        return slidingAt($lim, $key, $now, $record);
    }
    return fixedAt($lim, $key, $now, $record);
}

/**
 * Record one hit against `key` and return the `Result` (decision + remaining +
 * reset + retry-after). The counter is created and armed with the window TTL on
 * the first hit, so it resets on its own - nothing to reap.
 * @param limiter {Limiter} the configured limiter
 * @param key {string} the counter key (an IP, user, or API token)
 * @return {Result} the decision and rate-limit metadata
 */
export func check(limiter as Limiter, key as string) {
    return evalAt($limiter, $key, nowSeconds(), true);
}

/**
 * Report the current state for `key` **without** recording a hit: `allowed` is
 * whether the next hit would be within the limit, plus the same remaining / reset
 * / retry-after metadata.
 * @param limiter {Limiter} the configured limiter
 * @param key {string} the counter key
 * @return {Result} the current rate-limit state
 */
export func peek(limiter as Limiter, key as string) {
    return evalAt($limiter, $key, nowSeconds(), false);
}
