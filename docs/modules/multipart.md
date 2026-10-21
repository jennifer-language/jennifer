# `multipart` - multipart/form-data build and parse

Import with `import "multipart.j" as multipart;`. Build and parse
`multipart/form-data` bodies (RFC 7578) - the file-upload counterpart to
[`mime`](mime.md)'s email multipart. `build` turns a list of `Part`s (form
fields and files) into a `(contentType, body)` pair ready to POST; `parse` turns
a `Content-Type` header and a body back into parts. Bodies are `bytes`, so
binary file content round-trips intact. Pure `.j` over `strings` + `bytes`; runs
on both binaries.

```jennifer
import "multipart.j" as multipart;
use convert;

def parts as list of multipart.Part init [
    multipart.field("title", "hello"),
    multipart.file("doc", "a.txt", "text/plain", convert.bytesFromString("hi", "utf-8"))
];
def form as multipart.Built init multipart.build($parts);
# POST $form.body with header Content-Type: $form.contentType
def back as list of multipart.Part init multipart.parse($form.contentType, $form.body);
```

Runnable: [`examples/modules/multipart_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/multipart_demo.j).

## Parts

```jennifer
def struct multipart.Part {
    name as string,         # the field name
    filename as string,     # the file name ("" for a plain field)
    contentType as string,  # the part Content-Type ("" for a plain field)
    data as bytes           # the part body
};
```

| Call | Returns | |
| ---- | ------- | - |
| `multipart.field(name, value)` | `Part` | a plain text field |
| `multipart.file(name, filename, contentType, data)` | `Part` | a file part (body is `bytes`) |
| `multipart.text(p)` | `string` | decode a part's body as UTF-8 (for field values) |
| `multipart.isFile(p)` | `bool` | whether the part carries a filename |

## Building

| Call | Returns | |
| ---- | ------- | - |
| `multipart.build(parts)` | `Built` | encode with a fresh random boundary |
| `multipart.buildWith(parts, boundary)` | `Built` | encode with an explicit boundary (deterministic) |

`Built{ contentType, body }` pairs the ready-to-send `Content-Type` header
(carrying the boundary) with the encoded `body`. Use `build` normally;
`buildWith` when you need a fixed boundary (tests, reproducibility). The boundary
must not occur inside any part body - the random one from `build` makes that
effectively impossible.

## Parsing

`multipart.parse(contentType, body)` reads the boundary from the `Content-Type`
header and splits the body into `Part`s. It matches the delimiter only at
`CRLF--boundary` (normalising a leading boundary), so the boundary token
appearing *inside* a file body does not split it. A missing boundary or a part
without a header terminator throws `Error{kind: "multipart"}`.

```jennifer
def back as list of multipart.Part init multipart.parse($contentType, $body);
for (def p in $back) {
    if (multipart.isFile($p)) {
        # $p.filename, $p.contentType, $p.data
    } else {
        # multipart.text($p) is the field value
    }
}
```

## Scope

- **`form-data` only.** The generic `multipart/*` (mixed, alternative, related)
  used in email is [`mime`](mime.md)'s job; this is the browser/HTTP upload
  shape.
- **`Content-Disposition` / `Content-Type` headers only.** Other per-part
  headers and RFC 2231 extended (`name*=`) parameter encoding are not parsed;
  `name` / `filename` are read from the quoted `Content-Disposition` params.
- **No streaming.** The whole body is built / parsed in memory - fine for form
  posts, not for multi-gigabyte uploads.
- **Names / filenames are escaped, not percent-encoded.** `build` strips CR / LF
  and backslash-escapes `\` and `"` inside the quoted parameter (and `parse`
  undoes `\"` / `\\`); RFC 7578 percent-encoding is a follow-on.

## See also

- [mime.md](mime.md) - email-style multipart and header encoding.
- [http.md](http.md) / [web.md](web.md) - send an upload, or handle one.
- [modules/index.md](index.md) - the module catalog and import rules.
