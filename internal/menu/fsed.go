package menu

import (
	"os"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/door"
	"elebbs/internal/lang"
	"elebbs/internal/mail"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
)

const maxEditLines = 400

type fsedInfo struct {
	from, to, subj, area string
	num                  int
	priv                 bool
}

func (e *Engine) useExternalEditor() bool {
	if e.G == nil || pascal.Trim(e.G.RaConfig.ExternalEd) == "" {
		return false
	}
	return e.Line != nil && e.Line.User.Attribute&cfgrec.UserFSEd != 0
}

func (e *Engine) editMessage(info fsedInfo, quote []string, reply bool) []string {
	if e.T != nil {
		e.T.StopMore = false
		e.T.ResetLines(1)
	}
	if e.useExternalEditor() {
		cmd := pascal.Trim(e.G.RaConfig.ExternalEd)
		if pascal.UpCase(cmd) == "INTERNAL" {
			return e.internalFSED(info, quote)
		}
		return e.externalEditor(cmd, info, quote)
	}
	if reply && len(quote) > 0 && !e.T.AskYesNo(lang.AskQuote, true) {
		quote = nil
	}
	return e.raEditor(e.T.RalGet(lang.BeginMsg), quote)
}

func (e *Engine) internalFSED(info fsedInfo, quote []string) []string {
	f := &fsed{
		e:      e,
		info:   info,
		quotes: quote,
		minX:   1,
		maxX:   80,
		minY:   4,
		maxY:   20,
		col:    1,
		row:    4,
		insert: true,
	}
	return f.run()
}

type fsed struct {
	e                      *Engine
	info                   fsedInfo
	lines, quotes          []string
	minX, maxX, minY, maxY int
	col, row, line         int
	insert                 bool
	quoting, readOnly      bool
	saveLines              []string
	saveMinY, saveMaxY     int
	saveCol, saveRow       int
	saveLine               int
}

func (f *fsed) run() []string {
	t := f.e.T
	saveMore := t.MorePrompt
	saveDisp := true
	if f.e.Line != nil {
		saveDisp = f.e.Line.DispMorePrompt
		f.e.Line.DispMorePrompt = false
	}
	t.MorePrompt = false
	t.StopMore = false
	defer func() {
		t.MorePrompt = saveMore
		t.StopMore = false
		if f.e.Line != nil {
			f.e.Line.DispMorePrompt = saveDisp
		}
	}()

	t.ClearScreen()
	f.showHeader()
	f.minY = t.WhereY()
	if f.minY < 2 {
		f.minY = 4
	}
	f.row = f.minY
	f.col = f.minX
	f.showFooter()
	if y := t.WhereY(); y > f.minY {
		f.maxY = y
	}
	if f.maxY <= f.minY {
		f.maxY = f.minY + 16
		if f.maxY > 24 {
			f.maxY = 24
		}
	}
	f.showTextBuffer()
	f.updateCursor(true)
	if !f.handleKeys(len(f.quotes) > 0) {
		return nil
	}
	for i := len(f.lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(f.lines[i]) != "" {
			return append([]string{}, f.lines[:i+1]...)
		}
	}
	return nil
}

func (f *fsed) showHeader() {
	ins := "NO"
	if f.insert {
		ins = "YES"
	}
	priv := "NO"
	if f.info.priv {
		priv = "YES"
	}
	_, _ = quest.Exec(f.e.T, f.e.G, f.e.Line, "edithdr /N", quest.ScriptOpts{
		NoLog: true,
		Answers: map[int]string{
			1: f.info.from,
			2: f.info.to,
			3: f.info.subj,
			4: strconv.Itoa(f.info.num),
			5: f.info.area,
			6: priv,
			7: ins,
		},
	})
}

func (f *fsed) showFooter() {
	_, _ = quest.Exec(f.e.T, f.e.G, f.e.Line, "editftr /N", quest.ScriptOpts{NoLog: true})
}

func (f *fsed) get(i int) string {
	if i < 0 || i >= len(f.lines) {
		return ""
	}
	return f.lines[i]
}

func (f *fsed) put(i int, s string) {
	if i < 0 {
		return
	}
	for len(f.lines) <= i {
		f.lines = append(f.lines, "")
	}
	f.lines[i] = s
}

