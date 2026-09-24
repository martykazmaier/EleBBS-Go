package menu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

const (
	mailNext = iota
	mailLast
	mailAgain
	mailReply
	mailEnter
	mailStop
)

func (e *Engine) readMessages(data string) {
	up := pascal.UpCase(data)
	w, _ := firstWord(data)
	n := atoiMenu(w)
	var areas []uint16
	if strings.Contains(up, "/M") {
		areas = []uint16{e.Line.User.MsgArea}
	} else if n == 0 {
		for _, a := range e.Line.User.Combined {
			if a != 0 {
				areas = append(areas, a)
			}
		}
	} else {
		areas = []uint16{uint16(n)}
	}
	combined := !strings.Contains(up, "/M") && n == 0
	if len(areas) == 0 {
		e.T.WriteRA("`A15:" + e.T.RalGet(lang.EndMsgs))
		e.T.Println("")
		e.T.PressEnter()
		return
	}

	ch := byte(0)
	pause := false
	start := 0
	forward := true
	individual := false
	newScan := false
	first := true

	for i := 0; i < len(areas); i++ {
		a, ok := mail.FindArea(e.Msgs, areas[i])
		if !ok {
			a, ok = mail.FindArea(mail.LoadAll(e.G), areas[i])
		}
		if !ok || a.Name == "" {
			continue
		}
		if !mail.Accessible(a, e.Line.User, false, 0) {
			if !combined {
				e.T.Println("")
				e.T.WriteRA(e.T.RalGet(lang.NoReadAcc))
				e.T.Println("")
				e.T.PressEnter()
				return
			}
			continue
		}
		if !a.IsJAM() || a.JAMBase == "" {
			if !combined {
				e.T.Println("Hudson/Squish message reading is not ported yet in this Go node.")
				e.T.PressEnter()
				return
			}
			continue
		}
		st := mail.Stats(a.JAMBase)
		if st.Active == 0 {
			if !combined {
				e.T.WriteRA("`A12:" + e.T.RalGet(lang.NoMsgs))
				e.T.Println("")
				e.T.PressEnter()
				return
			}
			continue
		}
		if first {
			ch = e.getReadType()
			if ch == 0 || ch == e.ralKey(lang.Quit4, 'Q') {
				return
			}
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Reading message area #"+strconv.Itoa(int(a.AreaNum))+" : "+a.Name)
			e.T.WriteRA("`A11:" + e.T.RalGet(lang.MsgArea) + ` "` + a.Name + `" ` + e.T.RalGet(lang.Contains) + " " + strconv.Itoa(st.Active) + " " + e.T.RalGet(lang.Messages2))
			e.T.Println("")
			e.T.WriteRA("`A15:" + e.T.RalGet(lang.MsgRange) + " 1 - " + strconv.Itoa(st.High))
			e.T.Println("")
			nk := e.ralKey(lang.NewMsgs1, 'N')
			ik := e.ralKey(lang.Individ1, 'I')
			rk := e.ralKey(lang.Reverse1, 'R')
			newScan = ch == nk
			individual = ch == ik
			forward = ch != rk
			if !newScan {
				prompt := lang.MsgStart
				if individual {
					prompt = lang.Nr2Read
				}
				start = e.rangeEdit(e.T.RalGet(prompt), 1, st.High, 0)
				if individual && start == 0 {
					return
				}
			}
			e.T.WriteRA("`A15:")
			e.T.ResetLines(1)
			pause = e.T.AskYesNo(lang.Pause, true)
			first = false
		} else {
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Reading message area #"+strconv.Itoa(int(a.AreaNum))+" : "+a.Name)
		}
		areaStart := start
		if newScan {
			areaStart = mail.LastRead(a.JAMBase, e.Line.User.Name, e.Line.User.Handle) + 1
		} else if !forward && areaStart == 0 {
			areaStart = st.High
		}
		if areaStart == 0 {
			areaStart = 1
		}
		if newScan && areaStart > st.High {
			continue
		}
		act := e.doReadMail(a, areaStart, forward, individual, pause, newScan)
		if act == mailStop {
			break
		}
		if act == mailLast {
			if i == 0 {
				break
			}
			i -= 2
			start = 0
			continue
		}
		start = 0
	}
	if combined {
		end := lang.EndMsgs
		if newScan {
			end = lang.EndNewMsg
		}
		e.T.WriteRA("`A15:" + e.T.RalGet(end))
		e.T.Println("")
		e.T.PressEnter()
	}
}

