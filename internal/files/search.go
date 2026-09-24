package files

import (
	"os"
	"path/filepath"
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

// FileOnDisk finds name in the area directory. Relative FilePath values are
// also tried under the system path, because the node directory is the
// process cwd while FILES.RA paths are relative to the BBS.
func FileOnDisk(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, name string) string {
	return FileOnDiskSize(g, area, name, 0)
}

func FileOnDiskSize(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, name string, size int32) string {
	name = strings.TrimSpace(baseName(name))
	if name == "" {
		return ""
	}
	for _, dir := range areaDirs(g, area) {
		if size > 0 {
			if p := matchFileSize(dir, name, size); p != "" {
				return p
			}
		}
		if p := matchFile(dir, name); p != "" {
			return p
		}
	}
	return ""
}

func areaDirs(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) []string {
	p := strings.TrimSpace(area.FilePath)
	if p == "" {
		return nil
	}
	var dirs []string
	add := func(d string) {
		d = strings.TrimSpace(d)
		d = strings.TrimRight(d, `\/`)
		if d == "" {
			return
		}
		for _, e := range dirs {
			if strings.EqualFold(e, d) {
				return
			}
		}
		dirs = append(dirs, d)
	}
	add(p)
	if g != nil && !filepath.IsAbs(p) && !strings.HasPrefix(p, `\\`) {
		if sys := strings.TrimSpace(g.RaConfig.SysPath); sys != "" {
			add(filepath.Join(sys, p))
		}
	}
	return dirs
}

func matchFile(dir, name string) string {
	p := pascal.ForceBack(dir) + name
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := strings.ToLower(name)
	var prefix []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.EqualFold(n, name) {
			return pascal.ForceBack(dir) + n
		}
		ln := strings.ToLower(n)
		if strings.HasPrefix(ln, want) && len(n) > len(name) {
			prefix = append(prefix, n)
		}
	}
	if len(prefix) == 1 {
		return pascal.ForceBack(dir) + prefix[0]
	}
	return ""
}

func matchFileSize(dir, name string, size int32) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := strings.ToLower(name)
	var sized, unique string
	nPrefix := 0
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		ln := strings.ToLower(n)
		if !strings.EqualFold(n, name) && !strings.HasPrefix(ln, want) {
			continue
		}
		p := pascal.ForceBack(dir) + n
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		nPrefix++
		unique = p
		if size > 0 && (st.Size() == int64(size) || uint32(st.Size()) == uint32(size)) {
			sized = p
		}
	}
	if sized != "" {
		return sized
	}
	if nPrefix == 1 {
		return unique
	}
	return ""
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
			if !MatchName(base, e.Hdr.Name) && !MatchName(base, LongName(g, area, e.Hdr)) {
				continue
			}
			p := FileOnDisk(g, area, LongName(g, area, e.Hdr))
			if p == "" {
				p = FileOnDisk(g, area, e.Hdr.Name)
			}
			if p == "" {
				continue
			}
			out = append(out, Found{Area: area, Hdr: e.Hdr, Path: p})
		}
	}
	if len(out) == 0 && anyFile && !strings.ContainsAny(base, "*?") {
		if p := FileOnDisk(g, area, base); p != "" {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				h := cfgrec.FilesHdr{Name: base, Size: uint32(st.Size())}
				out = append(out, Found{Area: area, Hdr: h, Path: p})
			}
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