func (f *fsed) showTextLine(y int, s string) {
	t := f.e.T
	t.GotoXY(1, y)
	t.Print(s)
	t.WriteRA("`E:")
}

func (f *fsed) showTextBuffer() {
	top := f.line - (f.row - f.minY)
	if top < 0 {
		top = 0
	}
	for y := f.minY; y <= f.maxY; y++ {
		f.showTextLine(y, f.get(top+(y-f.minY)))
	}
}

func (f *fsed) updateCursor(force bool) {
	if f.col < f.minX {
		f.col = f.minX
	}
	if f.col > f.maxX {
		f.col = f.maxX
	}
	if f.row < f.minY {
		f.row = f.minY
		f.showTextBuffer()
		force = true
	}
	if f.row > f.maxY {
		f.row = f.maxY
		f.showTextBuffer()
		force = true
	}
	if f.line < 0 {
		f.line = 0
	}
	if f.line >= maxEditLines {
		f.line = maxEditLines - 1
	}
	_ = force
	f.e.T.GotoXY(f.col, f.row)
}

func (f *fsed) cursorUp() {
	if f.line > 0 {
		f.line--
		f.row--
	}
}

func (f *fsed) cursorDown() {
	if f.line < maxEditLines-1 {
		f.line++
		f.row++
	}
}

func (f *fsed) cursorLeft() {
	f.col--
	if f.col < 1 && f.line > 0 {
		f.cursorUp()
		f.gotoEnd()
	}
}

func (f *fsed) cursorRight() {
	f.col++
	if f.col > f.maxX && f.line < maxEditLines-1 {
		f.cursorDown()
		f.col = 1
	}
}

func (f *fsed) gotoStart() { f.col = 1 }

func (f *fsed) gotoEnd() {
	s := strings.TrimRight(f.get(f.line), " ")
	f.col = len(s) + 1
	f.updateCursor(true)
}

func (f *fsed) pageDown() {
	h := f.maxY - f.minY
	f.row += h
	f.line += h
	f.updateCursor(true)
}

func (f *fsed) pageUp() {
	h := f.maxY - f.minY
	f.row -= h
	f.line -= h
	if f.line < 0 {
		f.line = 0
	}
	f.updateCursor(true)
}

func (f *fsed) insertNewLine(s string) {
	if f.readOnly {
		return
	}
	at := f.line + 1
	if at > maxEditLines-1 {
		return
	}
	for len(f.lines) < at {
		f.lines = append(f.lines, "")
	}
	tail := append([]string{s}, f.lines[at:]...)
	f.lines = append(f.lines[:at], tail...)
	if len(f.lines) > maxEditLines {
		f.lines = f.lines[:maxEditLines]
	}
}

func (f *fsed) deleteLine() {
	if f.readOnly {
		return
	}
	if f.line >= len(f.lines) {
		return
	}
	f.lines = append(f.lines[:f.line], f.lines[f.line+1:]...)
	f.col = 1
	f.showTextBuffer()
	f.updateCursor(true)
}

func (f *fsed) addChar(ch byte) {
	if f.readOnly || ch < 32 {
		return
	}
	s := f.get(f.line)
	pos := f.col - 1
	if pos < 0 {
		pos = 0
	}
	if pos > len(s) {
		s += strings.Repeat(" ", pos-len(s))
	}
	ins := string([]byte{ch})
	if f.insert || pos >= len(s) {
		s = s[:pos] + ins + s[pos:]
	} else {
		s = s[:pos] + ins + s[pos+1:]
	}
	if len(s) >= f.maxX {
		sp := strings.LastIndexByte(s[:f.maxX-1], ' ')
		if sp > 0 {
			rest := strings.TrimLeft(s[sp+1:], " ")
			s = strings.TrimRight(s[:sp], " ")
			f.put(f.line, s)
			f.insertNewLine(rest)
			f.cursorDown()
			f.col = len(rest) + 1
			f.showTextBuffer()
			f.updateCursor(true)
			return
		}
		s = s[:f.maxX-1]
	}
	f.put(f.line, s)
	f.col++
	f.showTextLine(f.row, s)
	f.updateCursor(true)
}

