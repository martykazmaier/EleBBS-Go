package telsrv

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// maxWSMessage caps one client message; terminal input is a few bytes.
const maxWSMessage = 1 << 20

// WebSocket subprotocols web terminals ask for. "base64" and "plain" carry
// the bytes in text messages: base64 encoded, or one character per byte.
const (
	wsBinary = "binary"
	wsBase64 = "base64"
	wsPlain  = "plain"
)

const (
	opCont   = 0x0
	opText   = 0x1
	opBinary = 0x2
	opClose  = 0x8
	opPing   = 0x9
	opPong   = 0xA
)

// wsConn is a server-side WebSocket that reads and writes the raw telnet
// stream.
type wsConn struct {
	net.Conn
	r         *bufio.Reader
	proto     string
	wmu       sync.Mutex
	closeOnce sync.Once
	pend      []byte
	msg       []byte
	cont      byte
	done      bool
}

// upgradeWebSocket reads the HTTP upgrade request from c and answers it.
func upgradeWebSocket(c net.Conn) (*wsConn, error) {
	r := bufio.NewReaderSize(c, 4096)
	tp := textproto.NewReader(r)
	line, err := tp.ReadLine()
	if err != nil {
		return nil, fmt.Errorf("WebSocket request: %w", err)
	}
	f := strings.Fields(line)
	if len(f) != 3 || f[0] != "GET" || !strings.HasPrefix(f[2], "HTTP/1.") {
		wsReject(c, "400 Bad Request")
		return nil, fmt.Errorf("not a WebSocket request: %q", line)
	}
	h, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("WebSocket headers: %w", err)
	}
	key := strings.TrimSpace(h.Get("Sec-Websocket-Key"))
	if !headerHas(h.Get("Upgrade"), "websocket") || !headerHas(h.Get("Connection"), "upgrade") || key == "" {
		wsReject(c, "426 Upgrade Required")
		return nil, fmt.Errorf("not a WebSocket upgrade")
	}
	if v := strings.TrimSpace(h.Get("Sec-Websocket-Version")); v != "13" {
		_, _ = io.WriteString(c, "HTTP/1.1 426 Upgrade Required\r\nSec-WebSocket-Version: 13\r\nContent-Length: 0\r\n\r\n")
		return nil, fmt.Errorf("WebSocket version %q", v)
	}
	proto := pickProto(h.Values("Sec-Websocket-Protocol"))
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n"
	if proto != "" {
		resp += "Sec-WebSocket-Protocol: " + proto + "\r\n"
	}
	if _, err := io.WriteString(c, resp+"\r\n"); err != nil {
		return nil, err
	}
	if proto == "" {
		proto = wsBinary
	}
	return &wsConn{Conn: c, r: r, proto: proto}, nil
}

func wsReject(c net.Conn, status string) {
	_, _ = io.WriteString(c, "HTTP/1.1 "+status+"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
}

func wsAccept(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerHas(v, token string) bool {
	for _, p := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(p), token) {
			return true
		}
	}
	return false
}

// pickProto chooses binary, then base64, then plain from what the client
// offered; "" if it offered none of them (or nothing).
func pickProto(vals []string) string {
	offered := map[string]bool{}
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			offered[strings.ToLower(strings.TrimSpace(p))] = true
		}
	}
	for _, p := range []string{wsBinary, wsBase64, wsPlain} {
		if offered[p] {
			return p
		}
	}
	return ""
}

func (w *wsConn) Read(p []byte) (int, error) {
	for len(w.pend) == 0 {
		if w.done {
			return 0, io.EOF
		}
		if err := w.readFrame(); err != nil {
			return 0, err
		}
	}
	n := copy(p, w.pend)
	w.pend = w.pend[n:]
	return n, nil
}

func (w *wsConn) readFrame() error {
	var hdr [2]byte
	if _, err := io.ReadFull(w.r, hdr[:]); err != nil {
		return err
	}
	fin := hdr[0]&0x80 != 0
	op := hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	size := uint64(hdr[1] & 0x7F)
	switch size {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(w.r, b[:]); err != nil {
			return err
		}
		size = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(w.r, b[:]); err != nil {
			return err
		}
		size = binary.BigEndian.Uint64(b[:])
	}
	if !masked {
		return errors.New("WebSocket: unmasked client frame")
	}
	if size > maxWSMessage || uint64(len(w.msg))+size > maxWSMessage {
		w.writeFrame(opClose, []byte{0x03, 0xF1})
		return errors.New("WebSocket: message too large")
	}
	var mask [4]byte
	if _, err := io.ReadFull(w.r, mask[:]); err != nil {
		return err
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(w.r, data); err != nil {
		return err
	}
	for i := range data {
		data[i] ^= mask[i%4]
	}
	switch op {
	case opPing:
		return w.writeFrame(opPong, data)
	case opPong:
		return nil
	case opClose:
		code := []byte{0x03, 0xE8}
		if len(data) >= 2 {
			code = data[:2]
		}
		_ = w.writeFrame(opClose, code)
		w.done = true
		return nil
	case opText, opBinary:
		w.cont = op
		w.msg = append(w.msg[:0], data...)
	case opCont:
		w.msg = append(w.msg, data...)
	default:
		return fmt.Errorf("WebSocket: opcode %d", op)
	}
	if !fin {
		return nil
	}
	b, err := w.decode(w.cont, w.msg)
	w.msg = w.msg[:0]
	if err != nil {
		return err
	}
	w.pend = b
	return nil
}

func (w *wsConn) decode(op byte, msg []byte) ([]byte, error) {
	if op == opBinary {
		return append([]byte(nil), msg...), nil
	}
	switch w.proto {
	case wsBase64:
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(msg)))
	case wsPlain:
		out := make([]byte, 0, len(msg))
		for s := string(msg); s != ""; {
			r, n := utf8.DecodeRuneInString(s)
			out = append(out, byte(r))
			s = s[n:]
		}
		return out, nil
	}
	return append([]byte(nil), msg...), nil
}

func (w *wsConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var err error
	switch w.proto {
	case wsBase64:
		err = w.writeFrame(opText, []byte(base64.StdEncoding.EncodeToString(p)))
	case wsPlain:
		var s strings.Builder
		for _, b := range p {
			s.WriteRune(rune(b))
		}
		err = w.writeFrame(opText, []byte(s.String()))
	default:
		err = w.writeFrame(opBinary, p)
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsConn) writeFrame(op byte, data []byte) error {
	hdr := []byte{0x80 | op, 0}
	switch n := len(data); {
	case n < 126:
		hdr[1] = byte(n)
	case n <= 0xFFFF:
		hdr[1] = 126
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr[1] = 127
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	w.wmu.Lock()
	defer w.wmu.Unlock()
	_, err := w.Conn.Write(append(hdr, data...))
	return err
}

func (w *wsConn) Close() error {
	err := net.ErrClosed
	w.closeOnce.Do(func() {
		_ = w.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = w.writeFrame(opClose, []byte{0x03, 0xE8})
		err = w.Conn.Close()
	})
	return err
}
