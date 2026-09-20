package menu

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/door"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
)

func protocolUsable(p cfgrec.Protocol, errorFree bool) bool {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.DnCmdString) == "" {
		return false
	}
	switch p.Attribute {
	case 1:
		return true
	case 2:
		return errorFree
	default:
		return false
	}
}

func downloadAreaAndNames(data string, cur uint16) (uint16, []string) {
	area := cur
	var names []string
	rest := strings.TrimSpace(data)
	if v, r := getValue("/A=", rest); v != "" {
		if n := atoiMenu(v); n > 0 {
			area = uint16(n)
		}
		rest = r
	}
	for rest != "" {
		w, next := firstWord(rest)
		rest = next
		if w == "" {
			break
		}
		if strings.HasPrefix(w, "/") {
			continue
		}
		if len(names) == 0 && !strings.ContainsAny(w, ".*") {
			if n := atoiMenu(w); n > 0 {
				area = uint16(n)
				continue
			}
		}
		names = append(names, w)
	}
	return area, names
}

func (e *Engine) download(data string, global, specific, anyFile, allowEdit bool) {
	// Do not gate on TransferBaud / FixBaud. Remote telnet (e.g. 65529) may
	// download at any configured speed.
	areaNum, names := downloadAreaAndNames(data, e.Line.User.FileArea)
	group := strings.Contains(pascal.UpCase(data), "/FG")
	if specific {
		if len(names) == 0 {
			w := firstField(data)
			if w != "" && !strings.HasPrefix(w, "/") {
				names = append(names, w)
			}
		}
		e.addNamesToTag(names, areaNum, global, anyFile)
		e.showTaggedFiles(false)
	}
	if allowEdit {
		e.editTagList(false, anyFile, global, group, areaNum)
	}
	e.T.Println("")
	e.transferDownloads(e.taggedFound())
}

func (e *Engine) transferDownloads(tagged []files.Found) {
	if len(tagged) == 0 {
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.NoFiles))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	if e.Line.Baud == 0 {
		e.localDownload(tagged)
		return
	}
	prot, ok := e.pickDownloadProtocol()
	if !ok {
		return
	}
	logOff := false
	for {
		e.T.WriteRA("`A10:" + e.T.RalGet(lang.DefProt) + prot.Name)
		e.T.Println("")
		e.T.WriteRA("`A11:" + e.T.RalStr(lang.StrtAbr))
		keys := pascal.UpCase(e.T.RalKeys(lang.StrtAbr))
		if keys == "" {
			keys = "SLNA"
		}
		ch, err := e.T.GetKey(0)
		if err != nil {
			return
		}
		pos := 0
		if ch == '\r' || ch == '\n' {
			pos = 1
		} else {
			up := pascal.UpCase(string([]byte{ch}))
			if i := strings.Index(keys, up); i >= 0 {
				pos = i + 1
			}
		}
		switch pos {
		case 1:
			logOff = false
		case 2:
			logOff = true
		case 3:
			e.selectProtocol()
			prot, ok = e.pickDownloadProtocol()
			if !ok {
				return
			}
			continue
		case 4:
			return
		default:
			continue
		}
		break
	}

	e.T.Println("")
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.XferFiles) + strconv.Itoa(len(tagged)))
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.StartRecv))
	e.T.Println("")

	sent := e.runDownloadProtocol(prot, tagged)
	e.T.Println("")
	switch sent {
	case 0:
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.NoSent))
	case 1:
		e.T.WriteRA("`A12:1 " + e.T.RalGet(lang.FileSent))
	default:
		e.T.WriteRA("`A12:" + strconv.Itoa(sent) + " " + e.T.RalGet(lang.FilesSent))
	}
	e.T.Println("")
	e.T.PressEnter()
	if logOff {
		e.Hang = true
	}
}

