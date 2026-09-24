package files

import (
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

const TagListName = "taglist.ra"

// TagFile is Pascal TagFileRecord in taglist.ra (50 bytes).
type TagFile struct {
	Name       string
	Password   string
	Attrib     byte
	AreaNum    uint16
	RecordNum  uint16
	Size       int32
	FileDate   int32
	Cost       int32
	CDROM      bool
	FoundFirst bool
	XferTime   uint16
}

func (t TagFile) Free() bool { return t.Attrib&(1<<2) != 0 }

func ParseTagFile(b []byte) TagFile {
	r := pascal.NewBuf(b)
	return TagFile{
		Name:       r.PString(12),
		Password:   r.PString(15),
		Attrib:     r.U8(),
		AreaNum:    r.U16(),
		RecordNum:  r.U16(),
		Size:       r.I32(),
		FileDate:   r.I32(),
		Cost:       r.I32(),
		CDROM:      r.Bool(),
		FoundFirst: r.Bool(),
		XferTime:   r.U16(),
	}
}

func EncodeTagFile(t TagFile) []byte {
	w := &pascal.Writer{}
	w.PString(12, t.Name)
	w.PString(15, t.Password)
	w.U8(t.Attrib)
	w.U16(t.AreaNum)
	w.U16(t.RecordNum)
	w.I32(t.Size)
	w.I32(t.FileDate)
	w.I32(t.Cost)
	w.Bool(t.CDROM)
	w.Bool(t.FoundFirst)
	w.U16(t.XferTime)
	if len(w.B) < cfgrec.TagRecSize {
		w.Pad(cfgrec.TagRecSize - len(w.B))
	}
	if len(w.B) > cfgrec.TagRecSize {
		return w.B[:cfgrec.TagRecSize]
	}
	return w.B
}

func LoadTagList(dir string) []TagFile {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	var raw []byte
	var err error
	for _, n := range []string{TagListName, "TAGLIST.RA"} {
		raw, err = os.ReadFile(filepath.Join(dir, n))
		if err == nil && len(raw) > 0 {
			break
		}
		raw = nil
	}
	if len(raw) < cfgrec.TagRecSize {
		return nil
	}
	n := len(raw) / cfgrec.TagRecSize
	out := make([]TagFile, 0, n)
	for i := 0; i < n; i++ {
		t := ParseTagFile(raw[i*cfgrec.TagRecSize : (i+1)*cfgrec.TagRecSize])
		if strings.TrimSpace(t.Name) == "" && t.AreaNum == 0 {
			continue
		}
		out = append(out, t)
	}
	return out
}

func DisplayName(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, t TagFile) string {
	name := strings.TrimSpace(t.Name)
	if t.RecordNum == 0 {
		return name
	}
	a, ok := FindArea(areas, t.AreaNum)
	if !ok || g == nil {
		return name
	}
	ents, err := ReadFDB(g, a)
	if err != nil {
		return name
	}
	idx := int(t.RecordNum) - 1
	if idx < 0 || idx >= len(ents) {
		return name
	}
	if n := LongName(g, a, ents[idx].Hdr); n != "" {
		return n
	}
	return name
}

const MaxTagged = 100

func TagListFile(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	p := filepath.Join(dir, TagListName)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	up := filepath.Join(dir, "TAGLIST.RA")
	if _, err := os.Stat(up); err == nil {
		return up
	}
	return p
}

// ClearTagList is Pascal tTagFileObj.ClearTagList: erase taglist.ra.
func ClearTagList(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	for _, n := range []string{TagListName, "TAGLIST.RA"} {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

func SaveTagList(dir string, tags []TagFile) error {
	if len(tags) == 0 {
		ClearTagList(dir)
		return nil
	}
	p := TagListFile(dir)
	raw := make([]byte, 0, len(tags)*cfgrec.TagRecSize)
	for _, t := range tags {
		raw = append(raw, EncodeTagFile(t)...)
	}
	return os.WriteFile(p, raw, 0644)
}

func IsTagged(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, tags []TagFile, name string, areaNum uint16) bool {
	want := pascal.UpCase(strings.TrimSpace(name))
	if want == "" {
		return false
	}
	for _, t := range tags {
		if t.AreaNum != areaNum {
			continue
		}
		if pascal.UpCase(DisplayName(g, areas, t)) == want || pascal.UpCase(t.Name) == want {
			return true
		}
	}
	return false
}

func DeleteFromTagged(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, tags []TagFile, fname string) []TagFile {
	want := pascal.UpCase(strings.TrimSpace(fname))
	if want == "" {
		return tags
	}
	wild := strings.ContainsAny(want, "*?")
	out := make([]TagFile, 0, len(tags))
	removed := false
	for _, t := range tags {
		n := pascal.UpCase(DisplayName(g, areas, t))
		hit := false
		if !removed {
			if wild {
				hit = MatchName(want, n) || MatchName(want, pascal.UpCase(t.Name))
			} else {
				hit = n == want || pascal.UpCase(t.Name) == want
			}
		}
		if hit {
			removed = true
			continue
		}
		out = append(out, t)
	}
	return out
}

func tagDiskNames(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, t TagFile) []string {
	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		k := strings.ToLower(s)
		if seen[k] {
			return
		}
		seen[k] = true
		names = append(names, s)
	}
	add(DisplayName(g, []cfgrec.FilesArea{area}, t))
	add(t.Name)
	if t.RecordNum > 0 && g != nil {
		ents, err := ReadFDB(g, area)
		idx := int(t.RecordNum) - 1
		if err == nil && idx >= 0 && idx < len(ents) {
			add(ents[idx].Hdr.Name)
			add(LongName(g, area, ents[idx].Hdr))
		}
	}
	return names
}

func FoundFromTags(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, tags []TagFile) []Found {
	var out []Found
	seen := map[string]bool{}
	for _, t := range tags {
		a, ok := FindArea(areas, t.AreaNum)
		if !ok {
			continue
		}
		var f Found
		for _, name := range tagDiskNames(g, a, t) {
			p := FileOnDiskSize(g, a, name, t.Size)
			if p == "" {
				p = FileOnDisk(g, a, name)
			}
			if p == "" {
				continue
			}
			st, err := os.Stat(p)
			if err != nil || st.IsDir() {
				continue
			}
			h := cfgrec.FilesHdr{Name: filepath.Base(p), Size: uint32(st.Size())}
			if t.Size > 0 {
				h.Size = uint32(t.Size)
			}
			if t.RecordNum > 0 {
				h.RecordNum = t.RecordNum - 1
			}
			f = Found{Area: a, Hdr: h, Path: p}
			break
		}
		if f.Path == "" {
			continue
		}
		key := pascal.UpCase(f.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}
