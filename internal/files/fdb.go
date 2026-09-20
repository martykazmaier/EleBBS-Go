package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/pascal"
)

func FileBase(g *cfgrec.GlobalCfg) string {
	fb := strings.TrimRight(g.RaConfig.FileBase, `\/`)
	if fb == "" {
		fb = filepath.Join(g.RaConfig.SysPath, "filebase")
	}
	return fb
}

func FDBPath(g *cfgrec.GlobalCfg, kind string, area uint16) string {
	kind = strings.ToLower(kind)
	return filepath.Join(FileBase(g), kind, fmt.Sprintf("fdb%d.%s", area, kind))
}

func hdrPath(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) string {
	p := FDBPath(g, "hdr", area.AreaNum)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	name := fmt.Sprintf("fdb%d.hdr", area.AreaNum)
	cands := []string{
		filepath.Join(FileBase(g), name),
		filepath.Join(area.FilePath, name),
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return p
}

func idxPath(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) string {
	return FDBPath(g, "idx", area.AreaNum)
}

func txtPath(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) string {
	return FDBPath(g, "txt", area.AreaNum)
}

func EnsureFDBDirs(g *cfgrec.GlobalCfg) error {
	fb := FileBase(g)
	for _, k := range []string{"hdr", "idx", "txt", "lfn"} {
		if err := os.MkdirAll(filepath.Join(fb, k), 0755); err != nil {
			return err
		}
	}
	return nil
}

type Entry struct {
	Hdr  cfgrec.FilesHdr
	Desc string
}

func ReadFDB(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) ([]Entry, error) {
	raw, err := os.ReadFile(hdrPath(g, area))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	txt, _ := os.ReadFile(txtPath(g, area))
	n := len(raw) / cfgrec.FilesHdrSize
	out := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		h := cfgrec.ParseFilesHdr(raw[i*cfgrec.FilesHdrSize:(i+1)*cfgrec.FilesHdrSize], uint16(i))
		e := Entry{Hdr: h}
		if h.LongDescPtr >= 0 && int(h.LongDescPtr) < len(txt) {
			e.Desc = descAt(txt, int(h.LongDescPtr))
		}
		out = append(out, e)
	}
	return out, nil
}

func descAt(txt []byte, off int) string {
	if off < 0 || off >= len(txt) {
		return ""
	}
	end := off
	for end < len(txt) && txt[end] != 0 && txt[end] != 0x1A {
		end++
	}
	s := pascal.FromCP437(txt[off:end])
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, "\n")
}

func WriteFDB(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, entries []Entry) error {
	if err := EnsureFDBDirs(g); err != nil {
		return err
	}
	var hdr, idx, txt []byte
	for i, e := range entries {
		h := e.Hdr
		h.RecordNum = uint16(i)
		if strings.TrimSpace(e.Desc) != "" {
			h.LongDescPtr = int32(len(txt))
			txt = append(txt, pascal.ToCP437(strings.ReplaceAll(e.Desc, "\n", "\r\n"))...)
			txt = append(txt, 0)
		} else if h.LongDescPtr > 0 && e.Desc == "" {
			h.LongDescPtr = -1
		}
		if h.NameRaw[0] == 0 && h.Name != "" {
			pascal.PutString(h.NameRaw[:], h.Name)
		}
		ix := cfgrec.HeaderToIdx(h)
		for k := 0; k < 5; k++ {
			ix.KeywordCRC[k] = crc.RA(h.Keywords[k], true)
		}
		hdr = append(hdr, cfgrec.EncodeFilesHdr(h)...)
		idx = append(idx, cfgrec.EncodeFilesIdx(ix)...)
	}
	if err := os.WriteFile(hdrPath(g, area), hdr, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(idxPath(g, area), idx, 0644); err != nil {
		return err
	}
	return os.WriteFile(txtPath(g, area), txt, 0644)
}

func RebuildIndex(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) error {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return err
	}
	return WriteFDB(g, area, ents)
}

func Compress(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) (removed int, err error) {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	keep := make([]Entry, 0, len(ents))
	for _, e := range ents {
		if e.Hdr.Deleted() {
			removed++
			continue
		}
		keep = append(keep, e)
	}
	return removed, WriteFDB(g, area, keep)
}

func DiskFile(area cfgrec.FilesArea, name string) string {
	return filepath.Join(strings.TrimRight(area.FilePath, `\/`), name)
}

func PackDOSTime(t time.Time) uint32 {
	y := t.Year()
	if y < 1980 {
		y = 1980
	}
	dos := uint32(y-1980)<<25 |
		uint32(t.Month())<<21 |
		uint32(t.Day())<<16 |
		uint32(t.Hour())<<11 |
		uint32(t.Minute())<<5 |
		uint32(t.Second()/2)
	return dos
}

func UnpackDOSTime(v uint32) time.Time {
	if v == 0 {
		return time.Time{}
	}
	y := int((v>>25)&0x7F) + 1980
	m := time.Month((v >> 21) & 0x0F)
	d := int((v >> 16) & 0x1F)
	hh := int((v >> 11) & 0x1F)
	mm := int((v >> 5) & 0x3F)
	ss := int(v&0x1F) * 2
	return time.Date(y, m, d, hh, mm, ss, 0, time.Local)
}

func DaysAgo(v uint32) int {
	t := UnpackDOSTime(v)
	if t.IsZero() {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}

func MatchName(pat, name string) bool {
	pat = strings.ToUpper(pat)
	name = strings.ToUpper(name)
	if pat == "" || pat == "*" || pat == "*.*" {
		return true
	}
	ok, err := filepath.Match(pat, name)
	return err == nil && ok
}

func IncludeArea(area cfgrec.FilesArea, spec string) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "0" {
		return area.AreaNum != 0 && area.Name != ""
	}
	if spec[0] == '@' {
		return false // handled by caller
	}
	up := strings.ToUpper(spec)
	if strings.HasPrefix(up, "G") {
		rest := spec[1:]
		a, b := parseRange(rest)
		if b == 0 {
			b = a
		}
		gs := []uint16{area.Group, area.AltGroup[0], area.AltGroup[1], area.AltGroup[2]}
		for _, g := range gs {
			if g >= a && g <= b {
				return true
			}
		}
		return false
	}
	a, b := parseRange(spec)
	if b == 0 {
		b = a
	}
	return area.AreaNum >= a && area.AreaNum <= b
}

func parseRange(s string) (uint16, uint16) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '-'); i >= 0 {
		return u16atoi(s[:i]), u16atoi(s[i+1:])
	}
	n := u16atoi(s)
	return n, n
}

func u16atoi(s string) uint16 {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return uint16(n)
}

func LoadSpecFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		line = strings.ReplaceAll(line, ",", " ")
		out = append(out, strings.Fields(line)...)
	}
	return out, nil
}

func SelectAreas(areas []cfgrec.FilesArea, spec string) []cfgrec.FilesArea {
	spec = strings.TrimSpace(spec)
	if spec != "" && spec[0] == '@' {
		items, err := LoadSpecFile(spec[1:])
		if err != nil {
			return nil
		}
		var out []cfgrec.FilesArea
		seen := map[uint16]bool{}
		for _, it := range items {
			for _, a := range SelectAreas(areas, it) {
				if seen[a.AreaNum] {
					continue
				}
				seen[a.AreaNum] = true
				out = append(out, a)
			}
		}
		return out
	}
	var out []cfgrec.FilesArea
	for _, a := range areas {
		if IncludeArea(a, spec) {
			out = append(out, a)
		}
	}
	return out
}
