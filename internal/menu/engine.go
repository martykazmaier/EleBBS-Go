package menu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

type Engine struct {
	T              *term.IO
	G              *cfgrec.GlobalCfg
	Line           *cfgrec.LineCfg
	Files          []cfgrec.FilesArea
	Msgs           []cfgrec.MessageArea
	Hang           bool
	jump           bool
	hot            map[string]cfgrec.MenuItem
	choice         byte
	lbarFile       bool
	lbarOK         bool
	lbarItem       cfgrec.MenuItem
	dropLogonEnter bool
}

func (e *Engine) Enter() {
	e.dropLogonEnter = true
	if e.T != nil {
		e.T.DrainLineEnds()
	}
	e.Line.MenuStackPtr = 1
	e.Line.MenuStack[1] = cfgrec.DefaultMenu
	for !e.Hang {
		name := e.Line.MenuStack[e.Line.MenuStackPtr]
		items, bars, ok := e.load(name)
		if !ok {
			logx.Write(e.G, e.Line.RaNodeNr, '!', name+" menu missing")
			e.T.WriteRA("`A12:" + e.T.RalStr(lang.ErrMnu) + name)
			e.T.Println("")
			if strings.EqualFold(name, cfgrec.DefaultMenu) {
				e.T.WriteRA("`A12:" + e.T.RalStr(lang.ErrTop))
				e.T.Println("")
				e.fallback()
				return
			}
			e.Line.MenuStackPtr = 1
			e.Line.MenuStack[1] = cfgrec.DefaultMenu
			continue
		}
		if !e.run(name, items, bars) {
			return
		}
	}
}

func (e *Engine) load(name string) ([]cfgrec.MenuItem, []cfgrec.LightBar, bool) {
	items, ok := e.readMenu(name, false)
	if !ok || len(items) == 0 {
		return nil, nil, false
	}
	bars := e.alignBars(items, e.readBars(name, false))
	if !strings.Contains(strings.ToUpper(items[0].MiscData), "/NG") {
		if extra, ok := e.readMenu("globalra", true); ok {
			extraBars := e.readBars("globalra", true)
			items = append(items, extra...)
			bars = append(bars, extraBars...)
			bars = e.alignBars(items, bars)
		}
	}
	return items, bars, true
}

func (e *Engine) alignBars(items []cfgrec.MenuItem, bars []cfgrec.LightBar) []cfgrec.LightBar {
	out := make([]cfgrec.LightBar, len(items))
	copy(out, bars)
	return out
}

func (e *Engine) findMenuFile(name, ext string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	ext = strings.ToLower(ext)
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}
	dirs := []string{}
	if e.Line != nil {
		dirs = append(dirs, e.Line.Language.MenuPath)
	}
	if e.G != nil {
		dirs = append(dirs, e.G.RaConfig.MenuPath, e.G.RaConfig.SysPath)
	}
	dirs = append(dirs, ".")
	cands := []string{name + ext, strings.ToUpper(name) + strings.ToUpper(ext), name + strings.ToUpper(ext)}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		root := strings.TrimRight(d, `\/`)
		for _, n := range cands {
			p := filepath.Join(root, n)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func (e *Engine) existLightBars(name string) bool {
	if e.Line == nil || (!e.Line.AnsiOn && !e.Line.AvatarOn) {
		return false
	}
	if e.findMenuFile(name, ".mlb") != "" {
		return true
	}
	return e.findMenuFile("globalra", ".mnu") != ""
}

func (e *Engine) readBars(name string, skipPrompt bool) []cfgrec.LightBar {
	p := e.findMenuFile(name, ".mlb")
	if p == "" {
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) < cfgrec.LightBarSize {
		return nil
	}
	n := len(raw) / cfgrec.LightBarSize
	start := 0
	if skipPrompt {
		start = 1
	}
	if start >= n {
		return nil
	}
	bars := make([]cfgrec.LightBar, 0, n-start)
	for i := start; i < n; i++ {
		bars = append(bars, cfgrec.ParseLightBar(raw[i*cfgrec.LightBarSize:(i+1)*cfgrec.LightBarSize]))
	}
	return bars
}

func (e *Engine) readMenu(name string, skipPrompt bool) ([]cfgrec.MenuItem, bool) {
	p := e.findMenuFile(name, ".mnu")
	if p == "" {
		return nil, false
	}
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) < cfgrec.MenuSize {
		return nil, false
	}
	n := len(raw) / cfgrec.MenuSize
	start := 0
	if skipPrompt {
		start = 1
	}
	if start >= n {
		return nil, false
	}
	items := make([]cfgrec.MenuItem, 0, n-start)
	for i := start; i < n; i++ {
		items = append(items, cfgrec.ParseMenuItem(raw[i*cfgrec.MenuSize:(i+1)*cfgrec.MenuSize]))
	}
	return items, true
}

