package liveview

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAcceptKeyKnownAnswer checks the handshake accept-key against the exact
// vector from RFC 6455 section 1.3.
func TestAcceptKeyKnownAnswer(t *testing.T) {
	got := AcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("AcceptKey = %q, want %q", got, want)
	}
}

// maskedFrame builds a single masked client frame (FIN set) for opcode/payload.
func maskedFrame(opcode byte, payload []byte, mask [4]byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x80 | opcode)
	n := len(payload)
	switch {
	case n <= 125:
		buf.WriteByte(0x80 | byte(n))
	case n <= 0xFFFF:
		buf.WriteByte(0x80 | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		buf.Write(ext[:])
	default:
		buf.WriteByte(0x80 | 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		buf.Write(ext[:])
	}
	buf.Write(mask[:])
	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	buf.Write(masked)
	return buf.Bytes()
}

func readerConn(b []byte) *Conn {
	return &Conn{br: bufio.NewReader(bytes.NewReader(b))}
}

// TestReadMaskedFrame verifies the server unmasks client frames correctly.
func TestReadMaskedFrame(t *testing.T) {
	mask := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	payload := []byte("Hello, WebSocket!")
	c := readerConn(maskedFrame(opText, payload, mask))
	op, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if op != opText {
		t.Fatalf("opcode = %d, want text", op)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload = %q, want %q", data, payload)
	}
}

// TestFrameLengthEncodings round-trips payloads across the three length
// encodings (7-bit, 16-bit, 64-bit) through a pipe.
func TestFrameLengthEncodings(t *testing.T) {
	sizes := []int{5, 125, 126, 200, 0xFFFF, 0x10000 + 3}
	for _, n := range sizes {
		payload := bytes.Repeat([]byte{'x'}, n)
		a, b := net.Pipe()
		server := &Conn{conn: a, br: bufio.NewReader(a)}
		client := &Conn{conn: b, br: bufio.NewReader(b)}
		go func() { _ = server.WriteBinary(payload) }()
		op, data, err := client.ReadMessage()
		if err != nil {
			t.Fatalf("n=%d ReadMessage: %v", n, err)
		}
		if op != opBinary || len(data) != n {
			t.Fatalf("n=%d got op=%d len=%d", n, op, len(data))
		}
		a.Close()
		b.Close()
	}
}

// TestFragmentedMessage verifies continuation frames are reassembled.
func TestFragmentedMessage(t *testing.T) {
	var buf bytes.Buffer
	mask := [4]byte{1, 2, 3, 4}
	// First text frame, FIN=0.
	f1 := maskedFrame(opText, []byte("Hel"), mask)
	f1[0] &^= 0x80 // clear FIN
	buf.Write(f1)
	// Continuation, FIN=1.
	f2 := maskedFrame(opContinuation, []byte("lo"), mask)
	buf.Write(f2)

	c := readerConn(buf.Bytes())
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(data) != "Hello" {
		t.Fatalf("reassembled = %q, want Hello", data)
	}
}

// TestPingAnswered checks that a ping is answered with a pong and the following
// text message is still delivered.
func TestPingAnswered(t *testing.T) {
	a, b := net.Pipe()
	server := &Conn{conn: a, br: bufio.NewReader(a)}
	client := &Conn{conn: b, br: bufio.NewReader(b)}

	go func() {
		mask := [4]byte{9, 9, 9, 9}
		// Client sends ping then text over the pipe.
		_, _ = b.Write(maskedFrame(opPing, []byte("pi"), mask))
		_, _ = b.Write(maskedFrame(opText, []byte("hi"), mask))
	}()

	// Read the pong the server emits in response to the ping.
	go func() {
		op, data, err := client.ReadMessage()
		if err == nil && op == opText && string(data) == "" {
			_ = data
		}
	}()

	op, data, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if op != opText || string(data) != "hi" {
		t.Fatalf("got op=%d data=%q", op, data)
	}
	server.Close()
	client.Close()
}

// TestCloseFrameReturnsEOF verifies a close control frame ends the read loop.
func TestCloseFrameReturnsEOF(t *testing.T) {
	mask := [4]byte{4, 3, 2, 1}
	frame := maskedFrame(opClose, []byte{0x03, 0xE8}, mask)
	c := &Conn{conn: nopConn{}, br: bufio.NewReader(bytes.NewReader(frame))}
	_, _, err := c.ReadMessage()
	if err != io.EOF {
		t.Fatalf("err = %v, want EOF", err)
	}
}

// TestUnknownOpcode rejects reserved opcodes.
func TestUnknownOpcode(t *testing.T) {
	mask := [4]byte{1, 1, 1, 1}
	frame := maskedFrame(0x3, []byte("x"), mask)
	c := readerConn(frame)
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected error for unknown opcode")
	}
}

