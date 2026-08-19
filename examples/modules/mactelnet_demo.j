# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# mactelnet_demo.j - a Layer-2 (MAC-Telnet) console to a MikroTik RouterOS
# device, the way to give a fresh or just-reset router its first IP before
# switching to the IP-based `mikrotik` API.
#
# Run the offline part (MAC parsing + the flow it would perform) with no router:
#
#     jennifer run examples/modules/mactelnet_demo.j
#
# Run a real session by passing the local interface, the router's MAC, a user,
# and a password (blank password = two trailing args, or "" quoted):
#
#     jennifer run examples/modules/mactelnet_demo.j eth0 e4:8d:8c:11:22:33 admin ''
use io;
use os;
import "../../modules/mactelnet.j" as mactelnet;

# --- offline: MAC helpers are pure and need no network ----------------------
def mac as string init "e4:8d:8c:11:22:33";
def raw as bytes init mactelnet.parseMac($mac);
io.printf("parsed  %s  -> %d bytes\n", $mac, len($raw));
io.printf("round-trip: %s\n", mactelnet.formatMac($raw));

# --- live session only when the router details are supplied -----------------
if (len(os.ARGS) < 4) {
    io.printf("\nno router given - pass: <iface> <router-mac> <user> [password]\n");
    io.printf("e.g. jennifer run examples/modules/mactelnet_demo.j eth0 %s admin ''\n", $mac);
    exit 0;
}

def iface as string init os.ARGS[1];
def target as string init os.ARGS[2];
def user as string init os.ARGS[3];
def password as string init "";
if (len(os.ARGS) >= 5) {
    $password = os.ARGS[4];
}

io.printf("\nconnecting to %s via %s as %s ...\n", $target, $iface, $user);
def s as mactelnet.Session init mactelnet.connect($iface, $target, $user, $password);
io.printf("logged in.\n");

# The router's login banner / prompt.
def banner as string init mactelnet.recv($s, 1000);
io.printf("%s", $banner);

# Ask the router to identify itself, then read the reply.
mactelnet.send($s, "/system identity print\r\n");
def out as string init mactelnet.recv($s, 1500);
io.printf("%s\n", $out);

mactelnet.close($s);
io.printf("session closed.\n");
