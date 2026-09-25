package quest

import (
	"os"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/door"
	"elebbs/internal/logx"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
	"elebbs/internal/userbase"
)

func (q *vm) cmdExtra(cmd, rest string) {
	switch cmd {
	case "GOSUB":
		if len(q.gosub) < maxGoSub {
			q.gosub = append(q.gosub, q.pc)
		}
		q.gotoLabel(strings.TrimSpace(rest))
	case "RETURN":
		if n := len(q.gosub); n > 0 {
			q.pc = q.gosub[n-1]
			q.gosub = q.gosub[:n-1]
		}
	case "BREAK":
		q.skipWhile = true
	case "YIELD":
		time.Sleep(time.Millisecond)
	case "CALC":
		q.cmdCalc(rest)
	case "ORD":
		q.cmdOrd(rest)
	case "ASCII":
		q.cmdASCII(rest)
	case "LENGTH":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.put(atoi(w[0]), strconv.Itoa(len(q.get(atoi(w[1])))))
		}
	case "EXTRACTWORD":
		q.cmdExtractWord(rest)
	case "SUBSTRING":
		q.cmdSubstring(rest, false)
	case "SUBSTRINGVAR":
		q.cmdSubstring(rest, true)
	case "FILEDELETE":
		p := q.value(strings.TrimSpace(rest))
		p = online.ResolveSemaFile(q.g, p)
		_ = os.Remove(p)
	case "FILERESULT":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.put(atoi(w[1]), q.fileRes[atoi(w[0])])
		}
	case "GETENV":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.put(atoi(w[0]), os.Getenv(q.value(w[1])))
		}
	case "GETPARAMETER":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.put(atoi(w[0]), q.param(atoi(w[1])))
		}
	case "GETBBSOS":
		dst := atoi(strings.TrimSpace(rest))
		q.put(dst, "WIN32")
	case "GETVOLUMELABEL":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.put(atoi(w[0]), "")
		}
	case "GETRECORDINFO":
		w := strings.Fields(rest)
		if len(w) >= 2 && q.getInfo != nil {
			rec := atoi(q.value(w[0]))
			start := atoi(w[1])
			down := len(w) >= 3 && pascal.UpCase(w[2]) == "DOWN"
			q.getInfo(rec, start, down, q.put)
		}
	case "GETSYSTEMNAME":
		dst := atoi(strings.TrimSpace(rest))
		if q.g != nil {
			q.put(dst, q.g.RaConfig.SystemName)
		}
	case "GETTELNET":
		w := strings.Fields(rest)
		if len(w) >= 1 && q.line != nil {
			q.put(atoi(w[0]), q.line.TelnetFromIP)
		}
	case "GETGRAPH":
		dst := atoi(strings.TrimSpace(rest))
		if q.line != nil && q.line.AnsiOn {
			q.put(dst, "ANSI")
		} else {
			q.put(dst, "NONE")
		}
	case "GETFLAG":
		w := strings.Fields(rest)
		if len(w) >= 2 && q.line != nil {
			if q.line.User.Flags.HasNamed(w[1]) {
				q.put(atoi(w[0]), "YES")
			} else {
				q.put(atoi(w[0]), "NO")
			}
		}
	case "GETXY":
		w := strings.Fields(rest)
		if len(w) >= 2 && q.t != nil {
			q.curX = q.t.WhereX()
			q.curY = q.t.WhereY()
			q.put(atoi(w[0]), strconv.Itoa(q.curX))
			q.put(atoi(w[1]), strconv.Itoa(q.curY))
		}
	case "GETCHOICE":
		q.cmdGetChoice(rest)
	case "GETARROWKEY":
		q.cmdGetArrow(rest)
	case "GETRAWKEY":
		q.cmdGetRaw(rest)
	case "KEYPRESS":
		dst := atoi(strings.TrimSpace(rest))
		if _, ok := q.t.PeekKey(); ok {
			q.put(dst, "YES")
		} else {
			q.put(dst, "NO")
		}
	case "WASSYSOPKEY":
		dst := atoi(strings.TrimSpace(rest))
		if q.t != nil && q.t.FromSysop {
			q.put(dst, "YES")
		} else {
			q.put(dst, "NO")
		}
	case "SETRESULT":
		q.result = q.value(strings.TrimSpace(rest))
	case "SETRESULTVAR":
		q.result = q.get(atoi(strings.TrimSpace(rest)))
	case "DEFINEOUTPUT":
		q.outFile = q.value(strings.TrimSpace(rest))
	case "OUTPUTANSWER":
		n := atoi(strings.TrimSpace(rest))
		q.post = append(q.post, q.get(n)+"\r\n")
	case "COMMIT":
		q.commit()
	case "POSTINFO":
		name := ""
		if q.line != nil {
			name = q.line.User.Name
		}
		q.post = append(q.post, "*** "+name+" completed questionnaire at "+time.Now().Format("15:04")+" on "+time.Now().Format("01-02-06")+" ***\r\n")
	case "CHANGECOLOR":
		n, _ := firstWord(rest)
		q.t.WriteRA("`A" + strconv.Itoa(atoi(n)) + ":")
	case "CURSOR":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			if _, err1 := strconv.Atoi(w[0]); err1 == nil {
				if _, err2 := strconv.Atoi(w[1]); err2 == nil {
					q.curX = atoi(w[0])
					q.curY = atoi(w[1])
					q.t.WriteRA("`X" + strconv.Itoa(q.curX) + ":")
					q.t.WriteRA("`Y" + strconv.Itoa(q.curY) + ":")
					break
				}
			}
		}
		on := strings.Contains(strings.ToUpper(rest), "ON")
		if on {
			_ = q.t.WriteRaw([]byte("\x1b[?25h"))
		} else {
			_ = q.t.WriteRaw([]byte("\x1b[?25l"))
		}
	case "DISPLAYLOCAL":
		q.t.WriteRA(q.makeDisplayStr(rest))
	case "DOCONTINUE":
		// Pascal: StopMore → NO, else YES. It does not ask "Is this correct?"
		dst := atoi(strings.TrimSpace(rest))
		if q.t != nil && q.t.StopMore {
			q.put(dst, "NO")
		} else {
			q.put(dst, "YES")
		}
	case "EXEC":
		q.cmdExec(rest)
	case "EMULATEINPUT":
		q.t.PutInBuffer(q.value(strings.TrimSpace(rest)))
	case "EMULATESYSINPUT":
		q.t.PutSysopInBuffer(q.value(strings.TrimSpace(rest)))
	case "EMULATEVAR":
		q.t.PutInBuffer(q.get(atoi(strings.TrimSpace(rest))))
	case "EMULATESYSVAR":
		q.t.PutSysopInBuffer(q.get(atoi(strings.TrimSpace(rest))))
	case "MENUCMD", "MENUCMND":
		typWord, data := firstWord(rest)
		typ := atoi(typWord)
		if strings.HasPrefix(data, "#") {
			data = q.get(atoi(data[1:]))
		}
		if typ > 0 && q.t.RunMenu != nil {
			q.t.RunMenu(byte(typ), data)
		}
	case "PUSHX":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.curX = q.t.WhereX()
			q.savedX[n] = q.curX
		}
	case "PUSHY":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.curY = q.t.WhereY()
			q.savedY[n] = q.curY
		}
	case "PUSHCOLOR":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.savedAttr[n] = q.t.Attr
		}
	case "POPX":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.curX = q.savedX[n]
			if q.curX < 1 {
				q.curX = 1
			}
			q.t.WriteRA("`X" + strconv.Itoa(q.curX) + ":")
			q.curX = q.t.WhereX()
		}
	case "POPY":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.curY = q.savedY[n]
			q.t.WriteRA("`Y" + strconv.Itoa(q.curY) + ":")
		}
	case "POPCOLOR":
		n := atoi(strings.TrimSpace(rest))
		if n >= 1 && n <= 20 {
			q.t.WriteRA("`A" + strconv.Itoa(int(q.savedAttr[n])) + ":")
		}
	case "PAGEMODE":
		if q.line != nil {
			q.line.DispMorePrompt = strings.Contains(strings.ToUpper(rest), "ON")
		}
	case "SETFLAG":
		w := strings.Fields(rest)
		if len(w) >= 2 && q.line != nil {
			on := strings.EqualFold(w[1], "ON")
			q.line.User.Flags.SetNamed(w[0], on)
			state := "OFF"
			if on {
				state = "ON"
			}
			up := pascal.UpCase(w[0])
			for i := 0; i+1 < len(up); i += 2 {
				if up[i] >= 'A' && up[i] <= 'D' {
					logx.Write(q.g, q.line.RaNodeNr, '>', "Flag "+up[i:i+2]+" set "+state)
				}
			}
			_ = userbase.Write(q.g, q.line.User)
		}
	case "SETCOMMENT":
		if q.line != nil {
			q.line.User.Comment = q.value(strings.TrimSpace(rest))
			_ = userbase.Write(q.g, q.line.User)
		}
	case "SETOLM", "SETCHATREASON", "SETCHATWANTED", "SETSTATUSBAR":
		if q.g != nil && q.line != nil {
			logx.Write(q.g, q.line.RaNodeNr, '>', cmd+" "+q.value(rest))
		}
	case "SETUSERON":
		if q.g != nil && q.line != nil {
			desc := q.value(strings.TrimSpace(rest))
			if q.t != nil {
				desc = q.t.ExpandRA(desc)
			}
			_ = online.Write(q.g, q.line, desc, online.StatusBrowsing)
		}
	case "SETSECURITY":
		if q.line != nil {
			q.line.User.Security = uint16(atoi(q.value(strings.TrimSpace(rest))))
			_ = userbase.Write(q.g, q.line.User)
		}
	case "SETTIME":
		if q.line != nil {
			q.line.TimeLimit = uint16(atoi(q.value(strings.TrimSpace(rest))))
		}
	case "SETUSERVAR":
		q.cmdSetUserVar(rest)
	case "SETEDITOR":
		if q.g != nil {
			q.g.RaConfig.ExternalEd = q.value(strings.TrimSpace(rest))
		}
	case "CONVERTAREA":
		// area number stays as given
	case "DELETEMSG":
		if q.g != nil && q.line != nil {
			logx.Write(q.g, q.line.RaNodeNr, '>', "DELETEMSG "+rest)
		}
	case "CFG_FILEOPEN":
		q.cfgOpen(rest)
	case "CFG_FILECLOSE":
		q.cfgCloseSlot(atoi(strings.TrimSpace(rest)))
	case "CFG_FILEERROR":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			errn := 0
			if s := q.cfg[atoi(w[0])]; s != nil {
				errn = s.err
				s.err = 0
			}
			q.put(atoi(w[1]), strconv.Itoa(errn))
		}
	case "CFG_FILESEEK":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			q.cfgSeek(atoi(w[0]), atoi(q.value(w[1])))
		}
	case "CFG_FILEREAD":
		q.cfgRead(atoi(strings.TrimSpace(rest)))
	case "CFG_FILEWRITE":
		q.cfgWrite(atoi(strings.TrimSpace(rest)))
	case "CFG_FILEGETINFO", "CFG_GETINFO":
		w := strings.Fields(rest)
		if len(w) >= 3 {
			q.put(atoi(w[2]), q.cfgGet(atoi(w[0]), atoi(w[1])))
		}
	case "CFG_FILESETINFO", "CFG_SETINFO":
		w := strings.Fields(rest)
		if len(w) >= 3 {
			q.cfgSet(atoi(w[0]), atoi(w[1]), q.get(atoi(w[2])))
		}
	}
}

