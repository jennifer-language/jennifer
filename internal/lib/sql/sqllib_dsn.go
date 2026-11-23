// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// The TLS policy of sql.open / sql.openWith (M29.36). A connection that
// leaves the host needs verified TLS: a DSN that names no TLS mode gets the
// verifying one (Postgres `sslmode=verify-full`, MySQL `tls=true`), and one
// that names a weaker mode is refused unless sql.Options.allowInsecure is set.
// A Unix socket and a loopback host (`localhost`, 127.0.0.0/8, `::1`) are
// exempt: the traffic never leaves the host, so the DSN is used as written.
//
// The hosts come from the drivers' own parsers (pgconn.ParseConfig, which
// also reads PGHOST and a service file, and mysql.ParseDSN), so the host
// checked is the host dialled. No message here carries the DSN: the password
// is in it.

package sqllib

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// insecureFix is the opt-out every refusal names.
const insecureFix = "sql.openWith(driver, dsn, sql.Options{allowInsecure: true})"

// isLoopbackHost reports whether host is `localhost` or a loopback address.
// A name other than `localhost` is not resolved: a container's service name
// (`db`) reaches another host, so it is not exempt.
func isLoopbackHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// weakTLSReason says why a named TLS mode is refused, "" for a verifying one
// (or one the driver itself rejects later).
func weakTLSReason(mode string) string {
	switch mode {
	case "disable", "false":
		return "sends the password and the data in plaintext"
	case "allow", "prefer", "preferred":
		return "falls back to plaintext"
	case "require", "skip-verify":
		return "does not verify the server certificate"
	}
	return ""
}

func refuseWeakTLS(fn, param, mode, strong, host string) error {
	return fmt.Errorf("%s: %s=%s %s (host %s); use %s=%s, or %s to accept that", fn, param, mode, weakTLSReason(mode), host, param, strong, insecureFix)
}

// ---- Postgres ----