func (f *fsed) backspace() {
	if f.readOnly {
		return
	}
	if f.col <= 1 {
		if f.line == 0 {
			return
		}
		cur := f.get(f.line)
		if strings.TrimSpace(cur) == "" {
			f.deleteLine()
			f.cursorUp()
			f.gotoEnd()
			return
		}
		prev := f.get(f.line - 1)
		f.put(f.line-1, prev+cur)
		f.deleteLine()
		f.cursorUp()
		f.col = len(prev) + 1
		f.showTextBuffer()
		f.updateCursor(true)
		return
	}
	s := f.get(f.line)
	pos := f.col - 2
	if pos < 0 || pos >= len(s) {
		f.col--
		f.updateCursor(true)
		return
	}
	s = s[:pos] + s[pos+1:]
	f.put(f.line, s)
	f.col--
	f.showTextLine(f.row, s)
	f.updateCursor(true)
}

func (f *fsed) delChar() {
	if f.readOnly {
		return
	}
	s := f.get(f.line)
	pos := f.col - 1
	if pos < 0 || pos >= len(s) {
		return
	}
	s = s[:pos] + s[pos+1:]
	f.put(f.line, s)
	f.showTextLine(f.row, s)
	f.updateCursor(true)
}

func (f *fsed) doEnter() {
	if f.quoting {
		f.quoteLine()
		return
	}
	s := f.get(f.line)
	rest := ""
	pos := f.col - 1
	if f.insert && pos >= 0 && pos < len(s) {
		rest = s[pos:]
		s = s[:pos]
		f.put(f.line, s)
	}
	if !f.readOnly {
		f.insertNewLine(rest)
	}
	f.col = 1
	f.cursorDown()
	f.showTextBuffer()
	f.updateCursor(true)
}

func (f *fsed) quoteLine() {
	q := f.get(f.line)
	f.closeQuote(false)
	f.insertNewLine(q)
	f.line++
	f.saveLine = f.line
	f.showQuote()
}

func (f *fsed) showQuote() {
	if f.quoting {
		return
	}
	f.saveLines = f.lines
	f.saveMinY, f.saveMaxY = f.minY, f.maxY
	f.saveCol, f.saveRow, f.saveLine = f.col, f.row, f.line
	f.lines = append([]string{}, f.quotes...)
	f.quoting = true
	f.readOnly = true
	f.minY = f.maxY - 7
	if f.minY < f.saveMinY+1 {
		f.minY = f.saveMinY + 1
	}
	f.col, f.row, f.line = 1, f.minY, 0
	_, _ = quest.Exec(f.e.T, f.e.G, f.e.Line, "editqto "+strconv.Itoa(f.minY-1)+" /N", quest.ScriptOpts{NoLog: true})
	f.showTextBuffer()
	f.updateCursor(true)
}

func (f *fsed) closeQuote(redraw bool) {
	if !f.quoting {
		return
	}
	f.lines = f.saveLines
	f.minY, f.maxY = f.saveMinY, f.saveMaxY
	f.col, f.row, f.line = f.saveCol, f.saveRow, f.saveLine
	f.quoting = false
	f.readOnly = false
	if redraw {
		f.showTextBuffer()
		f.updateCursor(true)
	}
}

func (f *fsed) editMenu() byte {
	res, _ := quest.Exec(f.e.T, f.e.G, f.e.Line, "editmnu /N", quest.ScriptOpts{NoLog: true})
	res = pascal.UpCase(pascal.Trim(res))
	switch res {
	case "ABORT":
		return 27
	case "SAVE":
		return 26
	case "HELP":
		_, _ = quest.Exec(f.e.T, f.e.G, f.e.Line, "edithlp /N", quest.ScriptOpts{NoLog: true})
		f.redraw()
	}
	return 0
}

func (f *fsed) redraw() {
	f.e.T.ClearScreen()
	f.showHeader()
	f.showFooter()
	f.showTextBuffer()
	f.updateCursor(true)
}

