package comm

import (
	"io"
	"os"
	"sync"
	"time"
)

// pumped adds working read deadlines to a Stream whose SetReadDeadline is a
// no-op (SSH channels). A background goroutine reads the inner stream so Peek
// and CSI timeouts can return instead of blocking forever.
type pumped struct {
	inner Stream
	mu    sync.Mutex
	cond  *sync.Cond
	buf   []byte
	err   error
	rdl   time.Time
}

func PumpDeadlines(inner Stream) Stream {
	if inner == nil {
		return nil
	}
	p := &pumped{inner: inner}
	p.cond = sync.NewCond(&p.mu)
	go p.loop()
	return p
}

func (p *pumped) loop() {
	tmp := make([]byte, 512)
	for {
		n, err := p.inner.Read(tmp)
		p.mu.Lock()
		if n > 0 {
			p.buf = append(p.buf, tmp[:n]...)
			p.cond.Broadcast()
		}
		if err != nil {
			p.err = err
			p.cond.Broadcast()
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
	}
}

func (p *pumped) Local() bool { return p.inner.Local() }

func (p *pumped) Unwrap() Stream { return p.inner }

func (p *pumped) SocketHandle() uintptr { return SocketOf(p.inner) }

func (p *pumped) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if len(p.buf) > 0 {
			n := copy(b, p.buf)
			p.buf = p.buf[n:]
			return n, nil
		}
		if p.err != nil {
			return 0, p.err
		}
		if !p.rdl.IsZero() {
			d := time.Until(p.rdl)
			if d <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			timer := time.AfterFunc(d, func() {
				p.mu.Lock()
				p.cond.Broadcast()
				p.mu.Unlock()
			})
			p.cond.Wait()
			timer.Stop()
			continue
		}
		p.cond.Wait()
	}
}

func (p *pumped) Write(b []byte) (int, error) { return p.inner.Write(b) }

func (p *pumped) Close() error {
	err := p.inner.Close()
	p.mu.Lock()
	if p.err == nil {
		p.err = io.EOF
	}
	p.cond.Broadcast()
	p.mu.Unlock()
	return err
}

func (p *pumped) SetReadDeadline(t time.Time) error {
	p.mu.Lock()
	p.rdl = t
	p.cond.Broadcast()
	p.mu.Unlock()
	return nil
}

func (p *pumped) SetWriteDeadline(t time.Time) error {
	return p.inner.SetWriteDeadline(t)
}
