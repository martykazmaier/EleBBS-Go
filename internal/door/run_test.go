package door

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/term"
)

type memStream struct{}

func (m *memStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *memStream) Write(p []byte) (int, error)      { return len(p), nil }
func (m *memStream) Close() error                     { return nil }
func (m *memStream) SetReadDeadline(time.Time) error  { return nil }
func (m *memStream) SetWriteDeadline(time.Time) error { return nil }
func (m *memStream) Local() bool                      { return true }

func TestExpandStarsWInsertsHandle(t *testing.T) {
	line := &cfgrec.LineCfg{RaNodeNr: 2, Baud: 38400, AnsiOn: true}
	f := expandStars(nil, nil, line, nil, `c:\doors\lord.exe -H*W -XT`, "204")
	if !f.inherit || !f.dupOnCmd {
		t.Fatalf("flags inherit=%v dupOnCmd=%v", f.inherit, f.dupOnCmd)
	}
	if !strings.Contains(f.cmd, "-H204") {
		t.Fatalf("cmd %q should contain -H204", f.cmd)
	}
	if strings.Contains(f.cmd, "*W") {
		t.Fatalf("cmd still has *W: %q", f.cmd)
	}
	if !strings.Contains(f.cmd, "-XT") {
		t.Fatalf("door flag -XT dropped: %q", f.cmd)
	}
}

func TestExpandStarsMStaysInMemory(t *testing.T) {
	line := &cfgrec.LineCfg{RaNodeNr: 2, Baud: 38400}
	f := expandStars(nil, nil, line, nil, `c:\doors\door.exe *M *N`, "")
	if !f.memorySwap {
		t.Fatal("*M must set memorySwap")
	}
	if strings.Contains(f.cmd, "*M") {
		t.Fatalf("*M leaked onto door command: %q", f.cmd)
	}
	if !strings.Contains(f.cmd, " 2") && !strings.HasSuffix(f.cmd, "2") {
		t.Fatalf("*N dropped: %q", f.cmd)
	}
}

func TestExpandStarsYDoesNotInsertHandle(t *testing.T) {
	line := &cfgrec.LineCfg{RaNodeNr: 2, Baud: 38400}
	f := expandStars(nil, nil, line, nil, `c:\doors\lord.exe *Y`, "204")
	if !f.inherit || !f.noBatches {
		t.Fatalf("flags %+v", f)
	}
	if strings.Contains(f.cmd, "204") {
		t.Fatalf("*Y must not insert handle: %q", f.cmd)
	}
}

func TestWriteDropFilesDoor32Socket(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.SystemName = "TestBBS"
	line := &cfgrec.LineCfg{
		RaNodeNr:   3,
		AnsiOn:     true,
		Baud:       65529,
		TelnetServ: true,
		TimeLimit:  60,
		Modem:      cfgrec.Modem{ComPort: 1},
		User:       cfgrec.User{Name: "Sysop", Handle: "Sys", Security: 100, Record: 0},
	}
	if err := WriteDropFiles(g, line, nil, dir, starFlags{baud: 65529}, 204); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "door32.sys"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "2\r\n") {
		t.Fatalf("comtype want 2, got %q", s)
	}
	if !strings.Contains(s, "\r\n204\r\n") {
		t.Fatalf("socket handle missing: %q", s)
	}
	ds, err := os.ReadFile(filepath.Join(dir, "door.sys"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ds), "COM1:") {
		t.Fatalf("telnet DOOR.SYS want COM1:, got %q", ds)
	}
}

func TestRunWritesDropFiles(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{
		RaNodeNr:   1,
		AnsiOn:     true,
		LocalLogon: true,
		User:       cfgrec.User{Name: "Sysop", Record: 0},
	}
	tio := term.New(&memStream{}, g, line)
	Run(tio, g, line, nil, `cmd.exe /c exit 0`, false)
	for _, name := range []string{"door32.sys", "door.sys", "dorinfo1.def", "exitinfo.bbs"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestEnterNodeDirIsCwd(t *testing.T) {
	root := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{
		RaNodeNr: 2,
		Telnet:   cfgrec.TelnetCfg{NodeDirectories: filepath.Join(root, "NODE*N")},
	}
	want := filepath.Join(root, "NODE2")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := EnterNodeDir(g, line); err != nil {
		t.Fatal(err)
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("cwd %q want %q", got, want)
	}
}

func TestTmpBatchRunsNetfossCommand(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{RaNodeNr: 4}
	nf := filepath.Join(dir, "NF.BAT")
	if err := os.WriteFile(nf, []byte("netfoss.com\r\nnetcom.exe %1\r\nnetfoss.com /u\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bat, err := writeTmpBatch(g, line, starFlags{closeHandle: true}, nf, `c:\doors\lord\start.bat 4`)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bat)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, nf) || !strings.Contains(s, `start.bat 4`) {
		t.Fatalf("tmp batch missing door command: %q", s)
	}
	if strings.Contains(strings.ToLower(s), "cd /d") {
		t.Fatalf("pascal TMP.BAT does not cd: %q", s)
	}
	if strings.Contains(strings.ToLower(s), "pause") {
		t.Fatalf("pascal TMP.BAT does not pause: %q", s)
	}
	if strings.Contains(s, "SFOS.BAT") && !strings.Contains(s, "@echo off") {
		t.Fatalf("unexpected: %q", s)
	}
	exe, rest := viaComspec(bat, "")
	if !strings.EqualFold(filepath.Base(exe), "cmd.exe") && !strings.Contains(strings.ToLower(exe), "cmd") {
		t.Fatalf("comspec %q", exe)
	}
	if !strings.Contains(rest, "/C") || !strings.Contains(rest, filepath.Base(bat)) {
		t.Fatalf("rest %q", rest)
	}
	if filepath.Dir(bat) != dir {
		t.Fatalf("TMP.BAT should sit with door32.sys, got %q", bat)
	}
	if !strings.Contains(strings.ToLower(s), "call ") {
		t.Fatalf("TMP.BAT must call the door so cmd /C exits: %q", s)
	}
}

func TestTmpBatchLivesInNodeDir(t *testing.T) {
	root := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = root
	line := &cfgrec.LineCfg{
		RaNodeNr: 2,
		Telnet:   cfgrec.TelnetCfg{NodeDirectories: filepath.Join(root, "NODE*N")},
	}
	bat, err := writeTmpBatch(g, line, starFlags{}, `c:\ele\nf\nf.bat`, `c:\ele\kiwi\kiwi.bat 2`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "NODE2")
	if filepath.Dir(bat) != want {
		t.Fatalf("TMP.BAT in %q, want node dir %q", filepath.Dir(bat), want)
	}
}

func TestViaComspecWrapsBatch(t *testing.T) {
	exe, rest := viaComspec(`c:\ele\nf.bat`, `c:\ele\doors\lord\start.bat 1`)
	if !strings.Contains(strings.ToLower(exe), "cmd") {
		t.Fatalf("exe %q", exe)
	}
	if !strings.HasPrefix(rest, "/C ") || !strings.Contains(rest, `nf.bat`) || !strings.Contains(rest, `start.bat`) {
		t.Fatalf("rest %q", rest)
	}
}