func (f *fsed) handleKeys(doQuotes bool) bool {
	for {
		f.updateCursor(false)
		ch, err := f.e.T.GetKey(0)
		if err != nil {
			return false
		}
		if strings.TrimSpace(f.get(f.line)) == "" && ch == '/' {
			ch = f.editMenu()
			f.updateCursor(true)
		}
		if ch == 27 {
			ak := f.e.T.GetArrowKeys()
			switch ak {
			case 27:
				ch = f.editMenu()
			case 'A':
				f.cursorUp()
				ch = 0
			case 'B':
				f.cursorDown()
				ch = 0
			case 'C':
				f.cursorRight()
				ch = 0
			case 'D':
				f.cursorLeft()
				ch = 0
			case 'H':
				f.gotoStart()
				ch = 0
			case 'K':
				f.gotoEnd()
				ch = 0
			default:
				ch = 0
			}
			if ch == 26 {
				return true
			}
			if ch == 27 {
				return false
			}
			continue
		}
		switch ch {
		case 1: // ^A
			f.gotoStart()
		case 3: // ^C
			f.pageDown()
		case 4:
			f.cursorRight()
		case 5:
			f.cursorUp()
		case 6:
			f.gotoEnd()
		case 7, 127:
			f.delChar()
		case 8:
			f.backspace()
		case 12:
			f.redraw()
		case 13:
			f.doEnter()
		case 15:
			ch = f.editMenu()
			if ch == 26 {
				return true
			}
			if ch == 27 {
				return false
			}
		case 16:
			f.gotoEnd()
		case 17:
			if doQuotes {
				if !f.quoting {
					f.showQuote()
				} else {
					f.closeQuote(true)
				}
			}
		case 18:
			f.pageUp()
		case 19:
			f.cursorLeft()
		case 22:
			f.insert = !f.insert
			f.showHeader()
			f.updateCursor(true)
		case 23:
			f.gotoStart()
		case 24:
			f.cursorDown()
		case 25:
			f.deleteLine()
		case 26:
			return true
		default:
			f.addChar(ch)
		}
	}
}

func (e *Engine) externalEditor(cmd string, info fsedInfo, quote []string) []string {
	priv := "NO"
	if info.priv {
		priv = "YES"
	}
	_ = os.WriteFile("msginf", []byte(strings.Join([]string{
		info.from, info.to, info.subj, strconv.Itoa(info.num), info.area, priv,
	}, "\r\n")+"\r\n"), 0644)
	if len(quote) > 0 {
		_ = os.WriteFile("msgtmp", []byte(strings.Join(quote, "\r\n")+"\r\n"), 0644)
	} else {
		_ = os.Remove("msgtmp")
	}
	add := strconv.Itoa(int(e.Line.Modem.ComPort)) + " " +
		strconv.Itoa(int(e.Line.Baud)) + " " +
		strconv.Itoa(int(e.Line.TimeLimit)) + " " +
		strconv.Itoa(int(e.G.RaConfig.UserTimeOut))
	door.Run(e.T, e.G, e.Line, e.Files, cmd+" "+add, false)
	e.T.ClearScreen()
	_ = os.Remove("msginf")
	b, err := os.ReadFile("msgtmp")
	_ = os.Remove("msgtmp")
	if err != nil || len(b) == 0 {
		return nil
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "\x01") {
			continue
		}
		out = append(out, ln)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func buildQuoteLines(g *cfgrec.GlobalCfg, origTo, origFrom, origDate string, body string) []string {
	prefix := " > "
	if g != nil && pascal.Trim(g.RaConfig.QuoteString) != "" {
		prefix = g.RaConfig.QuoteString
		initU := pascal.UpCase(initials(origFrom))
		initL := strings.ToLower(initU)
		prefix = strings.ReplaceAll(prefix, "@", initU)
		prefix = strings.ReplaceAll(prefix, "#", initL)
	} else {
		prefix = initials(origFrom) + "> "
	}
	var out []string
	hdr := "* In a message originally to " + origTo + ", " + origFrom + " said:"
	out = append(out, hdr, "")
	for _, ln := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if pascal.Trim(ln) == "" {
			continue
		}
		out = append(out, prefix+ln)
	}
	return out
}

func nextMsgNum(a cfgrec.MessageArea) int {
	st := mail.Stats(a.JAMBase)
	if st.High < 1 {
		return 1
	}
	return st.High + 1
}
