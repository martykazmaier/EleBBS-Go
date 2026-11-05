package ftp

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
)

func TestFTPListAndRetrUseLongFileName(t *testing.T) {
	root := t.TempDir()
	filesDir := filepath.Join(root, "anime")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	lfn := "Armor Hunter Mellowlink - 01 - Wilderness.mkv"
	if err := os.WriteFile(filepath.Join(filesDir, lfn), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = root
	g.RaConfig.FileBase = filepath.Join(root, "filebase")
	area := cfgrec.FilesArea{AreaNum: 4, Name: "Anime", FilePath: filesDir}
	if err := os.MkdirAll(filepath.Dir(files.FDBPath(g, "txt", 4)), 0755); err != nil {
		t.Fatal(err)
	}
	txt := append([]byte{0}, append([]byte(lfn), 0)...)
	if err := os.WriteFile(files.FDBPath(g, "txt", 4), txt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(files.FDBPath(g, "hdr", 4)), 0755); err != nil {
		t.Fatal(err)
	}
	h := cfgrec.FilesHdr{Name: "Armor Hunter", Size: 5, LfnPtr: 1}
	if err := os.WriteFile(files.FDBPath(g, "hdr", 4), cfgrec.EncodeFilesHdr(h), 0644); err != nil {
		t.Fatal(err)
	}
	ss := &session{srv: &Server{cfg: Config{G: g}, areas: []cfgrec.FilesArea{area}}, group: 1, area: 4}
	shown := files.ConvDirName(files.LongName(g, area, h))
	if shown != files.ConvDirName(lfn) {
		t.Fatalf("list name %q want LFN, not 8.3 %q", shown, h.Name)
	}
	if len(h.Name) != 12 {
		t.Fatalf("fixture short name should be 12 chars, got %q", h.Name)
	}
	_, e, ok := ss.findFile(files.ConvDirName(lfn))
	if !ok || e.Hdr.Name != "Armor Hunter" {
		t.Fatalf("RETR of listed LFN failed: ok=%v hdr=%+v", ok, e.Hdr)
	}
	p := ss.diskPath(area, e)
	if filepath.Base(p) != lfn {
		t.Fatalf("disk path %q", p)
	}
}
