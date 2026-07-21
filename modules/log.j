# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Leveled, structured logging. A `log.Logger` carries a minimum level
 * (`debug` < `info` < `warn` < `error`), an output format (`text` / `logfmt` /
 * `json`), and a sink; `log.info(logger, message, fields)` (and the sibling
 * levels) render one record - a timestamp, the level, the message, and the
 * caller's key/value `fields` - and write it, dropping records below the
 * logger's level.
 *
 * Sinks: `stdout` / `stderr` (both binaries, via `io`), a `file` (append, via
 * `fs`), and an RFC 5424 **syslog** sink over UDP (`net`, so the syslog sink
 * needs the default `jennifer` binary; the console / file sinks work on both).
 * Over `io` / `fs` + `json` + `strings` + `time` + `os` (+ `net` for syslog).
 * @module log
 * @example
 * def lg as log.Logger init log.new("info", "logfmt");
 * def f as map of string to string init {"user": "ada", "id": "42"};
 * log.info($lg, "user logged in", $f);
 * # time=2026-... level=info msg="user logged in" user=ada id=42
 */
use io;
use fs;
use net;
use json;
use strings;
use convert;
use time;
use os;

/**
 * A value-semantic logger configuration. Build one with `new` / `toStderr` /
 * `toFile` / `toSyslog`.
 * @field level {string} the minimum level to emit: "debug", "info", "warn", or "error"
 * @field format {string} the record format: "text", "logfmt", or "json" (ignored by the syslog sink)
 * @field sink {string} where records go: "stdout", "stderr", "file", or "syslog"
 * @field target {string} the file path (file sink) or "host:port" (syslog sink); "" for a console sink
 * @field app {string} an application / tag name (the syslog APP-NAME; "" means none)
 */
export def struct Logger {
    level as string,
    format as string,
    sink as string,
    target as string,
    app as string
};

# --- constructors (exported) ------------------------------------------------

/**
 * A logger writing to standard output.
 * @param level {string} the minimum level ("debug"/"info"/"warn"/"error")
 * @param format {string} "text", "logfmt", or "json"
 * @return {Logger} the logger
 */
export func new(level as string, format as string) {
    return Logger{ level: $level, format: $format, sink: "stdout", target: "", app: "" };
}

/**
 * A logger writing to standard error.
 * @param level {string} the minimum level
 * @param format {string} "text", "logfmt", or "json"
 * @return {Logger} the logger
 */
export func toStderr(level as string, format as string) {
    return Logger{ level: $level, format: $format, sink: "stderr", target: "", app: "" };
}

/**
 * A logger appending to a file (created if missing).
 * @param level {string} the minimum level
 * @param format {string} "text", "logfmt", or "json"
 * @param path {string} the file path
 * @return {Logger} the logger
 */
export func toFile(level as string, format as string, path as string) {
    return Logger{ level: $level, format: $format, sink: "file", target: $path, app: "" };
}

/**
 * A logger sending RFC 5424 records to a syslog server over UDP. Needs the
 * default `jennifer` binary (`net`).
 * @param level {string} the minimum level
 * @param address {string} the syslog server "host:port" (e.g. "localhost:514")
 * @param app {string} the APP-NAME tag
 * @return {Logger} the logger
 */
export func toSyslog(level as string, address as string, app as string) {
    return Logger{ level: $level, format: "syslog", sink: "syslog", target: $address, app: $app };
}

# --- level filtering (private) ----------------------------------------------

# rank orders the levels; an unknown level ranks as info.
func rank(level as string) {
    if ($level == "debug") {
        return 0;
    }
    if ($level == "warn") {
        return 2;
    }
    if ($level == "error") {
        return 3;
    }
    return 1;
}

func shouldLog(logger as Logger, level as string) {
    return rank($level) >= rank($logger.level);
}

# --- record rendering (private) ---------------------------------------------

# quoteIfNeeded wraps a value in double quotes when it needs them - a space,
# quote, `=`, backslash, control character, or an empty value - and escapes the
# content so it round-trips and cannot forge a record. Backslash is escaped
# first (before the quote / newline escapes we add), and a raw newline is
# encoded rather than left to split the record into a forged second line.
func quoteIfNeeded(v as string) {
    def needs as bool init len($v) == 0
        or strings.contains($v, " ")
        or strings.contains($v, "\"")
        or strings.contains($v, "=")
        or strings.contains($v, "\\")
        or strings.contains($v, "\n")
        or strings.contains($v, "\r")
        or strings.contains($v, "\t");
    if (not $needs) {
        return $v;
    }
    def e as string init strings.replace($v, "\\", "\\\\");
    $e = strings.replace($e, "\"", "\\\"");
    $e = strings.replace($e, "\n", "\\n");
    $e = strings.replace($e, "\r", "\\r");
    $e = strings.replace($e, "\t", "\\t");
    return "\"" + $e + "\"";
}

# escapeNewlines encodes CR / LF so an untrusted string emitted unquoted (the
# text-format message) cannot inject a forged log line.
func escapeNewlines(v as string) {
    def e as string init strings.replace($v, "\r", "\\r");
    return strings.replace($e, "\n", "\\n");
}

# renderText: `<ts> <LEVEL> <message> k=v ...`.
func renderText(level as string, message as string, fields as map of string to string, ts as string) {
    def s as string init $ts + " " + strings.upper($level) + " " + escapeNewlines($message);
    for (def k in $fields) {
        $s = $s + " " + $k + "=" + quoteIfNeeded($fields[$k]);
    }
    return $s;
}

