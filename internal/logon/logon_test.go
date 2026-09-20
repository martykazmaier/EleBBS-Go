package logon

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/term"
)

type memStream struct{ out bytes.Buffer }

func (m *memStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *memStream) Write(p []byte) (int, error)      { return m.out.Write(p) }
func (m *memStream) Close() error                     { return nil }
func (m *memStream) SetReadDeadline(time.Time) error  { return nil }
func (m *memStream) SetWriteDeadline(time.Time) error { return nil }
func (m *memStream) Local() bool                      { return true }

func TestOnceOnlyNewerThanLastLogon(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ONCEONLY.ANS")
	if err := os.WriteFile(p, []byte("ONCE-ART"), 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 9, 16, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := term.New(st, g, line)
	if !onceOnlyNewer(tio, dir, "09-15-26", "10:00") {
		t.Fatal("ONCEONLY.ANS newer than last logon should show")
	}
	if onceOnlyNewer(tio, dir, "09-16-26", "13:00") {
		t.Fatal("ONCEONLY.ANS older than last logon should not show")
	}
}

func TestShowWelcomeFilesDisplaysOnceOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ONCEONLY.ANS"), []byte("ONCE-ART"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Security: 10}}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := term.New(st, g, line)
	showWelcomeFiles(tio, g, line, "01-01-20", "00:00")
	if !bytes.Contains(st.out.Bytes(), []byte("ONCE-ART")) {
		t.Fatalf("ONCEONLY not shown: %q", st.out.Bytes())
	}
}

func TestBirthdayToday(t *testing.T) {
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.Local)
	if !birthdayToday("09-16-90", now) {
		t.Fatal("birthday")
	}
	if birthdayToday("09-17-90", now) {
		t.Fatal("not birthday")
	}
}

func TestPerformTurnsOffMorePrompt(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.LogonPrompt = "Name: "
	tio := term.New(st, g, line)
	if Perform(tio, g, line) {
		t.Fatal("expected logon fail on EOF")
	}
	if line.DispMorePrompt {
		t.Fatal("more prompt must stay off through username")
	}
	if bytes.Contains(bytes.ToLower(st.out.Bytes()), []byte("more")) {
		t.Fatalf("more prompt during logon: %q", st.out.Bytes())
	}
}

type keyStream struct {
	out bytes.Buffer
	in  []byte
}

func (s *keyStream) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.in)
	s.in = s.in[n:]
	return n, nil
}
func (s *keyStream) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *keyStream) Close() error                     { return nil }
func (s *keyStream) SetReadDeadline(time.Time) error  { return nil }
func (s *keyStream) SetWriteDeadline(time.Time) error { return nil }
func (s *keyStream) Local() bool                      { return true }

func TestNewUserLanguagePromptStartsQuestionnaire(t *testing.T) {
	dir := t.TempDir()
	st := &keyStream{in: []byte("Y1\r24\rTown\rsecret\rsecret\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.NewSecurity = 10
	g.RaConfig.MinPwdLen = 4
	g.RaConfig.NewUserLang = 1
	g.RaConfig.LanguagePrompt = "\x0b@LANGSEL"
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true, RaNodeNr: 1}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	var got []string
	tio.RunScript = func(name, _ string) { got = append(got, name) }
	ral := lang.Load(g, line.Language)
	tio.Ral = ral
	if !newUser(tio, g, line, ral, "Test User") {
		t.Fatalf("newUser failed, out=%q scripts=%v", st.out.Bytes(), got)
	}
	found := false
	for _, n := range got {
		if strings.EqualFold(n, "LANGSEL") {
			found = true
		}
	}
	if !found {
		t.Fatalf("language prompt did not start LANGSEL.q-a, got %v out=%q", got, st.out.Bytes())
	}
	if bytes.Contains(bytes.ToLower(st.out.Bytes()), []byte("more")) || bytes.Contains(bytes.ToLower(st.out.Bytes()), []byte("paused")) {
		t.Fatalf("pause/more prompt during new user: %q", st.out.Bytes())
	}
}

func TestNewUserRequiresLocationDefaultsHandle(t *testing.T) {
	dir := t.TempDir()
	// empty location is rejected, then Town; empty handle keeps the real name
	st := &keyStream{in: []byte("Y24\r\rTown\r\rsecret\rsecret\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.NewSecurity = 10
	g.RaConfig.MinPwdLen = 4
	g.RaConfig.AskHandle = true
	line := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 1}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	ral := lang.Load(g, line.Language)
	tio.Ral = ral
	if !newUser(tio, g, line, ral, "Test User") {
		t.Fatalf("newUser failed, leftover=%q out=%q", st.in, st.out.Bytes())
	}
	if line.User.Location != "Town" {
		t.Fatalf("location %q want Town", line.User.Location)
	}
	if line.User.Handle != "Test User" {
		t.Fatalf("handle %q want real name", line.User.Handle)
	}
}

func TestWriteRALanguagePromptRunsQAWithoutPause(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	tio := term.New(st, &cfgrec.GlobalCfg{}, line)
	tio.MorePrompt = true
	tio.Lines = 80
	var got string
	tio.RunScript = func(name, _ string) {
		got = name
		if !tio.NoPause() {
			t.Fatal("questionnaire from language prompt must run with pause off")
		}
	}
	tio.WriteRA("\x0b@ASKLOC")
	if got != "ASKLOC" {
		t.Fatalf("got %q", got)
	}
	if bytes.Contains(bytes.ToLower(st.out.Bytes()), []byte("more")) {
		t.Fatalf("more prompt leaked: %q", st.out.Bytes())
	}
}
