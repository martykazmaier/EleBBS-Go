package files

import (
	"fmt"
	"os"
	"path/filepath"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func AreasPath(g *cfgrec.GlobalCfg) string {
	p := filepath.Join(g.RaConfig.SysPath, "files.ra")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(g.RaConfig.SysPath, "FILES.RA")
}

func LoadAreas(g *cfgrec.GlobalCfg) []cfgrec.FilesArea {
	b, err := os.ReadFile(AreasPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.FilesRecSize
	out := make([]cfgrec.FilesArea, 0, n)
	for i := 0; i < n; i++ {
		a := cfgrec.ParseFilesArea(b[i*cfgrec.FilesRecSize : (i+1)*cfgrec.FilesRecSize])
		if a.Name == "" && a.AreaNum == 0 {
			continue
		}
		out = append(out, a)
	}
	return out
}

func LoadAll(g *cfgrec.GlobalCfg) []cfgrec.FilesArea {
	b, err := os.ReadFile(AreasPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.FilesRecSize
	out := make([]cfgrec.FilesArea, n)
	for i := 0; i < n; i++ {
		out[i] = cfgrec.ParseFilesArea(b[i*cfgrec.FilesRecSize : (i+1)*cfgrec.FilesRecSize])
	}
	return out
}

func SearchNext(all []cfgrec.FilesArea, group uint16, cur uint16) uint16 {
	for _, a := range all {
		if a.Name == "" || a.AreaNum == 0 {
			continue
		}
		if a.Group == group || a.Attrib2&1 != 0 {
			return a.AreaNum
		}
		for _, g := range a.AltGroup {
			if g == group {
				return a.AreaNum
			}
		}
	}
	return cur
}

func FileIndex(all []cfgrec.FilesArea, areaNum uint16) int {
	for i, a := range all {
		if a.AreaNum == areaNum {
			return i + 1
		}
	}
	return 0
}

func FindArea(areas []cfgrec.FilesArea, num uint16) (cfgrec.FilesArea, bool) {
	for _, a := range areas {
		if a.AreaNum == num {
			return a, true
		}
	}
	if num > 0 && int(num) <= len(areas) {
		return areas[num-1], true
	}
	return cfgrec.FilesArea{}, false
}

func ListArea(t *term.IO, g *cfgrec.GlobalCfg, area cfgrec.FilesArea) {
	t.WriteRA(fmt.Sprintf("`A15:File area %d: %s`A7:\r\n", area.AreaNum, area.Name))
	ents, err := ReadFDB(g, area)
	if err != nil {
		t.Println("No file list for this area.")
		return
	}
	shown := 0
	for _, e := range ents {
		if e.Hdr.Deleted() || e.Hdr.Unlisted() || e.Hdr.Comment() {
			continue
		}
		t.Println(fmt.Sprintf("  %-12s %8d  %s", e.Hdr.Name, e.Hdr.Size, e.Hdr.Uploader))
		shown++
	}
	if shown == 0 {
		t.Println("No files.")
	}
}

func SelectArea(t *term.IO, g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, cur uint16) uint16 {
	t.Println("")
	t.WriteRA("`A15:" + t.RalStr(lang.FileAreas) + "`A7:")
	t.Println("")
	for _, a := range areas {
		if a.Name == "" {
			continue
		}
		mark := " "
		if a.AreaNum == cur {
			mark = ">"
		}
		t.Println(fmt.Sprintf("%s %3d  %s", mark, a.AreaNum, a.Name))
	}
	t.WriteRA("`A15:" + t.RalStr(lang.SelArea))
	s, _ := t.GetString(5, false, false)
	s = pascal.Trim(s)
	if s == "" {
		return cur
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if _, ok := FindArea(areas, uint16(n)); ok {
		return uint16(n)
	}
	return cur
}
