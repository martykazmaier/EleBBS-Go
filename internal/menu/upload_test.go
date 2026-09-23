package menu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func TestWriteUploadCtlEmptyDoesNotSetAttachCwd(t *testing.T) {
	dir := t.TempDir()
	node := filepath.Join(dir, "node")
	att := filepath.Join(dir, "attach", "AT120000")
	if err := os.MkdirAll(node, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(att, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{
		RaNodeNr: 1,
		Telnet:   cfgrec.TelnetCfg{NodeDirectories: node},
		User:     cfgrec.User{Name: "Bob", Record: -1},
	}
	tio := term.New(&seqStream{}, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	ctl := eng.writeUploadCtl(cfgrec.Protocol{CtlFileName: "DSZ.CTL"}, att)
	if ctl == "" {
		t.Fatal("no ctl")
	}
	if filepath.Dir(ctl) != node && filepath.Clean(filepath.Dir(ctl)) != filepath.Clean(node) {
		t.Fatalf("ctl %q should live in node dir %q", ctl, node)
	}
	raw, err := os.ReadFile(ctl)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "AT120000") || strings.Contains(pascal.UpCase(string(raw)), "ATTACH") {
		t.Fatalf("empty UpCtlString must not put attach path in DSZ.CTL: %q", raw)
	}
}

func TestTidyAttachUploadMovesPrefixedAndDropsJunk(t *testing.T) {
	dir := t.TempDir()
	att := filepath.Join(dir, "attach")
	uniq := filepath.Join(att, "AT150405")
	if err := os.MkdirAll(uniq, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "AT150405photo.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "dsz.ctl"), []byte("ctl"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(att, "dsl.log"), []byte("log"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uniq, "dszlog"), []byte("envlog"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uniq, "dsz.ctl"), []byte("ctl"), 0644); err != nil {
		t.Fatal(err)
	}
	tidyAttachUpload(uniq)
	if _, err := os.Stat(filepath.Join(att, "dsz.ctl")); err == nil {
		t.Fatal("dsz.ctl left in attach root")
	}
	if _, err := os.Stat(filepath.Join(att, "dsl.log")); err == nil {
		t.Fatal("dsl.log left in attach root")
	}
	if _, err := os.Stat(filepath.Join(uniq, "dsz.ctl")); err == nil {
		t.Fatal("dsz.ctl left in unique dir")
	}
	if _, err := os.Stat(filepath.Join(uniq, "dszlog")); err == nil {
		t.Fatal("dszlog left in unique dir")
	}
	got, err := os.ReadFile(filepath.Join(uniq, "photo.png"))
	if err != nil || string(got) != "png" {
		t.Fatalf("rescued file: %v %q", err, got)
	}
	files := mail.AttachFiles(uniq)
	if len(files) != 1 || filepath.Base(files[0]) != "photo.png" {
		t.Fatalf("unique dir files %v", files)
	}
}

func TestTidyDeletesDszLogEnvFileName(t *testing.T) {
	t.Setenv("DSZLOG", "dszlog")
	dir := t.TempDir()
	uniq := filepath.Join(dir, "AT120000")
	if err := os.MkdirAll(uniq, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uniq, "dszlog"), []byte("log"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uniq, "photo.png"), []byte("p"), 0644); err != nil {
		t.Fatal(err)
	}
	tidyAttachUpload(uniq)
	if _, err := os.Stat(filepath.Join(uniq, "dszlog")); err == nil {
		t.Fatal("DSZLOG file still in attach folder")
	}
	if _, err := os.Stat(filepath.Join(uniq, "photo.png")); err != nil {
		t.Fatal("real attach deleted")
	}
}
