package liveview

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// wsGUID is the RFC 6455 "magic string" appended to the client's
// Sec-WebSocket-Key before hashing to form the Sec-WebSocket-Accept response.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket opcodes as defined by RFC 6455 section 5.2.
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// AcceptKey computes the Sec-WebSocket-Accept value for a given
// Sec-WebSocket-Key per RFC 6455: base64(sha1(key + GUID)). It is exported so
// the handshake can be verified independently in tests against the RFC's
// known-answer vector.
func AcceptKey(key string) string {
	h := sha1.New()
	io.WriteString(h, key+wsGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// isWebSocketUpgrade reports whether r is a WebSocket upgrade request: it must
// carry Connection: Upgrade and Upgrade: websocket headers (matched
// case-insensitively) plus a Sec-WebSocket-Key.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		return false
	}
	return r.Header.Get("Sec-WebSocket-Key") != ""
}

// headerContainsToken reports whether a comma-separated header value contains
// token, compared case-insensitively and ignoring surrounding whitespace.
func headerContainsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// Conn is a minimal RFC 6455 WebSocket connection built directly on a hijacked
// net.Conn. It implements the framing needed by the liveview transport: reading
// masked client frames, writing unmasked server frames, and answering ping and
// close control frames. It is not safe for concurrent readers, nor for
// concurrent writers; the runtime serializes writes through a single goroutine.
type Conn struct {
	conn   net.Conn
	br     *bufio.Reader
	closed bool
}

// Upgrade performs the server side of the RFC 6455 opening handshake on an
// HTTP request that has already been validated as a WebSocket upgrade. It
// hijacks the underlying TCP connection, writes the 101 Switching Protocols
// response with the computed Sec-WebSocket-Accept header, and returns a [Conn]
// ready for framing. It returns an error if the request is not an upgrade, the
// ResponseWriter does not support hijacking, or the handshake write fails.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !isWebSocketUpgrade(r) {
		return nil, errors.New("liveview: not a websocket upgrade request")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("liveview: response writer does not support hijacking")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("liveview: hijack failed: %w", err)
	}
	accept := AcceptKey(r.Header.Get("Sec-WebSocket-Key"))
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := io.WriteString(conn, resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("liveview: handshake write failed: %w", err)
	}
	// Preserve any bytes the client already pipelined after the handshake.
	return &Conn{conn: conn, br: brw.Reader}, nil
}

// frame is a single decoded WebSocket frame.
type frame struct {
	fin     bool
	opcode  byte
	payload []byte
}

// readFrame reads and unmasks a single frame from the wire.
func (c *Conn) readFrame() (frame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return frame{}, err
	}
	fin := hdr[0]&0x80 != 0
	opcode := hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return frame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return frame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
			return frame{}, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return frame{}, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return frame{fin: fin, opcode: opcode, payload: payload}, nil
}

// writeFrame writes a single unmasked server frame with the FIN bit set.
func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	var hdr []byte
	b0 := byte(0x80) | opcode // FIN + opcode
	n := len(payload)
	switch {
	case n <= 125:
		hdr = []byte{b0, byte(n)}
	case n <= 0xFFFF:
		hdr = []byte{b0, 126, 0, 0}
		binary.BigEndian.PutUint16(hdr[2:], uint16(n))
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadMessage reads the next complete application message, transparently
// handling fragmentation and answering control frames: a ping is replied to
// with a pong, a pong is ignored, and a close causes the connection to be
// closed and [io.EOF] returned. The returned opcode is [opText] or [opBinary].
func (c *Conn) ReadMessage() (opcode byte, data []byte, err error) {
	var (
		msg     []byte
		msgOp   byte
		started bool
	)
	for {
		f, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch f.opcode {
		case opPing:
			if err := c.writeFrame(opPong, f.payload); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			_ = c.writeClose(f.payload)
			c.closeConn()
			return 0, nil, io.EOF
		case opText, opBinary:
			msgOp = f.opcode
			msg = append(msg, f.payload...)
			started = true
		case opContinuation:
			if !started {
				return 0, nil, errors.New("liveview: continuation frame without start")
			}
			msg = append(msg, f.payload...)
		default:
			return 0, nil, fmt.Errorf("liveview: unknown opcode 0x%X", f.opcode)
		}
		if f.fin {
			return msgOp, msg, nil
		}
	}
}

// WriteText sends a text (UTF-8) message.
func (c *Conn) WriteText(s string) error {
	return c.writeFrame(opText, []byte(s))
}

// WriteBinary sends a binary message.
func (c *Conn) WriteBinary(b []byte) error {
	return c.writeFrame(opBinary, b)
}

// Ping sends a ping control frame with an optional payload.
func (c *Conn) Ping(payload []byte) error {
	return c.writeFrame(opPing, payload)
}

// writeClose sends a close control frame echoing the peer's status code.
func (c *Conn) writeClose(payload []byte) error {
	if c.closed {
		return nil
	}
	return c.writeFrame(opClose, payload)
}

// Close sends a normal (1000) close frame and tears down the connection. It is
// idempotent.
func (c *Conn) Close() error {
	if c.closed {
		return nil
	}
	_ = c.writeFrame(opClose, []byte{0x03, 0xE8}) // status 1000
	return c.closeConn()
}

func (c *Conn) closeConn() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}
