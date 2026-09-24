package menu

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestMsgBarPassesFileAttach(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach", "AT120000")
	if err := os.MkdirAll(att, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "photo.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "email")
	if _, err := mail.AppendMsg(base, mail.Article{
		From: "Alice", To: "Bob", Subject: att, Body: "see photo\r\n", FAttach: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "msgbar.q-a"), []byte(
		"GETPARAMETER 1 1\r\nDISPLAY 1\r\nSETRESULT N\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{
		AnsiOn: true,
		User:   cfgrec.User{Name: "Bob", Security: 100, Record: -1},
	}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	a := cfgrec.MessageArea{AreaNum: 1, Name: "Email", Attribute: 1 << 7, JAMBase: base}
	art, ok := mail.ReadMsg(base, 1)
	if !ok || !art.FAttach {
		t.Fatal("need FAttach msg")
	}
	eng.showMessage(a, art, false)
	out := bytes.ToUpper(st.out.Bytes())
	if !bytes.Contains(out, []byte("FILEATTACH")) {
		t.Fatalf("MSGBAR missing FILEATTACH: %q", st.out.Bytes())
	}
}

func TestAttachDownloadFromReader(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach", "AT120001")
	if err := os.MkdirAll(att, 0755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("png-bytes")
	if err := os.WriteFile(filepath.Join(att, "photo.png"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	base := filepath.Join(dir, "email")
	if _, err := mail.AppendMsg(base, mail.Article{
		From: "Alice", To: "All", Subject: att, Body: "see photo\r\n", FAttach: true,
	}); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("F\rgot\r\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{
		User: cfgrec.User{Name: "Bob", Security: 100, Record: -1},
	}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	a := cfgrec.MessageArea{AreaNum: 1, Name: "Email", Attribute: 1 << 7, JAMBase: base}
	art, ok := mail.ReadMsg(base, 1)
	if !ok {
		t.Fatal("no msg")
	}
	eng.showMessage(a, art, false)
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("[F]iles")) {
		t.Fatalf("no Files option: %q", out)
	}
	if !bytes.Contains(bytes.ToLower(out), []byte("photo.png")) {
		t.Fatalf("did not list attach: %q", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "got", "photo.png"))
	if err != nil {
		t.Fatalf("not downloaded: %v\n%s", err, out)
	}
	if string(got) != string(payload) {
		t.Fatalf("copied %q", got)
	}
}

func TestShowMessageAttachUsesLanguageStrings(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach", "AT120002")
	if err := os.MkdirAll(att, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "photo.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "email")
	when := time.Date(2026, 9, 20, 15, 4, 0, 0, time.Local)
	if _, err := mail.AppendMsg(base, mail.Article{
		From: "Alice", To: "Bob", Subject: att, Body: "see photo\r\n",
		FAttach: true, Private: true, Date: when,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rdmsg.q-a"), []byte(
		"DISPLAY 3\r\nDISPLAY \"|\"\r\nDISPLAY 5\r\nDISPLAY \"|\"\r\nDISPLAY 10\r\nSETRESULT N\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{
		AnsiOn: true,
		User:   cfgrec.User{Name: "Bob", Security: 100, Record: -1, DateFormat: 5},
	}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	a := cfgrec.MessageArea{AreaNum: 1, Name: "Email", Attribute: 1 << 7, JAMBase: base}
	art, ok := mail.ReadMsg(base, 1)
	if !ok || !art.FAttach {
		t.Fatal("need FAttach msg")
	}
	eng.showMessage(a, art, false)
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("Files attached")) {
		t.Fatalf("RDMSG #10 want AttFiles2: %q", out)
	}
	if bytes.Contains(out, []byte("AT120002")) {
		t.Fatalf("raw attach path in RDMSG: %q", out)
	}
	if !bytes.Contains(out, []byte("File attach")) {
		t.Fatalf("RDMSG #5 want FileAtt1: %q", out)
	}
	if !bytes.Contains(out, []byte("20-09-2026")) {
		t.Fatalf("RDMSG #3 want user date format: %q", out)
	}
}

func TestShowMessagePagesSoftCRBody(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "soft")
	var b strings.Builder
	for i := 0; i < 40; i++ {
		if i > 0 {
			b.WriteByte(mail.SoftCR)
		}
		b.WriteString("word")
		b.WriteByte(' ')
		b.WriteString(strings.Repeat("x", 10))
	}
	if _, err := mail.AppendMsg(base, mail.Article{
		From: "Alice", To: "Bob", Subject: "soft", Body: b.String(),
	}); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: bytes.Repeat([]byte("Y"), 80)}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{
		AnsiOn:         true,
		DispMorePrompt: true,
		User:           cfgrec.User{Name: "Bob", Security: 100, Record: -1, ScreenLength: 10},
	}
	tio := term.New(st, g, line)
	tio.MorePrompt = true
	tio.Length = 10
	eng := &Engine{T: tio, G: g, Line: line}
	a := cfgrec.MessageArea{AreaNum: 1, Name: "Soft", Attribute: 1 << 7, JAMBase: base}
	art, ok := mail.ReadMsg(base, 1)
	if !ok {
		t.Fatal("read")
	}
	eng.showMessage(a, art, true)
	out := st.out.String()
	if !strings.Contains(out, "More") && !strings.Contains(out, "more") {
		t.Fatalf("expected page pause for soft-CR body: %q", out)
	}
}

func TestBuildQuoteLinesWrapsSoftCR(t *testing.T) {
	body := "First line of text that continues" + string(mail.SoftCR) + "without a hard return and needs wrapping for quotes."
	q := buildQuoteLines(nil, "Bob", "Alice", "01-02-06 15:04", body)
	if len(q) < 3 {
		t.Fatalf("expected multiple quote lines, got %#v", q)
	}
	joined := strings.Join(q, "\n")
	if strings.Contains(joined, string(mail.SoftCR)) {
		t.Fatalf("soft CR in quote: %#v", q)
	}
	if !strings.Contains(joined, "Alice") || !strings.Contains(joined, "First line") {
		t.Fatalf("missing content: %#v", q)
	}
	for _, ln := range q[2:] {
		if len(ln) > 78 {
			t.Fatalf("overlong quote line %q (%d)", ln, len(ln))
		}
	}
}
