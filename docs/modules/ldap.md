# `ldap` - LDAP v3 client and directory server

Import with `import "ldap.j" as ldap;`. An LDAP v3 client and a lightweight,
in-memory directory server (RFC 4511). The wire messages are ASN.1 BER, built
and parsed with the [`asn1`](../libraries/asn1.md) library; the transport is
[`net`](../libraries/net.md) TCP, with LDAPS / StartTLS through `net`'s TLS. SASL
SCRAM binds reuse the [`sasl`](sasl.md) module; the mutable server directory is
backed by the [`kv`](../libraries/kv.md) store. Needs the default `jennifer`
binary (`net`).

```jennifer
import "ldap.j" as ldap;
import "transport.j" as transport;
use io;

def c as ldap.Conn init ldap.connect("localhost:389", transport.Security.None);
ldap.bind($c, "cn=admin,dc=example,dc=org", "secret");
def hits as list of ldap.Entry init ldap.search($c,
    "ou=people,dc=example,dc=org", ldap.SCOPE_SUB,
    ldap.parseFilter("(uid=alice)"), ["cn", "mail"]);
for (def e in $hits) {
    io.printf("%s -> %s\n", $e.dn, ldap.firstValue($e, "mail"));
}
ldap.unbind($c);
```

Runnable: [`examples/modules/ldap_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ldap_demo.j).

## Values

Entries, attributes, and results come back as small structs:

```jennifer
export def struct Conn { handle as net.Conn, timeoutMs as int };
export def struct Attribute { name as string, values as list of string };
export def struct Entry { dn as string, attributes as list of Attribute };
export def struct Result { code as int, matchedDn as string, message as string };
```

An `Entry` is a DN plus multi-valued attributes. Read an attribute with
`ldap.values(entry, name)` (a `list of string`, case-insensitive name) or
`ldap.firstValue(entry, name)` (the first value, or `""`). A `Result` carries the
LDAP result `code` - `ldap.SUCCESS` (0) on success, `ldap.INVALID_CREDENTIALS`
(49) on a failed bind.

## Client

### Connecting

`ldap.connect(address, security)` opens a connection; `security` is a
[`transport.Security`](transport.md) value:

```jennifer
import "transport.j" as transport;

def a as ldap.Conn init ldap.connect("dir.example.org:389", transport.Security.None);      # plaintext
def b as ldap.Conn init ldap.connect("dir.example.org:636", transport.Security.Tls);       # LDAPS
def c as ldap.Conn init ldap.connect("dir.example.org:389", transport.Security.Starttls);  # StartTLS
```

`transport.Security.Starttls` connects in plaintext and upgrades in-band via the
StartTLS extended operation (or call `ldap.startTls(conn)` yourself). `ldap.close`
closes without an unbind; `ldap.unbind` sends an unbind request and closes.

### Binding

```jennifer
def r as ldap.Result init ldap.bind($c, "cn=admin,dc=example,dc=org", "secret");
if ($r.code == ldap.SUCCESS) { io.printf("bound\n"); }
```

`ldap.bind` performs a simple bind and returns the `Result` (it does not throw on
a bad password - check `.code`). An empty DN and password is an anonymous bind.
`ldap.bindSasl(conn, user, password, algo)` performs a SASL SCRAM bind (`algo`
`"sha1"` or `"sha256"`), verifying the server signature.

### Searching and filters

`ldap.search(conn, baseDn, scope, filter, attributes)` returns the matching
entries; an empty `attributes` list requests all user attributes. `scope` is one
of `ldap.SCOPE_BASE`, `ldap.SCOPE_ONE`, or `ldap.SCOPE_SUB`.

Build the filter from an RFC 4515 string with `ldap.parseFilter`, or from the
constructors:

```jennifer
def f1 as asn1.Value init ldap.parseFilter("(&(objectClass=person)(uid=alice))");
def f2 as asn1.Value init ldap.allOf([ldap.equals("objectClass", "person"), ldap.present("mail")]);
```

| Constructor | Filter |
| ----------- | ------ |
| `ldap.equals(attr, value)`         | `(attr=value)` |
| `ldap.present(attr)`               | `(attr=*)` |
| `ldap.greaterOrEqual(attr, value)` | `(attr>=value)` |
| `ldap.lessOrEqual(attr, value)`    | `(attr<=value)` |
| `ldap.approx(attr, value)`         | `(attr~=value)` |
| `ldap.substrings(attr, initial, anyParts, final)` | `(attr=initial*any*final)` |
| `ldap.allOf(filters)`              | `(&(...)(...))` |
| `ldap.anyOf(filters)`              | `(|(...)(...))` |
| `ldap.negate(filter)`              | `(!(...))` |

`ldap.searchPaged(conn, baseDn, scope, filter, attributes, pageSize)` pulls large
result sets a page at a time via the simple-paged-results control.

Attribute values come back as strings. A **binary** value (one that is not valid
UTF-8 - Active Directory `objectGUID` / `objectSid`, `userCertificate`,
`jpegPhoto`) is returned **base64-encoded** (the LDIF convention) rather than
throwing; decode it with `encoding.fromText(value, "base64")`.

### Writing

The write operations return a `Result` (check `.code` against `ldap.SUCCESS`):

```jennifer
ldap.add($c, "uid=jdoe,ou=people,dc=example,dc=org", {
    "objectClass": ["inetOrgPerson"], "uid": ["jdoe"], "cn": ["J Doe"], "mail": ["jdoe@example.org"]
});
ldap.modify($c, "uid=jdoe,ou=people,dc=example,dc=org", [
    ldap.change(ldap.MOD_REPLACE, "mail", ["j.doe@example.org"]),
    ldap.change(ldap.MOD_ADD, "description", ["created via ldap"])
]);
ldap.modifyDn($c, "uid=jdoe,ou=people,dc=example,dc=org", "uid=jd", true, "");   # rename (newSuperior "" keeps the parent)
ldap.passwordModify($c, "uid=jd,ou=people,dc=example,dc=org", "oldpw", "newpw"); # RFC 3062
ldap.delete($c, "uid=jd,ou=people,dc=example,dc=org");
```

`ldap.change(operation, name, values)` builds one modify change; `operation` is
`ldap.MOD_ADD`, `ldap.MOD_DELETE`, or `ldap.MOD_REPLACE` (an empty value list on
`MOD_DELETE` removes the whole attribute). `ldap.passwordModify` uses the RFC 3062
extended operation - pass `userDn` `""` for the currently-bound user, `oldPassword`
`""` for an administrative reset, and `newPassword` `""` to have the server
generate one (do it over TLS). These operations require a directory server that
accepts writes; the built-in [server](#server) is read-only over LDAP.

### Active Directory notes

The client works against Active Directory over LDAPS or StartTLS with a simple
bind (`user@domain`, `DOMAIN\user`, or a full DN). AD caps a result set at 1000
rows, so use `ldap.searchPaged` for large searches (it sends AD's required paged
control). Bind to a specific domain controller, or the Global Catalog on port
`3268`, to avoid referrals. AD's native SASL is Kerberos (GSSAPI), which this
module does not implement - use simple bind over TLS. Change a password with
`ldap.passwordModify` over TLS (AD requires an encrypted connection for it).

## Server

The server answers **simple bind** and **search** for an in-memory directory. It
is read-only over the LDAP protocol (it rejects add / modify / delete on the
wire), but the directory itself is **fully mutable from your own code** - so a
web interface or a database sync can update the directory a running server is
serving, and the change is visible immediately (the directory is a shared
`kv`-backed store). There is no schema enforcement and no ACLs.

### Building a directory

```jennifer
def dir as ldap.Directory init ldap.directory([
    ldap.entry("uid=alice,ou=people,dc=example,dc=org", {
        "objectClass": ["inetOrgPerson", "person"],
        "uid": ["alice"], "cn": ["Alice A"], "mail": ["alice@example.org"],
        "userPassword": [ldap.password("secret", "ssha")]
    }),
    ldap.group("cn=admins,ou=groups,dc=example,dc=org", ["uid=alice,ou=people,dc=example,dc=org"])
]);
```

`ldap.directory` is in-memory (ephemeral) - enough for a lightweight server. For
a directory that **persists across restarts**, use `ldap.openDirectory(path)`
instead: it is backed by a `kv.openFile` store, so every edit is written to disk.
A new file starts empty; seed it on first run by checking `ldap.listEntries` and
`ldap.addEntry`-ing when it is empty.

`ldap.entry(dn, attrs)` builds an entry from a `map of string to list of string`;
`ldap.group(dn, members)` builds a `groupOfNames` (its `cn` is taken from the DN's
RDN). `ldap.password(plain, scheme)`
hashes a password into a `userPassword` value. `scheme` is:

| scheme | stored form | notes |
| ------ | ----------- | ----- |
| `"plain"` | cleartext | testing only |
| `"sha"` / `"ssha"` | `{SHA}` / `{SSHA}` | SHA-1 (salted); fast hash, interop only |
| `"sha256"` / `"ssha256"` | `{SHA256}` / `{SSHA256}` | SHA-256 (salted); still a fast hash |
| `"pbkdf2"` | `{PBKDF2-SHA256}$iterations$salt$hash` | **recommended** - a slow KDF (`crypto.pbkdf2`, 100000 iterations) |
| `"pbkdf2-sha512"` | `{PBKDF2-SHA512}$...` | slow KDF over SHA-512 |

The `{SHA}` / `{SSHA}` names follow the OpenLDAP / RFC 2307 convention for
interop, but they are fast hashes - weak for password storage even when salted.
Prefer `"pbkdf2"` unless you need to interoperate with a directory that expects
`{SSHA}`. `ldap.verifyPassword` (used by the server on bind) accepts every scheme.
The server withholds `userPassword` from search results unless it is explicitly
requested.

### Mutating a live directory

```jennifer
ldap.addEntry($dir, ldap.entry("uid=bob,ou=people,dc=example,dc=org", { ... }));  # insert / replace by DN
ldap.modifyEntry($dir, $entry);                                                   # replace by DN
ldap.setAttribute($dir, "uid=bob,ou=people,dc=example,dc=org", "mail", ["bob@example.org"]);
ldap.deleteEntry($dir, "uid=bob,ou=people,dc=example,dc=org");
def e as ldap.Entry init ldap.getEntry($dir, "uid=bob,ou=people,dc=example,dc=org");
def all as list of ldap.Entry init ldap.listEntries($dir);
def ok as bool init ldap.hasEntry($dir, "uid=bob,ou=people,dc=example,dc=org");
```

These edits are visible to a running server immediately, so the directory can be
built from a database at startup and kept in sync by your own admin tooling while
the server serves auth requests from it. (Serialize your writes: a mutation is a
read-modify-write on the shared store, so a single admin path avoids a lost
update.)

### Serving

```jennifer
def dir as ldap.Directory init ldap.directory([ ... ]);
ldap.serve($dir, ":389");   # blocks, one spawn per connection
```

For a stoppable server, drive the listener yourself: `ldap.listen(address)` ->
`net.Listener`, `ldap.serveOn(dir, listener)` runs the accept loop until the
listener is closed (close it from another task to stop).

## Configuring Authelia against this server

[Authelia](https://www.authelia.com) can use the Jennifer LDAP server as its
authentication backend. Build a directory with an admin bind account, a people
subtree, and a groups subtree:

```jennifer
import "ldap.j" as ldap;

def dir as ldap.Directory init ldap.directory([
    ldap.entry("cn=admin,dc=example,dc=org", {
        "objectClass": ["person"], "cn": ["admin"],
        "userPassword": [ldap.password("adminpassword", "ssha")]
    }),
    ldap.entry("uid=alice,ou=people,dc=example,dc=org", {
        "objectClass": ["inetOrgPerson"], "uid": ["alice"],
        "cn": ["Alice A"], "displayName": ["Alice A"], "mail": ["alice@example.org"],
        "userPassword": [ldap.password("alicepassword", "ssha")]
    }),
    ldap.group("cn=admins,ou=groups,dc=example,dc=org", ["uid=alice,ou=people,dc=example,dc=org"])
]);
ldap.serve($dir, ":389");
```

Then point Authelia's `authentication_backend.ldap` at it:

```yaml
authentication_backend:
  ldap:
    address: 'ldap://127.0.0.1:389'
    implementation: 'custom'
    base_dn: 'dc=example,dc=org'
    additional_users_dn: 'ou=people'
    additional_groups_dn: 'ou=groups'
    users_filter: '(&({username_attribute}={input})(objectClass=inetOrgPerson))'
    username_attribute: 'uid'
    display_name_attribute: 'displayName'
    mail_attribute: 'mail'
    groups_filter: '(&(member={dn})(objectClass=groupOfNames))'
    group_name_attribute: 'cn'
    user: 'cn=admin,dc=example,dc=org'
    password: 'adminpassword'
```

Authelia binds as the `user` account, searches `users_filter` under
`additional_users_dn` for the login name, binds again as the found user DN to
check the password, then searches `groups_filter` for the user's groups - all of
which this server answers. Put TLS in front (`transport.Security.Tls` on a `636`
listener, or a reverse proxy) before exposing it beyond localhost. The same setup
works for any LDAP-authenticating application; Authelia is one example.

## PAM authentication

A Jennifer program can also be a system authentication helper for Linux PAM via
`pam_exec.so`, which passes the username in the `PAM_USER` environment variable
and (with `expose_authtok`) the password on stdin, then reads the helper's exit
code (0 = allow). [`examples/modules/pam_auth.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/pam_auth.j)
is a ready-made helper that binds against this directory to authenticate; point a
`/etc/pam.d/<service>` `auth` line at it (`auth sufficient pam_exec.so
expose_authtok quiet /path/to/pam_auth.j`). It fails closed - a missing user, an
empty password, or an unreachable server denies - and its `checkCredentials` is
easy to repoint at a `kvstore` or a PBKDF2-hash file instead of LDAP.

## Errors

Client and server failures throw an `Error` with kind `"ldap"` (a malformed
message, a transport failure, a refused StartTLS, a failed SASL verification).
LDAP-level result codes on a bind are returned in the `Result`, not thrown, so a
bad password is a value to inspect rather than an exception.
