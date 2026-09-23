package menu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/door"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
)

var dropFileNames = map[string]bool{
	"DOOR.SYS": true, "DORINFO1.DEF": true, "DOOR32.SYS": true,
	"EXITINFO.BBS": true, "CALLINFO.BBS": true, "CHAIN.TXT": true,
	"DOORFILE.SR": true, "TRIBBS.SYS": true, "SFDOORS.DAT": true,
}

func protocolUploadUsable(p cfgrec.Protocol, errorFree bool) bool {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.UpCmdString) == "" {
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

// uploadAttach is Pascal DoUpload(”, True, False, TempDir) for message file-attach.
func (e *Engine) uploadAttach(dest string) int {
	dir := strings.TrimRight(strings.TrimSpace(dest), `\/`)
	if dir == "" {
		return 0
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0
	}
	local := e.Line == nil || e.Line.Baud == 0
	if !local && e.G != nil && e.Line != nil {
		sysop := pascal.UpCase(pascal.Trim(e.G.RaConfig.Sysop))
		if sysop != "" && sysop == pascal.UpCase(pascal.Trim(e.Line.User.Name)) {
			local = e.T.AskYesNo(lang.LocAttach, false)
		}
	}
	if local {
		e.localAttachUpload(dir)
	} else {
		e.protocolAttachUpload(dir)
	}
	tidyAttachUpload(dir)
	return len(mail.AttachFiles(dir))
}

func (e *Engine) localAttachUpload(dir string) {
	e.T.WriteRA("`A10:" + e.T.RalGet(lang.EntrName))
	e.T.Println("")
	e.T.Println("")
	for {
		e.T.WriteRA("`A03:" + e.T.RalGet(lang.File1))
		name, err := e.T.GetString(80, false, false)
		e.T.Println("")
		if err != nil {
			return
		}
		name = pascal.Trim(name)
		if name == "" {
			return
		}
		matches := attachUploadMatches(name)
		if len(matches) == 0 {
			e.T.WriteRA("`A12:" + e.T.RalGet(lang.NotFound2) + " " + name)
			e.T.Println("")
			continue
		}
		for _, src := range matches {
			st, err := os.Stat(src)
			if err != nil || st.IsDir() {
				continue
			}
			to := filepath.Join(dir, filepath.Base(src))
			if _, err := os.Stat(to); err == nil {
				if !e.T.AskYesNo(lang.AlreadAtt, false) {
					continue
				}
			}
			if err := copyDownloadFile(src, to); err != nil {
				e.T.WriteRA("`A12:" + e.T.RalGet(lang.NotFound2) + " " + filepath.Base(src))
				e.T.Println("")
				continue
			}
			e.T.WriteRA("`A11:" + e.T.RalGet(lang.Sent1) + filepath.Base(src))
			e.T.Println("")
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload [Local] "+to)
		}
	}
}

func attachUploadMatches(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if strings.ContainsAny(name, "*?") {
		m, err := filepath.Glob(name)
		if err != nil {
			return nil
		}
		return m
	}
	if st, err := os.Stat(name); err == nil && !st.IsDir() {
		return []string{name}
	}
	return nil
}

func (e *Engine) pickUploadProtocol() (cfgrec.Protocol, bool) {
	for i := 0; i < 4; i++ {
		p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
		if protocolUploadUsable(p, e.Line.ErrorFreeConnect) {
			return p, true
		}
		e.selectProtocol()
		if e.Line.User.DefaultProto == 0 {
			return cfgrec.Protocol{}, false
		}
	}
	p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
	if protocolUploadUsable(p, e.Line.ErrorFreeConnect) {
		return p, true
	}
	e.T.Println("")
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.NoFiles))
	e.T.Println("")
	e.T.PressEnter()
	return cfgrec.Protocol{}, false
}

func (e *Engine) protocolAttachUpload(dir string) {
	prot, ok := e.pickUploadProtocol()
	if !ok {
		return
	}
	for {
		e.T.WriteRA("`A10:" + e.T.RalGet(lang.DefProt) + prot.Name)
		e.T.Println("")
		e.T.WriteRA("`A11:" + e.T.RalGet(lang.SNA))
		keys := pascal.UpCase(e.T.RalKeys(lang.SNA))
		if keys == "" {
			keys = "SNA"
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
		case 2:
			e.selectProtocol()
			prot, ok = e.pickUploadProtocol()
			if !ok {
				return
			}
			continue
		case 3:
			return
		default:
			continue
		}
		break
	}
	e.T.ClearScreen()
	e.T.WriteRA("`A11:" + e.T.RalGet(lang.Protocol1) + prot.Name)
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.StartUL))
	e.T.Println("")

	node := door.DropDir(e.G, e.Line)
	before := listPlainNames(node)
	ul := pascal.ForceBack(strings.TrimRight(dir, `\/`))
	ctl := e.writeUploadCtl(prot, ul)
	cmd := strings.ReplaceAll(prot.UpCmdString, "#", ul)
	if prot.LogFileName != "" {
		_ = os.Remove(expandNodeName(prot.LogFileName, e.Line.RaNodeNr))
	}
	e.T.ClearScreen()
	door.Run(e.T, e.G, e.Line, e.Files, cmd, false)
	e.harvestUploads(prot, dir, node, before)
	if ctl != "" {
		_ = os.Remove(ctl)
	}
	if prot.LogFileName != "" {
		_ = os.Remove(expandNodeName(prot.LogFileName, e.Line.RaNodeNr))
	}
}