func isAutoexec(it cfgrec.MenuItem) bool {
	return len(it.HotKey) > 0 && it.HotKey[0] == 1
}

func (e *Engine) displayItem(item, prompt cfgrec.MenuItem, promptItem bool) {
	ds := item.Display
	if ds == "" {
		return
	}
	attr := (item.Background << 4) | (item.Foreground & 0x0F)
	hiAttr := (byte(prompt.Security) << 4) | (prompt.Typ & 0x0F)
	var b strings.Builder
	b.WriteString("`A")
	b.WriteString(strconv.Itoa(int(attr)))
	b.WriteByte(':')
	hi := false
	for i := 0; i < len(ds); i++ {
		c := ds[i]
		if c == '^' {
			if hi {
				b.WriteString("`A")
				b.WriteString(strconv.Itoa(int(attr)))
				b.WriteByte(':')
				hi = false
			} else {
				b.WriteString("`A")
				b.WriteString(strconv.Itoa(int(hiAttr)))
				b.WriteByte(':')
				hi = true
			}
			continue
		}
		if c == ';' {
			if i != len(ds)-1 {
				b.WriteByte(';')
			}
			continue
		}
		b.WriteByte(c)
	}
	e.T.WriteRA(b.String())
	if !strings.HasSuffix(ds, ";") && !promptItem {
		e.T.Println("")
	}
}

func (e *Engine) run(name string, items []cfgrec.MenuItem, bars []cfgrec.LightBar) bool {
	prompt := cfgrec.MenuItem{}
	opts := []cfgrec.MenuItem{}
	if len(items) > 0 {
		prompt = items[0]
		opts = items[1:]
	}
	hot := map[string]cfgrec.MenuItem{}
	for _, it := range opts {
		if !e.access(it) || isAutoexec(it) {
			continue
		}
		k := hotKeyOf(it)
		if k == "" {
			continue
		}
		if _, exists := hot[k]; !exists {
			hot[k] = it
		}
	}
	e.hot = hot
	e.choice = 0
	e.lbarFile = e.existLightBars(name)
	e.lbarOK = false
	e.T.Lines = 0
	if e.Line != nil {
		e.Line.DispMorePrompt = false
	}
	for _, it := range opts {
		if !e.access(it) {
			continue
		}
		e.displayItem(it, prompt, false)
		if !isAutoexec(it) {
			continue
		}
		if !e.exec(it) {
			return false
		}
		if e.jump {
			return true
		}
		if e.choice != 0 {
			break
		}
	}
	supportLBar := false
	if e.lbarFile {
		for i := range items {
			if i < len(bars) && bars[i].Enabled() && e.access(items[i]) {
				supportLBar = true
				break
			}
		}
	}
	ch := e.choice
	if ch == 0 {
		if prompt.Display != "" {
			e.T.Println("")
			e.displayItem(prompt, prompt, true)
		}
		var ok bool
		e.lbarOK = false
		ch, ok = e.getMenuChoice(items, bars, supportLBar, hot)
		if !ok {
			return false
		}
	}
	if e.Line != nil {
		e.Line.DispMorePrompt = true
	}
	if e.T != nil {
		e.T.ResetLines(1)
	}
	if e.lbarOK {
		it := e.lbarItem
		e.lbarOK = false
		if it.Typ == 0 {
			return true
		}
		if !strings.Contains(strings.ToUpper(prompt.MiscData), "/NO") {
			if k := hotKeyOf(it); k != "" && k[0] >= 32 {
				e.T.Println(string(unicode.ToUpper(rune(k[0]))))
			}
		}
		if !e.access(it) {
			return true
		}
		return e.exec(it)
	}
	if ch == '\r' || ch == '\n' {
		return true
	}
	key := string([]byte{ch})
	up := pascal.UpCase(key)
	it, found := hot[up]
	if !found {
		it, found = hot[key]
	}
	if !found {
		for k, v := range hot {
			if strings.HasPrefix(k, up) {
				it, found = v, true
				break
			}
		}
	}
	if !strings.Contains(strings.ToUpper(prompt.MiscData), "/NO") && ch >= 32 {
		e.T.Println(string(unicode.ToUpper(rune(ch))))
	}
	if !found {
		return true
	}
	return e.exec(it)
}

func hotKeyOf(it cfgrec.MenuItem) string {
	k := strings.TrimSpace(it.HotKey)
	if k == "" {
		return ""
	}
	return pascal.UpCase(k[:1])
}

func (e *Engine) displayLightBar(lb cfgrec.LightBar, active bool) {
	if !lb.Enabled() {
		return
	}
	x, y := int(lb.LightX), int(lb.LightY)
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}
	e.T.GotoXY(x, y)
	if active {
		e.T.WriteRA(lb.SelectItem)
	} else {
		e.T.WriteRA(lb.LowItem)
	}
}

