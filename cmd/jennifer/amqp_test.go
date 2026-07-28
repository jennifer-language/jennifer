// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// --- minimal AMQP 0-9-1 broker-side wire helpers (stdlib only) --------------

func amqpReadFrame(r *bufio.Reader) (ftype byte, payload []byte, err error) {
	h := make([]byte, 7)
	if _, err = io.ReadFull(r, h); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(h[3:7])
	payload = make([]byte, size)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	end := make([]byte, 1)
	_, err = io.ReadFull(r, end)
	return h[0], payload, err
}

func amqpWriteFrame(w io.Writer, ftype byte, ch uint16, payload []byte) {
	buf := []byte{ftype}
	buf = binary.BigEndian.AppendUint16(buf, ch)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(payload)))
	buf = append(buf, payload...)
	buf = append(buf, 0xCE)
	w.Write(buf)
}

func amqpWriteMethod(w io.Writer, ch, class, method uint16, args []byte) {
	p := binary.BigEndian.AppendUint16(nil, class)
	p = binary.BigEndian.AppendUint16(p, method)
	p = append(p, args...)
	amqpWriteFrame(w, 1, ch, p)
}

func amqpShortStr(b []byte, s string) []byte { return append(append(b, byte(len(s))), s...) }
func amqpLongStr(b []byte, s string) []byte {
	return append(binary.BigEndian.AppendUint32(b, uint32(len(s))), s...)
}

// serveAMQP performs one full handshake and a declare / publish / get / ack /
// close exchange, echoing the published body back through Basic.Get - so the
// .j client's publish bytes must survive the round trip to the get assertion.
func serveAMQP(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	hdr := make([]byte, 8) // "AMQP\0\0\x09\x01"
	if _, err := io.ReadFull(r, hdr); err != nil {
		return
	}
	// Connection.Start
	start := []byte{0, 9, 0, 0, 0, 0} // version 0.9, empty server-properties table
	start = amqpLongStr(start, "PLAIN")
	start = amqpLongStr(start, "en_US")
	amqpWriteMethod(conn, 0, 10, 10, start)
	amqpReadFrame(r) // Start-Ok
	// Connection.Tune: channel-max 0, frame-max 131072, heartbeat 0
	amqpWriteMethod(conn, 0, 10, 30, []byte{0, 0, 0, 2, 0, 0, 0, 0})
	amqpReadFrame(r)                                     // Tune-Ok
	amqpReadFrame(r)                                     // Connection.Open
	amqpWriteMethod(conn, 0, 10, 41, []byte{0})          // Open-Ok (reserved shortstr "")
	amqpReadFrame(r)                                     // Channel.Open
	amqpWriteMethod(conn, 1, 20, 11, []byte{0, 0, 0, 0}) // Channel.Open-Ok

	// Queue.Declare: echo the requested queue name in Declare-Ok.
	_, decl, err := amqpReadFrame(r)
	if err != nil {
		return
	}
	qLen := int(decl[6]) // class(2) method(2) reserved(2) then queue shortstr
	qname := string(decl[7 : 7+qLen])
	declOk := amqpShortStr(nil, qname)
	declOk = append(declOk, 0, 0, 0, 5) // message-count 5
	declOk = append(declOk, 0, 0, 0, 0) // consumer-count 0
	amqpWriteMethod(conn, 1, 50, 11, declOk)

	// Basic.Publish: method + content-header + body; capture the body.
	amqpReadFrame(r)                 // publish method
	amqpReadFrame(r)                 // content header
	_, body, err := amqpReadFrame(r) // body
	if err != nil {
		return
	}

	// Basic.Get -> Get-Ok + header + body (the captured message).
	amqpReadFrame(r)
	getOk := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0} // delivery-tag 1, redelivered 0
	getOk = amqpShortStr(getOk, "")            // exchange
	getOk = amqpShortStr(getOk, "jobs")        // routing-key
	getOk = append(getOk, 0, 0, 0, 0)          // message-count 0
	amqpWriteMethod(conn, 1, 60, 71, getOk)
	header := []byte{0, 60, 0, 0}
	header = binary.BigEndian.AppendUint64(header, uint64(len(body)))
	header = append(header, 0, 0) // property flags: none
	amqpWriteFrame(conn, 2, 1, header)
	amqpWriteFrame(conn, 3, 1, body)

	amqpReadFrame(r)                      // Basic.Ack
	amqpReadFrame(r)                      // Connection.Close
	amqpWriteMethod(conn, 0, 10, 51, nil) // Connection.Close-Ok
}

