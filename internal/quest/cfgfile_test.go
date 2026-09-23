package quest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func TestCfgGetConfigSemPath(t *testing.T) {
	dir := t.TempDir()
	sem := filepath.Join(dir, "sem")
	if err := os.Mkdir(sem, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.SemPath = pascal.ForceBack(sem)
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	script := "" +
		"Cfg_FileOpen 1 config.ra CONFIG\r\n" +
		"Cfg_FileRead 1\r\n" +
		"Cfg_FileGetInfo 1 189 30\r\n" +
		"If 30 = \"\"\r\n" +
		"  Cfg_FileGetInfo 1 31 30\r\n" +
		"EndIf\r\n" +
		"Assign 1 node\r\n" +
		"Concat 42 30 1\r\n" +
		"Assign 34 2\r\n" +
		"Concat 42 42 34\r\n" +
		"Assign 1 .ra\r\n" +
		"Concat 42 42 1\r\n" +
		"Filedelete #42\r\n" +
		"Fileopen 1 #42\r\n" +
		"Filewrite 1 hello-sem\r\n" +
		"Fileclose 1\r\n" +
		"Assign 31 userdoes.\r\n" +
		"Concat 31 30 31\r\n" +
		"Assign 32 2\r\n" +
		"Concat 32 31 32\r\n" +
		"Fileopen 1 #32\r\n" +
		"Filewrite 1 Seeing Who's Online\r\n" +
		"Fileclose 1\r\n" +
		"QUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "whosend.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 1}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	Run(tio, g, line, "whosend", "")
	got := online.FindNodeFile(g, 2)
	if got == "" {
		t.Fatal("NODE2.RA not in SemPath")
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hello-sem") {
		t.Fatalf("NODE.RA %q body %q", got, b)
	}
	if !strings.EqualFold(filepath.Dir(got), sem) {
		t.Fatalf("NODE.RA dir %q want SemPath %q", filepath.Dir(got), sem)
	}
	ud := filepath.Join(sem, "userdoes.2")
	ub, err := os.ReadFile(ud)
	if err != nil {
		t.Fatalf("userdoes.2 missing in SemPath: %v", err)
	}
	if !strings.Contains(string(ub), "Seeing Who's Online") {
		t.Fatalf("userdoes.2 %q", ub)
	}
}

func TestSetUserOnWritesUseron(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	script := "SetuserOn Who Online\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "seton.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{
		AnsiOn:   true,
		RaNodeNr: 2,
		User:     cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", Location: "Calgary"},
	}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	Run(tio, g, line, "seton", "")
	rec, ok := online.ReadSlot(g, 2)
	if !ok {
		t.Fatal("USERON.BBS slot 2 missing")
	}
	if rec.Name != "Martin Kazmaier" {
		t.Fatalf("name %q", rec.Name)
	}
	if rec.StatDesc != "Who Online" {
		t.Fatalf("StatDesc %q", rec.StatDesc)
	}
}
