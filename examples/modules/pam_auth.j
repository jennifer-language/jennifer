#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A PAM authentication helper for pam_exec.so. It reads the username from the
 * PAM_USER environment variable and the password from stdin (pam_exec's
 * expose_authtok), verifies the pair, and exits 0 to allow / non-zero to deny.
 *
 * Wire it into PAM's auth phase:
 *   # /etc/pam.d/<service>
 *   auth  sufficient  pam_exec.so  expose_authtok quiet  /path/to/pam_auth.j
 *
 * pam_exec runs it with the caller's privileges (often root), so install it
 * root-owned and not world-writable, and never log the token. It fails closed:
 * a missing user, an empty password, or any error denies. The default backend
 * binds against an LDAP directory (the ldap module's own server works); swap
 * checkCredentials for a kvstore or PBKDF2-file backend as noted there.
 * Run: printf 'secret' | PAM_USER=alice jennifer run examples/modules/pam_auth.j; echo $?
 * @module pam_auth
 */
import "../../modules/ldap.j" as ldap;
import "../../modules/transport.j" as transport;
use os;
use io;

# The directory to authenticate against. Edit these, or set LDAP_ADDRESS in the
# environment (handy for pointing at an ephemeral test port).
def const DEFAULT_ADDRESS as string init "127.0.0.1:389";
def const USER_BASE as string init "ou=people,dc=example,dc=org";
def const UID_ATTR as string init "uid";

# ldapAddress is the configured directory address (env override, else default).
func ldapAddress() {
    def a as string init os.getEnv("LDAP_ADDRESS");
    if ($a == "") {
        return DEFAULT_ADDRESS;
    }
    return $a;
}

# readAuthtok reads the password from stdin. pam_exec may send it without a
# trailing newline, so guard on eof and return "" when there is nothing.
func readAuthtok() {
    if (io.eof()) {
        return "";
    }
    return io.readLine();
}

# checkCredentials returns true only for a valid user + password. It must never
# throw (fail closed). This default binds as the user's DN against LDAP; an empty
# password is rejected up front so an unauthenticated bind can never pass. To use
# a different backend, replace the body, e.g. look a stored PBKDF2 hash up in a
# kvstore or a `user:{PBKDF2-SHA256}$...` file and verify it with crypto.pbkdf2.
func checkCredentials(user as string, password as string) {
    if ($user == "" or $password == "") {
        return false;
    }
    def dn as string init UID_ATTR + "=" + $user + "," + USER_BASE;
    try {
        def c as ldap.Conn init ldap.connect(ldapAddress(), transport.Security.None);
        def r as ldap.Result init ldap.bind($c, $dn, $password);
        ldap.unbind($c);
        return $r.code == ldap.SUCCESS;
    } catch (e) {
        return false;
    }
}

# --- pam_exec entry point: environment + stdin in, exit code out ---
def user as string init os.getEnv("PAM_USER");
def password as string init readAuthtok();
if (checkCredentials($user, $password)) {
    exit 0;
}
exit 1;
