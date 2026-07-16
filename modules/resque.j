# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Background jobs on Redis, wire-compatible with Resque. Schedule a job onto a
 * named queue now (`enqueue`), reserve and run it from a worker later
 * (`reserve`). The Redis layout is the de-facto Resque standard - queues are
 * lists at `resque:queue:NAME`, the queue registry is a set at `resque:queues`,
 * and a job is the JSON envelope `{"class":"WorkerName","args":[...]}` - so a
 * job Jennifer enqueues can be processed by a Ruby-resque / php-resque worker
 * and vice versa. `args` is a `list of string` mapping to Ruby-resque's
 * positional arguments; the class string is resolved on the worker's side.
 * Built on the `redis` module, so it needs the default `jennifer` binary.
 * @module resque
 * @example
 * import "resque.j" as resque;
 * import "redis.j" as redis;
 * def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1", port: 6379, security: "none", user: "", password: "", db: 0});
 * resque.enqueue($db, "email", "SendWelcome", ["user@example.com", "en"]);
 * def job as resque.Job init resque.reserve($db, ["high", "email"]);
 */
use strings;
use convert;
use json;
use time;
import "./redis.j" as redis;

# The Redis key namespace (`resque` is the Resque default; both ends must agree).
def const NAMESPACE as string init "resque";

/**
 * A reserved job. A drained `reserve` returns an empty `Job` (`class` is "").
 * @field queue {string} the origin queue the job came from
 * @field class {string} the worker class to run
 * @field args {list of string} the job's positional string arguments
 */
export def struct Job {
    queue as string,
    class as string,
    args as list of string
};

# The on-the-wire job envelope, encoded to `{"class":...,"args":[...]}`.
def struct Payload {
    class as string,
    args as list of string
};

# The worker identity recorded on a failure. Resque uses "host:pid:queues";
# without a pid / hostname surface here, a stable module identifier is used.
def const WORKER as string init "jennifer-resque:1:*";

# --- keys (private) ------------------------------------------------

func queuesKey() {
    return NAMESPACE + ":queues";
}

func queueKey(queue as string) {
    return NAMESPACE + ":queue:" + $queue;
}

func failedKey() {
    return NAMESPACE + ":failed";
}

# --- envelope (private) --------------------------------------------

# encodePayload renders the Resque job envelope for `class` + `args`.
func encodePayload(class as string, args as list of string) {
    return json.encode(Payload{class: $class, args: $args});
}

# argAt reads one `args` element as a string, whatever JSON type it holds (so a
# job enqueued elsewhere with an int / bool arg still reserves cleanly).
func argAt(node as json.Value, i as int) {
    def ptr as string init "/args/" + convert.toString($i);
    def t as string init json.typeOf($node, $ptr);
    if ($t == "string") {
        return json.asString($node, $ptr);
    }
    if ($t == "int") {
        return convert.toString(json.asInt($node, $ptr));
    }
    if ($t == "float") {
        return convert.toString(json.asFloat($node, $ptr));
    }
    if ($t == "bool") {
        return convert.toString(json.asBool($node, $ptr));
    }
    return json.encode(json.get($node, $ptr));
}

# decodeJob parses a reserved envelope into a Job tagged with its origin queue.
func decodeJob(queue as string, raw as string) {
    def node as json.Value init json.decode($raw);
    def args as list of string init [];
    def n as int init json.length($node, "/args");
    def i as int init 0;
    while ($i < $n) {
        $args[] = argAt($node, $i);
        $i = $i + 1;
    }
    return Job{queue: $queue, class: json.asString($node, "/class"), args: $args};
}

# --- producer (exported) -------------------------------------------

/**
 * Schedule a job: register the queue, then push the envelope onto it.
 * @param session {redis.Session} the open Redis session
 * @param queue {string} the queue name to push onto
 * @param class {string} the worker class the job names
 * @param args {list of string} the job's positional arguments
 */
export func enqueue(session as redis.Session, queue as string, class as string,
    args as list of string) {
    redis.command($session, ["SADD", queuesKey(), $queue]);
    redis.command($session, ["RPUSH", queueKey($queue), encodePayload($class, $args)]);
}

# --- consumer (exported) -------------------------------------------

/**
 * Pop the next job from the first non-empty queue, checking `queues` in the
 * given priority order.
 * @param session {redis.Session} the open Redis session
 * @param queues {list of string} the queue names to check, in priority order
 * @return {Job} the reserved job, or an empty `Job` (`class` "") when all drained
 */
export func reserve(session as redis.Session, queues as list of string) {
    for (def queue in $queues) {
        def r as redis.Reply init redis.command($session, ["LPOP", queueKey($queue)]);
        if ($r.kind == "string") {
            return decodeJob($queue, $r.str);
        }
    }
    return Job{queue: "", class: "", args: []};
}

/**
 * Record a failed job on the failed list (a simplified failure entry).
 * @param session {redis.Session} the open Redis session
 * @param job {Job} the job that failed
 * @param message {string} the failure message
 */
export func fail(session as redis.Session, job as Job, message as string) {
    # Emit the standard Resque failure record (nested `payload`, plus
    # `failed_at` / `exception` / `error` / `backtrace` / `worker` / `queue`)
    # so a Ruby / PHP Resque failure UI can parse it. Built with the json write
    # API because the keys carry underscores, which struct fields cannot.
    def rec as json.Value init json.map();
    $rec = json.set($rec, "/failed_at", time.iso(time.utc()));
    $rec = json.set($rec, "/payload", json.map());
    $rec = json.set($rec, "/payload/class", $job.class);
    $rec = json.set($rec, "/payload/args", json.list());
    for (def a in $job.args) {
        $rec = json.append($rec, "/payload/args", $a);
    }
    $rec = json.set($rec, "/exception", $job.class);
    $rec = json.set($rec, "/error", $message);
    $rec = json.set($rec, "/backtrace", json.list());
    $rec = json.set($rec, "/worker", WORKER);
    $rec = json.set($rec, "/queue", $job.queue);
    redis.command($session, ["RPUSH", failedKey(), json.encode($rec)]);
}

# --- introspection (exported) --------------------------------------

/**
 * Return the number of pending jobs on one queue.
 * @param session {redis.Session} the open Redis session
 * @param queue {string} the queue name
 * @return {int} the number of pending jobs
 */
export func queueLength(session as redis.Session, queue as string) {
    return redis.command($session, ["LLEN", queueKey($queue)]).num;
}

/**
 * Return the registered queue names.
 * @param session {redis.Session} the open Redis session
 * @return {list of string} the registered queue names
 */
export func queues(session as redis.Session) {
    def out as list of string init [];
    for (def item in redis.command($session, ["SMEMBERS", queuesKey()]).items) {
        $out[] = $item.str;
    }
    return $out;
}

/**
 * Return the total number of pending jobs across every registered queue.
 * @param session {redis.Session} the open Redis session
 * @return {int} the total number of pending jobs
 */
export func size(session as redis.Session) {
    def total as int init 0;
    for (def queue in queues($session)) {
        $total = $total + queueLength($session, $queue);
    }
    return $total;
}
