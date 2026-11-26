# `s3presign` - S3 presigned URLs without a network

Import with `import "s3presign.j" as s3;`. The local half of [`s3`](s3.md):
presigned URLs for S3-compatible object storage (AWS S3, MinIO, Cloudflare R2,
Backblaze B2), signed with AWS Signature Version 4 query parameters. Building a
URL sends nothing, and the module imports no network module, so it runs on
**both** binaries, `jennifer-tiny` included.

It is a subset of `s3` with the same names and the same signing code (both
splice `s3_sigv4.inc.j`), so a program that only presigns moves between the two
by changing the import.

```jennifer
use io;
use os;
import "s3presign.j" as s3;

def c as s3.Client init s3.connect(
    "https://s3.us-east-1.amazonaws.com", "us-east-1", os.getEnv("S3_KEY"), os.getEnv("S3_SECRET"));
def url as string init s3.presign($c, "GET", "mybucket", "report.pdf", 3600);
io.printf("%s\n", $url);
```

## Surface

| Name | |
| ---- | - |
| `Client` | `endpoint`, `region`, `accessKey`, `secretKey`, `timeoutMs`, `allowInsecureHttp`: the shape of `s3.Client`; the last two only matter to `s3`'s requests |
| `connect(endpoint, region, accessKey, secretKey)` | validates the endpoint (`http(s)://host[:port]`, one trailing `/` dropped) and builds a `Client`; contacts nothing |
| `presign(client, method, bucket, key, expiresSeconds)` | the presigned URL (`X-Amz-*` query, `UNSIGNED-PAYLOAD`), stamped with the current UTC time |
| `S3Error` | `kind` (`"s3"`), `message`, `status` (0), `code` (""), `requestId` (""): every error the module throws |

## Errors

Each refusal is an `S3Error` of kind `"s3"`, a local check:

| Cause | |
| ----- | - |
| malformed endpoint (`connect`) | a path, query, fragment, userinfo, bad host or port |
| `expiresSeconds` outside 1..604800 | SigV4 query signing allows 1 s to 7 days |
| invalid bucket name | outside `[A-Za-z0-9._-]{1,255}`, or `.` / `..` |
| empty key | `/{bucket}/` would address the bucket itself |

## See also

- [s3.md](s3.md) - the full client: requests, listings, multipart, `errorOf`.
- [hash.md](../libraries/hash.md) - the `hmac` / `compute` primitives SigV4 builds on.
- [modules/index.md](index.md) - the module catalog and import rules.

## API reference

Every exported function, struct and constant of `s3presign`, with its signature, generated
from the module's doc comments: [the `s3presign` API reference](../api/s3presign.md).