func (q *vm) cmdCalc(rest string) {
	w := strings.Fields(rest)
	if len(w) < 4 {
		return
	}
	dst, a, op, b := atoi(w[0]), atoi(w[1]), w[2], atoi(w[3])
	x, y := atoi(q.get(a)), atoi(q.get(b))
	var r int
	switch op {
	case "*":
		r = x * y
	case "/":
		if y == 0 {
			y = 1
		}
		r = x / y
	case "+":
		r = x + y
	case "-":
		r = x - y
	case "%":
		if y == 0 {
			y = 1
		}
		r = x % y
	}
	q.put(dst, strconv.Itoa(r))
}

func (q *vm) cmdOrd(rest string) {
	w := strings.Fields(rest)
	if len(w) == 0 {
		return
	}
	dst := atoi(w[0])
	src := dst
	if len(w) >= 2 {
		src = atoi(w[1])
	}
	s := q.get(src)
	if s == "" {
		q.put(dst, "0")
		return
	}
	q.put(dst, strconv.Itoa(int(s[0])))
}

func (q *vm) cmdASCII(rest string) {
	w := strings.Fields(rest)
	if len(w) < 2 {
		return
	}
	dst := atoi(w[0])
	tok := w[1]
	n := atoi(tok)
	if tok != "" && (tok[0] == '#' || tok[0] == '%') {
		n = atoi(q.get(atoi(tok[1:])))
	}
	q.put(dst, string(rune(n)))
}

