package menu

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/door"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
	"elebbs/internal/userbase"
)

func getValue(id string, s string) (val, rest string) {
	up := pascal.UpCase(s)
	uid := pascal.UpCase(id)
	i := strings.Index(up, uid)
	if i < 0 {
		return "", s
	}
	after := s[i+len(id):]
	j := 0
	for j < len(after) && after[j] != ' ' && after[j] != '/' && after[j] != '*' {
		j++
	}
	val = after[:j]
	rest = strings.TrimSpace(s[:i] + after[j:])
	return val, rest
}

func stripMenuSwitches(data string) (ns bool, kbuf, rest string) {
	up := pascal.UpCase(data)
	if strings.Contains(up, "/NS") {
		ns = true
		data = strings.ReplaceAll(data, "/NS", "")
		data = strings.ReplaceAll(data, "/ns", "")
		data = strings.ReplaceAll(data, "/Ns", "")
	}
	kbuf, data = getValue("/K=", data)
	if indexStarU(data) >= 0 {
		_, data = getValue("*U", data)
	}
	return ns, kbuf, strings.TrimSpace(data)
}

func indexStarU(s string) int {
	up := pascal.UpCase(s)
	i := 0
	for {
		j := strings.Index(up[i:], "*U")
		if j < 0 {
			return -1
		}
		j += i
		if j == 0 || s[j-1] == ' ' || s[j-1] == '\t' {
			return j
		}
		i = j + 1
	}
}

func (e *Engine) textPath() string {
	if e.Line != nil && e.Line.Language.TextPath != "" {
		return e.Line.Language.TextPath
	}
	if e.G != nil {
		return e.G.RaConfig.TextPath
	}
	return "."
}

func (e *Engine) saveUser() {
	if e.G != nil && e.Line != nil {
		_ = userbase.Write(e.G, e.Line.User)
	}
}

// withoutMore is Pascal menu edit: no -- more -- after a one-line GetString.
func (e *Engine) withoutMore(fn func()) {
	saveMore, saveDisp := false, false
	if e.T != nil {
		saveMore = e.T.MorePrompt
		e.T.MorePrompt = false
		e.T.ResetLines(0)
	}
	if e.Line != nil {
		saveDisp = e.Line.DispMorePrompt
		e.Line.DispMorePrompt = false
	}
	fn()
	if e.T != nil {
		e.T.MorePrompt = saveMore
		e.T.ResetLines(1)
	}
	if e.Line != nil {
		e.Line.DispMorePrompt = saveDisp
	}
}

func (e *Engine) toggleBit(onMsg, offMsg int, attr *byte, bit byte, showOnOff bool) {
	on := *attr&bit != 0
	*attr ^= bit
	on = *attr&bit != 0
	e.T.Println("")
	onOff := "OFF"
	if on {
		onOff = "ON"
	}
	if e.T.Ral != nil {
		onOff = e.T.Ral.OnOff(on)
	}
	if showOnOff {
		e.T.WriteRA(e.T.RalStr(onMsg) + " " + onOff)
	} else if on {
		e.T.WriteRA(e.T.RalStr(onMsg))
	} else {
		e.T.WriteRA(e.T.RalStr(offMsg))
	}
	e.T.Println("")
	e.saveUser()
	e.T.PressEnter()
}

func (e *Engine) askPassword(want, menuName string) bool {
	want = pascal.UpCase(pascal.Trim(want))
	if want == "" {
		return true
	}
	e.T.Println("")
	e.T.WriteRA("`A12:" + e.T.RalGet(lang.Password))
	got, _ := e.T.GetString(15, true, true)
	got = pascal.UpCase(pascal.Trim(got))
	if got == want {
		return true
	}
	e.T.Println("")
	e.T.WriteRA("`A10:" + e.T.RalStr(lang.NoAccess))
	e.T.Println("")
	logx.Write(e.G, e.Line.RaNodeNr, '!', `Used password "`+got+`"`)
	logx.Write(e.G, e.Line.RaNodeNr, '!', `Attempted access to passworded menu "`+menuName+`"`)
	return false
}

