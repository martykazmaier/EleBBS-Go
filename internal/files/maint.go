package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/pascal"
)

func Status(msg string) { fmt.Println(" * " + msg) }

func Add(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, glob, uploader, desc string) error {
	if err := EnsureFDBDirs(g); err != nil {
		return err
	}
	dir := strings.TrimRight(area.FilePath, `\/`)
	pattern := glob
	if filepath.IsAbs(glob) || strings.ContainsAny(glob, `/\`) {
		dir = filepath.Dir(glob)
		pattern = filepath.Base(glob)
	}
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("cannot find file: %s", filepath.Join(dir, pattern))
	}
	ents, err := ReadFDB(g, area)
	if err != nil {
		return err
	}
	if uploader == "" {
		uploader = g.RaConfig.Sysop
	}
	added := 0
	for _, src := range matches {
		st, err := os.Stat(src)
		if err != nil || st.IsDir() {
			continue
		}
		name := st.Name()
		dest := DiskFile(area, name)
		if !strings.EqualFold(filepath.Clean(src), filepath.Clean(dest)) {
			if err := copyFile(src, dest); err != nil {
				Status(name + "  - copy failed: " + err.Error())
				continue
			}
		}
		dup := false
		for _, e := range ents {
			if !e.Hdr.Deleted() && strings.EqualFold(e.Hdr.Name, name) {
				dup = true
				break
			}
		}
		if dup {
			Status(fmt.Sprintf("%-12s  - Duplicate, rejected.", name))
			continue
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
			LongDescPtr: -1,
			LfnPtr:      -1,
		}
		pascal.PutString(h.NameRaw[:], name)
		ents = append(ents, Entry{Hdr: h, Desc: desc})
		Status(name + "  - Added succesfully.")
		added++
	}
	if added == 0 {
		return fmt.Errorf("no files added")
	}
	return WriteFDB(g, area, ents)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func Clean(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, killMissing bool) (int, error) {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	n := 0
	keep := make([]Entry, 0, len(ents))
	for _, e := range ents {
		if e.Hdr.Comment() {
			keep = append(keep, e)
			continue
		}
		if e.Hdr.Locked() {
			keep = append(keep, e)
			continue
		}
		expired := false
		if area.KillDaysFD > 0 && DaysAgo(e.Hdr.FileDate) > int(area.KillDaysFD) {
			expired = true
		}
		if area.KillDaysDL > 0 && DaysAgo(e.Hdr.LastDL) > int(area.KillDaysDL) {
			expired = true
		}
		missing := false
		if killMissing {
			if _, err := os.Stat(DiskFile(area, e.Hdr.Name)); err != nil {
				missing = true
			}
		}
		if expired || missing {
			Status("  " + e.Hdr.Name + " removed")
			if area.MoveArea == 0 {
				_ = os.Remove(DiskFile(area, e.Hdr.Name))
			}
			n++
			continue
		}
		keep = append(keep, e)
	}
	return n, WriteFDB(g, area, keep)
}

func SetAttrib(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, spec, op string) (int, error) {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range ents {
		if !MatchName(spec, ents[i].Hdr.Name) {
			continue
		}
		switch strings.ToUpper(op) {
		case "KILL":
			ents[i].Hdr.Attrib |= cfgrec.AttrDeleted
			n++
		case "LOCK":
			ents[i].Hdr.Attrib |= cfgrec.AttrLocked
			n++
		case "UNLOCK":
			ents[i].Hdr.Attrib &^= cfgrec.AttrLocked
			n++
		}
	}
	return n, WriteFDB(g, area, ents)
}

func Adopt(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, spec string) (int, error) {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, e := range ents {
		if !e.Hdr.Deleted() {
			known[strings.ToUpper(e.Hdr.Name)] = true
		}
	}
	dir := strings.TrimRight(area.FilePath, `\/`)
	matches, err := filepath.Glob(filepath.Join(dir, spec))
	if err != nil {
		return 0, err
	}
	if spec == "" {
		d, err := os.ReadDir(dir)
		if err != nil {
			return 0, err
		}
		matches = nil
		for _, f := range d {
			if !f.IsDir() {
				matches = append(matches, filepath.Join(dir, f.Name()))
			}
		}
	}
	n := 0
	now := PackDOSTime(time.Now())
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		name := st.Name()
		if known[strings.ToUpper(name)] {
			continue
		}
		h := cfgrec.FilesHdr{
			Name: name, Size: uint32(st.Size()), CRC32: ^uint32(0),
			Uploader: g.RaConfig.Sysop, UploadDate: now, FileDate: PackDOSTime(st.ModTime()),
			LastDL: now, LongDescPtr: -1, LfnPtr: -1,
		}
		pascal.PutString(h.NameRaw[:], name)
		ents = append(ents, Entry{Hdr: h})
		Status(name + "  - Adopted.")
		n++
	}
	if n == 0 {
		return 0, nil
	}
	return n, WriteFDB(g, area, ents)
}

func UpdateTimes(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, spec string, touchMod bool) (int, error) {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	n := 0
	now := PackDOSTime(time.Now())
	for i := range ents {
		if !MatchName(spec, ents[i].Hdr.Name) {
			continue
		}
		st, err := os.Stat(DiskFile(area, ents[i].Hdr.Name))
		if err != nil {
			continue
		}
		ents[i].Hdr.Size = uint32(st.Size())
		if touchMod {
			ents[i].Hdr.FileDate = PackDOSTime(st.ModTime())
		} else {
			ents[i].Hdr.FileDate = now
		}
		ents[i].Hdr.Attrib &^= cfgrec.AttrMissing
		n++
	}
	return n, WriteFDB(g, area, ents)
}

func Sort(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, byDate, reverse bool) error {
	ents, err := ReadFDB(g, area)
	if err != nil {
		return err
	}
	sort.SliceStable(ents, func(i, j int) bool {
		less := false
		if byDate {
			less = ents[i].Hdr.UploadDate < ents[j].Hdr.UploadDate
		} else {
			less = strings.ToUpper(ents[i].Hdr.Name) < strings.ToUpper(ents[j].Hdr.Name)
		}
		if reverse {
			return !less
		}
		return less
	})
	return WriteFDB(g, area, ents)
}

func FileList(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, outPath string, daysOld int, noHdr, sevenBit, formFeed bool) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	write := func(s string) {
		if sevenBit {
			b := pascal.ToCP437(s)
			for i, c := range b {
				if c >= 128 {
					b[i] = '?'
				}
			}
			s = string(b)
		}
		_, _ = io.WriteString(f, s)
	}
	total, kbytes := 0, 0
	for _, area := range areas {
		ents, err := ReadFDB(g, area)
		if err != nil {
			continue
		}
		if !noHdr {
			write(fmt.Sprintf("\r\nArea %d - %s\r\n", area.AreaNum, area.Name))
			write(strings.Repeat("-", 70) + "\r\n")
		}
		for _, e := range ents {
			if e.Hdr.Deleted() || e.Hdr.Unlisted() || e.Hdr.Comment() {
				continue
			}
			if daysOld > 0 && DaysAgo(e.Hdr.UploadDate) > daysOld {
				continue
			}
			write(fmt.Sprintf("%-12s %8d  %s\r\n", e.Hdr.Name, e.Hdr.Size, e.Hdr.Uploader))
			if e.Desc != "" {
				for _, line := range strings.Split(e.Desc, "\n") {
					write("  " + line + "\r\n")
				}
			}
			total++
			kbytes += int(e.Hdr.Size / 1024)
		}
		if formFeed {
			write("\f")
		}
	}
	write(fmt.Sprintf("\r\nTotal: %d files, %dK\r\n", total, kbytes))
	return nil
}

func Export(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, outPath string, raCompat bool) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	n := 0
	for _, area := range areas {
		ents, err := ReadFDB(g, area)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.Hdr.Deleted() || e.Hdr.Comment() {
				continue
			}
			desc := strings.ReplaceAll(e.Desc, "\n", " / ")
			fmt.Fprintf(f, "%s %s\r\n", e.Hdr.Name, desc)
			n++
		}
	}
	Status(fmt.Sprintf("Exported %d files to %s", n, outPath))
	return nil
}

func ImportList(g *cfgrec.GlobalCfg, area cfgrec.FilesArea, inPath, uploader string, erase, missingOnly bool) (int, error) {
	b, err := os.ReadFile(inPath)
	if err != nil {
		return 0, err
	}
	ents, err := ReadFDB(g, area)
	if err != nil {
		return 0, err
	}
	known := map[string]int{}
	for i, e := range ents {
		known[strings.ToUpper(e.Hdr.Name)] = i
	}
	if uploader == "" {
		uploader = g.RaConfig.Sysop
	}
	n := 0
	now := PackDOSTime(time.Now())
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		name := parts[0]
		desc := ""
		if len(parts) > 1 {
			desc = strings.Join(parts[1:], " ")
		}
		if i, ok := known[strings.ToUpper(name)]; ok {
			if desc != "" {
				ents[i].Desc = desc
			}
			continue
		}
		p := DiskFile(area, name)
		st, err := os.Stat(p)
		if err != nil {
			if missingOnly {
				continue
			}
			ents = append(ents, Entry{Hdr: cfgrec.FilesHdr{
				Name: name, Uploader: uploader, UploadDate: now, LastDL: now,
				LongDescPtr: -1, LfnPtr: -1, Attrib: cfgrec.AttrMissing,
			}, Desc: desc})
			pascal.PutString(ents[len(ents)-1].Hdr.NameRaw[:], name)
			n++
			continue
		}
		h := cfgrec.FilesHdr{
			Name: name, Size: uint32(st.Size()), CRC32: uint32(crc.RA(name, true)),
			Uploader: uploader, UploadDate: now, FileDate: PackDOSTime(st.ModTime()),
			LastDL: now, LongDescPtr: -1, LfnPtr: -1,
		}
		pascal.PutString(h.NameRaw[:], name)
		ents = append(ents, Entry{Hdr: h, Desc: desc})
		n++
	}
	if erase {
		_ = os.Remove(inPath)
	}
	return n, WriteFDB(g, area, ents)
}

func HTMLList(g *cfgrec.GlobalCfg, areas []cfgrec.FilesArea, outPath string, daysOld int) error {
	if outPath == "" {
		outPath = "files.htm"
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "<html><head><title>%s files</title></head><body>\n", g.RaConfig.SystemName)
	for _, area := range areas {
		ents, err := ReadFDB(g, area)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "<h2>Area %d - %s</h2>\n<table>\n", area.AreaNum, area.Name)
		for _, e := range ents {
			if e.Hdr.Deleted() || e.Hdr.Unlisted() || e.Hdr.Comment() {
				continue
			}
			if daysOld > 0 && DaysAgo(e.Hdr.UploadDate) > daysOld {
				continue
			}
			fmt.Fprintf(f, "<tr><td>%s</td><td>%d</td><td>%s</td></tr>\n", e.Hdr.Name, e.Hdr.Size, e.Hdr.Uploader)
		}
		fmt.Fprintln(f, "</table>")
	}
	fmt.Fprintln(f, "</body></html>")
	return nil
}