func (e *Engine) getReadType() byte {
	if e.Line.AnsiOn || e.Line.AvatarOn {
		res, ok := quest.Exec(e.T, e.G, e.Line, "READTYPE /N", quest.ScriptOpts{NoLog: true})
		if ok {
			res = pascal.UpCase(pascal.Trim(res))
			if res != "" {
				ch := res[0]
				e.sayReadType(ch)
				return ch
			}
		}
	}
	e.T.WriteRA("`A3:")
	e.T.Println("")
	e.T.WriteRA(e.T.RalStr(lang.Forward2) + ",  " + e.T.RalStr(lang.Reverse1) + ", " + e.T.RalStr(lang.Individ1) + ", " + e.T.RalStr(lang.Help3) + ",")
	e.T.Println("")
	e.T.WriteRA(e.T.RalStr(lang.Marked1) + ", " + e.T.RalStr(lang.NewMsgs1) + ",   " + e.T.RalStr(lang.Selected) + ", " + e.T.RalStr(lang.Quit4) + ".")
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.Select))
	good := map[byte]struct{}{
		e.ralKey(lang.Forward2, 'F'): {},
		e.ralKey(lang.Reverse1, 'R'): {},
		e.ralKey(lang.Individ1, 'I'): {},
		e.ralKey(lang.Marked1, 'M'):  {},
		e.ralKey(lang.NewMsgs1, 'N'): {},
		e.ralKey(lang.Selected, 'S'): {},
		e.ralKey(lang.Quit4, 'Q'):    {},
		e.ralKey(lang.Help3, 'H'):    {},
	}
	for {
		ch, err := e.T.GetKey(0)
		if err != nil {
			return e.ralKey(lang.Quit4, 'Q')
		}
		ch = pascal.UpCase(string(ch))[0]
		if _, ok := good[ch]; !ok {
			continue
		}
		if ch == e.ralKey(lang.Help3, 'H') {
			e.T.Println(e.T.RalGet(lang.Help3))
			term.DisplayHotFile(e.T, e.textPath(), "READHELP")
			continue
		}
		e.sayReadType(ch)
		return ch
	}
}

func (e *Engine) sayReadType(ch byte) {
	switch ch {
	case e.ralKey(lang.Forward2, 'F'):
		e.T.Println(e.T.RalGet(lang.Forward3))
	case e.ralKey(lang.Reverse1, 'R'):
		e.T.Println(e.T.RalGet(lang.Reverse2))
	case e.ralKey(lang.Individ1, 'I'):
		e.T.Println(e.T.RalGet(lang.Individ2))
	case e.ralKey(lang.NewMsgs1, 'N'):
		e.T.Println(e.T.RalGet(lang.NewMsgs2))
	case e.ralKey(lang.Quit4, 'Q'):
		e.T.Println(e.T.RalGet(lang.Quit4))
	default:
		e.T.Println("")
	}
}