func (e *Engine) gotoMenu(data string, gosub, clearStack bool, ns bool) {
	name, rest := firstWord(data)
	pass, _ := firstWord(rest)
	if name == "" {
		return
	}
	if !e.askPassword(pass, name) {
		return
	}
	if clearStack {
		e.Line.MenuStackPtr = 1
	} else if gosub {
		if e.Line.MenuStackPtr < cfgrec.MaxNestMenus {
			e.Line.MenuStackPtr++
		}
	}
	e.Line.MenuStack[e.Line.MenuStackPtr] = name
	if !ns {
		e.T.ClearScreen()
	}
	e.jump = true
}

func (e *Engine) ExecType(typ byte, data string) bool {
	ns, kbuf, data := stripMenuSwitches(data)
	if kbuf != "" {
		e.T.PutInBuffer(kbuf)
	}
	return e.doType(typ, data, ns)
}

func (e *Engine) doType(typ byte, data string, ns bool) bool {
	switch typ {
	case 0:
	case 1:
		e.gotoMenu(data, false, false, ns)
	case 2:
		e.gotoMenu(data, true, false, ns)
	case 3:
		if e.Line.MenuStackPtr > 1 {
			e.Line.MenuStackPtr--
		}
		if !ns {
			e.T.ClearScreen()
		}
		e.jump = true
	case 4:
		e.gotoMenu(data, false, true, ns)
	case 5:
		term.DisplayHotFile(e.T, e.textPath(), firstField(data))
	case 6:
		e.bulletMenu(data)
	case 7:
		e.raExec(data)
	case 8:
		e.showVersionInformation()
	case 9:
		e.Hang = true
		return false
	case 10:
		e.usageGraph()
	case 11:
		e.pageSysop(data)
	case 12:
		name, rest := firstWord(data)
		if name != "" && e.T.RunScript != nil {
			e.T.RunScript(name, rest)
		}
	case 13:
		e.showUserList(data)
	case 14:
		e.timeStats()
	case 15:
		e.Hang = true
		return false
	case 16:
		// Pascal MustEdit(Location, ralAskLoc, 25, CapLocation, min 1)
		e.withoutMore(func() {
			e.T.Println("")
			e.T.Println("")
			e.T.Println("")
			e.T.Println("")
			capLoc := e.G != nil && e.G.RaConfig.CapLocation
			for {
				e.T.WriteRA("`F14:`B0:")
				e.T.WriteRA(e.T.RalGet(lang.AskLoc))
				s, err := e.T.GetString(25, false, capLoc)
				if err != nil {
					return
				}
				s = pascal.Trim(s)
				if s != "" {
					e.Line.User.Location = s
					e.saveUser()
					break
				}
			}
			e.T.Println("")
		})
	case 17:
		e.changePassword()
	case 18:
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.AskLines))
		s, _ := e.T.GetString(2, false, false)
		n := atoiMenu(s)
		if n < 10 || n > 66 {
			n = 24
		}
		e.Line.User.ScreenLength = uint16(n)
		e.T.Length = n
		e.saveUser()
		e.T.Println(e.T.RalStr(lang.ScrChange) + " " + strconv.Itoa(n))
		e.T.PressEnter()
	case 19:
		e.toggleBit(lang.ClrAct, lang.ClrInact, &e.Line.User.Attribute, cfgrec.UserClrScr, false)
	case 20:
		e.toggleBit(lang.PagePause, lang.PagePause, &e.Line.User.Attribute, cfgrec.UserMore, true)
		e.Line.DispMorePrompt = e.Line.User.More()
	case 21:
		e.toggleBit(lang.AnsAct, lang.AnsInact, &e.Line.User.Attribute, cfgrec.UserANSI, false)
		e.Line.AnsiOn = e.Line.User.ANSI()
	case 22:
		e.checkMailBox(data)
	case 23:
		e.readMessages(data)
	case 24, 25:
		if a, ok := mail.FindArea(e.Msgs, e.Line.User.MsgArea); ok {
			mail.ListHeaders(e.T, a)
		} else {
			e.T.WriteRA(e.T.RalStr(lang.InvArea))
			e.T.Println("")
		}
		e.T.PressEnter()
	case 26:
		e.T.WriteRA(e.T.RalGet(lang.DelMsg))
		e.T.Println("")
		e.T.PressEnter()
	case 27:
		e.menuPost(data)
	case 28:
		e.selectCombined(data)
	case 29:
	case 30:
		e.dirList(data)
	case 31, 35, 36, 37:
		if a, ok := files.FindArea(e.Files, e.Line.User.FileArea); ok {
			files.ListArea(e.T, e.G, a)
		} else {
			e.T.WriteRA(e.T.RalStr(lang.InvArea))
			e.T.Println("")
		}
		e.T.PressEnter()
	case 32:
		e.download(data, false, false, false, true)
	case 48:
		e.download(data, false, true, false, false)
	case 55:
		e.download(data, false, false, true, true)
	case 59:
		e.download(data, true, false, false, true)
	case 33:
		e.upload(data)
	case 34:
		e.T.WriteRA(e.T.RalGet(lang.ViewName))
		name, _ := e.T.GetString(12, false, false)
		if name != "" {
			e.T.Println(e.T.RalStr(lang.InvViewNm))
		}
	case 38:
		term.DisplayHotFile(e.T, e.textPath(), firstField(data))
	case 39:
		// ShowDirectFile: MiscData is already a full path (MenuCmnd 39 #13).
		term.DisplayHotFile(e.T, e.textPath(), strings.TrimSpace(data))
	case 40:
		var keys map[byte]struct{}
		if !e.lbarFile {
			keys = hotKeyBytes(e.hot)
		}
		abort, _ := term.DisplayHot(e.T, e.textPath(), firstField(data), keys)
		if abort != 0 {
			e.choice = abort
		}
	case 41:
		e.toggleBit(lang.FSedAct, lang.FSedInact, &e.Line.User.Attribute, cfgrec.UserFSEd, false)
	case 42:
		e.toggleBit(lang.HotAct, lang.HotInact, &e.Line.User.Attribute2, cfgrec.User2HotKeys, false)
	case 43:
		e.T.WriteRA("`A15:" + e.T.RalStr(lang.MsgAreas))
		e.T.Println("")
		for _, a := range e.Msgs {
			if a.Name != "" {
				e.T.Println(fmt.Sprintf("  %3d  %s", a.AreaNum, a.Name))
			}
		}
		e.T.PressEnter()
	case 44:
		e.Line.User.Combined = [200]uint16{}
		e.saveUser()
	case 45, 46:
		term.DisplayFile(e.T, e.textPath(), firstField(data))
		e.T.PressEnter()
	case 47:
		msg := data
		if a, ok := files.FindArea(e.Files, e.Line.User.FileArea); ok {
			msg = strings.ReplaceAll(msg, "@", a.Name)
		}
		if a, ok := mail.FindArea(e.Msgs, e.Line.User.MsgArea); ok {
			msg = strings.ReplaceAll(msg, "`", a.Name)
		}
		logx.Write(e.G, e.Line.RaNodeNr, '>', msg)
	case 49:
		e.changeMessageArea(data)
	case 50:
		e.changeFileArea(data)
	case 51:
		e.todaysCallers()
	case 52:
		e.showUsersOnline(data, true)
	case 53:
		e.toggleBit(lang.Quiet, lang.Quiet, &e.Line.User.Attribute, cfgrec.UserQuiet, true)
		e.writeUserOn("", 0)
	case 54:
		e.bbsSendMessage(0, data)
	case 56:
		e.T.WriteRA(e.T.RalStr(lang.NoNLhelp))
		e.T.Println("")
		e.T.PressEnter()
	case 57:
		e.T.WriteRA(e.T.RalGet(lang.AskVoice))
		s, _ := e.T.GetString(15, false, false)
		if s != "" {
			e.Line.User.VoicePhone = s
			e.saveUser()
		}
	case 58:
		e.T.WriteRA(e.T.RalGet(lang.AskData))
		s, _ := e.T.GetString(15, false, false)
		if s != "" {
			e.Line.User.DataPhone = s
			e.saveUser()
		}
	case 60:
		e.withoutMore(func() {
			term.DisplayHotFile(e.T, e.textPath(), "HANDLE")
			for {
				e.T.Println("")
				e.T.WriteRA("`A9:")
				e.T.WriteRA(e.T.RalGet(lang.AskHandle))
				s, err := e.T.GetString(30, false, true)
				if err != nil {
					return
				}
				s = pascal.Trim(s)
				if s == "" {
					s = pascal.Trim(e.Line.User.Name)
				}
				if s != "" && pascal.UpCase(s) != pascal.UpCase(pascal.Trim(e.Line.User.Name)) {
					if u, ok := userbase.Search(e.G, s); ok && u.Record != e.Line.User.Record {
						e.T.Println("")
						e.T.WriteRA("`A12:" + e.T.RalStr(lang.InvHandle))
						continue
					}
				}
				if s != "" {
					e.Line.User.Handle = s
					e.saveUser()
					break
				}
			}
		})
	case 61:
		e.toggleBit(lang.AvtAct, lang.AvtInact, &e.Line.User.Attribute2, cfgrec.User2Avatar, false)
		e.Line.AvatarOn = e.Line.User.Attribute2&cfgrec.User2Avatar != 0
	case 62:
		e.toggleBit(lang.FSvAct, lang.FSvInAct, &e.Line.User.Attribute2, cfgrec.User2FSView, false)
	case 63:
		e.selectLanguage()
	case 64:
		e.askDateFormat()
	case 65:
		e.changeFlags(data)
	case 66:
		e.Line.TextfileShells = strings.Contains(pascal.UpCase(data), "ON")
	case 67:
		e.T.WriteRA(e.T.RalGet(lang.AskHandle))
		s, _ := e.T.GetString(35, false, true)
		e.Line.User.ForwardTo = s
		e.saveUser()
	case 68, 69, 70:
	case 71:
		e.viewTaggedFiles()
	case 72:
		e.selectProtocol()
	case 73:
		e.toggleBit(lang.IgnEchoM, lang.DisEchoM, &e.Line.User.Attribute2, cfgrec.User2NoEcho, false)
	case 74:
		e.T.WriteRA(e.T.RalGet(lang.AskLoc))
		s, _ := e.T.GetString(50, false, false)
		if s != "" {
			e.Line.User.Address1 = s
			e.saveUser()
		}
	case 75:
		e.toggleBit(lang.ScanSel, lang.ScanAll, &e.Line.User.Attribute3, cfgrec.User3MailScan, false)
	case 76:
		term.DisplayFile(e.T, e.textPath(), firstField(data))
	case 100:
		e.T.Println("IRC is not available on this node.")
		e.T.PressEnter()
	case 101:
		e.T.WriteRA(e.T.RalGet(lang.AskTelnet))
		_, _ = e.T.GetString(60, false, false)
		e.T.Println(e.T.RalStr(lang.NotAvail))
	default:
		e.T.Println("Menu type " + strconv.Itoa(int(typ)) + " is not yet ported.")
		e.T.PressEnter()
	}
	return true
}

