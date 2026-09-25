package sysop

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

// fakeWin is an 80x25 node window.
type fakeWin struct {
	rows [25][80]byte
}

func newFakeWin() *fakeWin {
	w := &fakeWin{}
	for y := range w.rows {
		for x := range w.rows[y] {
			w.rows[y][x] = ' '
		}
	}
	return w
}

func (w *fakeWin) WriteAt(x, y int, _ byte, s []byte) {
	for i, c := range s {
		if y >= 1 && y <= 25 && x+i >= 1 && x+i <= 80 {
			w.rows[y-1][x+i-1] = c
		}
	}
}
func (w *fakeWin) GotoXY(int, int) {}
func (w *fakeWin) Save() func() {
	saved := w.rows
	return func() { w.rows = saved }
}
func (w *fakeWin) text() string {
	var b strings.Builder
	for _, r := range w.rows {
		b.WriteString(pascal.FromCP437(r[:]))
		b.WriteByte('\n')
	}
	return b.String()
}

// fakeKeys hands out node-window keys; gate holds them back until it is true.
type fakeKeys struct {
	keys []comm.LocalKey
	gate func() bool
}

func (f *fakeKeys) Poll() (comm.LocalKey, bool) {
	if len(f.keys) == 0 || (f.gate != nil && !f.gate()) {
		return comm.LocalKey{}, false
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, true
}

func typed(s string) []comm.LocalKey {
	var ks []comm.LocalKey
	for i := 0; i < len(s); i++ {
		ks = append(ks, comm.LocalKey{Ch: s[i]})
	}
	return ks
}

type remote struct {
	out bytes.Buffer
	in  []byte
}

func (s *remote) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, os.ErrDeadlineExceeded
	}
	n := copy(p, s.in[:1])
	s.in = s.in[n:]
	return n, nil
}
func (s *remote) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *remote) Close() error                     { return nil }
func (s *remote) SetReadDeadline(time.Time) error  { return nil }
func (s *remote) SetWriteDeadline(time.Time) error { return nil }
func (s *remote) Local() bool                      { return false }

func newNode(t *testing.T, callerTyped string, keys ...comm.LocalKey) (*Node, *remote, *fakeWin) {
	t.Helper()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = t.TempDir()
	g.RaConfig.SysPath = g.RaConfig.TextPath
	g.RaConfig.WindFore, g.RaConfig.HiFore, g.RaConfig.BorderFore = 7, 15, 9
	line := &cfgrec.LineCfg{Baud: 38400, LoggedOn: true, RaNodeNr: 1}
	line.User = cfgrec.User{Name: "Joe Caller", Location: "Here", Record: -1}
	st := &remote{in: []byte(callerTyped)}
	tio := term.New(st, g, line)
	win := newFakeWin()
	kb := &fakeKeys{keys: keys}
	n := &Node{T: tio, G: g, Line: line, Win: win, Keys: kb, sleep: func(time.Duration) {}}
	tio.Local = kb
	tio.LocalCommand = n.Command
	return n, st, win
}

func TestAltHHangsUp(t *testing.T) {
	n, _, win := newNode(t, "")
	n.Command(ScanAltH)
	if !n.T.HungUp() {
		t.Fatal("Alt-H should hang up the caller")
	}
	if !strings.Contains(win.text(), "Terminating call") {
		t.Fatal("Alt-H should show Terminating call on the node window")
	}
}

func TestLimitLocalDisablesSysopKeys(t *testing.T) {
	n, _, _ := newNode(t, "")
	n.G.RaConfig.LimitLocal = true
	n.Command(ScanAltH)
	if n.T.HungUp() {
		t.Fatal("LimitLocal must disable Alt-H")
	}
}

func TestKeyboardPasswordGuardsSysopKeys(t *testing.T) {
	n, _, win := newNode(t, "", typed("wrong\r")...)
	n.G.RaConfig.KeyboardPwd = "secret"
	n.Command(ScanAltH)
	if n.T.HungUp() {
		t.Fatal("a wrong keyboard password must not hang up")
	}
	if strings.Contains(win.text(), "Password") {
		t.Fatal("the password box should be removed afterwards")
	}
	n.Keys.(*fakeKeys).keys = typed("SECRET\r")
	n.Command(ScanAltH)
	if !n.T.HungUp() {
		t.Fatal("the keyboard password is case-insensitive, like Pascal")
	}
}

func TestChatSysopTypesUntilEsc(t *testing.T) {
	n, st, _ := newNode(t, "", append(typed("ok"), comm.LocalKey{Ch: 0x1b})...)
	n.Chat()
	out := st.out.String()
	if !strings.Contains(out, "ok") {
		t.Fatalf("sysop text missing from chat: %q", out)
	}
	if ch, err := n.T.GetKey(time.Millisecond); err != nil || ch != 255 {
		t.Fatalf("chat should leave #255 to redraw the menu, got %v %v", ch, err)
	}
	if n.chatting {
		t.Fatal("chat flag should be cleared")
	}
}

