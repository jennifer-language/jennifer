# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# zmq_test.j - white-box tests for zmq.j. Run with:
#
#     jennifer test modules/zmq_test.j
#
# Each test pairs two Jennifer sockets on a loopback port: the listener side
# in the test body, the connecting side in a spawned task. zmq.j already `use`s
# net / binary / convert / strings / crypto / maps.
use testing;
use task;

func b(s as string) {
    return convert.bytesFromString($s, "utf-8");
}

func str(x as bytes) {
    return convert.stringFromBytes($x, "utf-8");
}

func lst(m as Message) {
    def out as list of string init [];
    for (def f in $m.frames) {
        $out[] = str($f);
    }
    return $out;
}

# --- wire format --------------------------------------------------------------

func testGreetingLayout() {
    def g as bytes init greeting("PLAIN", true);
    testing.assertEqual(len($g), 64);
    testing.assertEqual($g[0], 0xff);
    testing.assertEqual($g[9], 0x7f);
    testing.assertEqual([$g[10], $g[11]], [3, 1]);
    testing.assertEqual(str($g[12..17]), "PLAIN");
    testing.assertEqual($g[17], 0);
    testing.assertEqual($g[32], 1);
    testing.assertEqual(greeting("NULL", false)[32], 0);
}

func testFrameEncoding() {
    def short as bytes init encodeFrame(FLAG_MORE, b("hi"));
    testing.assertEqual(len($short), 4);
    testing.assertEqual([$short[0], $short[1]], [1, 2]);
    def big as bytes init encodeFrame(0, binary.make(300, 7));
    testing.assertEqual($big[0], FLAG_LONG);
    testing.assertEqual(beInt($big[1..9]), 300);
    testing.assertEqual(len($big), 309);
    def cmd as bytes init encodeCommand("READY", metadata("REQ", b("me")));
    testing.assertEqual($cmd[0], FLAG_COMMAND);
    def c as Command init parseCommand(Frame{flags: $cmd[0], body: $cmd[2..]});
    testing.assertEqual($c.name, "READY");
    def props as map of string to bytes init parseMetadata($c.data);
    testing.assertEqual(str($props["socket-type"]), "REQ");
    testing.assertEqual(str($props["identity"]), "me");
}

func badMetadata() {
    parseMetadata(b("\u0005Sock"));
}

func unknownKind() {
    connect("XREQ", "127.0.0.1:1");
}

func ipcAddress() {
    connect("REQ", "ipc:///tmp/x");
}

func zeroIdentity() {
    def o as Options init defaultOptions();
    $o.identity = binary.make(3, 0);
    connectWith("DEALER", "127.0.0.1:1", $o);
}

func testArgumentErrors() {
    testing.assertThrows("badMetadata", "zmq");
    testing.assertThrows("unknownKind", "zmq");
    testing.assertThrows("ipcAddress", "zmq");
    testing.assertThrows("zeroIdentity", "zmq");
    testing.assertEqual(stripScheme("tcp://h:1"), "h:1");
}

# --- socket pairs ---------------------------------------------------------------