func (e *Engine) bulletMenu(data string) {
	path := firstField(data)
	if path == "" {
		return
	}
	ents, err := os.ReadDir(path)
	if err != nil {
		e.T.Println(e.T.RalStr(lang.NotFnd))
		return
	}
	var files []string
	for _, ent := range ents {
		if !ent.IsDir() {
			files = append(files, ent.Name())
		}
	}
	for i, n := range files {
		e.T.Println(fmt.Sprintf("  %2d  %s", i+1, n))
	}
	e.T.WriteRA(e.T.RalGet(lang.Number))
	s, _ := e.T.GetString(4, false, false)
	n := atoiMenu(s)
	if n >= 1 && n <= len(files) {
		term.DisplayFile(e.T, path, files[n-1])
	}
}

func (e *Engine) raExec(data string) {
	door.Run(e.T, e.G, e.Line, e.Files, data, true)
}

func (e *Engine) showVersionInformation() {
	e.T.ClearScreen()
	e.T.Println("")
	e.T.Println("")
	e.T.WriteRA("`A15:" + cfgrec.FullProgName + " version v" + cfgrec.VersionID)
	e.T.Println("")
	e.T.Println("")
	e.T.WriteRA("`A11:Copyright 1996-2003 by Maarten Bekers, All rights reserved.")
	e.T.Println("")
	e.T.Println("")
	if e.G != nil && (e.G.RaConfig.Sysop != "" || e.G.RaConfig.SystemName != "") {
		e.T.WriteRA("`A14:Registered to: " + e.G.RaConfig.Sysop + ", " + e.G.RaConfig.SystemName)
		e.T.Println("")
	}
	e.T.Println("")
	e.T.WriteRA("`A12:JAM(mbp) - Copyright 1993 Joaquim Homrighausen, Andrew Milner,")
	e.T.Println("")
	e.T.WriteRA("`X27:Mats Birch, Mats Wallin.")
	e.T.Println("")
	e.T.WriteRA("`X27:ALL RIGHTS RESERVED.")
	e.T.Println("")
	e.T.Println("")
	e.T.PressEnter()
}

