# `webdav` - WebDAV client

Import with `import "webdav.j" as webdav;`. A **WebDAV client** (RFC 4918) for
any WebDAV server: Nextcloud, ownCloud, Apache `mod_dav`, nginx, rclone, a NAS.
List and stat, read and write files, create collections, move, copy and
delete, and ask for or set any property. Over `http` + `xml`; needs the
default **`jennifer`** binary (`net` via `http`).

```jennifer
import "webdav.j" as webdav;

def c as webdav.Client init webdav.connect("https://cloud.example.com/remote.php/dav/files/alice");
$c = webdav.withBasicAuth($c, "alice", fs.readString("/run/secrets/nextcloud"));

for (def r in webdav.readDir($c, "/Documents")) {
    io.printf("%s %d %s\n", $r.path, $r.size, $r.etag);
}
webdav.mkdirAll($c, "/Backups/2026/11");
webdav.put($c, "/Backups/2026/11/db.sql.gz", fs.readBytes("db.sql.gz"));
def back as bytes init webdav.get($c, "/Backups/2026/11/db.sql.gz", 0);
```

Runnable: [`examples/modules/webdav_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/webdav_demo.j).

## Client

`webdav.connect(url)` takes the base URL every path is relative to, so `/`
is that URL and `/Documents/a.txt` is below it. Nextcloud and ownCloud serve
a user's files at `/remote.php/dav/files/<user>`. The `Client` is a value;
the `with*` functions return a changed copy.

| Function | Returns | Does |
|---|---|---|
| `webdav.connect(url)` | `Client` | a client for `http(s)://host[/path]` (no credentials in it); 30 s idle timeout |
| `webdav.withBasicAuth(c, user, password)` | `Client` | HTTP Basic (use an app password where the server has them) |
| `webdav.withToken(c, token)` | `Client` | `Authorization: Bearer` |
| `webdav.withHeader(c, name, value)` | `Client` | another header on every request |
| `webdav.withCaCert(c, pem)` | `Client` | trust a certificate (PEM bytes) |
| `webdav.withInsecure(c)` | `Client` | accept any certificate |
| `webdav.withTimeout(c, ms)` | `Client` | the idle timeout; 0 for 30 s |

Basic credentials travel readable over `http://`; use `https://`.

## Files and collections

| Function | Returns | Does |
|---|---|---|
| `webdav.readDir(c, path)` | `list of Resource` | a collection's members (`Depth: 1`), without itself |
| `webdav.walk(c, path)` | `list of Resource` | everything below a collection, `Depth: 1` per level |
| `webdav.stat(c, path)` | `Resource` | one file or collection |
| `webdav.exists(c, path)` | `bool` | whether it exists (404 is `false`) |
| `webdav.get(c, path, maxBytes)` | `bytes` | read a file; `maxBytes` 0 for 64 MiB |
| `webdav.put(c, path, data)` | `string` | write a file; its new ETag, or "" |
| `webdav.putWith(c, path, data, options)` | `string` | write with a media type, `ifMatch` or `onlyIfNew` |
| `webdav.mkdir(c, path)` | | create a collection (`MKCOL`) |
| `webdav.mkdirAll(c, path)` | | create it and any missing parents |
| `webdav.delete(c, path)` | | delete a file, or a collection and its contents |
| `webdav.move(c, source, target, overwrite)` | | move or rename |
| `webdav.copy(c, source, target, overwrite)` | | copy, a collection with everything in it |

`walk` asks one collection at a time because many servers refuse
`Depth: infinity`; it stops past `webdav.MAX_WALK` entries. `readDir` (and
so `walk`) throws when the server lists a path that is not a direct member of
the collection, so a listing cannot lead the walk outside the tree. A collection
named without its trailing slash is fine: when a server redirects to the
slashed URL (Apache does), the request is sent there once.

### Not overwriting a change

`putWith` writes only if the file is unchanged since you read its ETag, or
only if it does not exist yet; a failed condition is an error with HTTP 412.
The ETag has to be strong: Apache reports a weak one (`W/"..."`) for a file
changed within the last second, and `putWith` refuses that rather than send
a condition that can never hold:

```jennifer
def r as webdav.Resource init webdav.stat($c, "/notes.txt");
webdav.putWith($c, "/notes.txt", $data, webdav.PutOptions{contentType: "text/plain", ifMatch: $r.etag, onlyIfNew: false});
```

## `Resource`

| Field | Meaning |
|---|---|
| `href as string` | the server path, percent-decoded; a collection's ends in `/` |
| `path as string` | the path below the client's URL, e.g. `/a.txt` |
| `name as string` | the last segment |
| `isDir as bool` | whether it is a collection |
| `size as int` | bytes; 0 for a collection |
| `modified as int` | last change, Unix seconds; 0 when unknown |
| `etag as string` | the entity tag as sent, quotes included |
| `contentType as string` | the media type; "" when unknown |
| `quotaUsed as int` | bytes used (RFC 4331); -1 when not reported |
| `quotaAvailable as int` | bytes available (RFC 4331); -1 when not reported |
| `props as map of string to string` | every other property by its Clark name |

## Any property

Properties are named in **Clark notation**, `{namespace}local`;
`webdav.clark(ns, local)` builds one.

```jennifer
def oc as string init "http://owncloud.org/ns";
def found as list of webdav.Resource init webdav.propfind($c, "/Documents", 1, [webdav.clark($oc, "fileid")]);
io.printf("%s\n", $found[0].props[webdav.clark($oc, "fileid")]);
webdav.proppatch($c, "/Documents/a.txt", {webdav.clark($oc, "favorite"): "1"}, []);
```

| Function | Returns | Does |
|---|---|---|
| `webdav.propfind(c, path, depth, props)` | `list of Resource` | depth 0 or 1; `[]` asks for all (`allprop`) |
| `webdav.proppatch(c, path, set, remove)` | | set and remove properties, all or none |
| `webdav.clark(ns, local)` | `string` | `{ns}local` |

A property that holds elements comes back as its XML; one the server reports
with a non-2xx status is left out.

## The protocol layer

The parts that do not send anything are exported, so a WebDAV server on
`httpd` (or a CalDAV / CardDAV client using `REPORT`) can share them:

| Function | Returns | Does |
|---|---|---|
| `webdav.parseMultistatus(body)` | `list of Resource` | decode a `207 Multi-Status` body |
| `webdav.multistatus(resources)` | `string` | encode resources as one, what a server answers a `PROPFIND` |
| `webdav.cleanPath(path)` | `string` | check and normalize a path |
| `webdav.encodeHref(path, isDir)` | `string` | a path as an href, each segment percent-encoded |
| `webdav.decodeHref(href)` | `string` | an href (path or full URL) as a decoded path |

`multistatus` and `parseMultistatus` are inverses: a `Resource` encoded and
decoded again comes back equal. `multistatus` writes every entry of `props`
as text, so a property that `parseMultistatus` returned as XML goes back as
escaped text, not as elements.

## Errors and safety

Every failure is a `webdav.WebdavError` (kind `"webdav"`), naming the method,
the path and the status: `webdav: MOVE /a: HTTP 412 Precondition Failed`. They
are fields too: `method`, `path`, `status` (412 a failed `ifMatch`, 423 locked,
507 out of space, ...; 0 when no response was read), and for a `207
Multi-Status` reply the first failed `member` with its status. Strings from
the caller or the server are cut short and stripped of control characters
in messages.

- Paths are checked before they reach a URL: no `.`, `..` or empty segment,
  no control character; each segment is percent-encoded.
- `delete`, `move` and `copy` refuse the client's own root.
- Entities are never resolved: a reply that uses one (external or not) is
  refused, so it can neither read a local file nor expand into gigabytes.
- `connect` refuses credentials in the URL (they would travel in every
  `Destination` header); use `withBasicAuth`. `withHeader` refuses the
  headers that frame a request (`Host`, `Content-Length`,
  `Transfer-Encoding`, `Connection`, ...).
- `http` keeps credentials on redirects to the same origin only.

Files are held in memory: `get` refuses one over its `maxBytes` (an `Error` of
kind `"limit"`, `webdav: GET <path>: ...`; a reply over `http`'s cap on any other
request is too), and `put`
sends one buffer. Locks (`LOCK` / `UNLOCK`) are not covered yet.
