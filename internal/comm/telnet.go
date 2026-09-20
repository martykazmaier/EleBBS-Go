package comm

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"
)

const (
	iac  = 255
	dont = 254
	do_  = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
)

// Telnet wraps a stream, stripping IAC sequences and answering WILL/DO with WONT/DONT
// except for binary/echo/sga which we accept.
type Telnet struct {
	s      Stream
	r      *bufio.Reader
	mu     sync.Mutex
	closed bool
}

func NewTelnet(s Stream) *Telnet {
	t := &Telnet{s: s, r: bufio.NewReaderSize(s, 4096)}
	// Offer: suppress GA, will echo, binary
	_, _ = s.Write([]byte{iac, will, 3, iac, will, 1, iac, will, 0, iac, do_, 0})
	return t
}

func (t *Telnet) Local() bool { return false }

func (t *Telnet) Unwrap() Stream { return t.s }

func (t *Telnet) SocketHandle() uintptr { return SocketOf(t.s) }

func (t *Telnet) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		b, err := t.r.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		if b != iac {
			// SyncTERM and other NVT clients send CR LF or CR NUL for Enter.
			if b == '\r' && t.r.Buffered() > 0 {
				if peek, err := t.r.Peek(1); err == nil && len(peek) == 1 && (peek[0] == '\n' || peek[0] == 0) {
					_, _ = t.r.ReadByte()
				}
			}
			p[n] = b
			n++
			if t.r.Buffered() == 0 {
				return n, nil
			}
			continue
		}
		cmd, err := t.r.ReadByte()
		if err != nil {
			return n, err
		}
		if cmd == iac {
			p[n] = iac
			n++
			continue
		}
		if cmd == sb {
			for {
				x, err := t.r.ReadByte()
				if err != nil {
					return n, err
				}
				if x == iac {
					y, err := t.r.ReadByte()
					if err != nil {
						return n, err
					}
					if y == se {
						break
					}
				}
			}
			continue
		}
		opt, err := t.r.ReadByte()
		if err != nil {
			return n, err
		}
		var reply byte
		switch cmd {
		case will, do_:
			switch opt {
			case 0, 1, 3:
				if cmd == will {
					reply = do_
				} else {
					reply = will
				}
			default:
				if cmd == will {
					reply = dont
				} else {
					reply = wont
				}
			}
		case wont:
			reply = dont
		case dont:
			reply = wont
		}
		if reply != 0 {
			_, _ = t.s.Write([]byte{iac, reply, opt})
		}
	}
	return n, nil
}

func (t *Telnet) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	escaped := make([]byte, 0, len(p)+8)
	for _, b := range p {
		escaped = append(escaped, b)
		if b == iac {
			escaped = append(escaped, iac)
		}
	}
	_, err := t.s.Write(escaped)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *Telnet) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.s.Close()
}

func (t *Telnet) SetReadDeadline(tm time.Time) error  { return t.s.SetReadDeadline(tm) }
func (t *Telnet) SetWriteDeadline(tm time.Time) error { return t.s.SetWriteDeadline(tm) }

type netStream struct{ net.Conn }

func (n netStream) Local() bool { return false }

func FromConn(c net.Conn) Stream { return netStream{c} }

func PutU32LE(n uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], n)
	return b[:]
}

var _ io.ReadWriteCloser = (*Telnet)(nil)
