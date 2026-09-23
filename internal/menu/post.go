package menu

import (
	"os"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
)

func (e *Engine) menuPost(data string) {
	up := pascal.UpCase(data)
	areaNr := ""
	w, rest := firstWord(data)
	if pascal.UpCase(w) == "/M" || strings.Contains(up, "/M") {
		areaNr = strconv.Itoa(int(e.Line.User.MsgArea))
	} else {
		areaNr = w
		data = rest
	}
	n := atoiMenu(areaNr)
	if n == 0 {
		e.changeMessageArea(data)
		n = int(e.Line.User.MsgArea)
		if n == 0 {
			return
		}
	}
	a, ok := mail.FindArea(e.Msgs, uint16(n))
	if !ok {
		a, ok = mail.FindArea(mail.LoadAll(e.G), uint16(n))
	}
	if !ok {
		e.T.WriteRA("`A12:" + e.T.RalStr(lang.InvArea))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	to := under2Norm(getMenuValue("/T=", data))
	subj := under2Norm(getMenuValue("/S=", data))
	okPost := e.writeMessage(a, e.Line.User.Name, to, subj, nil, false)
	if okPost && strings.Contains(up, "/L") {
		e.Hang = true
	}
}

func getMenuValue(key, s string) string {
	v, _ := getValue(key, s)
	return v
}

func under2Norm(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

func (e *Engine) writeMessage(a cfgrec.MessageArea, from, to, subj string, quote []string, reply bool) bool {
	if a.Name == "" || a.AreaNum == 0 {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "Invalid messageboard specified")
		return false
	}
	if !mail.WriteAccessible(a, e.Line.User) {
		e.T.Println("")
		e.T.WriteRA("`A3:" + e.T.RalGet(lang.NoWrite))
		e.T.Println("")
		e.T.PressEnter()
		return false
	}
	if a.Attribute&(1<<5) != 0 && pascal.Trim(e.Line.User.Handle) != "" {
		from = e.Line.User.Handle
	}
	e.T.ClearScreen()
	e.T.WriteRA("`A3:")
	e.T.Println("")
	kind := areaTypeName(e, a)
	if reply {
		e.T.WriteRA(e.T.RalGet(lang.Replying))
	} else {
		e.T.WriteRA(e.T.RalGet(lang.Posting))
	}
	e.T.WriteRA(" " + kind + " " + e.T.RalGet(lang.Area) + ` "` + a.Name + `".`)
	e.T.Println("")
	e.T.Println("")
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.From1) + "`A3:" + from)
	e.T.Println("")
	if a.Typ != cfgrec.MsgNews {
		to = e.askToWho(a, to)
		if to == "" {
			return false
		}
	} else if to == "" {
		to = "All"
	}
	if !reply || subj == "" {
		subj = e.askSubject(subj)
		if subj == "" {
			return false
		}
	} else {
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Subject1) + "`A3:" + subj)
		e.T.Println("")
	}
	if e.T.AskYesNo(lang.AskChange, false) {
		to, subj = e.changeHeader(a, to, subj)
		if to == "" || subj == "" {
			return false
		}
	}
	priv := false
	switch a.MsgKinds {
	case cfgrec.MsgKindBoth:
		if pascal.UpCase(to) != "ALL" {
			priv = e.T.AskYesNo(lang.Private1, false)
		}
	case cfgrec.MsgKindPrivate:
		priv = true
	}

	if e.T != nil {
		e.T.StopMore = false
		e.T.ResetLines(1)
	}
	lines := e.editMessage(fsedInfo{
		from: from,
		to:   to,
		subj: subj,
		area: a.Name,
		num:  nextMsgNum(a),
		priv: priv,
	}, quote, reply)
	if len(lines) == 0 {
		e.T.ClearScreen()
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.MsgAbort))
		e.T.Println("")
		e.T.PressEnter()
		return false
	}

	fAttach := false
	if a.AllowsAttach() && e.T.AskYesNo(lang.AttFiles1, false) {
		dir := mail.CreateAttachDir(e.G, e.Line.RaNodeNr)
		if dir == "" {
			logx.Write(e.G, e.Line.RaNodeNr, '!', "Unable to create attach directory!")
		} else {
			logx.Write(e.G, e.Line.RaNodeNr, '>', "Following message has files attached:")
			n := e.uploadAttach(dir)
			if n == 0 {
				logx.Write(e.G, e.Line.RaNodeNr, '!', "Did not receive any attaches, directory removed")
				_ = os.Remove(strings.TrimRight(dir, `\/`))
			} else {
				fAttach = true
				subj = pascal.ForceBack(dir)
			}
			logx.Write(e.G, e.Line.RaNodeNr, '>', "End of file attaches")
		}
	}

	if !a.IsJAM() || a.JAMBase == "" {
		e.T.Println("Hudson/Squish posting is not ported yet in this Go node.")
		e.T.PressEnter()
		return false
	}

	body := strings.Join(lines, "\r\n")
	tear := mail.TearLine()
	orig := mail.OriginLine(e.G, a)
	if a.Typ == cfgrec.MsgEchoMail || a.Typ == cfgrec.MsgLocal {
		body += "\r\n" + tear + "\r\n"
		if orig != "" {
			body += orig + "\r\n"
		}
	}
	attr := uint32(mailJamLocal | mailJamTypeLocal)
	switch a.Typ {
	case cfgrec.MsgEchoMail, cfgrec.MsgNews:
		attr = mailJamLocal | mailJamTypeEcho
	case cfgrec.MsgNetMail:
		attr = mailJamLocal | mailJamTypeNet
	}
	art := mail.Article{
		From:    from,
		To:      to,
		Subject: subj,
		Date:    time.Now(),
		Body:    body,
		Private: priv,
		FAttach: fAttach,
		Attr:    attr,
		Kludges: []string{
			"PID: " + cfgrec.PidName,
			mail.MsgIDKludge(e.G, a, 0),
		},
	}
	e.T.WriteRA("`A15:" + e.T.RalGet(lang.Saving))
	e.T.Println("")
	num, err := mail.AppendMsg(a.JAMBase, art)
	if err != nil {
		e.T.WriteRA("`A12:" + err.Error())
		e.T.Println("")
		e.T.PressEnter()
		return false
	}
	e.Line.User.MsgsPosted++
	e.saveUser()
	logx.Write(e.G, e.Line.RaNodeNr, '>', "Posted message #"+strconv.Itoa(num)+" in "+a.Name)
	return true
}