// isPostgresURL reports whether dsn is the URL form (pgx's own test).
func isPostgresURL(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// postgresSSLMode returns the sslmode a Postgres DSN names and whether it
// names one, reading the URL query or the keyword/value pairs the way pgx
// does (the last `sslmode=` of a keyword string wins, the first of a URL).
func postgresSSLMode(dsn string) (string, bool, error) {
	if isPostgresURL(dsn) {
		u, err := url.Parse(dsn)
		if err != nil {
			// url.Error carries the URL (and so the password); keep its cause.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}
			return "", false, err
		}
		if v, ok := u.Query()["sslmode"]; ok {
			return v[0], true, nil
		}
		return "", false, nil
	}
	settings, err := keywordSettings(dsn)
	if err != nil {
		return "", false, err
	}
	v, ok := settings["sslmode"]
	return v, ok, nil
}

// keywordSettings parses libpq's keyword/value form (`host=db
// password='a b'`) with pgconn's grammar: whitespace-separated pairs, a value
// bare (backslash escapes) or single-quoted.
func keywordSettings(s string) (map[string]string, error) {
	const space = " \t\n\r\v\f"
	settings := map[string]string{}
	s = strings.TrimLeft(s, space)
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			return nil, errors.New("invalid keyword/value")
		}
		key := strings.Trim(s[:eq], space)
		s = strings.TrimLeft(s[eq+1:], space)
		var val string
		switch {
		case len(s) == 0:
		case s[0] != '\'':
			end := 0
			for ; end < len(s); end++ {
				if strings.IndexByte(space, s[end]) >= 0 {
					break
				}
				if s[end] == '\\' {
					end++
					if end == len(s) {
						return nil, errors.New("invalid backslash")
					}
				}
			}
			val = strings.ReplaceAll(strings.ReplaceAll(s[:end], `\\`, `\`), `\'`, `'`)
			s = strings.TrimLeft(s[end:], space)
		default:
			s = s[1:]
			end := 0
			for ; end < len(s); end++ {
				if s[end] == '\'' {
					break
				}
				if s[end] == '\\' {
					end++
				}
			}
			if end >= len(s) {
				return nil, errors.New("unterminated quoted string")
			}
			val = strings.ReplaceAll(strings.ReplaceAll(s[:end], `\\`, `\`), `\'`, `'`)
			s = strings.TrimLeft(s[end+1:], space)
		}
		if key == "" {
			return nil, errors.New("invalid keyword/value")
		}
		settings[key] = val
	}
	return settings, nil
}

// postgresHosts lists every host a Postgres DSN may dial (a multi-host DSN
// tries each), from pgconn's parse, with whether it is a Unix socket.
func postgresHosts(dsn string) ([]string, []bool, error) {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		// ParseConfigError's text quotes the (redacted) DSN; keep its cause.
		if inner := errors.Unwrap(err); inner != nil {
			err = inner
		} else {
			err = errors.New("invalid connection string")
		}
		return nil, nil, err
	}
	hosts := []string{cfg.Host}
	ports := []uint16{cfg.Port}
	for _, fb := range cfg.Fallbacks {
		hosts = append(hosts, fb.Host)
		ports = append(ports, fb.Port)
	}
	unix := make([]bool, len(hosts))
	for i, h := range hosts {
		network, _ := pgconn.NetworkAddress(h, ports[i])
		unix[i] = network == "unix"
	}
	return hosts, unix, nil
}

// securePostgresDSN applies the TLS policy to a Postgres DSN and returns the
// DSN to open: unchanged, or with `sslmode=verify-full` added.
func securePostgresDSN(fn, dsn string, allowInsecure bool) (string, error) {
	mode, named, err := postgresSSLMode(dsn)
	if err != nil {
		return "", fmt.Errorf("%s: the Postgres DSN cannot be parsed: %s", fn, redactDSN(err.Error()))
	}
	hosts, unix, err := postgresHosts(dsn)
	if err != nil {
		return "", fmt.Errorf("%s: the Postgres DSN cannot be parsed: %s", fn, redactDSN(err.Error()))
	}
	remote := ""
	for i, h := range hosts {
		if !unix[i] && !isLoopbackHost(h) {
			remote = h
			break
		}
	}
	if remote == "" {
		return dsn, nil
	}
	if named {
		if weakTLSReason(mode) != "" && !allowInsecure {
			return "", refuseWeakTLS(fn, "sslmode", mode, "verify-full", remote)
		}
		return dsn, nil
	}
	if isPostgresURL(dsn) {
		base, frag, hasFrag := strings.Cut(dsn, "#")
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
			if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
				sep = ""
			}
		}
		out := base + sep + "sslmode=verify-full"
		if hasFrag {
			out += "#" + frag
		}
		return out, nil
	}
	if strings.TrimSpace(dsn) == "" {
		return "sslmode=verify-full", nil
	}
	return dsn + " sslmode=verify-full", nil
}

// ---- MySQL ----

// secureMySQLConfig applies the TLS policy to a MySQL DSN and returns the
// driver config to connect with (`tls=true` added where none is named).
func secureMySQLConfig(fn, dsn string, allowInsecure bool) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: the MySQL DSN cannot be parsed: %s", fn, redactDSN(err.Error()))
	}
	if cfg.Net == "unix" {
		return cfg, nil
	}
	host, _, serr := net.SplitHostPort(cfg.Addr)
	if serr != nil {
		host = cfg.Addr
	}
	if isLoopbackHost(host) {
		return cfg, nil
	}
	switch cfg.TLSConfig {
	case "":
		cfg.TLSConfig = "true"
		cfg.TLS = nil // NewConnector builds it, with the host as ServerName
	case "false", "skip-verify", "preferred":
		if !allowInsecure {
			return nil, refuseWeakTLS(fn, "tls", cfg.TLSConfig, "true", host)
		}
	}
	return cfg, nil
}
