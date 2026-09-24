package menu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/door"
	"elebbs/internal/files"
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

// upload is Pascal DoUpload(MiscData, False, False, ”).
// Transfers are allowed at any baud; TransferBaud is not enforced.
func (e *Engine) upload(data string) {
	e.doUpload(data, false, false, "")
}

// uploadAttach is Pascal DoUpload(”, True, False, TempDir) for message file-attach.
func (e *Engine) uploadAttach(dest string) int {
	return e.doUpload("", true, false, dest)
}

func (e *Engine) doUpload(misc string, attaching, upMsg bool, ulPath string) int {
	if e.Line == nil || e.G == nil {
		return 0
	}
	area, ok := e.uploadArea(misc, attaching, ulPath)
	if !ok {
		return 0
	}
	if !attaching && !upMsg {
		if !files.UploadAccess(area, e.Line.User) {
			e.T.Println("")
			e.T.WriteRA("`A12:")
			e.T.Println("")
			e.T.WriteRA(e.T.RalGet(lang.NoUpAcc))
			e.T.Println("")
			e.T.PressEnter()
			return 0
		}
	}
	dest := files.AreaUploadPath(e.G, area)
	if attaching {
		dest = strings.TrimRight(strings.TrimSpace(ulPath), `\/`)
		if dest == "" {
			return 0
		}
		if err := os.MkdirAll(dest, 0755); err != nil {
			return 0
		}
	} else {
		if dest == "" {
			logx.Write(e.G, e.Line.RaNodeNr, '!', "Upload path does not exist!")
			return 0
		}
		if st, err := os.Stat(dest); err != nil || !st.IsDir() {
			logx.Write(e.G, e.Line.RaNodeNr, '!', "Upload path ("+pascal.UpCase(dest)+") does not exist!")
			return 0
		}
	}

	if !attaching {
		e.T.ClearScreen()
	}

	local := e.Line.Baud == 0
	if attaching && !local {
		sysop := pascal.UpCase(pascal.Trim(e.G.RaConfig.Sysop))
		if sysop != "" && sysop == pascal.UpCase(pascal.Trim(e.Line.User.Name)) {
			if e.T.AskYesNo(lang.LocAttach, false) {
				local = true
			}
		}
	}

	var count int
	if local {
		count = e.localAreaUpload(area, dest, attaching, upMsg)
	} else {
		count = e.protocolAreaUpload(area, dest, attaching, upMsg)
	}

	if attaching {
		tidyAttachUpload(dest)
		return len(mail.AttachFiles(dest))
	}
	e.T.WriteRaw([]byte("\r\x1b[K"))
	e.T.WriteRA("`X1:`E:")
	switch count {
	case 0:
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.NoRec))
	case 1:
		e.T.WriteRA("`A12:1 " + e.T.RalGet(lang.FileRec))
	default:
		e.T.WriteRA("`A12:" + fmt.Sprintf("%d ", count) + e.T.RalGet(lang.FilesRec))
	}
	e.T.Println("")
	e.T.PressEnter()
	return count
}

func (e *Engine) uploadArea(misc string, attaching bool, ulPath string) (cfgrec.FilesArea, bool) {
	if attaching {
		return cfgrec.FilesArea{FilePath: ulPath}, true
	}
	num := e.Line.User.FileArea
	w, _ := firstWord(misc)
	if n := atoiMenu(w); n > 0 {
		num = uint16(n)
	}
	a, ok := files.FindArea(e.Files, num)
	if !ok {
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.InvArea))
		e.T.Println("")
		e.T.PressEnter()
		return cfgrec.FilesArea{}, false
	}
	if a.UploadArea > 0 {
		if u, uok := files.FindArea(e.Files, a.UploadArea); uok {
			a = u
		}
	}
	return a, true
}

func (e *Engine) pickUploadProtocol() (cfgrec.Protocol, bool) {
	for i := 0; i < 4; i++ {
		p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
		if protocolUploadUsable(p, e.Line.ErrorFreeConnect) {
			return p, true
		}
		e.selectProtocol()
		if e.Line.User.DefaultProto == 0 || e.Line.User.DefaultProto == ' ' {
			break
		}
	}
	p := config.FindProtocol(e.G, e.Line.User.DefaultProto)
	if protocolUploadUsable(p, e.Line.ErrorFreeConnect) {
		return p, true
	}
	e.T.Println("")
	e.T.WriteRA("`A12:No transfer protocol selected.")
	e.T.Println("")
	e.T.PressEnter()
	return cfgrec.Protocol{}, false
}