// nopConn is a net.Conn whose writes are discarded, for exercising control-frame
// replies without a peer.
type nopConn struct{}

func (nopConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (nopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (nopConn) Close() error                     { return nil }
func (nopConn) LocalAddr() net.Addr              { return nil }
func (nopConn) RemoteAddr() net.Addr             { return nil }
func (nopConn) SetDeadline(time.Time) error      { return nil }
func (nopConn) SetReadDeadline(time.Time) error  { return nil }
func (nopConn) SetWriteDeadline(time.Time) error { return nil }

// tcpConnPair returns two connected TCP endpoints. net.Pipe is unsuitable here:
// it is fully synchronous, so a Write blocks until the peer reads those exact
// bytes, and a test with several concurrent writers and one reader deadlocks on
// the pipe rather than on anything in Conn. A kernel-buffered socket is also what
// production actually hands Conn (a hijacked HTTP connection).
func tcpConnPair(t *testing.T) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	got := <-ch
	if got.err != nil {
		t.Fatalf("accept: %v", got.err)
	}
	t.Cleanup(func() { _ = got.c.Close(); _ = client.Close() })
	return got.c, client
}

// TestConnConcurrentWritersAndClose pins the concurrency contract Conn now
// documents. Under -race on the pre-fix code it fails two ways: writeFrame
// emitted each frame as two unsynchronised conn.Write calls, so parallel writers
// interleaved one frame's header with another's payload; and closed was a plain
// bool that Close wrote while ReadMessage read it.
//
// The assertion is about the framing, not merely the absence of a race report: a
// reader that parses every frame and counts every payload proves the writes did
// not interleave, which is the corruption the write mutex exists to prevent.
func TestConnConcurrentWritersAndClose(t *testing.T) {
	a, b := tcpConnPair(t)
	server := &Conn{conn: a, br: bufio.NewReader(a)}
	client := &Conn{conn: b, br: bufio.NewReader(b)}

	const (
		writers = 8
		each    = 25
	)
	// Differing lengths so a desynchronised reader cannot accidentally resync.
	payloads := make([]string, writers)
	for i := range payloads {
		payloads[i] = strings.Repeat(string(rune('a'+i)), 40+i*7)
	}

	type result struct {
		counts map[string]int
		err    error
	}
	done := make(chan result, 1)
	go func() {
		counts := make(map[string]int)
		for i := 0; i < writers*each; i++ {
			op, msg, err := client.ReadMessage()
			if err != nil {
				done <- result{counts, fmt.Errorf("after %d messages: %w", i, err)}
				return
			}
			if op != opText {
				done <- result{counts, fmt.Errorf("opcode = 0x%X, want text", op)}
				return
			}
			counts[string(msg)]++
		}
		done <- result{counts, nil}
	}()

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if err := server.WriteText(payloads[w]); err != nil {
					t.Errorf("writer %d: %v", w, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("reader: %v (frame stream corrupted by interleaved writes?)", got.err)
		}
		for w, p := range payloads {
			if got.counts[p] != each {
				t.Errorf("payload %d: read %d copies, want %d", w, got.counts[p], each)
			}
		}
	case <-time.After(30 * time.Second):
		t.Fatal("reader did not drain every frame within 30s")
	}

	// Close concurrently from several goroutines: it is documented idempotent, so
	// exactly one teardown must happen and none may race.
	var cwg sync.WaitGroup
	for i := 0; i < 4; i++ {
		cwg.Add(1)
		go func() { defer cwg.Done(); _ = server.Close() }()
	}
	cwg.Wait()
	if !server.isClosed() {
		t.Error("server should be closed after Close()")
	}
	if err := server.Close(); err != nil {
		t.Errorf("repeat Close() = %v, want nil (idempotent)", err)
	}
}
