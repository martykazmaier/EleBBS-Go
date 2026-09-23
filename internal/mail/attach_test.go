package mail

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func sampleMIME(filename, payload string) []byte {
	enc := base64.StdEncoding.EncodeToString([]byte(payload))
	return []byte("From: alice@example.com\r\n" +
		"To: \"Sysop\" <sysop@bbs.example>\r\n" +
		"Subject: with file\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"bound1\"\r\n" +
		"\r\n" +
		"--bound1\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"Hello body\r\n" +
		"--bound1\r\n" +
		"Content-Type: application/octet-stream; name=\"" + filename + "\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"" + filename + "\"\r\n" +
		"\r\n" +
		enc + "\r\n" +
		"--bound1--\r\n")
}

func TestParseMIMEExtractsPNGWithoutFilename(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	enc := base64.StdEncoding.EncodeToString(png)
	raw := []byte("From: a@b.c\r\nTo: sysop@bbs\r\nSubject: photo\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"x\"\r\n\r\n" +
		"--x\r\nContent-Type: text/plain\r\n\r\nSee photo\r\n" +
		"--x\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		enc + "\r\n--x--\r\n")
	_, files := parseMIME(raw)
	if len(files) != 1 || files[0].Name != "attach.png" {
		t.Fatalf("png part: %+v", files)
	}
	if string(files[0].Data) != string(png) {
		t.Fatalf("png bytes %q", files[0].Data)
	}
}

func TestParseMIMESniffsPNGFromTmpName(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	enc := base64.StdEncoding.EncodeToString(png)
	raw := []byte("From: a@b.c\r\nTo: sysop@bbs\r\nSubject: photo\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"x\"\r\n\r\n" +
		"--x\r\nContent-Type: application/octet-stream; name=\"ATT00001.tmp\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"ATT00001.tmp\"\r\n\r\n" +
		enc + "\r\n--x--\r\n")
	_, files := parseMIME(raw)
	if len(files) != 1 || files[0].Name != "attach.png" {
		t.Fatalf("tmp png: %+v", files)
	}
}

func TestSafeAttachDirRejectsSysPath(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.AttachPath = filepath.Join(dir, "attach")
	if err := os.WriteFile(filepath.Join(dir, "autoexec.bat"), []byte("rem"), 0644); err != nil {
		t.Fatal(err)
	}
	if isSafeAttachDir(g, dir) {
		t.Fatal("SysPath must not be used as attach folder")
	}
	if isSafeAttachDir(g, g.RaConfig.AttachPath) {
		t.Fatal("attach root must not be used as attach folder")
	}
	uniq := filepath.Join(g.RaConfig.AttachPath, "AT150405")
	if err := os.MkdirAll(uniq, 0755); err != nil {
		t.Fatal(err)
	}
	if !isSafeAttachDir(g, uniq) {
		t.Fatal("unique attach subdir should be allowed")
	}
}

func TestCreateTempDirIsDOS83(t *testing.T) {
	dir := t.TempDir()
	got := createTempDir(dir, 0)
	if got == "" {
		t.Fatal("createTempDir failed")
	}
	name := filepath.Base(got)
	if strings.Contains(name, ".") || len(name) != 8 || !looksLikeAttachTempDir(name) {
		t.Fatalf("not DOS 8.3 folder %q", name)
	}
}

func TestCreateAttachDirForceBack(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.AttachPath = dir
	got := CreateAttachDir(g, 1)
	if got == "" {
		t.Fatal("CreateAttachDir failed")
	}
	if !strings.HasSuffix(got, `\`) {
		t.Fatalf("Pascal ForceBack path required, got %q", got)
	}
	st, err := os.Stat(strings.TrimRight(got, `\`))
	if err != nil || !st.IsDir() {
		t.Fatalf("dir: %v", err)
	}
}

func TestAttachFilesSkipsProtocolJunk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("p"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dsz.ctl"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dsl.log"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dszlog"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	files := AttachFiles(dir)
	if len(files) != 1 || filepath.Base(files[0]) != "photo.png" {
		t.Fatalf("files %v", files)
	}
}

func TestProcessInboundAttachWritesAttachPath(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach")
	if err := os.Mkdir(att, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.AttachPath = pascal.ForceBack(att)
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	out, attachDir, has := processInboundAttach(g, 0, sampleMIME("note.txt", "Hello"))
	if !has || attachDir == "" {
		t.Fatalf("has=%v dir=%q", has, attachDir)
	}
	got, err := os.ReadFile(filepath.Join(strings.TrimRight(attachDir, `\/`), "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Hello" {
		t.Fatalf("saved %q", got)
	}
	if !strings.Contains(string(out), "Hello body") {
		t.Fatalf("body %q", out)
	}
}

func TestTossDeletesDszLogEnvFile(t *testing.T) {
	t.Setenv("DSZLOG", "dszlog")
	dir := t.TempDir()
	att := filepath.Join(dir, "attach")
	if err := os.Mkdir(att, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "dszlog"), []byte("log"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.AttachPath = pascal.ForceBack(att)
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	_, attachDir, has := processInboundAttach(g, 0, sampleMIME("note.txt", "Hello"))
	if !has {
		t.Fatal("expected attach")
	}
	if _, err := os.Stat(filepath.Join(att, "dszlog")); err == nil {
		t.Fatal("DSZLOG file still in attach root")
	}
	uniq := strings.TrimRight(attachDir, `\/`)
	if _, err := os.Stat(filepath.Join(uniq, "dszlog")); err == nil {
		t.Fatal("DSZLOG file still in unique attach dir")
	}
	if _, err := os.Stat(filepath.Join(uniq, "note.txt")); err != nil {
		t.Fatal("real attach deleted")
	}
}

func TestTossArticleSetsFAttach(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach")
	if err := os.Mkdir(att, 0755); err != nil {
		t.Fatal(err)
	}
	jam := filepath.Join(dir, "email")
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.AttachPath = pascal.ForceBack(att)
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	area := cfgrec.MessageArea{AreaNum: 1, Name: "Email", JAMBase: jam}
	art := NewsArticle{
		GroupName: "Sysop",
		AreaNum:   1,
		Attribute: ArtEmail,
		Body:      sampleMIME("note.txt", "Hello"),
	}
	if err := tossArticle(g, area, art, 0); err != nil {
		t.Fatal(err)
	}
	a, ok := ReadMsg(jam, 1)
	if !ok {
		t.Fatal("no tossed msg")
	}
	if !a.FAttach {
		t.Fatalf("FAttach not set: %+v", a)
	}
	if !isAttachDirSubject(a.Subject) {
		t.Fatalf("subject should be attach dir, got %q", a.Subject)
	}
	if !strings.Contains(a.Body, "Hello body") {
		t.Fatalf("body %q", a.Body)
	}
}

func TestProcessInboundAttachSavesUnnamedPNG(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach")
	if err := os.Mkdir(att, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.AttachPath = pascal.ForceBack(att)
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	enc := base64.StdEncoding.EncodeToString(png)
	raw := []byte("From: a@b.c\r\nTo: sysop@bbs\r\nSubject: photo\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"x\"\r\n\r\n" +
		"--x\r\nContent-Type: text/plain\r\n\r\nSee photo\r\n" +
		"--x\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		enc + "\r\n--x--\r\n")
	_, attachDir, has := processInboundAttach(g, 0, raw)
	if !has || attachDir == "" {
		t.Fatalf("has=%v dir=%q", has, attachDir)
	}
	if sameDir(attachDir, dir) || sameDir(attachDir, att) || sameDir(attachDir, g.RaConfig.SysPath) {
		t.Fatalf("must not dump into SysPath or attach root: %q", attachDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "attach.png")); err == nil {
		t.Fatal("png must not be written into SysPath")
	}
	got, err := os.ReadFile(filepath.Join(strings.TrimRight(attachDir, `\/`), "attach.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(png) {
		t.Fatalf("saved %q", got)
	}
}

func TestScanMsgAreaMIMEEncodesAttach(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	att := filepath.Join(dir, "attach", "AT150405")
	if err := os.MkdirAll(att, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "note.txt"), []byte("Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	jam := "email"
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.AttachPath = pascal.ForceBack(filepath.Join(dir, "attach"))
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.Address[0] = cfgrec.Addr{Zone: 1, Net: 2, Node: 3}
	area := cfgrec.MessageArea{AreaNum: 7, Name: "Email", JAMBase: jam, AkaAddress: 0}
	if err := writeMessagesRA(dir, area); err != nil {
		t.Fatal(err)
	}
	_, err := AppendMsg(jam, Article{
		From:    "Sysop",
		To:      "alice@example.com",
		Subject: att,
		Body:    "see attached\r\n",
		FAttach: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := EmailOutPath(g)
	if err := ScanMsgArea(g, 7, out, "bbs.example"); err != nil {
		t.Fatal(err)
	}
	arts, err := readNewsFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("out %d", len(arts))
	}
	raw := string(nulTrim(arts[0].Body))
	if !strings.Contains(raw, "multipart/mixed") {
		t.Fatalf("not MIME: %s", raw)
	}
	if !strings.Contains(raw, "filename=\"note.txt\"") && !strings.Contains(raw, "name=\"note.txt\"") {
		t.Fatalf("missing filename: %s", raw)
	}
	if !strings.Contains(raw, base64.StdEncoding.EncodeToString([]byte("Hello"))) {
		t.Fatalf("missing payload: %s", raw)
	}
	_, files := parseMIME([]byte(raw))
	if len(files) != 1 || string(files[0].Data) != "Hello" {
		t.Fatalf("roundtrip %+v", files)
	}
}

func writeMessagesRA(dir string, a cfgrec.MessageArea) error {
	w := &pascal.Writer{}
	w.U16(a.AreaNum)
	w.U16(0)
	w.PString(40, a.Name)
	w.U8(a.Typ)
	w.U8(a.MsgKinds)
	w.U8(a.Attribute)
	w.U8(a.DaysKill)
	w.U8(a.RecvKill)
	w.U16(a.CountKill)
	w.U16(a.ReadSecurity)
	w.Pad(8)
	w.U16(a.WriteSecurity)
	w.Pad(8)
	w.U16(a.SysopSecurity)
	w.Pad(8)
	w.PString(60, a.OriginLine)
	w.U8(a.AkaAddress)
	w.U8(a.Age)
	w.PString(60, a.JAMBase)
	w.U16(a.Group)
	w.U16(a.AltGroup[0])
	w.U16(a.AltGroup[1])
	w.U16(a.AltGroup[2])
	w.U8(a.Attribute2)
	w.U16(a.NetmailArea)
	buf := make([]byte, cfgrec.MessageSize)
	copy(buf, w.B)
	return os.WriteFile(filepath.Join(dir, "messages.ra"), buf, 0644)
}