func (e *Engine) askUploadStart(prot cfgrec.Protocol) (cfgrec.Protocol, bool) {
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
			return cfgrec.Protocol{}, false
		}
		e.T.FinishEnter(ch)
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
			return prot, true
		case 2:
			e.selectProtocol()
			var ok bool
			prot, ok = e.pickUploadProtocol()
			if !ok {
				return cfgrec.Protocol{}, false
			}
		case 3:
			return cfgrec.Protocol{}, false
		}
	}
}

func (e *Engine) protocolAreaUpload(area cfgrec.FilesArea, dest string, attaching, upMsg bool) int {
	_ = upMsg
	prot, ok := e.pickUploadProtocol()
	if !ok {
		return 0
	}
	prot, ok = e.askUploadStart(prot)
	if !ok {
		return 0
	}

	e.T.ClearScreen()
	e.T.WriteRA("`A11:" + e.T.RalGet(lang.Protocol1) + prot.Name)
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.StartUL))
	e.T.Println("")

	ul := pascal.ForceBack(dest)
	node := door.DropDir(e.G, e.Line)
	beforeDest := listPlainNames(dest)
	beforeNode := listPlainNames(node)
	ctl := e.writeUploadCtl(prot, ul)
	cmd := strings.ReplaceAll(prot.UpCmdString, "#", ul)
	if prot.LogFileName != "" {
		_ = os.Remove(expandNodeName(prot.LogFileName, e.Line.RaNodeNr))
	}
	e.T.ClearScreen()
	door.Run(e.T, e.G, e.Line, e.Files, cmd, false)
	names := e.collectUploadedNames(prot, dest, node, beforeDest, beforeNode, attaching)
	if ctl != "" {
		_ = os.Remove(ctl)
	}
	if prot.LogFileName != "" {
		_ = os.Remove(expandNodeName(prot.LogFileName, e.Line.RaNodeNr))
	}
	if attaching {
		return len(names)
	}
	return e.processUploads(area, names, prot.Name)
}

func (e *Engine) localAreaUpload(area cfgrec.FilesArea, dest string, attaching, upMsg bool) int {
	_ = upMsg
	if !attaching {
		e.T.ClearScreen()
		e.T.WriteRA("`A15:" + e.T.RalGet(lang.LocUpl))
		e.T.Println("")
		e.T.Println("")
	}
	e.T.WriteRA("`A10:" + e.T.RalGet(lang.EntrName))
	e.T.Println("")
	e.T.Println("")

	var names []string
	for {
		e.T.WriteRA("`A03:" + e.T.RalGet(lang.File1))
		name, err := e.T.GetString(80, false, false)
		e.T.Println("")
		if err != nil {
			break
		}
		name = pascal.Trim(name)
		if name == "" {
			break
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
			base := filepath.Base(src)
			to := filepath.Join(dest, base)
			if _, err := os.Stat(to); err == nil {
				if attaching {
					if !e.T.AskYesNo(lang.AlreadAtt, false) {
						continue
					}
				} else {
					e.T.WriteRA("`A11:" + base + ": " + e.T.RalGet(lang.Exists2))
					e.T.Println("")
					continue
				}
			}
			if !attaching {
				if files.NameInDupeScan(e.G, e.Files, base) {
					e.T.WriteRA("`A3:" + base + ": " + e.T.RalGet(lang.Exists1))
					e.T.Println("")
					continue
				}
			}
			if err := copyDownloadFile(src, to); err != nil {
				e.T.WriteRA("`A12:" + e.T.RalGet(lang.NotFound2) + " " + base)
				e.T.Println("")
				continue
			}
			e.T.WriteRA("`A11:" + e.T.RalGet(lang.Sent1) + base)
			e.T.Println("")
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload [Local] "+to)
			names = append(names, base)
		}
	}
	if attaching {
		return len(names)
	}
	return e.processUploads(area, names, "Local")
}