func (e *Engine) usageGraph() {
	e.T.ClearScreen()
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalStr(lang.PercUsage) + " " + e.G.RaConfig.SystemName)
	e.T.Println("")
	e.T.Println("  (hourly usage graph is not stored on this node)")
	e.T.PressEnter()
}

func (e *Engine) pageSysop(data string) {
	e.T.Println("")
	e.T.WriteRA(e.T.RalGet(lang.AskWhy))
	why, _ := e.T.GetString(60, false, false)
	logx.Write(e.G, e.Line.RaNodeNr, '!', "Page sysop: "+why+" "+data)
	e.T.Println(e.T.RalStr(lang.NotAvail))
	e.T.PressEnter()
}

func (e *Engine) showUserList(data string) {
	e.T.ClearScreen()
	e.T.Println("")
	e.T.WriteRA("`A14:" + e.T.RalGet(lang.UsrSearch))
	q, _ := e.T.GetString(35, false, false)
	q = pascal.UpCase(pascal.Trim(q))
	e.T.ClearScreen()
	users, err := userbase.List(e.G)
	if err != nil {
		e.T.Println(e.T.RalStr(lang.NoDataFil))
		return
	}
	e.T.Println(e.T.RalStr(lang.UsrLstHdr))
	shown := 0
	for _, u := range users {
		if u.Deleted() || u.Attribute2&cfgrec.User2Hidden != 0 {
			continue
		}
		if q != "" && !strings.Contains(pascal.UpCase(u.Name), q) && !strings.Contains(pascal.UpCase(u.Handle), q) {
			continue
		}
		e.T.Println(fmt.Sprintf("  %-28s  %s", u.Name, u.Location))
		shown++
	}
	if shown == 0 && q != "" {
		e.T.Println(q + " " + e.T.RalStr(lang.NoSuchUsr))
	}
	e.T.PressEnter()
}