func (e *Engine) lookupDownload(names []string, areaNum uint16, global, anyFile bool) []files.Found {
	var tagged []files.Found
	seen := map[string]bool{}
	add := func(found []files.Found) {
		for _, f := range found {
			key := pascal.UpCase(f.Path)
			if seen[key] {
				continue
			}
			seen[key] = true
			tagged = append(tagged, f)
		}
	}
	searchAreas := func(spec string) {
		if global {
			for _, a := range e.Files {
				if a.Name == "" {
					continue
				}
				if !files.DownloadAccess(a, e.Line.User) {
					continue
				}
				add(files.SearchNameInArea(e.G, a, spec, anyFile))
			}
			return
		}
		a, ok := files.FindArea(e.Files, areaNum)
		if !ok {
			return
		}
		if !files.DownloadAccess(a, e.Line.User) && !anyFile {
			return
		}
		add(files.SearchNameInArea(e.G, a, spec, anyFile))
	}
	for _, spec := range names {
		searchAreas(spec)
	}
	return tagged
}

func (e *Engine) pickDownloadProtocol() (cfgrec.Protocol, bool) {
	for i := 0; i < 4; i++ {
		p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
		if protocolUsable(p, e.Line.ErrorFreeConnect) {
			return p, true
		}
		e.selectProtocol()
		if e.Line.User.DefaultProto == 0 {
			return cfgrec.Protocol{}, false
		}
	}
	p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
	if protocolUsable(p, e.Line.ErrorFreeConnect) {
		return p, true
	}
	e.T.Println("")
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.NoFiles))
	e.T.Println("")
	e.T.PressEnter()
	return cfgrec.Protocol{}, false
}

func (e *Engine) runDownloadProtocol(p cfgrec.Protocol, tagged []files.Found) int {
	ctl := e.writeDownloadCtl(p, tagged)
	cmd := strings.ReplaceAll(p.DnCmdString, "#", "")
	if strings.TrimSpace(p.CtlFileName) == "" {
		if strings.Contains(cmd, "@") {
			cmd = strings.Replace(cmd, "@", quoteDl(tagged[0].Path), 1)
		} else {
			for _, f := range tagged {
				cmd += " " + quoteDl(f.Path)
			}
		}
	}
	if p.LogFileName != "" {
		_ = os.Remove(expandNodeName(p.LogFileName, e.Line.RaNodeNr))
	}
	e.T.ClearScreen()
	door.Run(e.T, e.G, e.Line, e.Files, cmd, false)
	sent := e.finishDownload(p, tagged)
	if ctl != "" {
		_ = os.Remove(ctl)
	}
	if p.LogFileName != "" {
		_ = os.Remove(expandNodeName(p.LogFileName, e.Line.RaNodeNr))
	}
	return sent
}