# renderLogfmt: `time=<ts> level=<level> msg="..." k=v ...`.
func renderLogfmt(level as string, message as string, fields as map of string to string, ts as string) {
    def s as string init "time=" + $ts + " level=" + $level + " msg=" + quoteIfNeeded($message);
    for (def k in $fields) {
        $s = $s + " " + $k + "=" + quoteIfNeeded($fields[$k]);
    }
    return $s;
}

# renderJson: a JSON object `{"time":..,"level":..,"msg":..,<fields>}`.
func renderJson(level as string, message as string, fields as map of string to string, ts as string) {
    def rec as map of string to string init {};
    $rec["time"] = $ts;
    $rec["level"] = $level;
    $rec["msg"] = $message;
    for (def k in $fields) {
        # A caller field colliding with a reserved key would otherwise overwrite
        # the record's own time/level/msg (defeating level-based alerting);
        # prefix it instead so the value survives without shadowing.
        if ($k == "time" or $k == "level" or $k == "msg") {
            $rec["field." + $k] = $fields[$k];
        } else {
            $rec[$k] = $fields[$k];
        }
    }
    return json.encode($rec);
}

# render produces one record line per the logger's format.
func render(logger as Logger, level as string, message as string, fields as map of string to string, t as time.Time) {
    def ts as string init time.iso($t);
    if ($logger.format == "json") {
        return renderJson($level, $message, $fields, $ts);
    }
    if ($logger.format == "logfmt") {
        return renderLogfmt($level, $message, $fields, $ts);
    }
    return renderText($level, $message, $fields, $ts);
}

# --- syslog (private) -------------------------------------------------------

# syslogSeverity maps a level to an RFC 5424 severity (err=3 .. debug=7).
func syslogSeverity(level as string) {
    if ($level == "error") {
        return 3;
    }
    if ($level == "warn") {
        return 4;
    }
    if ($level == "debug") {
        return 7;
    }
    return 6;
}

# syslogLine renders an RFC 5424 line: `<PRI>1 <ts> <host> <app> - - - <msg>`
# (facility 1 = user). Fields ride in the message as logfmt pairs.
func syslogLine(logger as Logger, level as string, message as string, fields as map of string to string, t as time.Time) {
    def pri as int init 8 + syslogSeverity($level);
    def host as string init os.getEnv("HOSTNAME");
    if ($host == "") {
        $host = "-";
    }
    def app as string init $logger.app;
    if ($app == "") {
        $app = "-";
    }
    def msg as string init $message;
    for (def k in $fields) {
        $msg = $msg + " " + $k + "=" + quoteIfNeeded($fields[$k]);
    }
    return "<" + convert.toString($pri) + ">1 " + time.iso($t) + " " + $host + " " + $app + " - - - " + $msg;
}

# sendSyslog opens a fresh UDP socket per record. A persistent socket would need
# mutable module state or a mutable handle on the value-semantic Logger, neither
# of which the module model allows, so this is a documented cost: high-volume
# syslog logging pays one socket setup/teardown syscall pair per line. For hot
# paths, prefer the file or console sink (or batch upstream).
func sendSyslog(logger as Logger, line as string) {
    def sock as net.UDPSocket init net.listenUDP(":0");
    defer net.close($sock);              # closed even when the send throws
    net.sendTo($sock, $logger.target, convert.bytesFromString($line, "utf-8"));
    return null;
}

# --- emit (private) ---------------------------------------------------------

func emit(logger as Logger, level as string, message as string, fields as map of string to string) {
    def t as time.Time init time.utc();
    if ($logger.sink == "syslog") {
        return sendSyslog($logger, syslogLine($logger, $level, $message, $fields, $t));
    }
    def line as string init render($logger, $level, $message, $fields, $t);
    if ($logger.sink == "stderr") {
        io.eprintf("%s\n", $line);
    } elseif ($logger.sink == "file") {
        fs.appendString($logger.target, $line + "\n");
    } else {
        io.printf("%s\n", $line);
    }
    return null;
}

# --- logging (exported) -----------------------------------------------------

/**
 * Emit a record at an explicit level (skipped if below the logger's level).
 * @param logger {Logger} the logger
 * @param level {string} "debug", "info", "warn", or "error"
 * @param message {string} the log message
 * @param fields {map of string to string} structured key/value fields ({} for none)
 * @throws {Error} on a sink failure (a positioned `fs` / `net` error)
 */
export func at(logger as Logger, level as string, message as string, fields as map of string to string) {
    if (shouldLog($logger, $level)) {
        emit($logger, $level, $message, $fields);
    }
    return null;
}

/**
 * Emit a debug record.
 * @param logger {Logger} the logger
 * @param message {string} the log message
 * @param fields {map of string to string} structured fields ({} for none)
 */
export func debug(logger as Logger, message as string, fields as map of string to string) {
    return at($logger, "debug", $message, $fields);
}

/**
 * Emit an info record.
 * @param logger {Logger} the logger
 * @param message {string} the log message
 * @param fields {map of string to string} structured fields ({} for none)
 */
export func info(logger as Logger, message as string, fields as map of string to string) {
    return at($logger, "info", $message, $fields);
}

/**
 * Emit a warn record.
 * @param logger {Logger} the logger
 * @param message {string} the log message
 * @param fields {map of string to string} structured fields ({} for none)
 */
export func warn(logger as Logger, message as string, fields as map of string to string) {
    return at($logger, "warn", $message, $fields);
}

/**
 * Emit an error record.
 * @param logger {Logger} the logger
 * @param message {string} the log message
 * @param fields {map of string to string} structured fields ({} for none)
 */
export func error(logger as Logger, message as string, fields as map of string to string) {
    return at($logger, "error", $message, $fields);
}