// serveAMQPQuorum handshakes, captures one Queue.Declare frame (sent on `got`),
// and replies Declare-Ok echoing the queue name. It lets the test assert that a
// declareQuorumQueue call put the x-queue-type=quorum arguments and the durable
// flag on the wire.
func serveAMQPQuorum(ln net.Listener, got chan<- []byte) {
	conn, err := ln.Accept()
	if err != nil {
		close(got)
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	hdr := make([]byte, 8)
	if _, err := io.ReadFull(r, hdr); err != nil {
		close(got)
		return
	}
	start := []byte{0, 9, 0, 0, 0, 0}
	start = amqpLongStr(start, "PLAIN")
	start = amqpLongStr(start, "en_US")
	amqpWriteMethod(conn, 0, 10, 10, start)
	amqpReadFrame(r)                                                 // Start-Ok
	amqpWriteMethod(conn, 0, 10, 30, []byte{0, 0, 0, 2, 0, 0, 0, 0}) // Tune
	amqpReadFrame(r)                                                 // Tune-Ok
	amqpReadFrame(r)                                                 // Connection.Open
	amqpWriteMethod(conn, 0, 10, 41, []byte{0})                      // Open-Ok
	amqpReadFrame(r)                                                 // Channel.Open
	amqpWriteMethod(conn, 1, 20, 11, []byte{0, 0, 0, 0})             // Channel.Open-Ok

	_, decl, err := amqpReadFrame(r) // Queue.Declare
	if err != nil {
		close(got)
		return
	}
	got <- decl

	qLen := int(decl[6])
	qname := string(decl[7 : 7+qLen])
	declOk := amqpShortStr(nil, qname)
	declOk = append(declOk, 0, 0, 0, 0, 0, 0, 0, 0) // message-count 0, consumer-count 0
	amqpWriteMethod(conn, 1, 50, 11, declOk)

	amqpReadFrame(r)                      // Connection.Close
	amqpWriteMethod(conn, 0, 10, 51, nil) // Close-Ok
}

// TestAmqpQuorumQueue proves declareQuorumQueue sends Queue.Declare with the
// durable flag set and an x-queue-type=quorum arguments field-table.
func TestAmqpQuorumQueue(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan []byte, 1)
	go serveAMQPQuorum(ln, got)

	addr := ln.Addr().(*net.TCPAddr)
	amqpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "amqp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as amqp;
def opts as amqp.Options init amqp.withPort(amqp.options("127.0.0.1", "guest", "guest"), %d);
def c as amqp.Conn init amqp.connect($opts);
def qi as amqp.QueueInfo init amqp.declareQuorumQueue($c, "critical");
testing.assertEqual($qi.name, "critical");
amqp.close($c);`, amqpMod, addr.Port)
	progPath := filepath.Join(dir, "amqp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("quorum program failed with code %d", code)
	}

	decl := <-got
	if decl == nil {
		t.Fatal("broker captured no Queue.Declare frame")
	}
	// decl = class(2) method(2) reserved(2) name(shortstr) flags(1) arguments-table.
	if decl[0] != 0 || decl[1] != 50 || decl[2] != 0 || decl[3] != 10 {
		t.Fatalf("not a Queue.Declare: class/method = %d/%d", uint16(decl[0])<<8|uint16(decl[1]), uint16(decl[2])<<8|uint16(decl[3]))
	}
	qLen := int(decl[6])
	flags := decl[7+qLen]
	if flags&2 == 0 {
		t.Errorf("durable flag not set: flags = 0x%02x", flags)
	}
	// The expected x-queue-type=quorum field-table.
	want := binary.BigEndian.AppendUint32(nil, 24)
	want = amqpShortStr(want, "x-queue-type")
	want = append(want, 'S')
	want = amqpLongStr(want, "quorum")
	if gotTable := decl[8+qLen:]; !bytes.Equal(gotTable, want) {
		t.Errorf("arguments table = %x, want %x", gotTable, want)
	}
}

// amqpHandshake plays the broker side of one full connection + channel
// handshake (protocol header in, Start / Tune / Open-Ok, Channel.Open-Ok),
// leaving the socket ready for post-handshake method exchange. Returns false if
// the peer hung up before sending the protocol header.
func amqpHandshake(conn net.Conn, r *bufio.Reader) bool {
	hdr := make([]byte, 8) // "AMQP\0\0\x09\x01"
	if _, err := io.ReadFull(r, hdr); err != nil {
		return false
	}
	start := []byte{0, 9, 0, 0, 0, 0} // version 0.9, empty server-properties table
	start = amqpLongStr(start, "PLAIN")
	start = amqpLongStr(start, "en_US")
	amqpWriteMethod(conn, 0, 10, 10, start)                          // Connection.Start
	amqpReadFrame(r)                                                 // Start-Ok
	amqpWriteMethod(conn, 0, 10, 30, []byte{0, 0, 0, 2, 0, 0, 0, 0}) // Connection.Tune
	amqpReadFrame(r)                                                 // Tune-Ok
	amqpReadFrame(r)                                                 // Connection.Open
	amqpWriteMethod(conn, 0, 10, 41, []byte{0})                      // Open-Ok
	amqpReadFrame(r)                                                 // Channel.Open
	amqpWriteMethod(conn, 1, 20, 11, []byte{0, 0, 0, 0})             // Channel.Open-Ok
	return true
}

// serveAMQPConsume handshakes, answers Basic.Consume with a Basic.Consume-Ok
// carrying a consumer tag, then server-pushes one message as a Basic.Deliver
// method frame + a content-header frame (class 60, body size) + a body frame.
// It lets the test assert receiveDelivery reassembles the pushed delivery.
func serveAMQPConsume(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	if !amqpHandshake(conn, r) {
		return
	}

	// Basic.Consume (class 60, method 20) -> Basic.Consume-Ok (60/21) echoing a
	// broker-generated consumer tag as a shortstr.
	if _, _, err := amqpReadFrame(r); err != nil {
		return
	}
	amqpWriteMethod(conn, 1, 60, 21, amqpShortStr(nil, "ctag-1"))

	// Basic.Deliver (60/60): consumer-tag, delivery-tag(u64), redelivered(bit),
	// exchange(shortstr), routing-key(shortstr).
	deliver := amqpShortStr(nil, "ctag-1")
	deliver = append(deliver, 0, 0, 0, 0, 0, 0, 0, 7) // delivery-tag 7
	deliver = append(deliver, 0)                      // redelivered = false
	deliver = amqpShortStr(deliver, "logs")           // exchange
	deliver = amqpShortStr(deliver, "events")         // routing-key
	amqpWriteMethod(conn, 1, 60, 60, deliver)

	// content-header frame (class 60, weight 0, body-size, no property flags) +
	// the body frame carrying the payload.
	body := []byte("delivered payload")
	header := []byte{0, 60, 0, 0}
	header = binary.BigEndian.AppendUint64(header, uint64(len(body)))
	header = append(header, 0, 0) // property flags: none
	amqpWriteFrame(conn, 2, 1, header)
	amqpWriteFrame(conn, 3, 1, body)

	amqpReadFrame(r)                      // Connection.Close
	amqpWriteMethod(conn, 0, 10, 51, nil) // Connection.Close-Ok
}

// TestAmqpConsume drives the amqp client through connect / consume /
// receiveDelivery / close against the fake broker, proving Basic.Consume ->
// Consume-Ok tag handling and that receiveDelivery reassembles a server-pushed
// Basic.Deliver method frame + content header + body frame into a Delivery.
func TestAmqpConsume(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveAMQPConsume(ln)

	addr := ln.Addr().(*net.TCPAddr)
	amqpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "amqp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use convert;
import %q as amqp;
def opts as amqp.Options init amqp.withPort(amqp.options("127.0.0.1", "guest", "guest"), %d);
def c as amqp.Conn init amqp.connect($opts);

def tag as string init amqp.consume($c, "events", false);
testing.assertEqual($tag, "ctag-1");

def d as amqp.Delivery init amqp.receiveDelivery($c);
testing.assertEqual($d.consumerTag, "ctag-1");
testing.assertEqual($d.deliveryTag, 7);
testing.assertTrue(not $d.redelivered);
testing.assertEqual($d.exchange, "logs");
testing.assertEqual($d.routingKey, "events");
testing.assertEqual(convert.stringFromBytes($d.body, "utf-8"), "delivered payload");

amqp.close($c);`, amqpMod, addr.Port)
	progPath := filepath.Join(dir, "amqp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("amqp consume program failed with code %d", code)
	}
}

