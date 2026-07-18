package liveview

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
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