func TestChatCallerEscIsIgnored(t *testing.T) {
	n, st, _ := newNode(t, "a\x1bb")
	kb := n.Keys.(*fakeKeys)
	kb.keys = []comm.LocalKey{{Ch: 0x1b}}
	kb.gate = func() bool { return len(st.in) == 0 }
	n.Chat()
	out := st.out.String()
	i := strings.Index(out, "a")
	if i < 0 || !strings.Contains(out[i:], "b") {
		t.Fatalf("caller text should continue past their Esc: %q", out)
	}
}

func TestAltCFromMenuKeyRunsChat(t *testing.T) {
	n, st, _ := newNode(t, "", comm.LocalKey{Scan: ScanAltC}, comm.LocalKey{Ch: 'z'}, comm.LocalKey{Ch: 0x1b})
	ch, err := n.T.GetKey(0)
	if err != nil || ch != 255 {
		t.Fatalf("after chat GetKey = %v %v, want 255", ch, err)
	}
	if !strings.Contains(st.out.String(), "z") {
		t.Fatal("chat text missing")
	}
}

func TestEditUserSavesChanges(t *testing.T) {
	keys := []comm.LocalKey{{Ch: '\t'}}
	keys = append(keys, typed("Ali\r")...)
	keys = append(keys, comm.LocalKey{Ch: 0x1b}, comm.LocalKey{Ch: 'y'})
	n, st, win := newNode(t, "", keys...)
	before := win.text()
	n.EditUser()
	if n.Line.User.Handle != "Ali" {
		t.Fatalf("Handle = %q, want Ali", n.Line.User.Handle)
	}
	if win.text() != before {
		t.Fatal("the node window should be restored after editing")
	}
	if st.out.Len() != 0 {
		t.Fatalf("the caller must not see the editor: %q", st.out.String())
	}
}

func TestEditUserDiscardsOnNo(t *testing.T) {
	keys := append(typed("\t\t"), typed("Elsewhere")...)
	keys = append(keys, comm.LocalKey{Ch: 0x1b}, comm.LocalKey{Ch: 'n'})
	n, _, _ := newNode(t, "", keys...)
	n.EditUser()
	if n.Line.User.Location != "Here" {
		t.Fatalf("Location = %q, want the original after N", n.Line.User.Location)
	}
}

func TestEditUserRejectsOutOfRangeNumbers(t *testing.T) {
	// Screen length accepts 1..99: 0 is re-edited, then 40 sticks.
	var keys []comm.LocalKey
	for i := 0; i < 27; i++ {
		keys = append(keys, comm.LocalKey{Ch: '\t'})
	}
	keys = append(keys, comm.LocalKey{Ch: 25})
	keys = append(keys, typed("0\r")...)
	keys = append(keys, comm.LocalKey{Ch: 25})
	keys = append(keys, typed("40\r")...)
	keys = append(keys, comm.LocalKey{Ch: 0x1b}, comm.LocalKey{Ch: 'y'})
	n, _, _ := newNode(t, "", keys...)
	n.Line.User.ScreenLength = 24
	n.EditUser()
	if n.Line.User.ScreenLength != 40 {
		t.Fatalf("ScreenLength = %d, want 40", n.Line.User.ScreenLength)
	}
}

func TestEditUserFlagsWindow(t *testing.T) {
	var keys []comm.LocalKey
	for i := 0; i < 31; i++ {
		keys = append(keys, comm.LocalKey{Ch: '\t'})
	}
	// Enter opens the flags window; N clears Deleted, Y sets Clear screen.
	keys = append(keys, typed("\rNY")...)
	keys = append(keys, comm.LocalKey{Ch: 0x1b}, comm.LocalKey{Ch: 0x1b}, comm.LocalKey{Ch: 'y'})
	n, _, _ := newNode(t, "", keys...)
	n.Line.User.Attribute = cfgrec.UserDeleted
	n.Line.User.ScreenLength = 24
	n.EditUser()
	if n.Line.User.Attribute != 1<<1 {
		t.Fatalf("Attribute = %08b, want only Clear screen", n.Line.User.Attribute)
	}
}

func TestFieldHelpers(t *testing.T) {
	if got := dateField("1-2-99"); got != "01-02-99" {
		t.Fatalf("dateField = %q", got)
	}
	if got := dateField("00-12-99"); got != "" {
		t.Fatalf("zero month should clear the date, got %q", got)
	}
	if got := timeField("7:5"); got != "07:05" {
		t.Fatalf("timeField = %q", got)
	}
	if flagsStr(0x81) != "X------X" || flagsByte("X------X") != 0x81 {
		t.Fatal("flags round trip")
	}
}
