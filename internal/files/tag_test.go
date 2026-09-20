package files

import (
	"os"
	"path/filepath"
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
}