func (e *Engine) timeStats() {
	now := time.Now()
	u := e.Line.User
	limit := e.Line.TimeLimit
	limStr := e.T.RalStr(lang.Unlimited)
	if limit != 32767 && limit != 0 {
		limStr = strconv.Itoa(int(limit)) + " " + e.T.RalStr(lang.Minutes)
	}
	e.T.Println("")
	e.T.Println(e.T.RalStr(lang.CurrTime) + now.Format("15:04"))
	e.T.Println(e.T.RalStr(lang.CuurDate) + now.Format("01-02-06"))
	e.T.Println(e.T.RalStr(lang.UsedTime) + strconv.Itoa(int(u.Elapsed)))
	e.T.Println(e.T.RalStr(lang.Rema2day) + limStr)
	e.T.Println(e.T.RalStr(lang.DailyLm1) + limStr)
	e.T.PressEnter()
}

func (e *Engine) changePassword() {
	e.getNewPassword(false, false)
}

// getNewPassword is Pascal GetNewPassword. Menu type 17 uses (false, false):
// current password, new password, matching verify.
func (e *Engine) getNewPassword(completeNew, pwCheck bool) {
	if e.Line != nil && (e.Line.GuestUser || e.Line.User.Guest()) {
		e.T.Println("")
		e.T.WriteRA("`A12:" + e.T.RalGet(lang.NGSC))
		e.T.PressEnter()
		return
	}
	if !completeNew && !pwCheck {
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.CurrPsw))
		got, _ := e.T.GetString(15, true, false)
		e.T.Println("")
		if !userbase.CheckPassword(e.Line.User, got, e.strictPwd()) {
			e.T.Println("")
			e.T.WriteRA(e.T.RalGet(lang.NoAccess))
			e.T.Println("")
			e.T.PressEnter()
			return
		}
	}
	if !pwCheck {
		term.DisplayHotFile(e.T, e.textPath(), "PassWord")
	}
	minLen := int(e.G.RaConfig.MinPwdLen)
	if minLen <= 0 {
		minLen = 4
	}
	strict := e.strictPwd()
	var pw string
	for pw == "" {
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.AskPsw1))
		pw, _ = e.T.GetString(15, true, false)
		e.T.Println("")
		if !strict {
			pw = pascal.UpCase(pw)
		}
		e.T.Println("")
		if strict && pw != "" && e.weakPassword(pw) {
			pw = ""
		}
		if pw == "" {
			e.T.WriteRA("`A14:" + e.T.RalGet(lang.InvPsw2))
			e.T.Println("")
		} else if len(pw) < minLen {
			e.T.WriteRA("`A14:" + e.T.RalGet(lang.PswShort1) + " " + strconv.Itoa(minLen) + " " + e.T.RalGet(lang.Chars))
			e.T.Println("")
			pw = ""
		} else if userbase.CheckPassword(e.Line.User, pw, strict) {
			e.T.WriteRA("`A12:" + e.T.RalGet(lang.OtherPsw))
			e.T.Println("")
			pw = ""
		}
		if pw == "" && !completeNew && !pwCheck {
			e.T.PressEnter()
			return
		}
		if pw == "" {
			continue
		}
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.AskPsw2))
		pw2, _ := e.T.GetString(15, true, false)
		e.T.Println("")
		if !strict {
			pw2 = pascal.UpCase(pw2)
		}
		if pw2 != pw {
			e.T.Println("")
			e.T.WriteRA("`A14:" + e.T.RalGet(lang.InvPsw1))
			e.T.Println("")
			pw = ""
			if !completeNew && !pwCheck {
				e.T.Println("")
				e.T.Println("")
				e.T.PressEnter()
				return
			}
		}
	}
	userbase.SetPassword(&e.Line.User, pw, strict)
	e.saveUser()
	if !completeNew {
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.PswChng2))
		e.T.Println("")
		e.T.PressEnter()
	}
}

