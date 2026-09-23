package menu

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/mail"
	"elebbs/internal/term"
)

func TestWriteMessageAsksAttachAndSaves(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("pic.png", []byte("png-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.AttachPath = filepath.Join(dir, "attach")
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	base := filepath.Join(dir, "email")
	a := cfgrec.MessageArea{
		AreaNum:   1,
		Name:      "Email",
		Typ:       cfgrec.MsgInternet,
		MsgKinds:  cfgrec.MsgKindPrivate,
		Attribute: (1 << 7) | (1 << 2),
		JAMBase:   base,
	}
	st := &seqStream{in: []byte("nHi\r\rSypic.png\r\r")}
	line := &cfgrec.LineCfg{
		User: cfgrec.User{Name: "Bob", Security: 100, Record: -1},
	}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.writeMessage(a, "Bob", "user@example.com", "hello", nil, false) {
		t.Fatalf("post failed: %q", st.out.Bytes())
	}
	art, ok := mail.ReadMsg(base, 1)
	if !ok {
		t.Fatal("no posted msg")
	}
	if !art.FAttach {
		t.Fatalf("FAttach not set: %+v\n%s", art, st.out.Bytes())
	}
	files := mail.AttachFiles(art.Subject)
	if len(files) != 1 || filepath.Base(files[0]) != "pic.png" {
		t.Fatalf("attach files %v subj %q\n%s", files, art.Subject, st.out.Bytes())
	}
	got, err := os.ReadFile(files[0])
	if err != nil || string(got) != "png-bytes" {
		t.Fatalf("saved %q err %v", got, err)
	}
}

func TestWriteMessageSkipsAttachWhenAreaDisallows(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.AttachPath = filepath.Join(dir, "attach")
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	base := filepath.Join(dir, "email")
	a := cfgrec.MessageArea{
		AreaNum:   1,
		Name:      "Email",
		Typ:       cfgrec.MsgInternet,
		MsgKinds:  cfgrec.MsgKindPrivate,
		Attribute: 1 << 7,
		JAMBase:   base,
	}
	st := &seqStream{in: []byte("nHi\r\rS")}
	line := &cfgrec.LineCfg{
		User: cfgrec.User{Name: "Bob", Security: 100, Record: -1},
	}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.writeMessage(a, "Bob", "user@example.com", "hello", nil, false) {
		t.Fatalf("post failed: %q", st.out.Bytes())
	}
	art, ok := mail.ReadMsg(base, 1)
	if !ok {
		t.Fatal("no posted msg")
	}
	if art.FAttach {
		t.Fatal("FAttach set on area without attach bit")
	}
	if art.Subject != "hello" {
		t.Fatalf("subject %q", art.Subject)
	}
}
