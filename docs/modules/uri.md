# `uri` - URL / URI parsing, building, and query strings (RFC 3986)

Import with `import "uri.j" as uri;`. The shared URL layer the network modules
(`http`, `rest`, `s3`, `oauth`, `influxdb`, ...) build on instead of
re-splitting strings and hand-rolling percent-encoding. `parse` splits a URL
into its parts; `build` reassembles one; `encode` / `decode` and `encodeForm` /
`decodeForm` are the two web encodings; `buildQuery` / `parseQuery` convert
between a `map of string to string` and a query string; and `resolve` applies a
relative reference to a base URL. Pure Jennifer over `strings` + `encoding` +
`convert` - no Go, no network - so it runs on **either binary**.

```jennifer
import "uri.j" as uri;

def u as uri.Uri init uri.parse("https://user@example.com:8443/a/b?x=1#top");
# $u.scheme "https", $u.user "user", $u.host "example.com", $u.port "8443",
# $u.path "/a/b", $u.query "x=1", $u.fragment "top"

def q as string init uri.buildQuery({"name": "a b", "tag": "x&y"});  # "name=a+b&tag=x%26y"
def abs as string init uri.resolve("http://h/a/b/page.html", "../img.png");  # "http://h/a/img.png"
```

Runnable: [`examples/modules/uri_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/uri_demo.j)

## The `Uri` struct

`parse` returns, and `build` consumes, a value-semantic `Uri`. Missing
components are the empty string, so a relative URL has an empty scheme and host.
`port` is a `string` ("" when absent) rather than an `int`, to distinguish "no
port" from port 0.

| Field      | Type     | Notes                                                     |
| ---------- | -------- | --------------------------------------------------------- |
| `scheme`   | `string` | the scheme without the trailing ":" (e.g. `"https"`)      |
| `user`     | `string` | the userinfo before "@" ("" when absent)                  |
| `host`     | `string` | the host name or IP literal ("" when absent)              |
| `port`     | `string` | the port after ":" ("" when absent)                       |
| `path`     | `string` | the path, including its leading "/" when present          |
| `query`    | `string` | the raw query string without the leading "?"              |
| `fragment` | `string` | the fragment without the leading "#"                      |

## Surface

| Call                          | Returns                    | Notes                                                              |
| ----------------------------- | -------------------------- | ------------------------------------------------------------------ |
| `uri.parse(raw)`              | `Uri`                      | Split a URL into its parts. Absent components are "".              |
| `uri.build(u)`                | `string`                   | Reassemble a URL from a `Uri` (the inverse of `parse`); no re-encoding. |
| `uri.encode(s)`               | `string`                   | RFC 3986 percent-encode (space -> `%20`). For a path segment.     |
| `uri.decode(s)`               | `string`                   | Reverse percent-encoding; a literal "+" stays "+".                |
| `uri.encodeForm(s)`           | `string`                   | `application/x-www-form-urlencoded` (space -> `+`). For a query value / form body. |
| `uri.decodeForm(s)`           | `string`                   | Reverse form-encoding; a "+" decodes to a space.                  |
| `uri.buildQuery(params)`      | `string`                   | A `map` -> `key=value&...` (form-encoded, insertion order), no leading "?". |
| `uri.parseQuery(q)`           | `map of string to string`  | A query string -> a `map` (form-decoded); a bare "key" maps to "". |
| `uri.resolve(base, ref)`      | `string`                   | Resolve a relative reference against a base URL (RFC 3986 §5).    |

## Percent vs form encoding

Two byte-level encodings share the same unreserved set (`A`-`Z` `a`-`z` `0`-`9`
`-` `.` `_` `~`, left literal) but differ on the space:

- **`encode` / `decode`** are **RFC 3986 percent-encoding**: a space is `%20`,
  and a literal "+" is preserved on decode. Use these for **path segments** and
  anywhere the RFC 3986 rules apply.
- **`encodeForm` / `decodeForm`** are **`application/x-www-form-urlencoded`**: a
  space is `+` (so a literal "+" encodes as `%2B`). Use these for **query-string
  values** and HTML form bodies. `buildQuery` / `parseQuery` use them, matching
  the query-string convention every browser and HTTP client follows.

Both sit on the [`encoding`](../libraries/encoding.md) library's `uri-percent` /
`uri-form` codecs, so a program that only needs the raw byte transform can call
`encoding.toText(b, "uri-percent")` directly. A malformed `%` escape is a
catchable error.

## Reference resolution (RFC 3986 §5)

`resolve(base, ref)` turns a possibly-relative reference into an absolute URL,
the operation a feed reader or crawler needs to follow relative links:

```jennifer
def base as string init "http://h/a/b/page.html";
uri.resolve($base, "sibling");     # "http://h/a/b/sibling"
uri.resolve($base, "../img.png");  # "http://h/a/img.png"
uri.resolve($base, "/root/x");     # "http://h/root/x"
uri.resolve($base, "//other/z");   # "http://other/z"  (network-path reference)
uri.resolve($base, "https://x/y"); # "https://x/y"     (absolute ref, returned as-is)
```

It applies the RFC 3986 §5.2.4 `remove_dot_segments` algorithm, so `.` and `..`
segments collapse (`/a/b/../c` -> `/a/c`).

## Notes and scope

- **IPv6 literals** keep their brackets in `host` (`[::1]`) with the port split
  out, per RFC 3986: `uri.parse("http://[::1]:9000/x")` gives `host` `"[::1]"`,
  `port` `"9000"`.
- **`build` does not re-encode.** Components are emitted verbatim, so a `Uri`
  you assembled by hand must already carry encoded values (use `encode` /
  `encodeForm` yourself). This keeps `parse` -> `build` an exact round-trip.
- **No scheme registry / normalization.** `uri` does not lowercase the scheme,
  drop a default port, or know that `http` implies port 80 - it is a syntactic
  parser, not a URL normalizer.

## See also

- [encoding.md](../libraries/encoding.md) - the `uri-percent` / `uri-form`
  byte-level codecs this module builds on.
- [rest.md](rest.md) / [http.md](http.md) - the HTTP clients that consume
  `buildQuery` and `resolve`.
- [modules/index.md](index.md) - the module catalog and import rules.