func (e *Engine) writeDownloadCtl(p cfgrec.Protocol, tagged []files.Found) string {
	name := strings.TrimSpace(p.CtlFileName)
	if name == "" {
		return ""
	}
	name = expandNodeName(name, e.Line.RaNodeNr)
	if !filepath.IsAbs(name) {
		dir := door.DropDir(e.G, e.Line)
		if dir != "" {
			name = filepath.Join(dir, name)
		}
	}
	var b strings.Builder
	if p.OpusType {
		port := 1
		if e.Line.Modem.ComPort != 0 {
			port = int(e.Line.Modem.ComPort)
		}
		fmt.Fprintf(&b, "Port %d\r\n", port)
		fmt.Fprintf(&b, "Baud %d\r\n", downloadBaud(e.Line.Baud))
		if p.LogFileName != "" {
			fmt.Fprintf(&b, "Log %s\r\n", p.LogFileName)
		}
		fmt.Fprintf(&b, "Time %d\r\n", e.Line.TimeLimit)
	}
	for _, f := range tagged {
		line := p.DnCtlString
		if strings.Contains(line, "@") {
			line = strings.ReplaceAll(line, "@", f.Path)
		} else if strings.TrimSpace(line) == "" {
			line = f.Path
		} else {
			line += f.Path
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	if err := os.WriteFile(name, []byte(b.String()), 0644); err != nil {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "ctl file: "+err.Error())
		return ""
	}
	return name
}

func (e *Engine) finishDownload(p cfgrec.Protocol, tagged []files.Found) int {
	matched := map[string]bool{}
	if kw := strings.TrimSpace(p.DnLogKeyWord); kw != "" && p.LogFileName != "" {
		raw, err := os.ReadFile(expandNodeName(p.LogFileName, e.Line.RaNodeNr))
		if err == nil {
			upkw := pascal.UpCase(kw)
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimRight(line, "\r")
				if !strings.Contains(pascal.UpCase(line), upkw) {
					continue
				}
				for _, f := range tagged {
					if strings.Contains(pascal.UpCase(line), pascal.UpCase(f.Hdr.Name)) ||
						strings.Contains(pascal.UpCase(line), pascal.UpCase(filepath.Base(f.Path))) {
						matched[pascal.UpCase(f.Path)] = true
					}
				}
			}
		}
	}
	if len(matched) == 0 {
		for _, f := range tagged {
			matched[pascal.UpCase(f.Path)] = true
		}
	}
	sent := 0
	for _, f := range tagged {
		if !matched[pascal.UpCase(f.Path)] {
			continue
		}
		sent++
		files.BumpTimesDL(e.G, f.Area, f.Hdr.RecordNum)
		e.Line.User.Downloads++
		e.Line.User.DownloadsK += int32(f.Hdr.Size / 1024)
		logx.Write(e.G, e.Line.RaNodeNr, '>', "Download ["+p.Name+"]: "+pascal.UpCase(f.Path))
	}
	e.saveUser()
	return sent
}

func (e *Engine) localDownload(tagged []files.Found) {
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.LocDown))
	e.T.Println("")
	e.T.WriteRA("`A10:" + e.T.RalGet(lang.AskMvDir))
	dest, _ := e.T.GetString(70, false, false)
	dest = pascal.Trim(dest)
	if dest == "" {
		return
	}
	dest = pascal.ForceBack(dest)
	if err := os.MkdirAll(strings.TrimRight(dest, `\/`), 0755); err != nil {
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.ErrMove))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	sent := 0
	for _, f := range tagged {
		to := dest + filepath.Base(f.Path)
		if err := copyDownloadFile(f.Path, to); err != nil {
			e.T.WriteRA("`A12:" + e.T.RalGet(lang.ErrMove) + " `A15:" + f.Hdr.Name)
			e.T.Println("")
			continue
		}
		sent++
		files.BumpTimesDL(e.G, f.Area, f.Hdr.RecordNum)
		e.Line.User.Downloads++
		e.Line.User.DownloadsK += int32(f.Hdr.Size / 1024)
		logx.Write(e.G, e.Line.RaNodeNr, '>', "Download [Local]: "+pascal.UpCase(f.Hdr.Name))
	}
	e.saveUser()
	e.T.Println("")
	switch sent {
	case 0:
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.NoSent))
	case 1:
		e.T.WriteRA("`A12:1 " + e.T.RalGet(lang.FileSent))
	default:
		e.T.WriteRA("`A12:" + strconv.Itoa(sent) + " " + e.T.RalGet(lang.FilesSent))
	}
	e.T.Println("")
	e.T.PressEnter()
}

func copyDownloadFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func quoteDl(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, `"`) {
		return `"` + s + `"`
	}
	return s
}

func expandNodeName(s string, node int) string {
	n := strconv.Itoa(node)
	s = strings.ReplaceAll(s, "*N", n)
	s = strings.ReplaceAll(s, "*n", n)
	return s
}

func downloadBaud(b uint16) int {
	if b == 11520 {
		return 115200
	}
	return int(b)
}
