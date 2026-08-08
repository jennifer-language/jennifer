# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ldap_demo.j - the ldap module end to end: start an in-memory directory server,
# then drive it with the LDAP client (bind, search with filters, group lookup),
# and mutate the live directory from the admin side.
#
#     jennifer run examples/modules/ldap_demo.j

import "../../modules/ldap.j" as ldap;
import "../../modules/transport.j" as transport;
use io;
use net;
use convert;

# Build a small directory: an admin bind account, two people, and a group.
def dir as ldap.Directory init ldap.directory([
    ldap.entry(
        "cn=admin,dc=example,dc=org",
        {
            "objectClass": ["person"],
            "cn": ["admin"],
            "userPassword": [ldap.password("adminpw", "ssha")]
        }),
    ldap.entry(
        "uid=alice,ou=people,dc=example,dc=org",
        {
            "objectClass": ["inetOrgPerson", "person"],
            "uid": ["alice"],
            "cn": ["Alice Alpha"],
            "mail": ["alice@example.org"],
            "userPassword": [ldap.password("alicepw", "ssha")]
        }),
    ldap.entry(
        "uid=bob,ou=people,dc=example,dc=org",
        {
            "objectClass": ["inetOrgPerson", "person"],
            "uid": ["bob"],
            "cn": ["Bob Beta"],
            "mail": ["bob@example.org"],
            "userPassword": [ldap.password("bobpw", "ssha")]
        }),
    ldap.group(
        "cn=staff,ou=groups,dc=example,dc=org",
        ["uid=alice,ou=people,dc=example,dc=org", "uid=bob,ou=people,dc=example,dc=org"])
]);

# Serve it on an ephemeral loopback port, one spawn per connection.
def listener as net.Listener init ldap.listen("127.0.0.1:0");
def addr as string init net.address($listener);
def server as task of null init spawn {
    ldap.serveOn($dir, $listener);
};
io.printf("directory server listening on %s\n\n", $addr);

# --- client ---
def c as ldap.Conn init ldap.connect($addr, transport.Security.None);

io.printf(
    "bind as admin:       %s\n",
    bindLabel(ldap.bind($c, "cn=admin,dc=example,dc=org", "adminpw")));
io.printf(
    "bind alice (wrong):  %s\n",
    bindLabel(ldap.bind($c, "uid=alice,ou=people,dc=example,dc=org", "nope")));

io.printf("\nsearch (mail present, sorted by scan order):\n");
def people as list of ldap.Entry init ldap.search(
    $c,
    "ou=people,dc=example,dc=org",
    ldap.SCOPE_SUB,
    ldap.parseFilter("(&(objectClass=person)(mail=*))"),
    ["cn", "mail"]);
for (def e in $people) {
    io.printf(
        "  %s|pad=44|align=left %s (%s)\n",
        $e.dn,
        ldap.firstValue($e, "cn"),
        ldap.firstValue($e, "mail"));
}

io.printf("\nwho is in cn=staff?\n");
def groups as list of ldap.Entry init ldap.search(
    $c,
    "ou=groups,dc=example,dc=org",
    ldap.SCOPE_SUB,
    ldap.parseFilter("(cn=staff)"),
    ["member"]);
for (def m in ldap.values($groups[0], "member")) {
    io.printf("  %s\n", $m);
}

# --- live mutation: onboard a new user while the server runs ---
io.printf("\nadding uid=carol and authenticating as her...\n");
ldap.addEntry(
    $dir,
    ldap.entry(
        "uid=carol,ou=people,dc=example,dc=org",
        {
            "objectClass": ["inetOrgPerson"],
            "uid": ["carol"],
            "cn": ["Carol Gamma"],
            "mail": ["carol@example.org"],
            "userPassword": [ldap.password("carolpw", "ssha256")]
        }));
io.printf(
    "  bind carol:        %s\n",
    bindLabel(ldap.bind($c, "uid=carol,ou=people,dc=example,dc=org", "carolpw")));
def now as list of ldap.Entry init ldap.search(
    $c,
    "ou=people,dc=example,dc=org",
    ldap.SCOPE_ONE,
    ldap.parseFilter("(uid=*)"),
    ["uid"]);
io.printf("  people now:        %d\n", len($now));

ldap.unbind($c);
net.close($listener);

func bindLabel(r as ldap.Result) {
    if ($r.code == ldap.SUCCESS) {
        return "ok";
    }
    return "rejected (code " + convert.toString($r.code) + ")";
}