func testReqRep() {
    def l as Listener init listen("REP", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of string init spawn {
        def s as Socket init connect("REQ", $addr);
        sendText($s, "ping");
        def m as Message init receive($s);
        close($s);
        return text($m);
    };
    def srv as Socket init accept($l);
    testing.assertEqual($srv.peerKind, "REQ");
    def req as Message init receive($srv);
    testing.assertEqual(lst($req), ["ping"]);
    testing.assertEqual(len($req.envelope), 0);
    reply($srv, $req, [b("pong")]);
    testing.assertEqual(task.wait($client), "pong");
    close($srv);
    closeListener($l);
}

# PUB sends everything; SUB keeps only what a subscription matches, and a
# multipart topic frame stays with its body.
func testPubSub() {
    def l as Listener init listen("PUB", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of list of string init spawn {
        def s as Socket init connect("SUB", $addr);
        $s = subscribe($s, "weather.");
        $s = subscribe($s, "news");
        $s = unsubscribe($s, "news");
        def got as list of string init [];
        for (def i in 0..2) {
            def m as Message init receive($s);
            $got[] = str($m.frames[0]) + "=" + str($m.frames[1]);
        }
        close($s);
        return $got;
    };
    def pub as Socket init accept($l);
    send($pub, [b("news"), b("ignored")]);
    send($pub, [b("weather.vienna"), b("12")]);
    send($pub, [b("sport"), b("ignored")]);
    send($pub, [b("weather.graz"), b("14")]);
    testing.assertEqual(task.wait($client), ["weather.vienna=12", "weather.graz=14"]);
    close($pub);
    closeListener($l);
}

# Frames survive as sent: empty ones, and long ones past the 255-byte short form.
func testPushPullMultipart() {
    def l as Listener init listen("PULL", "127.0.0.1:0");
    def addr as string init address($l);
    def big as bytes init binary.make(100000, 65);
    def client as task of null init spawn {
        def s as Socket init connect("PUSH", $addr);
        send($s, [b("a"), emptyBytes(), $big]);
        close($s);
        return;
    };
    def pull as Socket init accept($l);
    def m as Message init receive($pull);
    testing.assertEqual(len($m.frames), 3);
    testing.assertEqual(str($m.frames[0]), "a");
    testing.assertEqual(len($m.frames[1]), 0);
    testing.assertEqual($m.frames[2], $big);
    task.wait($client);
    close($pull);
    closeListener($l);
}

# A ROUTER sees the DEALER's identity as the envelope and addresses it with it.
func testDealerRouter() {
    def l as Listener init listen("ROUTER", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of string init spawn {
        def o as Options init defaultOptions();
        $o.identity = b("worker-1");
        def s as Socket init connectWith("DEALER", $addr, $o);
        send($s, [b("job"), b("42")]);
        def m as Message init receive($s);
        close($s);
        return text($m);
    };
    def r as Socket init accept($l);
    testing.assertEqual(str($r.peerIdentity), "worker-1");
    def m as Message init receive($r);
    testing.assertEqual(lst($m), ["job", "42"]);
    testing.assertEqual(str($m.envelope[0]), "worker-1");
    send($r, [b("worker-1"), b("done")]);
    testing.assertEqual(task.wait($client), "done");
    close($r);
    closeListener($l);
}

# A REP socket strips routing frames into the envelope and puts them back on
# the reply, as a REP behind a ROUTER-DEALER broker must.
func testRepKeepsEnvelope() {
    def l as Listener init listen("REP", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of list of string init spawn {
        def s as Socket init connect("DEALER", $addr);
        send($s, [b("client-7"), emptyBytes(), b("question")]);
        def m as Message init receive($s);
        close($s);
        return lst($m);
    };
    def rep as Socket init accept($l);
    def m as Message init receive($rep);
    testing.assertEqual(lst($m), ["question"]);
    testing.assertEqual(str($m.envelope[0]), "client-7");
    reply($rep, $m, [b("answer")]);
    testing.assertEqual(task.wait($client), ["client-7", "", "answer"]);
    close($rep);
    closeListener($l);
}

# A ROUTER with a peer that sent no identity makes one up, libzmq-style.
func testRouterGeneratesIdentity() {
    def l as Listener init listen("ROUTER", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of null init spawn {
        def s as Socket init connect("REQ", $addr);
        sendText($s, "hi");
        def m as Message init receive($s);
        close($s);
        return;
    };
    def r as Socket init accept($l);
    testing.assertEqual(len($r.peerIdentity), 5);
    testing.assertEqual($r.peerIdentity[0], 0);
    def m as Message init receive($r);
    testing.assertEqual(len($m.frames[0]), 0); # the REQ's empty delimiter
    reply($r, $m, [emptyBytes(), b("ok")]);
    task.wait($client);
    close($r);
    closeListener($l);
}

# --- PLAIN ------------------------------------------------------------------------

func plainOpts(user as string, pass as string) {
    def o as Options init defaultOptions();
    $o.mechanism = "PLAIN";
    $o.username = $user;
    $o.password = $pass;
    return $o;
}

func testPlainAccepts() {
    def l as Listener init listenWith("PAIR", "127.0.0.1:0", plainOpts("admin", "s3cret"));
    def addr as string init address($l);
    def client as task of string init spawn {
        def s as Socket init connectWith("PAIR", $addr, plainOpts("admin", "s3cret"));
        def m as Message init receive($s);
        close($s);
        return text($m);
    };
    def srv as Socket init accept($l);
    sendText($srv, "welcome");
    testing.assertEqual(task.wait($client), "welcome");
    close($srv);
    closeListener($l);
}

func testPlainRejectsBadPassword() {
    def l as Listener init listenWith("PAIR", "127.0.0.1:0", plainOpts("admin", "s3cret"));
    def addr as string init address($l);
    def client as task of string init spawn {
        def why as string init "";
        try {
            connectWith("PAIR", $addr, plainOpts("admin", "guess"));
        } catch (e) {
            $why = $e.kind + ": " + $e.message;
        }
        return $why;
    };
    def kind as string init "";
    try {
        accept($l);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "zmq");
    testing.assertContains(task.wait($client), "invalid username or password");
    closeListener($l);
}

# --- mismatches ----------------------------------------------------------------------

func testIncompatibleTypes() {
    def l as Listener init listen("PULL", "127.0.0.1:0");
    def addr as string init address($l);
    def client as task of string init spawn {
        def why as string init "";
        try {
            connect("REQ", $addr);
        } catch (e) {
            $why = $e.message;
        }
        return $why;
    };
    def msg as string init "";
    try {
        accept($l);
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "cannot talk to a");
    testing.assertTrue(len(task.wait($client)) > 0);
    closeListener($l);
}

func testMechanismMismatch() {
    def l as Listener init listenWith("PAIR", "127.0.0.1:0", plainOpts("u", "p"));
    def addr as string init address($l);
    def client as task of string init spawn {
        def why as string init "";
        try {
            connect("PAIR", $addr);
        } catch (e) {
            $why = $e.message;
        }
        return $why;
    };
    def msg as string init "";
    try {
        accept($l);
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "mechanism");
    testing.assertContains(task.wait($client), "mechanism");
    closeListener($l);
}

# --- limits, poll and commands ---------------------------------------------------------

func testMessageLimit() {
    def o as Options init defaultOptions();
    $o.maxMessageBytes = 1000;
    def l as Listener init listenWith("PULL", "127.0.0.1:0", $o);
    def addr as string init address($l);
    def client as task of null init spawn {
        def s as Socket init connect("PUSH", $addr);
        send($s, [binary.make(600, 1), binary.make(600, 2)]);
        close($s);
        return;
    };
    def pull as Socket init accept($l);
    def kind as string init "";
    try {
        receive($pull);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "limit");
    task.wait($client);
    close($pull);
    closeListener($l);
}

# poll comes back empty when nothing arrives; a PING is answered with a PONG
# carrying its context, and the message behind it is delivered.
func testPollAndPing() {
    def l as Listener init listen("PAIR", "127.0.0.1:0");
    def addr as string init address($l);
    def dial as task of Socket init spawn {
        return connect("PAIR", $addr);
    };
    def srv as Socket init accept($l);
    def peer as Socket init task.wait($dial);
    testing.assertEqual(len(poll($srv, 20)), 0);
    def ping as bytes init beBytes(0, 2);
    net.writeBytes($peer.conn, encodeCommand("PING", binary.concat($ping, b("ctx"))));
    sendText($peer, "after");
    def got as list of Message init poll($srv, 2000);
    if (len($got) == 0) {
        $got = poll($srv, 2000);
    }
    testing.assertEqual(text($got[0]), "after");
    def pong as Command init parseCommand(readFrame($peer.conn, 100));
    testing.assertEqual($pong.name, "PONG");
    testing.assertEqual(str($pong.data), "ctx");
    close($peer);
    close($srv);
    closeListener($l);
}

func testWrongDirection() {
    def l as Listener init listen("PULL", "127.0.0.1:0");
    def addr as string init address($l);
    def dial as task of Socket init spawn {
        return connect("PUSH", $addr);
    };
    def pull as Socket init accept($l);
    def push as Socket init task.wait($dial);
    def msg as string init "";
    try {
        sendText($pull, "x");
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "cannot send");
    try {
        receive($push);
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertContains($msg, "cannot receive");
    close($push);
    close($pull);
    closeListener($l);
}
