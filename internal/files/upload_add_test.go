package files

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
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

func TestAddToFDBAppendsWithoutRewritingTxt(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "OLD.ZIP"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "NEW.ZIP"), []byte("newfile"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "fb")
	area := cfgrec.FilesArea{AreaNum: 7, Name: "Uploads", FilePath: filesDir}
	if err := EnsureFDBDirs(g); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("KEEP-DESC\x00LONGNAM.TXT\x00")
	if err := os.WriteFile(txtPath(g, area), sentinel, 0644); err != nil {
		t.Fatal(err)
	}
	old := cfgrec.FilesHdr{Name: "OLD.ZIP", Size: 3, CRC32: 0xFFFFFFFF, LongDescPtr: 0, LfnPtr: 10}
	pascal.PutString(old.NameRaw[:], old.Name)
	if err := os.WriteFile(hdrPath(g, area), cfgrec.EncodeFilesHdr(old), 0644); err != nil {
		t.Fatal(err)
	}
	hdrBefore, err := os.ReadFile(hdrPath(g, area))
	if err != nil {
		t.Fatal(err)
	}
	if err := AddToFDB(g, area, "NEW.ZIP", "Bob", "fresh", 0); err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(txtPath(g, area))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(txt, sentinel) {
		t.Fatalf("upload rewrote existing TXT: %q", txt)
	}
	hdr, err := os.ReadFile(hdrPath(g, area))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(hdr, hdrBefore) {
		t.Fatalf("upload rewrote existing HDR records")
	}
	if len(hdr) != 2*cfgrec.FilesHdrSize {
		t.Fatalf("hdr len %d want %d", len(hdr), 2*cfgrec.FilesHdrSize)
	}
	ents, err := ReadFDB(g, area)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 2 || ents[0].Hdr.Name != "OLD.ZIP" || ents[0].Hdr.LfnPtr != 10 {
		t.Fatalf("old record damaged: %+v", ents)
	}
	if ents[0].Desc != "KEEP-DESC" {
		t.Fatalf("old description %q", ents[0].Desc)
	}
	if ents[1].Hdr.Name != "NEW.ZIP" || ents[1].Desc != "fresh" {
		t.Fatalf("new record %+v", ents[1])
	}
}

func TestAfterUploadUpdatesExistingHdrInPlace(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filesDir, "GAME.ZIP")
	if err := os.WriteFile(path, []byte("ab"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "fb")
	area := cfgrec.FilesArea{AreaNum: 3, Name: "Games", FilePath: filesDir}
	if err := AddToFDB(g, area, "GAME.ZIP", "Ann", "game", 0); err != nil {
		t.Fatal(err)
	}
	txtBefore, err := os.ReadFile(txtPath(g, area))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AfterUpload(g, area, path, "Ann"); err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(txtPath(g, area))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(txt, txtBefore) {
		t.Fatalf("re-upload rewrote TXT: %q vs %q", txt, txtBefore)
	}
	ents, err := ReadFDB(g, area)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("duplicate hdr records: %d", len(ents))
	}
	if ents[0].Hdr.Size != 6 {
		t.Fatalf("size %d want 6", ents[0].Hdr.Size)
	}
	if ents[0].Desc != "game" {
		t.Fatalf("desc %q", ents[0].Desc)
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
