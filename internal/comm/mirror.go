package comm

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// mirrored is Pascal snooping for remote callers: everything written to the
// caller is also shown on the node's local screen. The screen is fed from a
// queue so a stalled console (QuickEdit selection) cannot block the session.
type mirrored struct {
	Stream
	mu      sync.Mutex
	q       chan []byte
	closed  bool
	on      func() bool
	scr     sync.Mutex // held while the screen is drawn or paused
	pending atomic.Int64
}

const mirrorQueue = 1024

// Mirror tees writes on s to screen while on reports true (nil = always).
// Local streams already render on the console and are returned unchanged.
func Mirror(s Stream, screen io.Writer, on func() bool) Stream {
	if s == nil || screen == nil || s.Local() {
		return s
	}
	m := &mirrored{Stream: s, q: make(chan []byte, mirrorQueue), on: on}
	go func() {
		for b := range m.q {
			m.scr.Lock()
			_, _ = screen.Write(b)
			m.scr.Unlock()
			m.pending.Add(-1)
		}
	}()
	return m
}

func (m *mirrored) Write(b []byte) (int, error) {
	n, err := m.Stream.Write(b)
	if n > 0 && (m.on == nil || m.on()) {
		m.mu.Lock()
		if !m.closed {
			select {
			case m.q <- append([]byte(nil), b[:n]...):
				m.pending.Add(1)
			default:
			}
		}
		m.mu.Unlock()
	}
	return n, err
}

// Pause lets a sysop window own the node screen: queued output is drawn
// first, later output waits until the returned resume is called.
func (m *mirrored) Pause() (resume func()) {
	for deadline := time.Now().Add(time.Second); m.pending.Load() > 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	m.scr.Lock()
	return m.scr.Unlock
}

func (m *mirrored) Close() error {
	err := m.Stream.Close()
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.q)
	}
	m.mu.Unlock()
	return err
}

func (m *mirrored) Unwrap() Stream { return m.Stream }

func (m *mirrored) SocketHandle() uintptr { return SocketOf(m.Stream) }