func (e *Engine) buildLightBars(items []cfgrec.MenuItem, bars []cfgrec.LightBar, active int) {
	for i := range items {
		if i >= len(bars) || !e.access(items[i]) {
			continue
		}
		e.displayLightBar(bars[i], i == active)
	}
}

func (e *Engine) barOK(i int, items []cfgrec.MenuItem, bars []cfgrec.LightBar) bool {
	return i >= 1 && i < len(items) && i < len(bars) && bars[i].Enabled() && e.access(items[i])
}

func (e *Engine) correctArrow(arrow int, items []cfgrec.MenuItem, bars []cfgrec.LightBar, goingUp bool) int {
	n := len(items)
	if n < 2 {
		return 1
	}
	if arrow >= n {
		arrow = 1
	}
	if arrow >= 1 && !e.barOK(arrow, items, bars) {
		if !goingUp {
			for i := arrow; i < n; i++ {
				if e.barOK(i, items, bars) {
					return i
				}
			}
			for i := 1; i < n; i++ {
				if e.barOK(i, items, bars) {
					return i
				}
			}
		} else {
			for i := arrow; i >= 1; i-- {
				if e.barOK(i, items, bars) {
					return i
				}
			}
			for i := n - 1; i >= 1; i-- {
				if e.barOK(i, items, bars) {
					return i
				}
			}
		}
		arrow = 1
	}
	if arrow < 1 {
		for i := n - 1; i >= 1; i-- {
			if e.barOK(i, items, bars) {
				return i
			}
		}
		arrow = 1
	}
	return arrow
}

func (e *Engine) searchRight(arrow int, items []cfgrec.MenuItem, bars []cfgrec.LightBar) int {
	if arrow < 0 || arrow >= len(bars) {
		return arrow
	}
	y := bars[arrow].LightY
	x := bars[arrow].LightX
	for i := arrow; i < len(items); i++ {
		if i < len(bars) && bars[i].LightY == y && bars[i].LightX > x && e.barOK(i, items, bars) {
			return i
		}
	}
	for i := 0; i < arrow; i++ {
		if i < len(bars) && bars[i].LightY == y && bars[i].LightX > x && e.barOK(i, items, bars) {
			return i
		}
	}
	return arrow
}

func (e *Engine) searchLeft(arrow int, items []cfgrec.MenuItem, bars []cfgrec.LightBar) int {
	if arrow < 0 || arrow >= len(bars) {
		return arrow
	}
	y := bars[arrow].LightY
	x := bars[arrow].LightX
	for i := arrow; i < len(items); i++ {
		if i < len(bars) && bars[i].LightY == y && bars[i].LightX < x && e.barOK(i, items, bars) {
			return i
		}
	}
	for i := arrow; i >= 0; i-- {
		if i < len(bars) && bars[i].LightY == y && bars[i].LightX < x && e.barOK(i, items, bars) {
			return i
		}
	}
	return arrow
}

func (e *Engine) getMenuChoice(items []cfgrec.MenuItem, bars []cfgrec.LightBar, supportLBar bool, hot map[string]cfgrec.MenuItem) (byte, bool) {
	arrow := 1
	if len(items) > 0 {
		if v, _ := getValue("/BS=", items[0].MiscData); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				arrow = n + 1
			}
		}
	}
	if supportLBar {
		arrow = e.correctArrow(arrow, items, bars, false)
	}
	saveX, saveY := e.T.WhereX(), e.T.WhereY()
	if supportLBar {
		e.buildLightBars(items, bars, arrow)
		e.T.GotoXY(saveX, saveY)
	}
	if e.dropLogonEnter {
		if e.T != nil {
			e.T.DrainLineEnds()
		}
		e.dropLogonEnter = false
	}
	inHot := func(ch byte) bool {
		if ch == '\r' || ch == '\n' || ch == 255 {
			return true
		}
		k := pascal.UpCase(string([]byte{ch}))
		_, ok := hot[k]
		return ok
	}
	for {
		ch, err := e.T.GetKey(0)
		if err != nil {
			e.Hang = true
			return 0, false
		}
		if ch == 0x1b {
			ak := e.T.GetArrowKeys()
			if supportLBar && (ak == 'A' || ak == 'B' || ak == 'C' || ak == 'D') {
				if arrow < len(bars) {
					e.displayLightBar(bars[arrow], false)
				}
				switch ak {
				case 'A':
					arrow--
				case 'B':
					arrow++
				case 'C':
					arrow = e.searchRight(arrow, items, bars)
				case 'D':
					arrow = e.searchLeft(arrow, items, bars)
				}
				arrow = e.correctArrow(arrow, items, bars, ak == 'A')
				if arrow < len(bars) {
					e.displayLightBar(bars[arrow], true)
				}
				e.T.GotoXY(saveX, saveY)
				continue
			}
			if inHot(ak) {
				return ak, true
			}
			continue
		}
		if supportLBar && (ch == '\r' || ch == '\n') {
			if arrow >= 0 && arrow < len(items) && items[arrow].Typ != 0 {
				e.lbarOK = true
				e.lbarItem = items[arrow]
			}
			if arrow >= 0 && arrow < len(items) {
				k := hotKeyOf(items[arrow])
				if k != "" {
					return k[0], true
				}
			}
			return '\r', true
		}
		if inHot(ch) {
			return ch, true
		}
	}
}

