package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestEncodeTagFileSize(t *testing.T) {
	b := EncodeTagFile(TagFile{Name: "GAME.ZIP", AreaNum: 1, Size: 12345})
	if len(b) != cfgrec.TagRecSize {
		t.Fatalf("size %d want %d", len(b), cfgrec.TagRecSize)
	}
	got := ParseTagFile(b)
	if got.Name != "GAME.ZIP" || got.AreaNum != 1 || got.Size != 12345 {
		t.Fatalf("roundtrip %+v", got)
	}
}

func TestFoundFromTagsUsesLongNameUnderSysPath(t *testing.T) {
	root := t.TempDir()
	filesDir := filepath.Join(root, "games")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "My Game.zip"), []byte("zip"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = root
	g.RaConfig.FileBase = filepath.Join(root, "filebase")
	area := cfgrec.FilesArea{AreaNum: 2, Name: "Games", FilePath: "games"}
	if err := os.MkdirAll(filepath.Dir(FDBPath(g, "lfn", 2)), 0755); err != nil {
		t.Fatal(err)
	}
	lfn := append([]byte{0}, append([]byte("My Game.zip"), 0)...)
	if err := os.WriteFile(FDBPath(g, "lfn", 2), lfn, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hdrPath(g, area)), 0755); err != nil {
		t.Fatal(err)
	}
	h := cfgrec.FilesHdr{Name: "GAME.ZIP", Size: 3, LfnPtr: 1}
	if err := os.WriteFile(hdrPath(g, area), cfgrec.EncodeFilesHdr(h), 0644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	away := t.TempDir()
	if err := os.Chdir(away); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	got := FoundFromTags(g, []cfgrec.FilesArea{area}, []TagFile{{
		Name: "GAME.ZIP", AreaNum: 2, RecordNum: 1, Size: 3,
	}})
	if len(got) != 1 || !strings.EqualFold(filepath.Base(got[0].Path), "My Game.zip") {
		t.Fatalf("path %+v", got)
	}
}

func TestFoundFromTagsUsesTxtLFN(t *testing.T) {
	root := t.TempDir()
	filesDir := filepath.Join(root, "anime")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	lfn := "Armor Hunter Mellowlink - 01 - Wilderness.mkv"
	payload := []byte("video")
	if err := os.WriteFile(filepath.Join(filesDir, lfn), payload, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = root
	g.RaConfig.FileBase = filepath.Join(root, "filebase")
	area := cfgrec.FilesArea{AreaNum: 4, Name: "Anime", FilePath: filesDir}
	if err := os.MkdirAll(filepath.Dir(txtPath(g, area)), 0755); err != nil {
		t.Fatal(err)
	}
	txt := append([]byte{0}, append([]byte(lfn), 0)...)
	if err := os.WriteFile(txtPath(g, area), txt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hdrPath(g, area)), 0755); err != nil {
		t.Fatal(err)
	}
	h := cfgrec.FilesHdr{Name: "Armor Hunter", Size: uint32(len(payload)), LfnPtr: 1}
	if err := os.WriteFile(hdrPath(g, area), cfgrec.EncodeFilesHdr(h), 0644); err != nil {
		t.Fatal(err)
	}
	got := FoundFromTags(g, []cfgrec.FilesArea{area}, []TagFile{{
		Name: "Armor Hunter", AreaNum: 4, RecordNum: 1, Size: int32(len(payload)),
	}})
	if len(got) != 1 || !strings.EqualFold(filepath.Base(got[0].Path), lfn) {
		t.Fatalf("path %+v", got)
	}
}

func TestLoadTagListFromNodeDir(t *testing.T) {
	dir := t.TempDir()
	rec := EncodeTagFile(TagFile{Name: "GAME.ZIP", AreaNum: 3, Size: 4096})
	if err := os.WriteFile(filepath.Join(dir, "taglist.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	tags := LoadTagList(dir)
	if len(tags) != 1 || tags[0].Name != "GAME.ZIP" || tags[0].AreaNum != 3 {
		t.Fatalf("tags %+v", tags)
	}
}

func TestDeleteFromTaggedAndSave(t *testing.T) {
	dir := t.TempDir()
	tags := []TagFile{
		{Name: "GAME.ZIP", AreaNum: 1, Size: 1},
		{Name: "TOOL.ZIP", AreaNum: 1, Size: 2},
	}
	if err := SaveTagList(dir, tags); err != nil {
		t.Fatal(err)
	}
	got := DeleteFromTagged(nil, nil, LoadTagList(dir), "GAME.ZIP")
	if err := SaveTagList(dir, got); err != nil {
		t.Fatal(err)
	}
	left := LoadTagList(dir)
	if len(left) != 1 || left[0].Name != "TOOL.ZIP" {
		t.Fatalf("left %+v", left)
	}
	if err := SaveTagList(dir, nil); err != nil {
		t.Fatal(err)
	}
	if LoadTagList(dir) != nil {
		t.Fatal("clear")
	}
	if _, err := os.Stat(filepath.Join(dir, TagListName)); !os.IsNotExist(err) {
		t.Fatalf("taglist.ra should be erased: %v", err)
	}
}

func TestClearTagListErasesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, TagListName)
	if err := os.WriteFile(p, EncodeTagFile(TagFile{Name: "X.ZIP"}), 0644); err != nil {
		t.Fatal(err)
	}
	ClearTagList(dir)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("still present: %v", err)
	}
	if LoadTagList(dir) != nil {
		t.Fatal("load after clear")
	}
}
