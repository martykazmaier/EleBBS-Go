package files

import (
	"os"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

// Found is a file selected for download.
type Found struct {
	Area cfgrec.FilesArea
	Hdr  cfgrec.FilesHdr
	Path string
}

func DiskPath(area cfgrec.FilesArea, name string) string {
	return pascal.ForceBack(area.FilePath) + name
}

func listedForDownload(h cfgrec.FilesHdr, anyFile bool) bool {
	if h.Deleted() || h.Comment() {
		return false
	}
	if anyFile {
		return true
	}
	if h.Unlisted() || h.Missing() || h.NotAvail() || h.Locked() {
		return false
	}
	return true
}

func baseName(spec string) string {
	spec = strings.TrimSpace(spec)
	if i := strings.LastIndexAny(spec, `/\`); i >= 0 {
		return spec[i+1:]
	}
	return spec
}

// SearchNameInArea matches FDB names in area (Pascal SearchNameInArea).
func SearchNameInArea(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, spec string, anyFile bool) []Found {
	spec = strings.TrimSpace(spec)
	if spec == "" || area.Name == "" && area.AreaNum == 0 && area.FilePath == "" {
		return nil
	}
	base := baseName(spec)
	var out []Found
	ents, err := ReadFDB(g, area)
	if err == nil {
		for _, e := range ents {
			if !listedForDownload(e.Hdr, anyFile) {
				continue
			}
			if !MatchName(base, e.Hdr.Name) {
				continue
			}
			p := DiskPath(area, e.Hdr.Name)
			if _, err := os.Stat(p); err != nil {
				continue
			}
			out = append(out, Found{Area: area, Hdr: e.Hdr, Path: p})
		}
	}
	if len(out) == 0 && anyFile && !strings.ContainsAny(base, "*?") {
		p := DiskPath(area, base)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			h := cfgrec.FilesHdr{Name: base, Size: uint32(st.Size())}
			out = append(out, Found{Area: area, Hdr: h, Path: p})
		}
	}
	return out
}

func BumpTimesDL(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, rec uint16) {
	ents, err := ReadFDB(g, area)
	if err != nil || int(rec) >= len(ents) {
		return
	}
	ents[rec].Hdr.TimesDL++
	_ = WriteFDB(g, area, ents)
}
