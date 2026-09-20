package menu

import (
	"strconv"
	"strings"
	"unicode"

	"elebbs/internal/cfgrec"
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
	if e.Line.AnsiOn {
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
	dt := art.Date.Format("01-02-06")
	tm := art.Date.Format("15:04")
	res, ran := quest.Exec(e.T, e.G, e.Line, script+" /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1:  a.Name,
			2:  strconv.Itoa(art.Num),
			3:  dt,
			4:  tm,
			5:  mail.AttrString(art),
			6:  art.From,
			7:  "",
			8:  art.To,
			9:  "",
			10: art.Subject,
		},
	})
	_ = res
	if !ran {
		e.T.Println("")
		e.T.WriteRA("`A15:" + e.T.RalGet(lang.Message) + " #" + strconv.Itoa(art.Num) + " - " + a.Name + "  " + mail.AttrString(art))
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Dt) + "`A10:" + dt + " " + tm)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.By) + "`A11:" + art.From)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.To3) + "`A11:" + art.To)
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Re) + "`A11:" + art.Subject)
		e.T.Println("")
		e.T.Println("")
	}
	for _, ln := range strings.Split(strings.ReplaceAll(art.Body, "\r\n", "\n"), "\n") {
		e.showMsgLine(ln)
	}
	return e.msgBar(a)
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

func (e *Engine) msgBar(a cfgrec.MessageArea) int {
	cmd := ""
	if mail.WriteAccessible(a, e.Line.User) {
		if a.MsgKinds != cfgrec.MsgKindROnly && a.MsgKinds != cfgrec.MsgKindNoReply {
			cmd += "REPLY,"
		}
		if a.MsgKinds != cfgrec.MsgKindROnly {
			cmd += "ENTER,"
		}
	}
	cmd = strings.Trim(cmd, ",")
	if e.Line.AnsiOn {
		res, ok := quest.Exec(e.T, e.G, e.Line, "MSGBAR "+cmd+" /N", quest.ScriptOpts{NoLog: true, Args: cmd})
		if ok {
			res = pascal.UpCase(pascal.Trim(res))
			if res != "" {
				return msgBarAction(res[0])
			}
		}
	}
	e.T.Println("")
	e.T.WriteRA("`A14:[N]ext [L]ast [A]gain [R]eply [E]nter [S]top: ")
	ch, err := e.T.GetKey(0)
	e.T.Println("")
	if err != nil {
		return mailStop
	}
	return msgBarAction(pascal.UpCase(string(ch))[0])
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
