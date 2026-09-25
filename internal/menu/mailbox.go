package menu

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

func (e *Engine) checkMailBox(data string) {
	defer func() {
		if rec := recover(); rec != nil {
			node := 0
			if e.Line != nil {
				node = e.Line.RaNodeNr
			}
			logx.Write(e.G, node, '!', "Mailbox scan aborted")
			if e.T != nil {
				e.T.Println("")
				e.T.WriteRA("`A12:Mailbox scan aborted.")
				e.T.Println("")
			}
		}
	}()
	saveMore := true
	saveDisp := true
	if e.T != nil {
		saveMore = e.T.MorePrompt
		e.T.MorePrompt = false
		e.T.StopMore = false
	}
	if e.Line != nil {
		saveDisp = e.Line.DispMorePrompt
		e.Line.DispMorePrompt = false
	}
	// /MG = all message groups (Pascal CheckMsgAreaAccess Group=false).
	checkGroup := !strings.Contains(pascal.UpCase(data), "/MG")
	grp := e.Line.User.MsgGroup
	old := ""
	hits := mail.ScanMailbox(e.G, e.Line.User, checkGroup, grp, func(a cfgrec.MessageArea, idx int) {
		e.showChkMail(a, idx, &old)
	})
	e.T.Println("")
	e.T.Println("")
	if e.T != nil {
		e.T.MorePrompt = saveMore
		e.T.StopMore = false
	}
	if e.Line != nil {
		e.Line.DispMorePrompt = saveDisp
	}
	if len(hits) == 0 {
		e.T.WriteRA("`A7:" + e.T.RalGet(lang.NoNewMail))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	hdr := e.T.RalGet(lang.NewMail)
	e.T.WriteRA("`A10:" + hdr)
	e.T.Println("")
	e.T.WriteRA("`A02:" + strings.Repeat("\u2500", noColorLen(hdr)))
	e.T.Println("")
	for _, h := range hits {
		name := makeLen(h.Area.Name, 30)
		word := e.T.RalGet(lang.Message)
		if h.Count() != 1 {
			word = e.T.RalGet(lang.Messages2)
		}
		e.T.WriteRA("`A11:" + name + " `A14:: " + strconv.Itoa(h.Count()) + " " + word)
		e.T.Println("")
	}
	e.T.ResetLines(0)
	e.askReadMail(hits)
}

func (e *Engine) showChkMail(a cfgrec.MessageArea, idx int, old *string) {
	s := e.T.RalGet(lang.ChkMail)
	s = strings.ReplaceAll(s, "\u2642Y", a.Name)
	s = strings.ReplaceAll(s, "\u2642y", a.Name)
	s = strings.ReplaceAll(s, "\x0bY", a.Name)
	s = strings.ReplaceAll(s, "\x0by", a.Name)
	s = strings.ReplaceAll(s, "\u26421", strconv.Itoa(idx))
	s = strings.ReplaceAll(s, "\x0b1", strconv.Itoa(idx))
	if len(s) < len(*old) {
		s += strings.Repeat(" ", len(*old)-len(s))
	}
	for noColorLen(s) > 75 && s != "" {
		r := []rune(s)
		if len(r) == 0 {
			break
		}
		s = string(r[:len(r)-1])
	}
	*old = s
	e.T.WriteRA("`A14:`X1:" + s)
}

func (e *Engine) askReadMail(hits []mail.MailHit) {
	help := false
	if p, ok := term.FindTextFile(e.T, e.textPath(), "MAILHELP"); ok && p != "" {
		help = true
	}
	arg := "NO"
	if help {
		arg = "YES"
	}
	ch := byte(1)
	if e.Line.AnsiOn || e.Line.AvatarOn {
		res, ran := quest.Exec(e.T, e.G, e.Line, "MAILREAD "+arg+" /N", quest.ScriptOpts{NoLog: true})
		if ran {
			res = pascal.UpCase(pascal.Trim(res))
			if res != "" {
				ch = res[0]
			}
		}
	}
	if ch == 1 {
		e.T.WriteRA(e.T.RalGet(lang.ReadNow))
		s, _ := e.T.GetString(1, false, true)
		s = pascal.UpCase(pascal.Trim(s))
		if s == "" {
			ch = 'Y'
		} else {
			ch = s[0]
		}
	}
	yes := e.T.RalKeys(lang.Yes)
	if yes == "" {
		yes = "Y"
	}
	if !strings.Contains(yes, string(ch)) && ch != 'Y' && ch != 'R' {
		return
	}
	e.T.WriteRA("`A15:")
	e.T.Println(e.T.RalGet(lang.Yes))
	e.readMailboxHits(hits)
}

func (e *Engine) readMailboxHits(hits []mail.MailHit) {
	if e.T != nil {
		e.T.StopMore = false
		e.T.ResetLines(1)
	}
	var found []mail.MailHit
	for _, h := range hits {
		for _, n := range h.Msgs {
			found = append(found, mail.MailHit{Area: h.Area, Msgs: []int{n}})
		}
	}
	if len(found) == 0 {
		return
	}
	pause := true
	i := 0
	for i >= 0 && i < len(found) {
		h := found[i]
		art, ok := mail.ReadMsg(h.Area.JAMBase, h.Msgs[0])
		if !ok {
			i++
			continue
		}
		act := e.showMessage(h.Area, art, pause)
		mail.SetLastRead(h.Area.JAMBase, e.Line.User.Name, e.Line.User.Handle, art.Num)
		mail.SetReceived(h.Area.JAMBase, art.Num)
		switch act {
		case mailAgain:
			continue
		case mailLast:
			i--
			if i < 0 {
				e.endMailboxMsgs()
				return
			}
		case mailReply:
			e.writeMessage(h.Area, e.Line.User.Name, art.From, art.Orig, "Re: "+art.Subject, buildQuoteLines(e.G, art.To, art.From, art.Date.Format("01-02-06 15:04"), art.Body), true)
			if e.T != nil {
				e.T.StopMore = false
				e.T.ResetLines(1)
			}
			i++
		case mailEnter:
			e.writeMessage(h.Area, e.Line.User.Name, "", "", "", nil, false)
			i++
		case mailStop:
			return
		default:
			i++
		}
	}
	e.endMailboxMsgs()
}

func (e *Engine) endMailboxMsgs() {
	e.T.Println("")
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.EndMsgs))
	e.T.Println("")
	e.T.PressEnter()
}

func makeLen(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

func noColorLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '`' && i+1 < len(s) && ((s[i+1] >= 'A' && s[i+1] <= 'Z') || (s[i+1] >= 'a' && s[i+1] <= 'z')) {
			j := i + 2
			for j < len(s) && s[j] != ':' && s[j] != '`' {
				j++
			}
			if j < len(s) && s[j] == ':' {
				j++
			}
			i = j
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		n++
		i += sz
	}
	return n
}