func (q *vm) cmdExtractWord(rest string) {
	w := strings.Fields(rest)
	if len(w) < 3 {
		return
	}
	dst, src, nth := atoi(w[0]), atoi(w[1]), atoi(w[2])
	parts := strings.Fields(q.get(src))
	if nth >= 1 && nth <= len(parts) {
		q.put(dst, parts[nth-1])
		return
	}
	q.put(dst, "")
}

func (q *vm) cmdSubstring(rest string, fromVar bool) {
	w := strings.Fields(rest)
	if len(w) < 4 {
		return
	}
	dst := atoi(w[0])
	src := q.get(atoi(w[1]))
	start := atoi(w[2])
	n := atoi(w[3])
	if fromVar {
		start = atoi(q.get(atoi(w[2])))
		n = atoi(q.get(atoi(w[3])))
	}
	rs := []rune(src)
	if start < 1 {
		start = 1
	}
	i := start - 1
	if i >= len(rs) {
		q.put(dst, "")
		return
	}
	end := i + n
	if n < 0 || end > len(rs) {
		end = len(rs)
	}
	q.put(dst, string(rs[i:end]))
}

func (q *vm) cmdGetChoice(rest string) {
	dstWord, keys := firstWord(rest)
	dst := atoi(dstWord)
	keys = pascal.UpCase(q.value(keys))
	ch, _ := q.t.GetKey(0)
	up := pascal.UpCase(string(ch))
	if up != "" && strings.Contains(keys, up) {
		q.put(dst, up)
		return
	}
	q.put(dst, up)
}