// serveAMQPConfirm handshakes then scripts the method-frame replies for
// declareExchange, bindQueue, confirmSelect, a publish + waitConfirm (answered
// with Basic.Ack), and a nack. It exercises the pure method-exchange verbs that
// carry no server-pushed content of their own.
func serveAMQPConfirm(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	if !amqpHandshake(conn, r) {
		return
	}

	amqpReadFrame(r)                      // Exchange.Declare
	amqpWriteMethod(conn, 1, 40, 11, nil) // Exchange.Declare-Ok

	amqpReadFrame(r)                      // Queue.Bind
	amqpWriteMethod(conn, 1, 50, 21, nil) // Queue.Bind-Ok

	amqpReadFrame(r)                      // Confirm.Select
	amqpWriteMethod(conn, 1, 85, 11, nil) // Confirm.Select-Ok

	amqpReadFrame(r) // Basic.Publish method
	amqpReadFrame(r) // content header
	amqpReadFrame(r) // body

	// waitConfirm reads one method: Basic.Ack (60/80) -> true.
	amqpWriteMethod(conn, 1, 60, 80, []byte{0, 0, 0, 0, 0, 0, 0, 1, 0})

	amqpReadFrame(r) // Basic.Nack

	amqpReadFrame(r)                      // Connection.Close
	amqpWriteMethod(conn, 0, 10, 51, nil) // Connection.Close-Ok
}

