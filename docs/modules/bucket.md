# `bucket` - S3-compatible object storage

Import with `import "bucket.j" as bucket;`. An **object-storage client** for
Amazon S3 and every S3-compatible store - the endpoint is configurable, so **one
module** serves AWS S3, MinIO, Cloudflare R2, and Backblaze B2 (a selectable
backend, not a module per vendor). Every request is signed with **AWS Signature
Version 4** (HMAC-SHA256 key-chaining), built on `hash.hmac` + `hash.compute` +
`encoding` (hex) + `time` (the request timestamp) + `http`. Needs the default
**`jennifer`** binary (`net` via `http`).

Named `bucket` (not `s3`) because a module namespace is letters-only - the same
reason `pop` is not `pop3`.

```jennifer
import "bucket.j" as bucket;
import "http.j" as http;

def c as bucket.Client init bucket.connect(
    "https://s3.us-east-1.amazonaws.com", "us-east-1", accessKey, secretKey);

def put as http.Response init bucket.put($c, "mybucket", "hello.txt", "hi there");
def obj as http.Response init bucket.get($c, "mybucket", "hello.txt");
io.printf("%d\n%s\n", $obj.status, $obj.body);
```

Runnable: [`examples/modules/bucket_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/bucket_demo.j).

## Client

`bucket.connect(endpoint, region, accessKey, secretKey)` returns a value-semantic
`bucket.Client`. The `endpoint` is any S3-compatible base URL (`scheme://host`,
no trailing slash); addressing is **path-style** (`{endpoint}/{bucket}/{key}`),
which works uniformly across AWS and self-hosted stores.

Every request carries a **timeout** so a hung S3 endpoint fails instead of
blocking forever (the classic way a slow store exhausts a worker pool). `connect`
defaults `Client.timeout` to 30 000 ms; set it to change the bound, or to `0` to
disable it:

```jennifer
def c as bucket.Client init bucket.connect(endpoint, region, key, secret);
$c.timeout = 5000;   # fail a request that stalls for 5 s
```

| Store | endpoint | region |
| ----- | -------- | ------ |
| AWS S3 | `https://s3.<region>.amazonaws.com` | your bucket's region |
| MinIO | `http://host:9000` | `us-east-1` (or as configured) |
| Cloudflare R2 | `https://<account>.r2.cloudflarestorage.com` | `auto` |
| Backblaze B2 | `https://s3.<region>.backblazeb2.com` | your bucket's region |

## Operations

Every call returns an `http.Response` (`status` / `headers` / `body`); reading it
needs `import "http.j"`. A non-2xx status is a value to branch on, not an error.

| Call | Method | Notes |
| ---- | ------ | ----- |
| `bucket.get(client, bucket, key)` | GET | `body` is the object contents; a missing object is a 404. |
| `bucket.put(client, bucket, key, body)` | PUT | Upload / overwrite; 200 on success. |
| `bucket.delete(client, bucket, key)` | DELETE | 204 on success. |
| `bucket.listObjects(client, bucket)` | GET `?list-type=2` | `body` is the ListObjectsV2 XML. |
| `bucket.objectKeys(xml)` | - | Pull the `<Key>` values out of a `listObjects` body -> `list of string`. |

(The list op is `listObjects`, not `list`, because `list` is a reserved type
keyword.)

```jennifer
def r as http.Response init bucket.listObjects($c, "mybucket");
for (def k in bucket.objectKeys($r.body)) {
    io.printf("%s\n", $k);
}
```

## Signing

Requests are signed with SigV4 for service `s3`: the canonical request covers the
method, the URI-encoded path (object keys keep their `/`), the query, the signed
headers `host` / `x-amz-content-sha256` / `x-amz-date`, and the SHA-256 of the
body; the string-to-sign is HMAC-chained through
`AWS4<secret> -> date -> region -> s3 -> aws4_request` to the signature. The
payload hash is a real SHA-256 of the body (not `UNSIGNED-PAYLOAD`), so the whole
request is integrity-covered. The signature is pinned in the tests against two
independent SigV4 implementations.

## Scope

- **Path-style, SigV4, `s3` service.** Virtual-hosted addressing and the older
  SigV2 are out of scope.
- **String bodies.** Objects are sent / received as text (`http`'s current body
  type); a `bytes` body accessor is a planned follow-on, alongside a binary
  object path.
- **Core object ops.** Multipart upload, presigned URLs, bucket create / delete,
  and ACL / policy management are not covered.
- **`listObjects` returns the raw XML** (plus `objectKeys`); pagination
  (continuation tokens) and full metadata parsing are follow-ons.

## See also

- [hash.md](../libraries/hash.md) - the `hmac` / `compute` primitives SigV4 builds on.
- [http.md](http.md) - the client transport requests go through.
- [webhook.md](webhook.md) / [totp.md](totp.md) - the other `hash.hmac`-based modules.
- [modules/index.md](index.md) - the module catalog and import rules.
