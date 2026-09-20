package menu

import (
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

func flagVal(data, key string) (int, bool) {
	up := pascal.UpCase(data)
	k := pascal.UpCase(key)
	i := strings.Index(up, k)
	if i < 0 {
		return 0, false
	}
	return atoiMenu(data[i+len(key):]), true
}

func (e *Engine) changeMessageArea(data string) {
	if e.T != nil {
		e.T.ResetLines(0)
	}
	up := pascal.UpCase(data)
	checkGroup := strings.Contains(up, "/MG")
	onlyGroup := strings.Contains(up, "/OG")
	if onlyGroup && !strings.Contains(up, "/MG=0") {
		data += "/MG=0"
		up = pascal.UpCase(data)
	}
	ok := true
	if strings.Contains(up, "/MG=0") || e.Line.User.MsgGroup == 0 {
		ok = e.changeGroup(true, data)
	} else if n, set := flagVal(data, "/MG="); set && n > 0 {
		e.Line.User.MsgGroup = uint16(n)
	}
	if onlyGroup {
		e.saveUser()
		return
	}
	if !ok {
		return
	}
	all := mail.LoadAll(e.G)
	u := e.Line.User
	grp := u.MsgGroup
	if result, ran := e.runAreaChanger("MA-CHNG", all, u.MsgArea, checkGroup, grp); ran {
		e.applyMsgResult(all, result, checkGroup, grp)
	} else {
		e.fallbackSelArea(true, data)
	}
	e.saveUser()
	e.T.ResetLines(1)
	e.T.Println("")
	e.T.Println("")
}

func (e *Engine) changeFileArea(data string) {
	if e.T != nil {
		e.T.ResetLines(0)
	}
	up := pascal.UpCase(data)
	checkGroup := strings.Contains(up, "/FG")
	onlyGroup := strings.Contains(up, "/OG")
	if onlyGroup && !strings.Contains(up, "/FG=0") {
		data += "/FG=0"
		up = pascal.UpCase(data)
	}
	ok := true
	if strings.Contains(up, "/FG=0") || e.Line.User.FileGroup == 0 {
		ok = e.changeGroup(false, data)
	} else if n, set := flagVal(data, "/FG="); set && n > 0 {
		e.Line.User.FileGroup = uint16(n)
	}
	if onlyGroup {
		e.saveUser()
		return
	}
	if !ok {
		return
	}
	all := files.LoadAll(e.G)
	u := e.Line.User
	grp := u.FileGroup
	if result, ran := e.runFileChanger("FA-CHNG", all, u.FileArea, checkGroup, grp); ran {
		e.applyFileResult(all, result, checkGroup, grp)
	} else {
		e.fallbackSelArea(false, data)
	}
	e.saveUser()
	e.T.ResetLines(1)
	e.T.Println("")
	e.T.Println("")
}

func (e *Engine) changeGroup(message bool, data string) bool {
	if e.T != nil {
		e.T.ResetLines(0)
	}
	var all []cfgrec.Group
	name := "FG-CHNG"
	cur := e.Line.User.FileGroup
	if message {
		all = mail.LoadGroups(e.G)
		name = "MG-CHNG"
		cur = e.Line.User.MsgGroup
	} else {
		all = files.LoadAllGroups(e.G)
	}
	u := e.Line.User
	if result, ran := e.runGroupChanger(name, all, cur, u); ran {
		return e.applyGroupResult(message, all, result, u)
	}
	hot := "FGROUPS"
	if message {
		hot = "MGROUPS"
	}
	if !term.DisplayHotFile(e.T, e.textPath(), hot) {
		e.T.ClearScreen()
		e.T.Println("")
		e.T.WriteRA("`A15:")
		e.showGroupList(all, u)
	}
	e.T.Println("")
	e.T.WriteRA("`A15:" + e.T.RalStr(lang.SelGroup))
	s, _ := e.T.GetString(5, false, false)
	return e.applyGroupResult(message, all, pascal.Trim(s), u)
}

func (e *Engine) applyGroupResult(message bool, all []cfgrec.Group, result string, u cfgrec.User) bool {
	n := atoiMenu(result)
	if n <= 0 {
		return false
	}
	var gr cfgrec.Group
	ok := false
	if n >= 1 && n <= len(all) && groupOK(all[n-1], u) {
		gr, ok = all[n-1], true
	}
	if !ok {
		e.T.Println("")
		e.T.WriteRA("`A12:" + e.T.RalStr(lang.InvGrNo))
		e.T.Println("")
		e.T.PressEnter()
		return false
	}
	if message {
		e.Line.User.MsgGroup = gr.AreaNum
		e.Line.User.MsgArea = mail.SearchNext(mail.LoadAll(e.G), gr.AreaNum, e.Line.User.MsgArea)
	} else {
		e.Line.User.FileGroup = gr.AreaNum
		e.Line.User.FileArea = files.SearchNext(files.LoadAll(e.G), gr.AreaNum, e.Line.User.FileArea)
	}
	e.T.ResetLines(1)
	e.T.Println("")
	e.T.Println("")
	return true
}

func groupOK(gr cfgrec.Group, u cfgrec.User) bool {
	if gr.Name == "" || gr.AreaNum == 0 {
		return false
	}
	if !checkFlagAccess(u.Flags, gr.Flags, gr.NotFlags) {
		return false
	}
	return files.GroupAccess(gr, u)
}

func (e *Engine) showGroupList(all []cfgrec.Group, u cfgrec.User) {
	for _, gr := range all {
		if !groupOK(gr, u) {
			continue
		}
		e.T.Println(strconv.Itoa(int(gr.AreaNum)) + "  " + gr.Name)
	}
}

func (e *Engine) runAreaChanger(script string, all []cfgrec.MessageArea, cur uint16, checkGroup bool, group uint16) (string, bool) {
	total, raIdx, before := 0, mail.FileIndex(all, cur), 0
	u := e.Line.User
	for _, a := range all {
		if mail.Accessible(a, u, checkGroup, group) {
			total++
		}
		if a.AreaNum == cur {
			before = total
		}
	}
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		msgGetInfo(all, u, checkGroup, group, false, recordNum, start, down, put)
	}
	return quest.Exec(e.T, e.G, e.Line, script+" /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1: strconv.Itoa(total),
			2: strconv.Itoa(raIdx),
			3: strconv.Itoa(before),
		},
		GetInfo: hook,
	})
}

