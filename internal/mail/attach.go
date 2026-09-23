package mail

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
)

func attachRoot(g *cfgrec.GlobalCfg) string {
	if g == nil {
		return ""
	}
	p := strings.TrimSpace(g.RaConfig.AttachPath)
	if p == "" {
		p = filepath.Join(strings.TrimRight(g.RaConfig.SysPath, `\/`), "attach")
	}
	return strings.TrimRight(p, `\/`)
}

// CreateAttachDir is Pascal ForceBack(CreateTempDir(AttachPath, node)).
func CreateAttachDir(g *cfgrec.GlobalCfg, node int) string {
	dir := createTempDir(attachRoot(g), node)
	if dir == "" {
		return ""
	}
	return pascal.ForceBack(dir)
}

// createTempDir makes a DOS 8.3 folder: AT + HHMMSS (8 characters, no dot).
func createTempDir(base string, node int) string {
	base = strings.TrimRight(strings.TrimSpace(base), `\/`)
	if base == "" {
		return ""
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return ""
	}
	now := time.Now()
	start, _ := strconv.Atoi(now.Format("150405"))
	_ = node
	for n := 0; n < 10000; n++ {
		name := "AT" + strconv.Itoa(1000000 + ((start + n) % 1000000))[1:]
		dir := filepath.Join(base, name)
		if _, err := os.Stat(dir); err == nil {
			if ents, _ := os.ReadDir(dir); len(ents) > 0 {
				continue
			}
			return dir
		}
		if err := os.Mkdir(dir, 0755); err != nil {
			continue
		}
		return dir
	}
	return ""
}

func listAttachFiles(dir string) []string {
	dir = strings.TrimSpace(strings.TrimRight(dir, `\/`))
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() || ProtocolJunk(e.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// AttachFiles is the directory listing other readers use: JAM subject path.
func AttachFiles(subj string) []string {
	return listAttachFiles(subj)
}

func addAttachToFileArea(g *cfgrec.GlobalCfg, areaNum int, name string, data []byte) {
	if g == nil || areaNum <= 0 || name == "" {
		return
	}
	a, ok := files.FindArea(files.LoadAll(g), uint16(areaNum))
	if !ok || strings.TrimSpace(a.FilePath) == "" {
		return
	}
	dir := strings.TrimRight(a.FilePath, `\/`)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return
	}
	ents, err := files.ReadFDB(g, a)
	if err != nil {
		ents = nil
	}
	now := files.PackDOSTime(time.Now())
	st, _ := os.Stat(dest)
	sz := uint32(len(data))
	if st != nil {
		sz = uint32(st.Size())
	}
	ents = append(ents, files.Entry{Hdr: cfgrec.FilesHdr{
		Name:        name,
		Size:        sz,
		CRC32:       0xFFFFFFFF,
		Uploader:    "EleMAIL",
		UploadDate:  now,
		FileDate:    now,
		LastDL:      now,
		LongDescPtr: -1,
		LfnPtr:      -1,
	}})
	_ = files.WriteFDB(g, a, ents)
}

// processInboundAttach is Pascal ProcessAttachments with DoAttach=TRUE.
func processInboundAttach(g *cfgrec.GlobalCfg, attachArea int32, raw []byte) (out []byte, attachDir string, hasAtt bool) {
	out, parts := parseMIME(raw)
	if len(parts) == 0 {
		return raw, "", false
	}
	dir := createTempDir(attachRoot(g), 0)
	if dir == "" {
		return out, "", false
	}
	for _, p := range parts {
		name := sniffAttachName(p.Type, p.Name, p.Data)
		logx.Write(g, 0, '>', "Converting attachment (mime: "+p.Type+"), file="+name)
		if attachArea > 0 {
			addAttachToFileArea(g, int(attachArea), name, p.Data)
		}
		_ = os.WriteFile(filepath.Join(dir, name), p.Data, 0644)
		hasAtt = true
	}
	if !hasAtt {
		_ = os.Remove(dir)
		return out, "", false
	}
	removeDszLogFile(dir)
	removeDszLogFile(attachRoot(g))
	return out, pascal.ForceBack(dir), true
}

func dszLogFileName() string {
	return filepath.Base(strings.TrimSpace(os.Getenv("DSZLOG")))
}

func removeDszLogFile(dir string) {
	dir = strings.TrimRight(strings.TrimSpace(dir), `\/`)
	name := dszLogFileName()
	if dir == "" || name == "" || name == "." {
		return
	}
	_ = os.Remove(filepath.Join(dir, name))
}

// ProtocolJunk is DSZ/SEXYZ ctl+log that must never live in AttachPath.
func ProtocolJunk(name string) bool {
	up := strings.ToUpper(strings.TrimSpace(filepath.Base(name)))
	switch up {
	case "DSZ.CTL", "DSZ.LOG", "DSL.LOG", "DSZ.AUD", "DSZLOG", "SEXYZ.CTL", "SEXYZ.LOG", "ZMODEM.TMP":
		return true
	default:
		return false
	}
}