func (e *Engine) doReadMail(a cfgrec.MessageArea, start int, forward, individual, pause, newScan bool) int {
	if !combinedScanNotice(e, a) {
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA("`A15:" + e.T.RalGet(lang.Scanning))
	}
	sysop := e.Line.User.Security >= a.SysopSecurity
	cur := start
	for {
		art, ok := mail.NextActive(a.JAMBase, cur, forward)
		if !ok {
			return mailNext
		}
		if newScan && art.Num < start {
			if forward {
				cur = art.Num + 1
			} else {
				cur = art.Num - 1
			}
			if cur < 1 {
				return mailNext
			}
			continue
		}
		if !mail.CanRead(art, e.Line.User, sysop) {
			if forward {
				cur = art.Num + 1
			} else {
				cur = art.Num - 1
				if cur < 1 {
					return mailNext
				}
			}
			continue
		}
		act := e.showMessage(a, art, pause)
		mail.SetLastRead(a.JAMBase, e.Line.User.Name, e.Line.User.Handle, art.Num)
		switch act {
		case mailAgain:
			cur = art.Num
			continue
		case mailLast:
			if forward {
				cur = art.Num - 1
				if cur < 1 {
					return mailLast
				}
				forward = false
			} else {
				cur = art.Num + 1
				forward = true
			}
		case mailReply:
			e.writeMessage(a, e.Line.User.Name, art.From, "Re: "+art.Subject, buildQuoteLines(e.G, art.To, art.From, art.Date.Format("01-02-06 15:04"), art.Body), true)
			if e.T != nil {
				e.T.StopMore = false
				e.T.ResetLines(1)
			}
			if forward {
				cur = art.Num + 1
			} else {
				cur = art.Num - 1
			}
			continue
		case mailEnter:
			e.writeMessage(a, e.Line.User.Name, "", "", nil, false)
			if e.T != nil {
				e.T.StopMore = false
				e.T.ResetLines(1)
			}
			if forward {
				cur = art.Num + 1
			} else {
				cur = art.Num - 1
			}
			continue
		case mailStop:
			return mailStop
		default:
			if individual {
				return mailNext
			}
			if forward {
				cur = art.Num + 1
			} else {
				cur = art.Num - 1
				if cur < 1 {
					return mailNext
				}
			}
		}
	}
}

func combinedScanNotice(e *Engine, a cfgrec.MessageArea) bool {
	_ = a
	return false
}

func (e *Engine) showMessage(a cfgrec.MessageArea, art mail.Article, pause bool) int {
	saveMore := e.T.MorePrompt
	saveDisp := e.Line.DispMorePrompt
	e.T.MorePrompt = pause
	e.Line.DispMorePrompt = pause
	e.T.StopMore = false
	e.T.ResetLines(1)
	if pause {
		// Pascal ClearTheScreen(True) before RDMSGPS: the template is
		// drawn with "|" from the current row; Cursor X Y then fills fields.
		e.T.ClearScreen()
		e.T.GotoXY(1, 1)
	}
	defer func() {
		e.T.MorePrompt = saveMore
		e.Line.DispMorePrompt = saveDisp
		e.T.StopMore = false
	}()

	script := "RDMSG"
	if pause {
		script = "RDMSGPS"
	}
	dtFmt := byte(0)
	if e.Line != nil {
		dtFmt = e.Line.User.DateFormat
	}
	dt := lang.FormatDate(art.Date, dtFmt, e.T.Ral)
	tm := art.Date.In(time.Local).Format("15:04")
	subj := art.Subject
	if art.FAttach {
		subj = e.T.RalGet(lang.AttFiles2)
	}
	attr := e.attr2Str(art)
	res, ran := quest.Exec(e.T, e.G, e.Line, script+" /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1:  a.Name,
			2:  strconv.Itoa(art.Num),
			3:  dt,
			4:  tm,
			5:  attr,
			6:  art.From,
			7:  "",
			8:  art.To,
			9:  "",
			10: subj,
		},
	})
	_ = res
	if !ran {
		e.T.Println("")
		e.T.WriteRA("`A15:" + e.T.RalGet(lang.Message) + " #" + strconv.Itoa(art.Num) + " - " + a.Name + "  " + attr)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Dt) + "`A10:" + dt + " " + tm)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.By) + "`A11:" + art.From)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.To3) + "`A11:" + art.To)
		e.T.Println("")
		if art.FAttach {
			e.T.WriteRA("`A14:" + e.T.RalGet(lang.Re) + "`A15:" + subj)
		} else {
			e.T.WriteRA("`A14:" + e.T.RalGet(lang.Re) + "`A11:" + subj)
		}
		e.T.Println("")
		e.T.Println("")
	}
	// Pascal GetString(79) after the header: count rows already used so
	// -- More -- fires for the rest of the screen, not a full Length of body.
	if pause {
		if y := e.T.WhereY(); y > 1 {
			e.T.ResetLines(y - 1)
		}
		e.T.MorePrompt = true
		e.Line.DispMorePrompt = true
		e.T.StopMore = false
	}
	for _, ln := range mail.WrapLines(art.Body, 79) {
		e.showMsgLine(ln)
	}
	return e.msgBar(a, art)
}