func (e *Engine) runFileChanger(script string, all []cfgrec.FilesArea, cur uint16, checkGroup bool, group uint16) (string, bool) {
	total, raIdx, before := 0, files.FileIndex(all, cur), 0
	u := e.Line.User
	for _, a := range all {
		if fileAccessible(a, u, checkGroup, group) {
			total++
		}
		if a.AreaNum == cur {
			before = total
		}
	}
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		fileGetInfo(all, u, checkGroup, group, recordNum, start, down, put)
	}
	return quest.Exec(e.T, e.G, e.Line, script+" /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1: strconv.Itoa(total),
			2: strconv.Itoa(raIdx),
			3: strconv.Itoa(before),
		},
		GetInfo: hook,
	})
}

func (e *Engine) runGroupChanger(script string, all []cfgrec.Group, cur uint16, u cfgrec.User) (string, bool) {
	total, raIdx, before := 0, 0, 0
	if cur != 0 {
		raIdx = mail.GroupIndex(all, cur)
	}
	for _, gr := range all {
		if groupOK(gr, u) {
			total++
		}
		if cur != 0 && gr.AreaNum == cur {
			before = total
		}
	}
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		groupGetInfo(all, u, recordNum, start, down, put)
	}
	return quest.Exec(e.T, e.G, e.Line, script+" /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1: strconv.Itoa(total),
			2: strconv.Itoa(raIdx),
			3: strconv.Itoa(before),
		},
		GetInfo: hook,
	})
}

// putInfoEOF matches Pascal GetInfoHook at end of file: FilePos DIV SizeOf
// is the record count when scanning down, 0 when scanning up past the start.
func putInfoEOF(nRecords int, down bool, start int, put func(int, string)) {
	idx := 0
	if down {
		idx = nRecords
	}
	put(start, strconv.Itoa(idx))
	put(start+1, "0")
	put(start+2, "")
	put(start+3, " ")
}

func msgGetInfo(all []cfgrec.MessageArea, u cfgrec.User, checkGroup bool, group uint16, combined bool, recordNum, start int, down bool, put func(int, string)) {
	i := recordNum - 1
	found := -1
	for i >= 0 && i < len(all) {
		if mail.Accessible(all[i], u, checkGroup, group) {
			found = i
			break
		}
		if down {
			i++
		} else {
			i--
		}
	}
	if found < 0 {
		putInfoEOF(len(all), down, start, put)
		return
	}
	a := all[found]
	put(start, strconv.Itoa(found+1))
	put(start+1, strconv.Itoa(int(a.AreaNum)))
	put(start+2, a.Name)
	mark := " "
	if combined && combinedOn(u, a.AreaNum) {
		mark = ">"
	} else if !combined && mail.HasNewMail(a, u.Name, u.Handle) {
		mark = "*"
	}
	put(start+3, mark)
}

func combinedOn(u cfgrec.User, area uint16) bool {
	for _, n := range u.Combined {
		if n == area {
			return true
		}
	}
	return false
}

func toggleCombined(u *cfgrec.User, area uint16) {
	if area == 0 {
		return
	}
	for i, n := range u.Combined {
		if n == area {
			u.Combined[i] = 0
			return
		}
	}
	for i, n := range u.Combined {
		if n == 0 {
			u.Combined[i] = area
			return
		}
	}
}

func fileGetInfo(all []cfgrec.FilesArea, u cfgrec.User, checkGroup bool, group uint16, recordNum, start int, down bool, put func(int, string)) {
	i := recordNum - 1
	found := -1
	for i >= 0 && i < len(all) {
		if fileAccessible(all[i], u, checkGroup, group) {
			found = i
			break
		}
		if down {
			i++
		} else {
			i--
		}
	}
	if found < 0 {
		putInfoEOF(len(all), down, start, put)
		return
	}
	a := all[found]
	put(start, strconv.Itoa(found+1))
	put(start+1, strconv.Itoa(int(a.AreaNum)))
	put(start+2, a.Name)
	put(start+3, " ")
}

