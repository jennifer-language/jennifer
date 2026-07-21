// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// encodeRemLenBroker encodes an MQTT remaining-length varint (broker side).
func encodeRemLenBroker(n int) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n > 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			break
		}
	}
	return out
}

// readPacketBroker reads one MQTT control packet: the fixed-header byte, the
// remaining-length varint, then that many body bytes.
func readPacketBroker(r *bufio.Reader) (byte, []byte, error) {
	hb, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	mult, val := 1, 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		val += int(b&0x7f) * mult
		mult *= 128
		if b&0x80 == 0 {
			break
		}
	}
	body := make([]byte, val)
	if val > 0 {
		if _, err := io.ReadFull(r, body); err != nil {
			return 0, nil, err
		}
	}
	return hb, body, nil
}

// fakeBroker accepts one connection and speaks just enough MQTT 3.1.1 to
// exercise the client: CONNACK on CONNECT, SUBACK on SUBSCRIBE, a loopback
// echo of each QoS-0 PUBLISH back to the subscriber, PINGRESP on PINGREQ, and
// close on DISCONNECT. It runs the whole binary framing (fixed header,
// remaining-length varint, length-prefixed topic) over a real socket.
func fakeBroker(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		hb, body, err := readPacketBroker(r)
		if err != nil {
			return
		}
		switch hb >> 4 {
		case 1: // CONNECT -> CONNACK (accepted)
			_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00})
		case 8: // SUBSCRIBE -> SUBACK (granted QoS 0), echoing the packet id
			_, _ = conn.Write([]byte{0x90, 0x03, body[0], body[1], 0x00})
		case 3: // PUBLISH (QoS 0) -> echo back to the subscriber verbatim
			echo := []byte{0x30}
			echo = append(echo, encodeRemLenBroker(len(body))...)
			echo = append(echo, body...)
			_, _ = conn.Write(echo)
		case 12: // PINGREQ -> PINGRESP
			_, _ = conn.Write([]byte{0xd0, 0x00})
		case 14: // DISCONNECT
			return
		}
	}
}

// TestMqttPubSub drives the mqtt client end to end against the in-process fake
// broker: connect, subscribe, a publish/receive round-trip, a publish/poll
// round-trip, a poll that times out (net.setDeadline), ping, and disconnect. A
// mismatch throws in the .j program and fails loadForTest, so this runs the
// real binary MQTT dialogue in CI with no broker install.
func TestMqttPubSub(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeBroker(ln)

	mqttMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mqtt.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use convert;
import %q as mqtt;
def o as mqtt.Options init mqtt.Options{host: "127.0.0.1", port: %d, clientId: "t", keepalive: 30, security: "none", username: "", password: ""};
def c as mqtt.Client init mqtt.connect($o);
mqtt.subscribe($c, "test/topic");
mqtt.publish($c, "test/topic", "hello");
def m as mqtt.Message init mqtt.receive($c);
testing.assertEqual($m.topic, "test/topic");
testing.assertEqual(convert.stringFromBytes($m.payload, "utf-8"), "hello");
mqtt.publish($c, "test/topic", "world");
def msgs as list of mqtt.Message init mqtt.poll($c, 1000);
testing.assertEqual(len($msgs), 1);
testing.assertEqual(convert.stringFromBytes($msgs[0].payload, "utf-8"), "world");
def empty as list of mqtt.Message init mqtt.poll($c, 100);
testing.assertEqual(len($empty), 0);
mqtt.ping($c);
mqtt.disconnect($c);`, mqttMod, port)
	progPath := filepath.Join(dir, "pubsub.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mqtt pub/sub program failed with code %d", code)
	}
}