func (q *vm) cmdGetArrow(rest string) {
	dst := atoi(strings.TrimSpace(rest))
	ch, err := q.t.GetKey(0)
	if err != nil {
		q.put(dst, "\x1b")
		return
	}
	if ch >= 'a' && ch <= 'z' {
		ch -= 32
	}
	q.put(dst, "")
	switch ch {
	case 127:
		q.put(dst, "DELETE")
		return
	case 22:
		n, _ := q.t.GetKey(0)
		if n == 9 {
			q.put(dst, "INSERT")
			return
		}
	case 27:
		q.putArrow(dst, q.t.GetArrowKeys())
		return
	}
	if ch == '\r' || ch == '\n' {
		ch = '|'
	}
	if q.get(dst) == "" {
		q.put(dst, strings.TrimSpace(string(ch)))
	}
}

func (q *vm) cmdGetRaw(rest string) {
	dst := atoi(strings.TrimSpace(rest))
	ch, err := q.t.GetKey(0)
	if err != nil {
		q.put(dst, "\x1b")
		return
	}
	q.put(dst, "")
	switch ch {
	case 127:
		q.put(dst, "DELETE")
		return
	case 22:
		n, _ := q.t.GetKey(0)
		if n == 9 {
			q.put(dst, "INSERT")
			return
		}
		ch = n
	case 27:
		q.putArrow(dst, q.t.GetArrowKeys())
		return
	}
	if ch == '\n' {
		ch = '\r'
	}
	if q.get(dst) == "" {
		q.put(dst, pascal.FromCP437([]byte{ch}))
	}
}

func (q *vm) putArrow(dst int, ak byte) {
	switch ak {
	case 'A':
		q.put(dst, "UP")
	case 'B':
		q.put(dst, "DOWN")
	case 'C':
		q.put(dst, "RIGHT")
	case 'D':
		q.put(dst, "LEFT")
	case 'H':
		q.put(dst, "HOME")
	case 'K':
		q.put(dst, "END")
	case 0x1b:
		q.put(dst, "\x1b")
	default:
		q.put(dst, "")
	}
}

func (q *vm) cmdExec(rest string) {
	rest = q.value(strings.TrimSpace(rest))
	if rest == "" {
		return
	}
	door.Run(q.t, q.g, q.line, nil, rest, true)
}

func (q *vm) param(n int) string {
	f := strings.Fields(q.params)
	if n >= 1 && n <= len(f) {
		return f[n-1]
	}
	return ""
}

func (q *vm) cmdSetUserVar(rest string) {
	code, val := firstWord(rest)
	if q.line == nil {
		return
	}
	val = q.value(val)
	u := &q.line.User
	switch strings.ToUpper(code) {
	case "L":
		u.Location = val
	case "H":
		u.Handle = val
	case "C":
		u.Comment = val
	case "V":
		u.VoicePhone = val
	case "D":
		u.DataPhone = val
	case "F":
		u.ForwardTo = val
	case "P":
		userbase.SetPassword(u, val, q.g != nil && q.g.RaConfig.StrictPwdChecking)
	}
	_ = userbase.Write(q.g, *u)
}

func (q *vm) commit() {
	if q.outFile == "" || len(q.post) == 0 {
		return
	}
	f, err := os.OpenFile(q.outFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, s := range q.post {
		_, _ = f.WriteString(s)
	}
	q.post = nil
}

func (q *vm) closeCfg() {
	for i := 1; i <= maxCfgFiles; i++ {
		q.cfgCloseSlot(i)
	}
}