func (e *Engine) writeUploadCtl(p cfgrec.Protocol, ulPath string) string {
	name := strings.TrimSpace(p.CtlFileName)
	if name == "" {
		return ""
	}
	name = expandNodeName(name, e.Line.RaNodeNr)
	if !filepath.IsAbs(name) {
		if d := door.DropDir(e.G, e.Line); d != "" {
			name = filepath.Join(d, name)
		}
	}
	ulPath = pascal.ForceBack(strings.TrimRight(strings.TrimSpace(ulPath), `\/`))
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
	line := p.UpCtlString
	if strings.Contains(line, "@") {
		line = strings.ReplaceAll(line, "@", ulPath)
	}
	b.WriteString(line)
	b.WriteString("\r\n")
	if err := os.WriteFile(name, []byte(b.String()), 0644); err != nil {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "ctl file: "+err.Error())
		return ""
	}
	return name
}

func (e *Engine) harvestUploads(p cfgrec.Protocol, dest, node string, before map[string]bool) {
	dest = strings.TrimRight(dest, `\/`)
	if kw := strings.TrimSpace(p.UpLogKeyWord); kw != "" && p.LogFileName != "" {
		raw, err := os.ReadFile(expandNodeName(p.LogFileName, e.Line.RaNodeNr))
		if err == nil {
			upkw := pascal.UpCase(kw)
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimRight(line, "\r")
				if !strings.Contains(pascal.UpCase(line), upkw) {
					continue
				}
				name := uploadLogName(line)
				if name == "" || mail.ProtocolJunk(name) {
					continue
				}
				src := name
				if !filepath.IsAbs(src) {
					src = filepath.Join(node, filepath.Base(name))
				}
				to := filepath.Join(dest, filepath.Base(name))
				if sameFile(src, to) {
					continue
				}
				if err := moveUploadFile(src, to); err == nil {
					logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload ["+p.Name+"] "+to)
				}
			}
		}
	}
	for _, name := range listPlainNamesList(node) {
		up := pascal.UpCase(name)
		if before[up] || dropFileNames[up] {
			continue
		}
		src := filepath.Join(node, name)
		if mail.ProtocolJunk(name) {
			continue
		}
		to := filepath.Join(dest, name)
		if sameFile(src, to) {
			continue
		}
		if err := moveUploadFile(src, to); err == nil {
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload ["+p.Name+"] "+to)
		}
	}
}

func uploadLogName(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[len(fields)-1], `"'`)
}

func moveUploadFile(src, dst string) error {
	if err := copyDownloadFile(src, dst); err != nil {
		return err
	}
	if !sameFile(src, dst) {
		_ = os.Remove(src)
	}
	return nil
}

func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return pascal.UpCase(a) == pascal.UpCase(b)
	}
	return pascal.UpCase(aa) == pascal.UpCase(bb)
}

func listPlainNames(dir string) map[string]bool {
	out := map[string]bool{}
	for _, n := range listPlainNamesList(dir) {
		out[pascal.UpCase(n)] = true
	}
	return out
}

func listPlainNamesList(dir string) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	return out
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

func tidyAttachUpload(unique string) {
	unique = strings.TrimRight(strings.TrimSpace(unique), `\/`)
	if unique == "" {
		return
	}
	prefix := filepath.Base(unique)
	parent := filepath.Dir(unique)
	for _, dir := range []string{unique, parent} {
		removeDszLogFile(dir)
		for _, name := range listPlainNamesList(dir) {
			if mail.ProtocolJunk(name) {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}
	if prefix == "" || parent == "" {
		return
	}
	upPrefix := pascal.UpCase(prefix)
	for _, name := range listPlainNamesList(parent) {
		if mail.ProtocolJunk(name) {
			continue
		}
		if !strings.HasPrefix(pascal.UpCase(name), upPrefix) {
			continue
		}
		rest := name[len(prefix):]
		if rest == "" {
			continue
		}
		_ = moveUploadFile(filepath.Join(parent, name), filepath.Join(unique, rest))
	}
}