const (
	mailJamLocal     = 0x00000001
	mailJamTypeLocal = 0x00800000
	mailJamTypeEcho  = 0x01000000
	mailJamTypeNet   = 0x02000000
)

func (e *Engine) askToWho(a cfgrec.MessageArea, to string) string {
	if pascal.UpCase(to) == "SYSOP" && a.Typ != cfgrec.MsgNetMail && e.G != nil {
		to = e.G.RaConfig.Sysop
	}
	if to != "" {
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.To1) + "`A03:" + to)
		e.T.Println("")
		return to
	}
	max := 35
	if a.IsJAM() {
		max = 65
	}
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.To1))
	s, _ := e.T.GetString(max, false, e.G != nil && e.G.ElConfig.CapitalizeUsername && a.Typ != cfgrec.MsgInternet)
	s = pascal.Trim(s)
	e.T.Println("")
	if pascal.UpCase(s) == "SYSOP" && a.Typ != cfgrec.MsgNetMail && e.G != nil {
		s = e.G.RaConfig.Sysop
	}
	return s
}

func (e *Engine) askSubject(subj string) string {
	if subj != "" {
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Subject1) + "`A3:" + subj)
		e.T.Println("")
		return subj
	}
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.Subject1))
	s, _ := e.T.GetString(72, false, false)
	e.T.Println("")
	return pascal.Trim(s)
}

func (e *Engine) changeHeader(a cfgrec.MessageArea, to, subj string) (string, string) {
	e.T.Println("")
	e.T.WriteRA("`A03:" + e.T.RalGet(lang.ChngWhat))
	e.T.Println("")
	e.T.WriteRA(e.T.RalStr(lang.ToName1) + " " + e.T.RalStr(lang.SubAb))
	ch, err := e.T.GetKey(0)
	if err != nil {
		return to, subj
	}
	ch = pascal.UpCase(string(ch))[0]
	e.T.Println("")
	if ch == e.ralKey(lang.ToName1, 'T') {
		to = e.askToWho(a, "")
	}
	if ch == e.ralKey(lang.SubAb, 'S') {
		subj = e.askSubject("")
	}
	return to, subj
}

func areaTypeName(e *Engine, a cfgrec.MessageArea) string {
	switch a.Typ {
	case cfgrec.MsgNetMail:
		return e.T.RalGet(lang.Netmail1)
	case cfgrec.MsgEchoMail:
		return e.T.RalGet(lang.Echomail)
	case cfgrec.MsgInternet:
		return e.T.RalGet(lang.Internet)
	case cfgrec.MsgNews:
		return e.T.RalGet(lang.NewsGrp)
	case cfgrec.MsgForum:
		return e.T.RalGet(lang.ForumGrp)
	default:
		return e.T.RalGet(lang.Local1)
	}
}
