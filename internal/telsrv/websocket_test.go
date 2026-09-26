package telsrv

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// wsClient upgrades over a pipe the way a browser does and returns the
// client end, its reader and the server side.
func wsClient(t *testing.T, protos string) (net.Conn, *bufio.Reader, *wsConn, *http.Response) {
	t.Helper()
	cli, srv := net.Pipe()
	got := make(chan *wsConn, 1)
	go func() {
		w, err := upgradeWebSocket(srv)
		if err != nil {
			t.Error(err)
		}
		got <- w
	}()
	req := "GET /telnet HTTP/1.1\r\nHost: bbs\r\nUpgrade: websocket\r\nConnection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
	if protos != "" {
		req += "Sec-WebSocket-Protocol: " + protos + "\r\n"
	}
	go func() { _, _ = io.WriteString(cli, req+"\r\n") }()
	r := bufio.NewReader(cli)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cli, r, <-got, resp
}

func clientFrame(op byte, data []byte) []byte {
	mask := [4]byte{1, 2, 3, 4}
	f := []byte{0x80 | op}
	if len(data) < 126 {
		f = append(f, 0x80|byte(len(data)))
	} else {
		f = append(f, 0x80|126)
		f = binary.BigEndian.AppendUint16(f, uint16(len(data)))
	}
	f = append(f, mask[:]...)
	for i, b := range data {
		f = append(f, b^mask[i%4])
	}
	return f
}

func serverFrame(t *testing.T, r *bufio.Reader) (byte, []byte) {
	t.Helper()
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		t.Fatal(err)
	}
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		_, _ = io.ReadFull(r, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		t.Fatal(err)
	}
	return h[0] & 0x0F, data
}

func TestWebSocketBinary(t *testing.T) {
	cli, r, w, resp := wsClient(t, "binary, base64, plain")
	if resp.StatusCode != 101 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept %q", got)
	}
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "binary" {
		t.Fatalf("protocol %q", got)
	}

	go func() { _, _ = cli.Write(clientFrame(opBinary, []byte{0xFF, 0xFB, 1, 'y'})) }()
	buf := make([]byte, 16)
	n, err := w.Read(buf)
	if err != nil || string(buf[:n]) != "\xff\xfb\x01y" {
		t.Fatalf("read %q %v", buf[:n], err)
	}

	go func() { _, _ = w.Write([]byte("CONNECT\xff")) }()
	if op, data := serverFrame(t, r); op != opBinary || string(data) != "CONNECT\xff" {
		t.Fatalf("frame %d %q", op, data)
	}

	go func() { _, _ = cli.Write(clientFrame(opPing, []byte("hi"))) }()
	go func() { _, _ = w.Read(buf) }()
	if op, data := serverFrame(t, r); op != opPong || string(data) != "hi" {
		t.Fatalf("pong %d %q", op, data)
	}
}

func TestWebSocketBase64AndPlain(t *testing.T) {
	cli, r, w, resp := wsClient(t, "base64")
	if resp.Header.Get("Sec-WebSocket-Protocol") != "base64" {
		t.Fatal(resp.Header)
	}
	go func() {
		_, _ = cli.Write(clientFrame(opText, []byte(base64.StdEncoding.EncodeToString([]byte{0xFF, 'a'}))))
	}()
	buf := make([]byte, 16)
	if n, _ := w.Read(buf); string(buf[:n]) != "\xffa" {
		t.Fatalf("base64 read %q", buf[:n])
	}
	go func() { _, _ = w.Write([]byte{0xFF, 'b'}) }()
	if op, data := serverFrame(t, r); op != opText || string(data) != base64.StdEncoding.EncodeToString([]byte{0xFF, 'b'}) {
		t.Fatalf("base64 frame %d %q", op, data)
	}

	cli, r, w, _ = wsClient(t, "plain")
	go func() { _, _ = cli.Write(clientFrame(opText, []byte("\u00ffz"))) }()
	if n, _ := w.Read(buf); string(buf[:n]) != "\xffz" {
		t.Fatalf("plain read %q", buf[:n])
	}
	go func() { _, _ = w.Write([]byte{0xFF}) }()
	if _, data := serverFrame(t, r); string(data) != "\u00ff" {
		t.Fatalf("plain frame %q", data)
	}
}

func TestWebSocketCloseIsEOF(t *testing.T) {
	cli, r, w, _ := wsClient(t, "")
	go func() { _, _ = cli.Write(clientFrame(opClose, []byte{0x03, 0xE8})) }()
	done := make(chan error, 1)
	go func() {
		_, err := w.Read(make([]byte, 4))
		done <- err
	}()
	if op, _ := serverFrame(t, r); op != opClose {
		t.Fatalf("op %d", op)
	}
	if err := <-done; err != io.EOF {
		t.Fatalf("err %v", err)
	}
}

func TestWebSocketRejectsPlainHTTP(t *testing.T) {
	cli, srv := net.Pipe()
	go func() { _, _ = io.WriteString(cli, "GET / HTTP/1.1\r\nHost: bbs\r\n\r\n") }()
	errc := make(chan error, 1)
	go func() {
		_, err := upgradeWebSocket(srv)
		errc <- err
		srv.Close()
	}()
	resp, err := http.ReadResponse(bufio.NewReader(cli), nil)
	if err != nil || resp.StatusCode != 426 || !strings.Contains(resp.Status, "Upgrade") {
		t.Fatalf("%v %v", resp, err)
	}
	if <-errc == nil {
		t.Fatal("expected error")
	}
}
