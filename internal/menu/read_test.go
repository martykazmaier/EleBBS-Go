package menu

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/mail"
	"elebbs/internal/term"
)

func jamArea(t *testing.T, dir, name string, bodies ...string) cfgrec.MessageArea {
	t.Helper()
	base := filepath.Join(dir, name)
	for i, body := range bodies {
		_, err := mail.AppendMsg(base, mail.Article{
			From:    "Alice",
			To:      "All",
			Subject: body,
			Body:    body + " body",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	return cfgrec.MessageArea{
		AreaNum:   1,
		Name:      name,
		Attribute: 1 << 7,
		JAMBase:   base,
	}
}

func TestNewScanContinuesAfterReply(t *testing.T) {
	dir := t.TempDir()
	a := jamArea(t, dir, "scan", "First", "Second", "Third")
	// R reply, n AskChange, n Private, y AskQuote, empty line, S save, N next, N next
	st := &seqStream{in: []byte("Rnny\rSN\rN\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{User: cfgrec.User{Name: "Bob", Security: 100, Record: -1}}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	act := eng.doReadMail(a, 1, true, false, false, true)
	if act == mailStop {
		t.Fatal("scan stopped after reply")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("Second")) || !bytes.Contains(out, []byte("Third")) {
		t.Fatalf("remaining messages skipped after reply: %q", out)
	}
}

func TestReplyLoadsInternalFSED(t *testing.T) {
	dir := t.TempDir()
	a := jamArea(t, dir, "fsed", "Hello")
	if err := os.WriteFile(filepath.Join(dir, "edithdr.q-a"), []byte("DISPLAY \"EDITHDR-LOADED\"\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "editftr.q-a"), []byte("DISPLAY \"EDITFTR-LOADED\"\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("RnnHi\x1a\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.ExternalEd = "INTERNAL"
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{
		AnsiOn: true,
		User:   cfgrec.User{Name: "Bob", Security: 100, Attribute: cfgrec.UserFSEd, Record: -1},
	}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.doReadMail(a, 1, true, false, false, true)
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("EDITHDR-LOADED")) {
		t.Fatalf("internal FSED header not loaded: %q", out)
	}
	if bytes.Contains(out, []byte("Begin entering")) || bytes.Contains(bytes.ToLower(out), []byte("beginmsg")) {
		t.Fatalf("fell back to line editor: %q", out)
	}
}
