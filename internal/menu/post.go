package menu

import (
	"os"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/door"
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
	okPost := e.writeMessage(a, e.Line.User.Name, to, "", subj, nil, false)
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

// writeMessage is Pascal WriteMessage. toAddr is the netmail destination
// (a reply passes the original's origin address).
func (e *Engine) writeMessage(a cfgrec.MessageArea, from, to, toAddr, subj string, quote []string, reply bool) bool {
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
		to, toAddr = e.askToWho(a, to, toAddr)
		if to == "" {
			return false
		}
	} else if to == "" {
		to = "All"
	}
	if a.Typ == cfgrec.MsgNetMail && toAddr == "" {
		return false
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
		to, toAddr, subj = e.changeHeader(a, to, toAddr, subj)
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
	killSent, crash := false, false
	if a.Typ == cfgrec.MsgNetMail && e.G != nil {
		switch e.G.RaConfig.KillSent {
		case 0: // Yes
			killSent = true
		case 2: // Ask
			killSent = e.T.AskYesNo(lang.AskDelSnt, false)
		}
		sec := e.Line.User.Security
		if e.G.RaConfig.CrashAskSec <= sec {
			if e.G.RaConfig.CrashSec <= sec {
				crash = true
			} else {
				crash = e.T.AskYesNo(lang.AskCrash, false)
			}
		}
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

	e.T.WriteRA("`A15:" + e.T.RalGet(lang.Saving))
	e.T.Println("")
	num, err := e.saveArticle(a, from, to, subj, lines, postFlags{
		priv: priv, fAttach: fAttach, killSent: killSent, crash: crash, dest: toAddr,
	})
	if err != nil {
		e.T.WriteRA("`A12:" + err.Error())
		e.T.Println("")
		e.T.PressEnter()
		return false
	}
	e.Line.User.MsgsPosted++
	// A caller who failed the password (BadPwdArea comment) is not logged on;
	// their user record must not be rewritten.
	if e.Line.LoggedOn {
		e.saveUser()
	}
	logx.Write(e.G, e.Line.RaNodeNr, '>', "Posted message #"+strconv.Itoa(num)+" in "+a.Name)
	return true
}

// postFlags are the Pascal PostMessage options beyond the header.
type postFlags struct {
	priv, fAttach, killSent, crash bool
	dest                           string // netmail destination address
}

func (e *Engine) saveArticle(a cfgrec.MessageArea, from, to, subj string, lines []string, f postFlags) (int, error) {
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
	fromAddr := mail.AreaAka(e.G, a)
	dest := ""
	if a.Typ == cfgrec.MsgNetMail {
		dest = cfgrec.ParseAddr(f.dest, fromAddr).String()
	}
	origAddr := ""
	if !fromAddr.IsZero() {
		origAddr = fromAddr.String()
	}
	num, err := mail.AppendMsg(a.JAMBase, mail.Article{
		From:     from,
		To:       to,
		Subject:  subj,
		Date:     time.Now(),
		Body:     body,
		Private:  f.priv,
		FAttach:  f.fAttach,
		KillSent: f.killSent || a.Typ == cfgrec.MsgInternet,
		Crash:    f.crash,
		Orig:     origAddr,
		Dest:     dest,
		Attr:     attr,
		Kludges: []string{
			"PID: " + cfgrec.PidName,
			mail.MsgIDKludge(e.G, a, 0),
		},
	})
	if err == nil && e.Line != nil {
		switch a.Typ {
		case cfgrec.MsgNetMail:
			e.Line.NetMailEntered = true
		case cfgrec.MsgEchoMail:
			e.Line.EchoMailEntered = true
		}
	}
	return num, err
}

func (e *Engine) msgArea(nr int) (cfgrec.MessageArea, bool) {
	if nr <= 0 || nr > 0xFFFF {
		return cfgrec.MessageArea{}, false
	}
	if a, ok := mail.FindArea(e.Msgs, uint16(nr)); ok {
		return a, true
	}
	return mail.FindArea(mail.LoadAll(e.G), uint16(nr))
}

// WriteMessageTo is Pascal WriteMessage(area, ToWho, FromWho, '', ...) for
// hooks outside the menu engine (BadPwdArea comment at logon).
func (e *Engine) WriteMessageTo(areaNr int, toWho, from string) bool {
	a, ok := e.msgArea(areaNr)
	if !ok {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "Invalid messageboard specified")
		return false
	}
	return e.writeMessage(a, from, toWho, "", "", nil, false)
}

// FilePost is Pascal FilePost: post file (found like OpenRaFile, node
// directory first, then SysPath) as a private message, headed by addText.
// It returns false only when the file cannot be read.
func (e *Engine) FilePost(areaNr int, from, to, subj, file, addText string) bool {
	path := e.raFile(file)
	if path == "" {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var lines []string
	if addText != "" {
		lines = append(lines, addText)
	}
	text := strings.TrimRight(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\x1a")
	if text != "" {
		for _, l := range strings.Split(text, "\n") {
			lines = append(lines, e.T.ExpandRA(strings.TrimRight(l, "\r")))
		}
	}
	a, ok := e.msgArea(areaNr)
	if !ok {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "Invalid messageboard specified")
		return true
	}
	if !a.IsJAM() || a.JAMBase == "" {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "FilePost: only JAM areas are supported ("+a.Name+")")
		return true
	}
	num, err := e.saveArticle(a, from, to, subj, lines, postFlags{priv: true})
	if err != nil {
		logx.Write(e.G, e.Line.RaNodeNr, '!', "FilePost: "+err.Error())
		return true
	}
	logx.Write(e.G, e.Line.RaNodeNr, '>', "Posted message #"+strconv.Itoa(num)+" in "+a.Name)
	return true
}

func (e *Engine) raFile(name string) string {
	lower := strings.ToLower(name)
	dirs := []string{door.DropDir(e.G, e.Line)}
	if e.G != nil {
		dirs = append(dirs, e.G.RaConfig.SysPath)
	}
	return config.FindFile(dirs, lower, strings.ToUpper(name))
}

const (
	mailJamLocal     = 0x00000001
	mailJamTypeLocal = 0x00800000
	mailJamTypeEcho  = 0x01000000
	mailJamTypeNet   = 0x02000000
)

// askToWho is Pascal AskToWho: the addressee, and for netmail also the
// destination address. An empty name aborts the message.
func (e *Engine) askToWho(a cfgrec.MessageArea, to, addr string) (string, string) {
	netmail := a.Typ == cfgrec.MsgNetMail
	if !netmail || addr == "0:0/0" {
		addr = ""
	}
	if pascal.UpCase(to) == "SYSOP" && !netmail && e.G != nil {
		to = e.G.RaConfig.Sysop
	}
	if to != "" && (addr != "" || !netmail) {
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.To1) + "`A03:" + to)
		if addr != "" {
			e.T.WriteRA(" " + e.T.RalGet(lang.On2) + " (" + addr + ")")
		}
		e.T.Println("")
		return to, addr
	}
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.To1))
	s := to
	if s != "" {
		// A netmail reply to a message without an origin address.
		e.T.WriteRA("`A03:" + s)
	} else {
		max := 35
		if a.IsJAM() {
			max = 65
		}
		s, _ = e.T.GetString(max, false, e.G != nil && e.G.ElConfig.CapitalizeUsername && a.Typ != cfgrec.MsgInternet)
		s = pascal.Trim(s)
	}
	e.T.Println("")
	if s == "" {
		return "", addr
	}
	if netmail {
		addr = e.askAddress(addr, a)
		if addr == "" {
			return "", ""
		}
		e.T.Println("")
	}
	if pascal.UpCase(s) == "SYSOP" && !netmail && e.G != nil {
		s = e.G.RaConfig.Sysop
	}
	return s, addr
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

func (e *Engine) changeHeader(a cfgrec.MessageArea, to, addr, subj string) (string, string, string) {
	e.T.Println("")
	e.T.WriteRA("`A03:" + e.T.RalGet(lang.ChngWhat))
	e.T.Println("")
	e.T.WriteRA(e.T.RalStr(lang.ToName1) + " " + e.T.RalStr(lang.SubAb))
	ch, err := e.T.GetKey(0)
	if err != nil {
		return to, addr, subj
	}
	ch = pascal.UpCase(string(ch))[0]
	e.T.Println("")
	if ch == e.ralKey(lang.ToName1, 'T') {
		to, addr = e.askToWho(a, "", "")
	}
	if ch == e.ralKey(lang.SubAb, 'S') {
		subj = e.askSubject("")
	}
	return to, addr, subj
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