func (e *Engine) access(it cfgrec.MenuItem) bool {
	if e.Line == nil {
		return true
	}
	u := e.Line.User
	if it.Security > 0 && u.Security < it.Security {
		return false
	}
	if it.MaxSec > 0 && u.Security > it.MaxSec {
		return false
	}
	if !checkFlagAccess(u.Flags, it.Flags, it.NotFlags) {
		return false
	}
	if it.TimeLeft > 0 && uint16(e.Line.TimeLimit) < it.TimeLeft && e.Line.TimeLimit != 32767 {
		return false
	}
	if it.TermAttrib&0x07 != 0 {
		ok := false
		if it.TermAttrib&1 != 0 && e.Line.AnsiOn {
			ok = true
		}
		if it.TermAttrib&2 != 0 && e.Line.AvatarOn {
			ok = true
		}
		if it.TermAttrib&4 != 0 && e.Line.RipOn {
			ok = true
		}
		if !ok {
			return false
		}
	}
	if !check32(it.Node, e.Line.RaNodeNr) {
		return false
	}
	if !check32(it.Group, int(u.Group)) {
		return false
	}
	return true
}

func checkFlagAccess(user, on, off cfgrec.Flag) bool {
	for col := 0; col < 4; col++ {
		for row := 0; row < 8; row++ {
			bit := byte(1 << uint(row))
			if on[col]&bit != 0 && user[col]&bit == 0 {
				return false
			}
			if off[col]&bit != 0 && user[col]&bit != 0 {
				return false
			}
		}
	}
	return true
}

func check32(bits [32]byte, n int) bool {
	all := true
	for _, b := range bits {
		if b != 0 {
			all = false
			break
		}
	}
	if all {
		return true
	}
	if n < 1 {
		n = 1
	}
	row := 0
	for n > 7 {
		row++
		n -= 8
	}
	if row < 0 || row > 31 {
		row = 31
	}
	return bits[row]&(1<<uint(n)) != 0
}

func firstWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] != ' ' {
		i++
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func (e *Engine) exec(it cfgrec.MenuItem) bool {
	e.jump = false
	return e.ExecType(it.Typ, it.MiscData)
}

func (e *Engine) fallback() {
	e.T.Println("")
	e.T.Println(cfgrec.PidName + " — no TOP.MNU found, using built-in menu.")
	for !e.Hang {
		e.T.Println("")
		e.T.Println("[F]ile areas   [M]essage areas   [L]ist files")
		e.T.Println("[R]ead msgs    [W]ho           [G]oodbye")
		e.T.Print("Select: ")
		ch, err := e.T.GetKey(0)
		if err != nil {
			e.Hang = true
			return
		}
		e.T.Println(string(ch))
		switch unicode.ToUpper(rune(ch)) {
		case 'F':
			e.changeFileArea("")
		case 'M':
			e.changeMessageArea("")
		case 'L':
			if a, ok := files.FindArea(e.Files, e.Line.User.FileArea); ok {
				files.ListArea(e.T, e.G, a)
			}
			e.T.PressEnter()
		case 'R':
			e.readMessages("/M")
		case 'W':
			e.T.Println("Node " + itoa(e.Line.RaNodeNr) + ": " + e.Line.User.Name)
			e.T.PressEnter()
		case 'G', 'Q':
			e.Hang = true
			return
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func firstField(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		end := s[0]
		if i := strings.IndexByte(s[1:], end); i >= 0 {
			return s[1 : 1+i]
		}
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return s
	}
	return f[0]
}

func hotKeyBytes(hot map[string]cfgrec.MenuItem) map[byte]struct{} {
	m := map[byte]struct{}{'\r': {}}
	for k := range hot {
		if k == "" {
			continue
		}
		c := k[0]
		m[c] = struct{}{}
		if c >= 'A' && c <= 'Z' {
			m[c+32] = struct{}{}
		} else if c >= 'a' && c <= 'z' {
			m[c-32] = struct{}{}
		}
	}
	return m
}