// TestAmqpConfirmAndBind covers declareExchange / bindQueue / confirmSelect /
// waitConfirm / nack: each writes a method frame whose reply the broker scripts,
// proving the client's method framing and reply matching for the confirm and
// binding verbs.
func TestAmqpConfirmAndBind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveAMQPConfirm(ln)

	addr := ln.Addr().(*net.TCPAddr)
	amqpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "amqp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as amqp;
def opts as amqp.Options init amqp.withPort(amqp.options("127.0.0.1", "guest", "guest"), %d);
def c as amqp.Conn init amqp.connect($opts);

amqp.declareExchange($c, "events", "topic");
amqp.bindQueue($c, "jobs", "events", "job.#");
amqp.confirmSelect($c);
amqp.publishText($c, "events", "job.new", "hi");
testing.assertTrue(amqp.waitConfirm($c));
amqp.nack($c, 1, true);

amqp.close($c);`, amqpMod, addr.Port)
	progPath := filepath.Join(dir, "amqp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("amqp confirm program failed with code %d", code)
	}
}

// TestAmqpRoundTrip drives the amqp client through connect / declareQueue /
// publish / get / ack / close against the fake broker, proving the handshake,
// method framing, content frames, and Basic.Get body reassembly on the wire.
func TestAmqpRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveAMQP(ln)

	addr := ln.Addr().(*net.TCPAddr)
	amqpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "amqp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use convert;
import %q as amqp;
def opts as amqp.Options init amqp.withPort(amqp.options("127.0.0.1", "guest", "guest"), %d);
def c as amqp.Conn init amqp.connect($opts);

def qi as amqp.QueueInfo init amqp.declareQueue($c, "jobs", true);
testing.assertEqual($qi.name, "jobs");
testing.assertEqual($qi.messageCount, 5);

amqp.publishText($c, "", "jobs", "hello amqp");

def m as amqp.Message init amqp.get($c, "jobs", false);
testing.assertTrue(not $m.empty);
testing.assertEqual($m.deliveryTag, 1);
testing.assertEqual($m.routingKey, "jobs");
testing.assertEqual(convert.stringFromBytes($m.body, "utf-8"), "hello amqp");
amqp.ack($c, $m.deliveryTag);

amqp.close($c);`, amqpMod, addr.Port)
	progPath := filepath.Join(dir, "amqp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("amqp program failed with code %d", code)
	}
}