func (e *Engine) strictPwd() bool {
	return e.G != nil && e.G.RaConfig.StrictPwdChecking
}

// weakPassword is GetNewPassword's StrictPwdChecking test: the password may
// not be listed in pwdtrash.ctl or equal the user's full, first or last name.
func (e *Engine) weakPassword(pw string) bool {
	up := pascal.UpCase(pascal.Trim(pw))
	name := pascal.UpCase(pascal.Trim(e.Line.User.Name))
	if up == name {
		return true
	}
	if words := strings.Fields(name); len(words) > 0 && (up == words[0] || up == words[len(words)-1]) {
		return true
	}
	return e.searchCtlFile("pwdtrash.ctl", pw)
}

// searchCtlFile is Pascal SearchCtlFile without wildcards: true when any line
// of the ctl file contains s (case-insensitive).
func (e *Engine) searchCtlFile(ctl, s string) bool {
	want := pascal.UpCase(pascal.Trim(s))
	if want == "" {
		return false
	}
	path := e.raFile(ctl)
	if path == "" {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(pascal.UpCase(pascal.Trim(strings.TrimRight(line, "\r"))), want) {
			return true
		}
	}
	return false
}

func (e *Engine) dirList(data string) {
	p := firstField(data)
	if p == "" {
		p = "."
	}
	ents, err := os.ReadDir(p)
	if err != nil {
		e.T.Println(e.T.RalStr(lang.NotFnd))
		return
	}
	for _, ent := range ents {
		info, _ := ent.Info()
		sz := int64(0)
		if info != nil {
			sz = info.Size()
		}
		e.T.Println(fmt.Sprintf("  %-12s %8d", ent.Name(), sz))
	}
	e.T.PressEnter()
}

func (e *Engine) todaysCallers() {
	e.T.ClearScreen()
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalStr(lang.Calls2day) + " " + e.G.RaConfig.SystemName)
	e.T.Println("")
	e.T.Println(e.T.RalStr(lang.CallsHdr))
	p := filepath.Join(e.G.RaConfig.SysPath, "lastcall.bbs")
	b, err := os.ReadFile(p)
	if err != nil {
		b, err = os.ReadFile(filepath.Join(e.G.RaConfig.SysPath, "LASTCALL.BBS"))
	}
	if err != nil {
		e.T.Println(e.T.RalStr(lang.NoDataFil))
		e.T.PressEnter()
		return
	}
	sz := cfgrec.LastCallSize
	if len(b)%cfgrec.LastCallSizeRA == 0 && len(b)%cfgrec.LastCallSize != 0 {
		sz = cfgrec.LastCallSizeRA
	}
	n := len(b) / sz
	for i := 0; i < n; i++ {
		rec := b[i*sz : (i+1)*sz]
		line := rec[0]
		name := ""
		if len(rec) > 1 {
			r := pascal.NewBuf(rec[1:])
			name = r.PString(35)
		}
		if name != "" {
			e.T.Println(fmt.Sprintf("  %-20s  %3d", name, line))
		}
	}
	e.T.PressEnter()
}

