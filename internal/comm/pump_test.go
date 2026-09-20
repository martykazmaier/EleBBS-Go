package comm

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

type connStream struct {
	net.Conn
}

func (c connStream) Local() bool { return false }

func TestPumpReadDeadline(t *testing.T) {
	a, b := net.Pipe()
	s := PumpDeadlines(connStream{a})
	defer s.Close()
	defer b.Close()

	_ = s.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
	buf := make([]byte, 1)
	n, err := s.Read(buf)
	if n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("deadline: n=%d err=%v", n, err)
	}

	_ = s.SetReadDeadline(time.Time{})
	done := make(chan error, 1)
	go func() {
		_, err := b.Write([]byte{'F'})
		done <- err
	}()
	n, err = s.Read(buf)
	if err != nil || n != 1 || buf[0] != 'F' {
		t.Fatalf("read F: n=%d err=%v b=%q", n, err, buf[:n])
	}
	<-done
}