func groupGetInfo(all []cfgrec.Group, u cfgrec.User, recordNum, start int, down bool, put func(int, string)) {
	i := recordNum - 1
	found := -1
	for i >= 0 && i < len(all) {
		if groupOK(all[i], u) {
			found = i
			break
		}
		if down {
			i++
		} else {
			i--
		}
	}
	if found < 0 {
		putInfoEOF(len(all), down, start, put)
		return
	}
	gr := all[found]
	put(start, strconv.Itoa(found+1))
	put(start+1, strconv.Itoa(int(gr.AreaNum)))
	put(start+2, gr.Name)
	put(start+3, " ")
}

func fileAccessible(a cfgrec.FilesArea, u cfgrec.User, checkGroup bool, group uint16) bool {
	if strings.TrimSpace(a.Name) == "" || a.AreaNum == 0 || strings.TrimSpace(a.FilePath) == "" {
		return false
	}
	if !files.ListAccess(a, u) {
		return false
	}
	if checkGroup && !files.AreaInGroup(a, group) {
		return false
	}
	return true
}

func (e *Engine) applyMsgResult(all []cfgrec.MessageArea, result string, checkGroup bool, group uint16) {
	n := atoiMenu(result)
	if n <= 0 {
		return
	}
	if n >= 1 && n <= len(all) && mail.Accessible(all[n-1], e.Line.User, checkGroup, group) {
		e.Line.User.MsgArea = all[n-1].AreaNum
		return
	}
	e.T.Println("")
	e.T.WriteRA("`A12:" + e.T.RalStr(lang.InvArea))
	e.T.Println("")
	e.T.PressEnter()
}

func (e *Engine) applyFileResult(all []cfgrec.FilesArea, result string, checkGroup bool, group uint16) {
	n := atoiMenu(result)
	if n <= 0 {
		return
	}
	if n >= 1 && n <= len(all) && fileAccessible(all[n-1], e.Line.User, checkGroup, group) {
		e.Line.User.FileArea = all[n-1].AreaNum
		return
	}
	e.T.Println("")
	e.T.WriteRA("`A12:" + e.T.RalStr(lang.InvArea))
	e.T.Println("")
	e.T.PressEnter()
}

func (e *Engine) selectCombined(data string) {
	up := pascal.UpCase(data)
	checkGroup := strings.Contains(up, "/MG")
	ok := true
	if strings.Contains(up, "/MG=0") || e.Line.User.MsgGroup == 0 {
		ok = e.changeGroup(true, data)
	} else if n, set := flagVal(data, "/MG="); set && n > 0 {
		e.Line.User.MsgGroup = uint16(n)
	}
	if !ok {
		return
	}
	all := mail.LoadAll(e.G)
	grp := e.Line.User.MsgGroup
	for {
		total, raIdx, before := 0, mail.FileIndex(all, e.Line.User.MsgArea), 0
		for _, a := range all {
			if mail.Accessible(a, e.Line.User, checkGroup, grp) {
				total++
			}
			if a.AreaNum == e.Line.User.MsgArea {
				before = total
			}
		}
		hook := func(recordNum, start int, down bool, put func(int, string)) {
			msgGetInfo(all, e.Line.User, checkGroup, grp, true, recordNum, start, down, put)
		}
		result, ran := quest.Exec(e.T, e.G, e.Line, "COMBINED /N", quest.ScriptOpts{
			NoLog: true,
			Answers: map[int]string{
				1: strconv.Itoa(total),
				2: strconv.Itoa(raIdx),
				3: strconv.Itoa(before),
			},
			GetInfo: hook,
		})
		if !ran {
			e.T.WriteRA("`A15:" + e.T.RalStr(lang.TogglArea))
			result, _ = e.T.GetString(80, false, false)
		}
		result = pascal.Trim(result)
		if result == "" {
			break
		}
		for _, w := range strings.Fields(result) {
			n := atoiMenu(w)
			if n >= 1 && n <= len(all) {
				toggleCombined(&e.Line.User, all[n-1].AreaNum)
			}
		}
		e.T.ResetLines(1)
	}
	e.saveUser()
	e.T.ResetLines(1)
	e.T.Println("")
	e.T.Println("")
}

func (e *Engine) fallbackSelArea(message bool, data string) {
	_ = data
	e.T.ClearScreen()
	e.T.Println("")
	if message {
		e.T.WriteRA("`A15:" + e.T.RalStr(lang.MsgAreas))
		e.T.Println("")
		e.T.Println("")
		e.Line.User.MsgArea = mail.SelectArea(e.T, e.Msgs, e.Line.User.MsgArea)
		return
	}
	e.T.WriteRA("`A15:" + e.T.RalStr(lang.FileAreas))
	e.T.Println("")
	e.T.Println("")
	e.Line.User.FileArea = files.SelectArea(e.T, e.G, e.Files, e.Line.User.FileArea)
}