func clipOnline(s string, n int) string {
	s = pascal.Trim(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func onlineDisplayName(u cfgrec.User) string {
	if h := pascal.Trim(u.Handle); h != "" {
		return h
	}
	return u.Name
}

func (e *Engine) selectLanguage() {
	langs := config.ListLanguages(e.G)
	e.T.Println("")
	prompt := e.G.RaConfig.LanguagePrompt
	if prompt == "" {
		prompt = "Select language: "
	}
	for i, l := range langs {
		e.T.Println(fmt.Sprintf("  %2d  %s", i+1, l.Name))
	}
	e.T.WriteRA("`A15:" + prompt)
	s, _ := e.T.GetString(3, false, false)
	n := atoiMenu(s)
	if n < 1 || n > len(langs) {
		return
	}
	e.Line.User.Language = byte(n)
	e.saveUser()
	config.ApplyUserLanguage(e.G, e.Line)
	e.T.Ral = lang.Load(e.G, e.Line.Language)
}

func (e *Engine) askDateFormat() {
	e.T.Println("  1 DD-MM-YY")
	e.T.Println("  2 MM-DD-YY")
	e.T.Println("  3 YY-MM-DD")
	e.T.Println("  4 DD-Mmm-YY")
	e.T.Println("  5 DD-MM-YYYY")
	e.T.Println("  6 MM-DD-YYYY")
	e.T.Println("  7 YYYY-MM-DD")
	e.T.Println("  8 DD-Mmm-YYYY")
	e.T.WriteRA(e.T.RalGet(lang.Number))
	s, _ := e.T.GetString(1, false, false)
	n := atoiMenu(s)
	if n >= 1 && n <= 8 {
		e.Line.User.DateFormat = byte(n)
		e.saveUser()
	}
}

func (e *Engine) changeFlags(data string) {
	e.Line.User.Flags.ChangeMenu(data)
	e.saveUser()
}

func (e *Engine) selectProtocol() {
	prots := config.LoadProtocols(e.G)
	// Pascal: DontDisplay := (DisplayHotFile('XFERPROT', []) = #01)
	listMenu := !term.DisplayHotFile(e.T, e.textPath(), "XFERPROT")
	keys := ""
	for _, p := range prots {
		if p.ActiveKey == 0 {
			continue
		}
		keys += string([]byte{p.ActiveKey})
	}
	if listMenu {
		e.T.ClearScreen()
		e.T.WriteRA("`A3:" + e.T.RalGet(lang.SelProt))
		e.T.Println("")
		for _, p := range prots {
			e.T.WriteRA(fmt.Sprintf("`A15:(%c)  `A12:%s", p.ActiveKey, p.Name))
			if p.Batch {
				e.T.WriteRA("`X24:`A7:(*)")
			}
			e.T.Println("")
		}
		if keys == "" {
			e.T.Println("  @  Auto")
			keys = "@"
		}
		e.T.WriteRA("`A3:")
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.AstrBatch))
		e.T.Println("")
	} else if keys == "" {
		keys = "@"
	}
	e.T.WriteRA("`A3:")
	e.T.Println("")
	e.T.WriteRA(e.T.RalGet(lang.Protocol2))
	ch, _ := e.T.GetKey(0)
	e.T.FinishEnter(ch)
	up := pascal.UpCase(string(ch))
	if ch == '\r' || ch == '\n' || up == "" {
		return
	}
	if ch == '?' {
		term.DisplayHotFile(e.T, e.textPath(), "XFERHELP")
		return
	}
	if strings.Contains(pascal.UpCase(keys), up) {
		e.T.Println(up)
		if n := config.ProtocolName(e.G, up[0]); n != "" {
			e.T.WriteRA(n)
			e.T.Println("")
		}
		e.Line.User.DefaultProto = up[0]
		e.saveUser()
	}
}

func atoiMenu(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}
