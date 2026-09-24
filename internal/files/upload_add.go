package files

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

// AreaUploadPath is the on-disk directory for area uploads (SysPath-relative FilePath).
func AreaUploadPath(g *cfgrec.GlobalCfg, area cfgrec.FilesArea) string {
	for _, d := range areaDirs(g, area) {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	dirs := areaDirs(g, area)
	if len(dirs) == 0 {
		return ""
	}
	return dirs[0]
}

// AreaDupeCheck is FILES.RA Attrib bit 1: include this area in upload dupe scans.
func AreaDupeCheck(a cfgrec.FilesArea) bool { return a.Attrib&(1<<1) != 0 }

// AreaLongDesc is FILES.RA Attrib bit 2: multi-line upload descriptions.
func AreaLongDesc(a cfgrec.FilesArea) bool { return a.Attrib&(1<<2) != 0 }

// NameInDupeScan is Pascal SearchIdxFast for upload duplicate detection.
func NameInDupeScan(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, name string) bool {
	want := pascal.UpCase(pascal.Trim(filepath.Base(name)))
	if want == "" {
		return false
	}
	for _, a := range areas {
		if a.Name == "" && a.AreaNum == 0 {
			continue
		}
		if !AreaDupeCheck(a) {
			continue
		}
		ents, err := ReadFDB(g, a)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.Hdr.Deleted() {
				continue
			}
			if pascal.UpCase(pascal.Trim(e.Hdr.Name)) == want {
				return true
			}
			if ln := LongName(g, a, e.Hdr); ln != "" && pascal.UpCase(pascal.Trim(ln)) == want {
				return true
			}
		}
	}
	return false
}

// BadFileName is Pascal SearchCtlFile('badfiles.ctl', ...).
func BadFileName(g *cfgrec.GlobalCfg, name string) bool {
	want := pascal.UpCase(pascal.Trim(filepath.Base(name)))
	if want == "" || g == nil {
		return false
	}
	for _, base := range []string{g.RaConfig.SysPath, g.RaConfig.TextPath, "."} {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		for _, fn := range []string{"badfiles.ctl", "BADFILES.CTL"} {
			raw, err := os.ReadFile(filepath.Join(base, fn))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(raw), "\n") {
				line = pascal.Trim(strings.TrimRight(line, "\r"))
				if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
					continue
				}
				pat := pascal.UpCase(filepath.Base(line))
				if pat == want || MatchWild(pat, want) {
					return true
				}
			}
		}
	}
	return false
}

// AddToFDB is Pascal AddToFdb: catalogue a file already present in the area directory.
func AddToFDB(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, name, uploader, desc string, attrib byte) error {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" {
		return os.ErrNotExist
	}
	dir := AreaUploadPath(g, area)
	if dir == "" {
		return os.ErrNotExist
	}
	src := filepath.Join(dir, name)
	st, err := os.Stat(src)
	if err != nil {
		// Case-insensitive match.
		if p := matchFile(dir, name); p != "" {
			src = p
			name = filepath.Base(p)
			st, err = os.Stat(src)
		}
		if err != nil {
			return err
		}
	}
	if st.IsDir() {
		return os.ErrInvalid
	}
	ents, err := ReadFDB(g, area)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if !e.Hdr.Deleted() && strings.EqualFold(e.Hdr.Name, name) {
			return os.ErrExist
		}
	}
	if uploader == "" && g != nil {
		uploader = g.RaConfig.Sysop
	}
	now := PackDOSTime(time.Now())
	h := cfgrec.FilesHdr{
		Name:        name,
		Size:        uint32(st.Size()),
		CRC32:       ^uint32(0),
		Uploader:    uploader,
		UploadDate:  now,
		FileDate:    PackDOSTime(st.ModTime()),
		LastDL:      now,
		Attrib:      attrib,
		Cost:        area.DefCost,
		LongDescPtr: -1,
		LfnPtr:      -1,
	}
	pascal.PutString(h.NameRaw[:], name)
	ents = append(ents, Entry{Hdr: h, Desc: desc})
	return WriteFDB(g, area, ents)
}
