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
	"time"
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
import %q as transport;
def o as mqtt.Options init mqtt.Options{host: "127.0.0.1", port: %d, clientId: "t", keepalive: 30, security: transport.Security.None, username: "", password: ""};
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
mqtt.disconnect($c);`, mqttMod, filepath.Join(filepath.Dir(mqttMod), "transport.j"), port)
	progPath := filepath.Join(dir, "pubsub.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mqtt pub/sub program failed with code %d", code)
	}
}

// buildQos1PublishBroker builds a QoS-1 PUBLISH the broker pushes to the client:
// fixed-header flags byte 0x32 (type 3, QoS 1), the remaining-length varint, then
// a 2-byte topic length + topic + 2-byte packet id + payload.
func buildQos1PublishBroker(topic string, packetId int, payload string) []byte {
	tb := []byte(topic)
	var body []byte
	body = append(body, byte(len(tb)>>8), byte(len(tb)))
	body = append(body, tb...)
	body = append(body, byte(packetId>>8), byte(packetId))
	body = append(body, []byte(payload)...)
	pkt := []byte{0x32}
	pkt = append(pkt, encodeRemLenBroker(len(body))...)
	pkt = append(pkt, body...)
	return pkt
}

// fakeBrokerQos1 speaks the QoS-1 handshakes on one connection: CONNACK on
// CONNECT; for the client's QoS-1 PUBLISH it reads the packet id (the 2-byte
// value after the topic in the variable header) and replies with a PUBACK
// (0x40 0x02 <id-hi> <id-lo>) for that exact id; on SUBSCRIBE it replies with a
// SUBACK granting QoS 1, then pushes a QoS-1 PUBLISH to the client and reads the
// client's PUBACK for it, reporting the acknowledged packet id on ackCh (or -1
// on any framing failure) so the test can verify the ack round-trips.
func fakeBrokerQos1(ln net.Listener, ackCh chan int) {
	const pushPacketID = 7
	conn, err := ln.Accept()
	if err != nil {
		ackCh <- -1
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
		case 3: // QoS-1 PUBLISH from publishQos1 -> PUBACK for its packet id
			tlen := int(body[0])<<8 | int(body[1])
			idx := 2 + tlen
			pid := int(body[idx])<<8 | int(body[idx+1])
			_, _ = conn.Write([]byte{0x40, 0x02, byte(pid >> 8), byte(pid)})
		case 8: // SUBSCRIBE -> SUBACK granting QoS 1, then push a QoS-1 PUBLISH
			_, _ = conn.Write([]byte{0x90, 0x03, body[0], body[1], 0x01})
			_, _ = conn.Write(buildQos1PublishBroker("q1/sub", pushPacketID, "pushed"))
			ahb, abody, err := readPacketBroker(r)
			if err != nil || ahb>>4 != 4 || len(abody) < 2 {
				ackCh <- -1
				return
			}
			ackCh <- int(abody[0])<<8 | int(abody[1])
		case 14: // DISCONNECT
			return
		}
	}
}

// TestMqttQos1 drives the mqtt client's QoS-1 surface against a fake broker that
// speaks the PUBACK handshakes: publishQos1 (a QoS-1 PUBLISH that blocks for the
// matching PUBACK) must return without throwing, and subscribeQos1 + receive
// must return the broker-pushed QoS-1 message and answer it with a PUBACK. The
// broker reports the acknowledged packet id so the test confirms the client's
// PUBACK round-trips with the pushed message's id.
func TestMqttQos1(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	ackCh := make(chan int, 1)
	go fakeBrokerQos1(ln, ackCh)

	mqttMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mqtt.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use convert;
import %q as mqtt;
import %q as transport;
def o as mqtt.Options init mqtt.Options{host: "127.0.0.1", port: %d, clientId: "t", keepalive: 30, security: transport.Security.None, username: "", password: ""};
def c as mqtt.Client init mqtt.connect($o);
def pl as bytes init convert.bytesFromString("q1payload", "utf-8");
mqtt.publishQos1($c, "q1/pub", $pl, false);
$c = mqtt.subscribeQos1($c, "q1/sub");
def m as mqtt.Message init mqtt.receive($c);
testing.assertEqual($m.topic, "q1/sub");
testing.assertEqual(convert.stringFromBytes($m.payload, "utf-8"), "pushed");
mqtt.disconnect($c);`, mqttMod, filepath.Join(filepath.Dir(mqttMod), "transport.j"), port)
	progPath := filepath.Join(dir, "qos1.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mqtt QoS-1 program failed with code %d", code)
	}

	// The client must have PUBACKed the broker-pushed QoS-1 message with the
	// packet id the broker sent (7).
	select {
	case ackedID := <-ackCh:
		if ackedID != 7 {
			t.Fatalf("client PUBACK acknowledged packet id %d, want 7", ackedID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broker never received the client's PUBACK for the pushed QoS-1 message")
	}
}
