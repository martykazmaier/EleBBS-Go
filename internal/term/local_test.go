package term

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
)

type fakeKeys struct{ keys []comm.LocalKey }

func (f *fakeKeys) Poll() (comm.LocalKey, bool) {
	if len(f.keys) == 0 {
		return comm.LocalKey{}, false
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, true
}

// remoteStream is a caller connection whose reads time out when idle.
type remoteStream struct {
	out bytes.Buffer
	in  []byte
}

func (s *remoteStream) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, os.ErrDeadlineExceeded
	}
	n := copy(p, s.in)
	s.in = s.in[n:]
	return n, nil
}
func (s *remoteStream) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *remoteStream) Close() error                     { return nil }
func (s *remoteStream) SetReadDeadline(time.Time) error  { return nil }
func (s *remoteStream) SetWriteDeadline(time.Time) error { return nil }
func (s *remoteStream) Local() bool                      { return false }

func newRemoteIO(in string, keys ...comm.LocalKey) (*IO, *remoteStream) {
	st := &remoteStream{in: []byte(in)}
	t := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{Baud: 38400})
	t.Local = &fakeKeys{keys: keys}
	return t, st
}

func TestLocalKeysAreSysopInput(t *testing.T) {
	tio, _ := newRemoteIO("y", comm.LocalKey{Ch: 'x'})
	ch, err := tio.GetKey(0)
	if err != nil || ch != 'x' || !tio.FromSysop {
		t.Fatalf("got %q %v sysop=%v, want local x", ch, err, tio.FromSysop)
	}
	ch, err = tio.GetKey(0)
	if err != nil || ch != 'y' || tio.FromSysop {
		t.Fatalf("got %q %v sysop=%v, want caller y", ch, err, tio.FromSysop)
	}
}

func TestLocalCommandRunsForExtendedKeys(t *testing.T) {
	tio, _ := newRemoteIO("", comm.LocalKey{Scan: 46}, comm.LocalKey{Ch: 'a'})
	var got []byte
	tio.LocalCommand = func(scan byte) { got = append(got, scan) }
	ch, err := tio.GetKey(0)
	if err != nil || ch != 'a' {
		t.Fatalf("got %q %v", ch, err)
	}
	if string(got) != "\x2e" {
		t.Fatalf("LocalCommand scans %v, want [46]", got)
	}
}

func TestLocalCursorKeysBecomeANSI(t *testing.T) {
	tio, _ := newRemoteIO("", comm.LocalKey{Scan: 72})
	var seq []byte
	for i := 0; i < 3; i++ {
		ch, err := tio.GetKey(0)
		if err != nil {
			t.Fatal(err)
		}
		seq = append(seq, ch)
	}
	if string(seq) != "\x1b[A" {
		t.Fatalf("Up arrow = %q", seq)
	}
}

func TestHangUpEndsInputAndOutput(t *testing.T) {
	tio, st := newRemoteIO("never read", comm.LocalKey{Scan: 35})
	tio.LocalCommand = func(byte) { tio.HangUp() }
	if _, err := tio.GetKey(0); !errors.Is(err, ErrHangup) {
		t.Fatalf("GetKey err = %v, want ErrHangup", err)
	}
	tio.Print("goodbye")
	tio.GotoXY(1, 1)
	if st.out.Len() != 0 {
		t.Fatalf("output after hangup: %q", st.out.String())
	}
	if !tio.HungUp() {
		t.Fatal("HungUp should report the hangup")
	}
}
