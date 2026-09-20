package menu

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

func (e *Engine) showUsersOnline(misc string, addCR bool) {
	if e.scriptWhosOnline(addCR) {
		return
	}
	e.T.ClearScreen()
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalStr(lang.Online) + " " + e.G.RaConfig.SystemName + "\r\n")
	e.T.Println("")
	e.T.WriteRA("`A10:" + e.T.RalGet(lang.OnlineHdr) + "\r\n")
	useHandle := strings.Contains(pascal.UpCase(misc), "/H")
	for _, rec := range online.ReadAll(e.G) {
		if !rec.Visible() {
			continue
		}
		name := rec.Name
		if useHandle {
			name = rec.Handle
		} else if h := pascal.Trim(rec.Handle); h != "" {
			name = h
		}
		name = clipOnline(name, 30)
		city := clipOnline(rec.City, 21)
		e.T.WriteRA("`X01:`A11:" + name +
			"`X31:`A15:" + rec.DisplayLine() +
			"`X38:`A15:" + strconv.Itoa(int(rec.Baud)) +
			"`X48:`A15:" + e.useronStatus(rec) +
			"`X59:`A14:" + city + "\r\n")
	}
	e.T.Println("")
	if addCR {
		e.T.PressEnter()
	}
}

func (e *Engine) scriptWhosOnline(addCR bool) bool {
	if quest.Kind(e.G, e.Line, "WHONLINE") == "" {
		return false
	}
	recs := online.ReadAll(e.G)
	total := 0
	for _, rec := range recs {
		if rec.Visible() {
			total++
		}
	}
	add := "NO"
	if addCR {
		add = "YES"
	}
	_, ok := quest.Exec(e.T, e.G, e.Line, "WHONLINE /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1: strconv.Itoa(total),
			2: add,
		},
		GetInfo: func(recordNum, start int, down bool, put func(n int, s string)) {
			e.whosOnlineHook(recs, recordNum, start, down, put)
		},
	})
	return ok
}

func (e *Engine) whosOnlineHook(recs []online.Record, recordNum, start int, down bool, put func(n int, s string)) {
	if recordNum < 1 {
		recordNum = 1
	}
	idx := recordNum - 1
	step := 1
	if !down {
		step = -1
	}
	var rec online.Record
	for {
		if idx < 0 || idx >= len(recs) {
			break
		}
		cand := recs[idx]
		idx += step
		if cand.Visible() {
			rec = cand
			break
		}
	}
	put(start, rec.Name)
	put(start+1, rec.Handle)
	put(start+2, rec.DisplayLine())
	put(start+3, strconv.Itoa(int(rec.Baud)))
	put(start+4, rec.City)
	put(start+5, e.useronStatus(rec))
	put(start+6, strconv.Itoa(int(rec.NoCalls)))
	put(start+7, strconv.Itoa(idx+1))
}

func (e *Engine) useronStatus(rec online.Record) string {
	switch rec.Status {
	case online.StatusBrowsing:
		return e.T.RalStr(lang.Browsing)
	case online.StatusXfer:
		return e.T.RalStr(lang.XFer)
	case online.StatusMsgs:
		return e.T.RalStr(lang.Messages1)
	case online.StatusDoor:
		return e.T.RalStr(lang.Door)
	case online.StatusChat:
		return e.T.RalStr(lang.SysopChat)
	case online.StatusQuest:
		return e.T.RalStr(lang.Questnr)
	case online.StatusConf:
		return e.T.RalStr(lang.Conf)
	case online.StatusLogon:
		return e.T.RalStr(lang.LogingOn)
	case online.StatusCustom:
		if rec.StatDesc != "" {
			return rec.StatDesc
		}
	}
	return e.T.RalStr(lang.UnKnown)
}

