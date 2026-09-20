package files

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestMatchName(t *testing.T) {
	if !MatchName("*.ZIP", "GAME.ZIP") {
		t.Fatal("wildcard")
	}
	if MatchName("A.ZIP", "B.ZIP") {
		t.Fatal("mismatch")
	}
}

func TestSearchNameInArea(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "dl")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "GAME.ZIP"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "filebase")
	area := cfgrec.FilesArea{AreaNum: 1, Name: "Games", FilePath: filesDir}
	if err := WriteFDB(g, area, []Entry{{Hdr: cfgrec.FilesHdr{Name: "GAME.ZIP", Size: 1}}}); err != nil {
		t.Fatal(err)
	}
	got := SearchNameInArea(g, area, "game.zip", false)
	if len(got) != 1 || got[0].Hdr.Name != "GAME.ZIP" {
		t.Fatalf("got %+v", got)
	}
	if SearchNameInArea(g, area, "NOPE.ZIP", false) != nil {
		t.Fatal("missing file matched")
	}
}

func TestIncludeArea(t *testing.T) {
	a := struct {
		AreaNum uint16
		Name    string
		Group   uint16
	}{10, "Games", 2}
	_ = a
}