func (e *Engine) showMsgLine(ln string) {
	trim := strings.TrimLeft(ln, " \t")
	switch {
	case strings.Contains(ln[:min(len(ln), 15)], ">"):
		e.T.WriteRA("`A7:")
		e.T.Print(ln)
		e.T.WriteRA("`A3:")
		e.T.Println("")
	case strings.HasPrefix(trim, "---") || strings.HasPrefix(ln, " * Origin:"):
		e.T.WriteRA("`A2:")
		e.T.Print(ln)
		e.T.WriteRA("`A3:")
		e.T.Println("")
	default:
		e.T.Println(ln)
	}
}

func (e *Engine) msgBar(a cfgrec.MessageArea, art mail.Article) int {
	cmd := ""
	if mail.WriteAccessible(a, e.Line.User) {
		if a.MsgKinds != cfgrec.MsgKindROnly && a.MsgKinds != cfgrec.MsgKindNoReply {
			cmd += "REPLY,"
		}
		if a.MsgKinds != cfgrec.MsgKindROnly {
			cmd += "ENTER,"
		}
	}
	hasFiles := art.FAttach
	if hasFiles {
		if cmd != "" {
			cmd += ","
		}
		cmd += "FILEATTACH"
	}
	cmd = strings.Trim(cmd, ",")
	if e.Line.AnsiOn || e.Line.AvatarOn {
		res, ok := quest.Exec(e.T, e.G, e.Line, "MSGBAR "+cmd+" /N", quest.ScriptOpts{NoLog: true, Args: cmd})
		if ok {
			res = pascal.UpCase(pascal.Trim(res))
			if res != "" {
				if hasFiles && e.isFilesBarKey(res) {
					e.listFAttach(a, art)
					return mailAgain
				}
				return msgBarAction(res[0])
			}
		}
	}
	e.T.Println("")
	prompt := "`A14:[N]ext [L]ast [A]gain [R]eply [E]nter [S]top: "
	if hasFiles {
		prompt = "`A14:[N]ext [L]ast [A]gain [R]eply [E]nter [F]iles [S]top: "
	}
	e.T.WriteRA(prompt)
	ch, err := e.T.GetKey(0)
	e.T.Println("")
	if err != nil {
		return mailStop
	}
	up := pascal.UpCase(string(ch))[0]
	if hasFiles && up == e.ralKey(lang.Files2, 'F') {
		e.listFAttach(a, art)
		return mailAgain
	}
	return msgBarAction(up)
}

func (e *Engine) isFilesBarKey(res string) bool {
	if res == "" {
		return false
	}
	if strings.Contains(res, "FILEATTACH") || strings.HasPrefix(res, "FILES") {
		return true
	}
	return res[0] == e.ralKey(lang.Files2, 'F')
}