func (e *Engine) bbsSendMessage(lineNr int, misc string) {
	lineChosen := ""
	if lineNr == 0 || lineNr == 255 {
		e.showUsersOnline(misc, false)
	} else {
		e.T.Println("")
		e.T.Println("")
		lineChosen = strconv.Itoa(lineNr)
	}
	if lineNr == 0 {
		e.T.Println("")
		e.T.WriteRA("`A11:" + e.T.RalGet(lang.ChUser))
		s, err := e.T.GetString(6, false, false)
		e.T.Println("")
		if err != nil || pascal.Trim(s) == "" {
			return
		}
		lineChosen = pascal.Trim(s)
	}
	dest, err := strconv.Atoi(lineChosen)
	if err != nil || dest < 1 {
		return
	}
	rec, ok := e.checkOkToSend(dest, true)
	if !ok {
		return
	}
	useHandle := strings.Contains(pascal.UpCase(misc), "/H")
	nameToUse := rec.Name
	if useHandle {
		nameToUse = rec.Handle
	} else if h := pascal.Trim(rec.Handle); h != "" {
		nameToUse = h
	}
	hdr := e.T.RalGet(lang.MsgTo) + " " + nameToUse + " " + e.T.RalGet(lang.Node2) + " " + lineChosen
	lines := e.raEditor(hdr, nil)
	if lines == nil {
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.Aborted))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	sender := e.Line.User.Name
	if useHandle {
		sender = e.Line.User.Handle
	} else if h := pascal.Trim(e.Line.User.Handle); h != "" {
		sender = h
	}
	e.T.Println("")
	e.T.WriteRA(e.T.RalGet(lang.Process))
	logx.Write(e.G, e.Line.RaNodeNr, '>', "Sent on-line message to "+rec.Name+" (Line "+strconv.Itoa(int(rec.Line))+")")
	_ = online.AppendNodeMsg(e.G, dest, sender, e.Line.RaNodeNr, lines)
	e.T.WriteRA(e.T.RalGet(lang.MsgSent) + "\r\n")
	e.T.Println("")
	e.T.PressEnter()
}

func (e *Engine) checkOkToSend(lineNr int, showUser bool) (online.Record, bool) {
	rec, ok := online.ReadSlot(e.G, lineNr)
	if ok && (int(rec.Line) == lineNr || int(rec.NodeNumber) == lineNr) && pascal.Trim(rec.Name) != "" && !rec.Hidden() {
		if rec.Quiet() {
			if showUser {
				first := rec.Name
				if i := strings.IndexByte(first, ' '); i > 0 {
					first = first[:i]
				}
				e.T.WriteRA("`A12:" + first + " " + e.T.RalGet(lang.NoDisturb))
				e.T.Println("")
				sysop := pascal.UpCase(e.G.RaConfig.Sysop)
				if pascal.UpCase(e.Line.User.Name) == sysop || pascal.UpCase(e.Line.User.Handle) == sysop {
					e.T.Println("")
					if e.T.AskYesNo(lang.SysOverr, false) {
						return rec, true
					}
				}
				e.T.PressEnter()
			}
			return rec, false
		}
		return rec, true
	}
	if showUser {
		e.T.WriteRA(e.T.RalGet(lang.NoUser2))
		e.T.Println("")
		e.T.PressEnter()
	}
	return rec, false
}

func (e *Engine) checkNodeMsg() {
	if e == nil || e.G == nil || e.Line == nil || e.nodeCheckOff {
		return
	}
	now := time.Now()
	if !e.lastNodeCheck.IsZero() && now.Sub(e.lastNodeCheck) < 3*time.Second {
		return
	}
	e.lastNodeCheck = now
	if !online.NodeMsgReady(e.G, e.Line.RaNodeNr) {
		return
	}
	e.nodeCheckOff = true
	p := online.NodePath(e.G, e.Line.RaNodeNr)
	term.DisplayHotFile(e.T, filepath.Dir(p), p)
	online.ClearNodeMsg(e.G, e.Line.RaNodeNr)
	e.nodeCheckOff = false
}

func (e *Engine) writeUserOn(stat string, status int) {
	_ = online.Write(e.G, e.Line, stat, status)
}
