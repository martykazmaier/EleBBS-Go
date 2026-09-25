package comm

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type memStream struct {
	bytes.Buffer
	local bool
}

func (m *memStream) Close() error                      { return nil }
func (m *memStream) SetReadDeadline(time.Time) error  { return nil }
func (m *memStream) SetWriteDeadline(time.Time) error { return nil }
func (m *memStream) Local() bool                       { return m.local }

func waitFor(t *testing.T, screen *lockedBuf, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if screen.String() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("screen = %q, want %q", screen.String(), want)
}

func TestMirrorEchoesRemoteOutput(t *testing.T) {
	inner := &memStream{}
	screen := &lockedBuf{}
	on := true
	st := Mirror(inner, screen, func() bool { return on })
	st.Write([]byte("\x1b[1;33mHello"))
	on = false
	st.Write([]byte(" hidden"))
	on = true
	st.Write([]byte(" caller"))
	if got := inner.String(); got != "\x1b[1;33mHello hidden caller" {
		t.Fatalf("caller got %q", got)
	}
	waitFor(t, screen, "\x1b[1;33mHello caller")
	st.Close()
	if _, err := st.Write([]byte("after close")); err != nil {
		t.Fatal(err)
	}
}

func TestMirrorLeavesLocalStreamAlone(t *testing.T) {
	inner := &memStream{local: true}
	if st := Mirror(inner, &lockedBuf{}, nil); st != Stream(inner) {
		t.Fatal("local stream should not be wrapped")
	}
}

func TestMirrorKeepsSocketHandle(t *testing.T) {
	inner := &memStream{}
	st := Mirror(inner, &lockedBuf{}, nil)
	if u, ok := st.(interface{ Unwrap() Stream }); !ok || u.Unwrap() != Stream(inner) {
		t.Fatal("mirror must unwrap to the caller stream")
	}
}
