package files

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestAddToFDBAndDupeScan(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "GAME.ZIP"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "fb")
	area := cfgrec.FilesArea{AreaNum: 1, Name: "Games", FilePath: filesDir, Attrib: 1 << 1}
	if err := AddToFDB(g, area, "GAME.ZIP", "Bob", "fun game", 0); err != nil {
		t.Fatal(err)
	}
	if !NameInDupeScan(g, []cfgrec.FilesArea{area}, "game.zip") {
		t.Fatal("expected dupe hit")
	}
	areaOff := area
	areaOff.Attrib = 0
	if NameInDupeScan(g, []cfgrec.FilesArea{areaOff}, "game.zip") {
		t.Fatal("dupe scan should skip areas without bit 1")
	}
}

func TestBadFileName(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	if err := os.WriteFile(filepath.Join(dir, "badfiles.ctl"), []byte("VIRUS.ZIP\r\n*.COM\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !BadFileName(g, "virus.zip") {
		t.Fatal("exact bad name")
	}
	if !BadFileName(g, "x.com") {
		t.Fatal("wildcard bad name")
	}
	if BadFileName(g, "ok.zip") {
		t.Fatal("clean name rejected")
	}
}

func TestAreaUploadPathUnderSysPath(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "up")
	if err := os.MkdirAll(rel, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	area := cfgrec.FilesArea{FilePath: "up"}
	got := AreaUploadPath(g, area)
	if got != rel && filepath.Clean(got) != filepath.Clean(rel) {
		t.Fatalf("got %q want %q", got, rel)
	}
}