func (e *Engine) listFAttach(a cfgrec.MessageArea, art mail.Article) {
	e.T.ClearScreen()
	e.T.WriteRA("`A10:" + e.T.RalGet(lang.FilesAtt))
	e.T.Println("")
	e.T.WriteRA("`A15:")
	e.T.Println("")
	hdr := e.T.RalGet(lang.FileHdr)
	if strings.TrimSpace(hdr) == "" {
		hdr = "Filename      Size        Date"
	}
	e.T.WriteRA("`A14:" + hdr)
	e.T.Println("")
	e.T.WriteRA("`A14:-------------- ----------- -----------")
	e.T.Println("")
	paths := mail.AttachFiles(art.Subject)
	n := 0
	var sum int64
	var tagged []files.Found
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		n++
		sum += st.Size()
		name := filepath.Base(p)
		if len(name) < 14 {
			name += strings.Repeat(" ", 14-len(name))
		}
		e.T.WriteRA("`A11:" + name + " ")
		e.T.WriteRA("`A10:`X16:" + strconv.FormatInt(st.Size(), 10))
		e.T.WriteRA("`A9:`X28:" + st.ModTime().Format("01-02-06"))
		e.T.Println("")
		tagged = append(tagged, files.Found{
			Hdr:  cfgrec.FilesHdr{Name: filepath.Base(p), Size: uint32(st.Size()), Attrib: 1 << 2},
			Path: p,
		})
	}
	e.T.WriteRA("`A14:-------------- ----------- ---------")
	e.T.Println("")
	if n == 0 {
		e.T.WriteRA("`A12:")
		e.T.Println(e.T.RalGet(lang.NoFiles1))
		e.T.Println("")
		e.T.PressEnter()
		mail.ClearFAttach(a.JAMBase, art.Num)
		return
	}
	e.T.WriteRA("`A15:" + strconv.Itoa(n) + " " + e.T.RalGet(lang.Files1) + "`X16:" + strconv.FormatInt(sum, 10) + " " + e.T.RalGet(lang.Bytes))
	e.T.Println("")
	e.T.Println("")
	e.T.PressEnter()
	e.T.WriteRA("`A12:")
	e.T.Println("")
	e.transferDownloads(tagged)
	if e.canKillAttach(a, art) && e.T.AskYesNo(lang.CheckAtt, false) {
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.KillAtt))
		e.T.Println("")
		for _, f := range tagged {
			_ = os.Remove(f.Path)
		}
		_ = os.Remove(strings.TrimRight(art.Subject, `\/`))
		mail.ClearFAttach(a.JAMBase, art.Num)
	}
}

func (e *Engine) canKillAttach(a cfgrec.MessageArea, art mail.Article) bool {
	if e.Line == nil {
		return false
	}
	u := e.Line.User
	up := func(s string) string { return pascal.UpCase(pascal.Trim(s)) }
	name := up(u.Name)
	if name != "" && (name == up(art.From) || name == up(art.To)) {
		return true
	}
	if e.G != nil && name != "" && name == up(e.G.RaConfig.Sysop) {
		return true
	}
	return u.Security >= a.SysopSecurity && a.SysopSecurity > 0
}

// attr2Str is Pascal Attr2Str: RAL flags for RDMSG/RDMSGPS answer #5.
func (e *Engine) attr2Str(art mail.Article) string {
	var b strings.Builder
	add := func(nr int) {
		s := e.T.RalGet(nr)
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	if art.Private {
		add(lang.Private2)
	}
	if art.Received {
		add(lang.Received1)
	}
	if art.FAttach {
		add(lang.FileAtt1)
	}
	if art.Attr&0x00000020 != 0 {
		add(lang.KillSent1)
	}
	if art.Attr&0x00000100 != 0 {
		add(lang.Crash1)
	}
	if art.Attr&0x00010000 != 0 {
		add(lang.ReqRec1)
	}
	if art.Attr&0x00020000 != 0 {
		add(lang.AuditReq)
	}
	return b.String()
}

func msgBarAction(ch byte) int {
	switch ch {
	case 'L', '-':
		return mailLast
	case 'A':
		return mailAgain
	case 'R':
		return mailReply
	case 'E':
		return mailEnter
	case 'S', 'Q':
		return mailStop
	default:
		return mailNext
	}
}

func (e *Engine) ralKey(nr int, def byte) byte {
	if e.T != nil && e.T.Ral != nil {
		if k := e.T.Ral.GetKey(nr); k != 0 {
			return k
		}
	}
	return def
}

func (e *Engine) rangeEdit(prompt string, lo, hi, def int) int {
	e.T.WriteRA("`A15:" + prompt + " ")
	lb, rb := "[", "]"
	if e.G != nil && e.G.RaConfig.LeftBracket != 0 {
		lb = string(e.G.RaConfig.LeftBracket)
		rb = string(e.G.RaConfig.RightBracket)
	}
	e.T.Print(lb + strconv.Itoa(lo) + "-" + strconv.Itoa(hi) + rb + ": ")
	s, _ := e.T.GetString(6, false, false)
	s = pascal.Trim(s)
	if s == "" {
		return def
	}
	n := atoiMenu(s)
	if n < lo || n > hi {
		return def
	}
	return n
}

func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		if len(r) > 0 {
			b.WriteRune(unicode.ToUpper(r[0]))
		}
	}
	if b.Len() == 0 {
		return " "
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