func (e *Engine) collectUploadedNames(p cfgrec.Protocol, dest, node string, beforeDest, beforeNode map[string]bool, attaching bool) []string {
	seen := map[string]bool{}
	var names []string
	add := func(path string) {
		base := filepath.Base(path)
		if base == "" || mail.ProtocolJunk(base) || dropFileNames[pascal.UpCase(base)] {
			return
		}
		up := pascal.UpCase(base)
		if seen[up] {
			return
		}
		to := filepath.Join(dest, base)
		src := path
		if !filepath.IsAbs(src) {
			if st, err := os.Stat(filepath.Join(dest, base)); err == nil && !st.IsDir() {
				src = filepath.Join(dest, base)
			} else {
				src = filepath.Join(node, base)
			}
		}
		if sameFile(src, to) {
			seen[up] = true
			names = append(names, base)
			return
		}
		if _, err := os.Stat(src); err != nil {
			return
		}
		if err := moveUploadFile(src, to); err != nil {
			return
		}
		seen[up] = true
		names = append(names, base)
		if attaching {
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload ["+p.Name+"] "+to)
		}
	}

	if kw := strings.TrimSpace(p.UpLogKeyWord); kw != "" && p.LogFileName != "" {
		raw, err := os.ReadFile(expandNodeName(p.LogFileName, e.Line.RaNodeNr))
		if err == nil {
			upkw := pascal.UpCase(kw)
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimRight(line, "\r")
				if !strings.Contains(pascal.UpCase(line), upkw) {
					continue
				}
				if name := uploadLogName(line); name != "" {
					add(name)
				}
			}
		}
	}
	for _, name := range listPlainNamesList(dest) {
		if beforeDest[pascal.UpCase(name)] {
			continue
		}
		add(filepath.Join(dest, name))
	}
	for _, name := range listPlainNamesList(node) {
		if beforeNode[pascal.UpCase(name)] {
			continue
		}
		add(filepath.Join(node, name))
	}
	return names
}

func (e *Engine) processUploads(area cfgrec.FilesArea, names []string, protName string) int {
	okCount := 0
	first := true
	for _, name := range names {
		if name == "" {
			continue
		}
		if files.BadFileName(e.G, name) {
			e.T.Println("")
			e.T.WriteRA("`A12:" + e.T.RalGet(lang.Rejected2))
			e.T.Println("")
			e.T.Println("")
			e.T.PressEnter()
			_ = os.Remove(filepath.Join(files.AreaUploadPath(e.G, area), name))
			continue
		}
		if files.NameInDupeScan(e.G, e.Files, name) {
			e.T.Println("")
			e.T.WriteRA("`A12:" + e.T.RalGet(lang.Rejected1))
			e.T.Println("")
			e.T.Println("")
			e.T.PressEnter()
			continue
		}
		desc, attrib, ok := e.askUploadDesc(area, name, first)
		first = false
		if !ok {
			continue
		}
		if err := files.AddToFDB(e.G, area, name, e.Line.User.Name, desc, attrib); err != nil {
			continue
		}
		path := filepath.Join(files.AreaUploadPath(e.G, area), name)
		st, _ := os.Stat(path)
		size := int64(0)
		if st != nil {
			size = st.Size()
		}
		e.Line.User.Uploads++
		e.Line.User.UploadsK += int32(size / 1024)
		okCount++
		logx.Write(e.G, e.Line.RaNodeNr, '>', "Upload ["+protName+"] "+pascal.ForceBack(files.AreaUploadPath(e.G, area))+name)
		logx.Write(e.G, e.Line.RaNodeNr, '>', fmt.Sprintf("(%d bytes, %dk)", size, size/1024))
	}
	if okCount > 0 {
		e.saveUser()
	}
	return okCount
}

func (e *Engine) askUploadDesc(area cfgrec.FilesArea, name string, first bool) (desc string, attrib byte, ok bool) {
	e.T.WriteRA("`A12:")
	e.T.Println("")
	if first {
		e.T.WriteRA(e.T.RalGet(lang.Descr))
		e.T.Println("")
		e.T.PressEnter()
	}
	for {
		e.T.Println("")
		e.T.WriteRA("`A2:" + e.T.RalGet(lang.PlsDesc) + " `A3:")
		e.T.WriteRA(padName(name, 14) + "`A15::")
		s, err := e.T.GetString(40, false, false)
		if err != nil {
			return "", 0, false
		}
		s = pascal.Trim(s)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "/") {
			attrib = cfgrec.AttrUnlisted | cfgrec.AttrNotAvail
			s = strings.TrimSpace(s[1:])
		}
		if files.AreaLongDesc(area) {
			var lines []string
			if s != "" {
				lines = append(lines, s)
			}
			for len(lines) < 20 {
				more, err := e.T.GetString(70, false, false)
				if err != nil {
					break
				}
				more = pascal.Trim(more)
				if more == "" {
					break
				}
				lines = append(lines, more)
			}
			s = strings.Join(lines, "\n")
		}
		if pascal.Trim(s) == "" {
			continue
		}
		return s, attrib, true
	}
}

func padName(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
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
