package term

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
)

// idleStream blocks in Read until the deadline, then reports a timeout.
type idleStream struct {
	out bytes.Buffer
	mu  sync.Mutex
	dl  time.Time
}

func (s *idleStream) Read([]byte) (int, error) {
	s.mu.Lock()
	dl := s.dl
	s.mu.Unlock()
	if !dl.IsZero() {
		if w := time.Until(dl); w > 0 {
			time.Sleep(w)
		}
	}
	return 0, idleTimeout{}
}

func (s *idleStream) Write(p []byte) (int, error) { return s.out.Write(p) }
func (s *idleStream) Close() error                { return nil }
func (s *idleStream) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	s.dl = t
	s.mu.Unlock()
	return nil
}
func (s *idleStream) SetWriteDeadline(time.Time) error { return nil }
func (s *idleStream) Local() bool                      { return false }

func idleIO(t *testing.T, baud uint16) (*IO, *idleStream, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "node.log")
	st := &idleStream{}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.LogFileName = logPath
	line := &cfgrec.LineCfg{Baud: baud, AnsiOn: true}
	io := New(st, g, line)
	io.Ral = nil
	return io, st, logPath
}

func TestIdleTimeoutDisconnectsRemote(t *testing.T) {
	io, st, logPath := idleIO(t, 2400)
	io.ArmIdle(40 * time.Millisecond)
	_, err := io.GetKey(time.Second)
	if !errors.Is(err, ErrIdle) {
		t.Fatalf("err=%v", err)
	}
	if !io.IdleHung() {
		t.Fatal("expected IdleHung")
	}
	out := st.out.String()
	if !strings.Contains(out, "Inactivity Timeout") {
		t.Fatalf("output %q", out)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), ">  Inactivity timeout") {
		t.Fatalf("log %q", raw)
	}
	_, err = io.GetKey(time.Second)
	if !errors.Is(err, ErrIdle) {
		t.Fatalf("second err=%v", err)
	}
}

func TestIdleWarnsBeforeDisconnect(t *testing.T) {
	io, st, _ := idleIO(t, 2400)
	io.ArmIdle(60 * time.Second)
	io.idleLimit = time.Now().Add(20 * time.Second)
	_, err := io.GetKey(30 * time.Millisecond)
	if !errors.Is(err, idleTimeout{}) && !readTimeout(err) {
		t.Fatalf("err=%v", err)
	}
	if io.IdleHung() {
		t.Fatal("warning should not hang up")
	}
	out := st.out.String()
	if !strings.Contains(out, "disconnected for inactivity") {
		t.Fatalf("output %q", out)
	}
	if !bytes.Contains(st.out.Bytes(), []byte{7, 7}) {
		t.Fatal("expected two BEL")
	}
	st.out.Reset()
	io.idleLimit = time.Now().Add(20 * time.Second)
	_, _ = io.GetKey(30 * time.Millisecond)
	if strings.Contains(st.out.String(), "disconnected for inactivity") {
		t.Fatal("warning repeated inside the same idle stretch")
	}
}

func TestIdleIgnoresLocalLogon(t *testing.T) {
	io, st, _ := idleIO(t, 0)
	io.ArmIdle(30 * time.Millisecond)
	_, err := io.GetKey(80 * time.Millisecond)
	if errors.Is(err, ErrIdle) {
		t.Fatal("local logon timed out")
	}
	if strings.Contains(st.out.String(), "Inactivity") {
		t.Fatalf("output %q", st.out.String())
	}
}

func TestIdleResetsOnKey(t *testing.T) {
	io, _, _ := idleIO(t, 2400)
	io.ArmIdle(time.Hour)
	io.idleLimit = time.Now().Add(5 * time.Second)
	io.PutBack("Z")
	ch, err := io.GetKey(0)
	if err != nil || ch != 'Z' {
		t.Fatalf("ch=%q err=%v", ch, err)
	}
	if time.Until(io.idleLimit) < 30*time.Minute {
		t.Fatalf("limit not reset: %s", time.Until(io.idleLimit))
	}
}

func TestSuspendIdleSkipsTimeout(t *testing.T) {
	io, _, _ := idleIO(t, 2400)
	io.ArmIdle(30 * time.Millisecond)
	resume := io.SuspendIdle()
	_, err := io.GetKey(80 * time.Millisecond)
	if errors.Is(err, ErrIdle) {
		t.Fatal("timed out while suspended")
	}
	resume()
	if time.Until(io.idleLimit) < 10*time.Millisecond {
		t.Fatal("resume did not reset the idle clock")
	}
}
