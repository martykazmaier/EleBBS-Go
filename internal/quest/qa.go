package quest

import (
	"bufio"
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

const (
	maxAnswers   = 100
	maxFileStack = 5
	maxWhile     = 5
	maxGoSub     = 20
	maxCfgFiles  = 5
)

type qaFile struct {
	f *os.File
	r *bufio.Reader
}

type vm struct {
	t          *term.IO
	g          *cfgrec.GlobalCfg
	line       *cfgrec.LineCfg
	lines      []string
	pc         int
	ans        [maxAnswers + 1]string
	labels     map[string]int
	skipIf     bool
	ifNest     int
	ignIf      int
	skipWhile  bool
	ignWhile   int
	whilePC    []int
	whileIters int
	gosub      []int
	files      [maxFileStack + 1]*qaFile
	fileRes    [maxFileStack + 1]string
	cfg        [maxFileStack + 1]*cfgSlot
	params     string
	result     string
	outFile    string
	post       []string
	savedX     [21]int
	savedY     [21]int
	savedAttr  [21]byte
	curX       int
	curY       int
	done       bool
	cap        bool
	quesDir    string
	getInfo    GetInfoFunc
}

// GetInfoFunc is Pascal QInfo.GetInfoHook for GETRECORDINFO.
type GetInfoFunc func(recordNum, start int, down bool, put func(n int, s string))

// ScriptOpts is Pascal Quest.Process extras (preset answers, GetInfoHook, /N).
type ScriptOpts struct {
	Args    string
	NoLog   bool
	Answers map[int]string
	GetInfo GetInfoFunc
}

// Run executes an EleBBS Q-A questionnaire (name.q-a in QuesPath).
func Run(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, name, args string) {
	_, _ = Exec(t, g, line, name, ScriptOpts{Args: args})
}

// Kind is Pascal GetScriptType: "q-a", "elm", or empty if missing.
func Kind(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, name string) string {
	_, kind := findScript(g, line, name)
	return kind
}

func splitQuestCmd(name, args string) (base, rest string, nolog bool) {
	s := strings.TrimSpace(name + " " + args)
	up := pascal.UpCase(s)
	nolog = strings.Contains(up, "/N")
	s = strings.ReplaceAll(s, "/N", "")
	s = strings.ReplaceAll(s, "/n", "")
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	base = strings.TrimSpace(s[:i])
	rest = strings.TrimSpace(s[i:])
	return base, rest, nolog
}

// Exec runs a questionnaire and returns SETRESULT plus whether it completed (error 0).
func Exec(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, name string, opt ScriptOpts) (result string, ok bool) {
	if t != nil {
		saveMore := t.MorePrompt
		saveStop := t.StopMore
		savePause := t.NoPause()
		saveDisp := true
		if line != nil {
			saveDisp = line.DispMorePrompt
			line.DispMorePrompt = false
		}
		t.MorePrompt = false
		t.StopMore = false
		t.SetNoPause(true)
		t.ResetLines(0)
		defer func() {
			t.MorePrompt = saveMore
			t.StopMore = saveStop
			t.SetNoPause(savePause)
			if line != nil {
				line.DispMorePrompt = saveDisp
			}
		}()
	}
	name, args, nolog := splitQuestCmd(name, opt.Args)
	if nolog {
		opt.NoLog = true
	}
	if strings.TrimSpace(opt.Args) == "" {
		opt.Args = args
	}
	if name == "" {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(name))
	base := name
	if ext != "" {
		base = strings.TrimSuffix(name, filepath.Ext(name))
	}
	base = strings.ToLower(base)
	path, kind := findScript(g, line, base)
	if path == "" {
		if !opt.NoLog {
			logx.Write(g, line.RaNodeNr, '!', "Unable to find questionnaire file: "+base+".q-a")
		}
		return "", false
	}
	if kind != "q-a" {
		if !opt.NoLog {
			logx.Write(g, line.RaNodeNr, '!', "EleXer script "+base+".elm is not executed by this port; add "+base+".q-a")
		}
		return "", false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !opt.NoLog {
			logx.Write(g, line.RaNodeNr, '!', "Unable to find questionnaire file: "+base+".q-a")
		}
		return "", false
	}
	if !opt.NoLog {
		logx.Write(g, line.RaNodeNr, '>', "Questionnaire "+base+" initiated:")
	}
	text := pascal.FromCP437(b)
	q := &vm{
		t: t, g: g, line: line,
		labels:  map[string]int{},
		quesDir: filepath.Dir(path),
		params:  opt.Args,
		curX:    1,
		curY:    1,
		getInfo: opt.GetInfo,
	}
	for n, s := range opt.Answers {
		q.put(n, s)
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		q.lines = append(q.lines, strings.TrimRight(sc.Text(), "\r"))
	}
	for i, ln := range q.lines {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, ":") {
			lab := strings.ToUpper(strings.TrimSpace(s[1:]))
			if j := strings.IndexByte(lab, ' '); j >= 0 {
				lab = lab[:j]
			}
			q.labels[lab] = i
		}
	}
	for q.pc < len(q.lines) && !q.done && !q.t.Gone() {
		q.exec(q.lines[q.pc])
		q.pc++
	}
	q.closeFiles()
	q.closeCfg()
	q.commit()
	if !opt.NoLog {
		logx.Write(g, line.RaNodeNr, '>', "Questionnaire completed")
	}
	return q.result, true
}

func qaDirs(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, extra ...string) []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		d = strings.TrimSpace(d)
		d = strings.TrimRight(d, `\/`)
		if d == "" {
			return
		}
		key := strings.ToLower(d)
		if seen[key] {
			return
		}
		seen[key] = true
		dirs = append(dirs, d)
	}
	for _, d := range extra {
		add(d)
	}
	if line != nil {
		add(line.Language.QuesPath)
		add(line.Language.TextPath)
		add(line.Language.MenuPath)
	}
	if g != nil {
		add(g.RaConfig.TextPath)
		add(g.RaConfig.SysPath)
		add(g.RaConfig.MenuPath)
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	add(".")
	return dirs
}

func findScript(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, base string, extra ...string) (string, string) {
	base = strings.TrimSpace(base)
	if i := strings.LastIndexAny(base, `\/`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(base, ".q-a"), ".elm"))
	if base == "" {
		return "", ""
	}
	dirs := qaDirs(g, line, extra...)
	if p := lookupQA(dirs, base, ".q-a"); p != "" {
		return p, "q-a"
	}
	if p := lookupQA(dirs, base, ".elm"); p != "" {
		return p, "elm"
	}
	return "", ""
}

func lookupQA(dirs []string, base, ext string) string {
	want := strings.ToLower(base + ext)
	for _, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			p := filepath.Join(d, base+ext)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if strings.ToLower(e.Name()) == want {
				return filepath.Join(d, e.Name())
			}
		}
	}
	return ""
}

func (q *vm) exec(raw string) {
	line := trimRA(raw)
	if line == "" || line[0] == ';' || line[0] == ':' {
		return
	}
	cmd, rest := splitQACmd(line)

	if q.skipIf {
		switch cmd {
		case "IF":
			q.ignIf++
		case "ENDIF":
			if q.ignIf > 0 {
				q.ignIf--
			} else {
				if q.ifNest > 0 {
					q.ifNest--
				}
				q.skipIf = false
			}
		case "ELSE":
			if q.ignIf == 0 {
				q.skipIf = false
			}
		}
		return
	}
	if q.skipWhile {
		switch cmd {
		case "WHILE":
			q.ignWhile++
		case "ENDWHILE":
			if q.ignWhile > 0 {
				q.ignWhile--
			} else {
				q.skipWhile = false
			}
		}
		return
	}

	switch cmd {
	case "QUIT":
		q.done = true
	case "DISPLAY", "RAWDISPLAY":
		q.t.WriteRA(q.makeDisplayStr(rest))
	case "DISPLAYFILE":
		name, _ := firstWord(rest)
		if name != "" {
			if q.t != nil {
				name = q.t.ExpandRA(q.value(name))
			}
			tp := ""
			if q.g != nil {
				tp = q.g.RaConfig.TextPath
			}
			if q.line != nil && q.line.Language.TextPath != "" {
				tp = q.line.Language.TextPath
			}
			term.DisplayHotFile(q.t, tp, name)
		}
	case "CLEARSCREEN":
		q.t.ClearScreen()
	case "WAITENTER":
		q.t.WaitEnter()
	case "BEEP":
		_ = q.t.WriteRaw([]byte{7})
	case "DELAY":
		n, _ := strconv.Atoi(strings.TrimSpace(rest))
		if n > 0 {
			time.Sleep(time.Duration(n) * time.Millisecond)
		}
	case "RANDOM":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			max := atoi(w[0])
			dst := atoi(w[1])
			if max < 1 {
				max = 1
			}
			q.put(dst, strconv.Itoa(rand.Intn(max)))
		}
	case "ASSIGN":
		dstWord, val := firstWord(rest)
		dst := atoi(dstWord)
		if dstWord != "" && dstWord[0] == '%' {
			dst = atoi(q.get(atoi(dstWord[1:])))
		}
		if val != "" && (val[0] == '#' || val[0] == '%') {
			q.put(dst, q.value(val))
		} else if q.t != nil {
			q.put(dst, q.t.ExpandRA(q.value(val)))
		} else {
			q.put(dst, q.value(val))
		}
	case "GOTO":
		q.gotoLabel(strings.TrimSpace(rest))
	case "IF":
		ok := q.testIf(rest)
		q.ifNest++
		if !ok {
			q.skipIf = true
		}
	case "ELSE":
		q.skipIf = true
	case "ENDIF":
		if q.ifNest > 0 {
			q.ifNest--
		}
		q.skipIf = false
	case "ASK":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			max := atoi(w[0])
			dst := atoi(w[1])
			hidden := len(w) >= 3 && strings.EqualFold(w[2], "YES")
			if max <= 0 {
				max = 40
			}
			if q.t != nil {
				q.t.DrainLineEnds()
			}
			s, _ := q.t.GetString(max, hidden, q.cap)
			q.put(dst, s)
		}
	case "INC":
		n := atoi(strings.TrimPrefix(strings.TrimSpace(rest), "%"))
		q.put(n, strconv.Itoa(atoi(q.get(n))+1))
	case "DEC":
		n := atoi(strings.TrimPrefix(strings.TrimSpace(rest), "%"))
		q.put(n, strconv.Itoa(atoi(q.get(n))-1))
	case "CONCAT":
		w := strings.Fields(rest)
		if len(w) >= 3 {
			q.put(atoi(w[0]), q.get(atoi(w[1]))+q.get(atoi(w[2])))
		}
	case "DELIMIT":
		q.cmdDelimit(rest)
	case "FILEOPEN":
		q.cmdFileOpen(rest)
	case "FILEREAD":
		q.cmdFileRead(rest)
	case "FILEWRITE":
		q.cmdFileWrite(rest)
	case "FILECLOSE":
		slot, _ := firstWord(rest)
		q.closeSlot(atoi(slot))
	case "WHILE":
		q.cmdWhile(rest)
	case "ENDWHILE":
		q.cmdEndWhile()
	case "SETX":
		n, _ := firstWord(rest)
		q.curX = atoi(n)
		if q.curX < 1 {
			q.curX = 1
		}
		q.t.WriteRA("`X" + strconv.Itoa(q.curX) + ":")
		q.curX = q.t.WhereX()
	case "SETY":
		n, _ := firstWord(rest)
		q.curY = atoi(n)
		if q.curY < 1 {
			q.curY = 1
		}
		q.t.WriteRA("`Y" + strconv.Itoa(q.curY) + ":")
		q.curY = q.t.WhereY()
	case "UPPERCASE":
		n := atoi(strings.TrimSpace(rest))
		q.put(n, pascal.UpCase(q.get(n)))
	case "LOWERCASE":
		n := atoi(strings.TrimSpace(rest))
		q.put(n, strings.ToLower(q.get(n)))
	case "LISTANSWER":
		n := atoi(strings.TrimSpace(rest))
		q.t.WriteRA(q.get(n) + "\r\n")
	case "FILEEXIST":
		w := strings.Fields(rest)
		if len(w) >= 2 {
			dst := atoi(w[0])
			p := q.value(w[1])
			if _, err := os.Stat(p); err == nil {
				q.put(dst, "YES")
			} else {
				q.put(dst, "NO")
			}
		}
	case "CAPITALISE", "CAPITALIZE":
		q.cap = strings.EqualFold(strings.TrimSpace(rest), "ON") || rest == ""
	case "INCLUDEQA":
		spec, _ := firstWord(rest)
		q.includeQA(spec)
	default:
		q.cmdExtra(cmd, rest)
	}
}

func qaFileBase(spec string) string {
	spec = strings.TrimSpace(unquote(spec))
	spec = strings.Trim(spec, `"'`)
	if spec == "" {
		return ""
	}
	spec = strings.ReplaceAll(spec, `\`, "/")
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		spec = spec[i+1:]
	}
	low := strings.ToLower(spec)
	for _, suf := range []string{".q-a", ".elm", ".qa"} {
		if strings.HasSuffix(low, suf) {
			spec = spec[:len(spec)-len(suf)]
			break
		}
	}
	return strings.TrimSpace(spec)
}

func (q *vm) includeQA(spec string) {
	spec = strings.TrimSpace(unquote(spec))
	if spec == "" {
		return
	}
	if spec[0] == '#' || spec[0] == '%' {
		spec = q.get(atoi(spec[1:]))
		spec = strings.TrimSpace(spec)
	}
	base := qaFileBase(spec)
	if base == "" {
		return
	}
	extra := []string{q.quesDir}
	if q.line != nil {
		extra = append(extra, q.line.Language.QuesPath, q.line.Language.TextPath)
	}
	if q.g != nil {
		extra = append(extra, q.g.RaConfig.SysPath, q.g.RaConfig.TextPath)
	}
	path, kind := findScript(q.g, q.line, base, extra...)
	if path == "" || kind != "q-a" {
		if q.g != nil && q.line != nil {
			logx.Write(q.g, q.line.RaNodeNr, '!', "Unable to find questionnaire file: "+base+".q-a")
		}
		return
	}
	if q.g != nil && q.line != nil {
		logx.Write(q.g, q.line.RaNodeNr, '>', "INCLUDEQA "+base)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if q.g != nil && q.line != nil {
			logx.Write(q.g, q.line.RaNodeNr, '!', "Unable to find questionnaire file: "+base+".q-a")
		}
		return
	}
	text := pascal.FromCP437(b)
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	labels := map[string]int{}
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, ":") {
			lab := strings.ToUpper(strings.TrimSpace(s[1:]))
			if j := strings.IndexByte(lab, ' '); j >= 0 {
				lab = lab[:j]
			}
			labels[lab] = i
		}
	}
	saveLines, savePC, saveLabels, saveDir := q.lines, q.pc, q.labels, q.quesDir
	q.lines = lines
	q.labels = labels
	q.quesDir = filepath.Dir(path)
	q.pc = 0
	for q.pc < len(q.lines) && !q.done && !q.t.Gone() {
		q.exec(q.lines[q.pc])
		q.pc++
	}
	q.lines, q.pc, q.labels, q.quesDir = saveLines, savePC, saveLabels, saveDir
}

func (q *vm) cmdWhile(rest string) {
	rest = stripWhileDo(rest)
	if q.testIf(rest) {
		if len(q.whilePC) < maxWhile {
			q.whilePC = append(q.whilePC, q.pc)
			q.whileIters = 0
		}
		return
	}
	q.skipWhile = true
}

func (q *vm) cmdEndWhile() {
	if len(q.whilePC) == 0 {
		return
	}
	start := q.whilePC[len(q.whilePC)-1]
	q.whilePC = q.whilePC[:len(q.whilePC)-1]
	if start < 0 || start >= len(q.lines) {
		return
	}
	_, rest := splitQACmd(q.lines[start])
	if q.testIf(stripWhileDo(rest)) {
		q.whileIters++
		if q.whileIters > 10000 {
			return
		}
		if len(q.whilePC) < maxWhile {
			q.whilePC = append(q.whilePC, start)
		}
		q.pc = start // loop: for-loop increments to start+1 (body)
	}
}

func stripWhileDo(s string) string {
	s = trimRA(s)
	if strings.HasSuffix(strings.ToLower(s), " do") {
		return trimRA(s[:len(s)-3])
	}
	return s
}

func (q *vm) cmdFileOpen(rest string) {
	slotWord, spec := firstWord(rest)
	slot := atoi(slotWord)
	if slot < 1 || slot > maxFileStack {
		return
	}
	spec = q.value(spec)
	spec = unquote(spec)
	if q.t != nil {
		spec = q.t.ExpandRA(spec)
	}
	spec = online.ResolveSemaFile(q.g, spec)
	q.closeSlot(slot)
	if spec == "" {
		q.fileRes[slot] = "NO"
		return
	}
	extra := []string{q.quesDir}
	if q.g != nil {
		extra = append(extra, q.g.RaConfig.SemPath, q.g.RaConfig.SysPath)
	}
	if q.line != nil {
		extra = append(extra, q.line.Language.TextPath, q.line.Language.QuesPath)
	}
	f, err := openQAFile(q.g, spec, extra...)
	if err != nil {
		q.fileRes[slot] = "NO"
		return
	}
	q.fileRes[slot] = "YES"
	q.files[slot] = &qaFile{f: f, r: bufio.NewReader(f)}
}

func openQAFile(g *cfgrec.GlobalCfg, spec string, extra ...string) (*os.File, error) {
	cands := config.PathCandidates(g, spec, extra...)
	var last error
	for _, p := range cands {
		f, err := os.OpenFile(p, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		last = err
		f, err = os.Open(p)
		if err == nil {
			return f, nil
		}
		last = err
	}
	for _, p := range cands {
		if _, err := os.Stat(filepath.Dir(p)); err != nil {
			continue
		}
		base := strings.ToLower(filepath.Base(p))
		if base == "users.bbs" || base == "usersidx.bbs" || base == "lastread.bbs" {
			continue
		}
		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0644)
		if err == nil {
			return f, nil
		}
		last = err
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return nil, last
}

func (q *vm) cmdFileRead(rest string) {
	slotWord, dstWord := firstWord(rest)
	slot := atoi(slotWord)
	dst := atoi(dstWord)
	if slot < 1 || slot > maxFileStack || q.files[slot] == nil || q.files[slot].r == nil {
		q.put(dst, "")
		return
	}
	s, err := q.files[slot].r.ReadBytes('\n')
	s = bytes.TrimRight(s, "\r\n \t")
	if err != nil && len(s) == 0 {
		q.put(dst, "")
		return
	}
	q.put(dst, pascal.FromCP437(s))
}

func (q *vm) cmdFileWrite(rest string) {
	slotWord, val := firstWord(rest)
	slot := atoi(slotWord)
	if slot < 1 || slot > maxFileStack || q.files[slot] == nil || q.files[slot].f == nil {
		return
	}
	val = q.value(val)
	_, _ = q.files[slot].f.Write(append(pascal.ToCP437(val), '\r', '\n'))
}

func (q *vm) closeSlot(slot int) {
	if slot < 1 || slot > maxFileStack || q.files[slot] == nil {
		return
	}
	_ = q.files[slot].f.Close()
	q.files[slot] = nil
}

func (q *vm) closeFiles() {
	for i := 1; i <= maxFileStack; i++ {
		q.closeSlot(i)
	}
}

func (q *vm) cmdDelimit(rest string) {
	dstWord, rest := firstWord(rest)
	lenWord, extra := firstWord(rest)
	dst := atoi(dstWord)
	lim := atoi(lenWord)
	s := q.get(dst)
	if extra == "" {
		rs := []rune(s)
		if lim >= 0 && lim < len(rs) {
			s = string(rs[:lim])
		}
		q.put(dst, s)
		return
	}
	fill := ' '
	fore := false
	w1, extra := firstWord(extra)
	if strings.EqualFold(w1, "ZERO") {
		fill = '0'
	}
	w2, _ := firstWord(extra)
	if strings.EqualFold(w2, "FRONT") {
		fore = true
	}
	rs := []rune(s)
	if lim > len(rs) {
		pad := strings.Repeat(string(fill), lim-len(rs))
		if fore {
			s = pad + s
		} else {
			s = s + pad
		}
	} else if lim >= 0 && lim < len(rs) {
		if fore {
			s = string(rs[len(rs)-lim:])
		} else {
			s = string(rs[:lim])
		}
	}
	q.put(dst, s)
}

func (q *vm) gotoLabel(lab string) {
	lab = strings.TrimSpace(lab)
	if strings.HasPrefix(lab, "#") {
		lab = q.get(atoi(lab[1:]))
	}
	// WELC#1 → WELC + answers[1]
	if i := strings.IndexByte(lab, '#'); i >= 0 {
		lab = lab[:i] + q.get(atoi(lab[i+1:]))
	}
	lab = strings.ToUpper(lab)
	if pc, ok := q.labels[lab]; ok {
		q.pc = pc
		q.ifNest = 0
		q.skipIf = false
		q.ignIf = 0
		q.skipWhile = false
		q.ignWhile = 0
		q.whilePC = q.whilePC[:0]
	}
}

func (q *vm) testIf(rest string) bool {
	rest = strings.TrimSpace(rest)
	numeric := false
	left := ""
	i := 0
	for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9' || rest[i] == '~') {
		if rest[i] == '~' {
			numeric = true
			i++
			break
		}
		left += string(rest[i])
		i++
	}
	if i < len(rest) && rest[i] == '~' {
		numeric = true
		i++
	}
	rest = strings.TrimSpace(rest[i:])
	op, rest2 := firstWord(rest)
	right := stripWhileDo(strings.TrimSpace(rest2))
	lv := q.get(atoi(left))
	upOp := strings.ToUpper(op)
	if upOp == "IN" || upOp == "NIN" {
		at := 0
		rv := right
		if rv != "" && (rv[0] == '#' || rv[0] == '%') {
			w1, more := firstWord(rv[1:])
			aw, more := firstWord(more)
			if strings.EqualFold(aw, "AT") {
				atw, _ := firstWord(more)
				at = atoi(atw)
			}
			rv = q.get(atoi(w1))
		} else {
			rv = q.value(right)
		}
		lvU, rvU := pascal.UpCase(lv), pascal.UpCase(rv)
		pos := strings.Index(rvU, lvU)
		found := pos >= 0
		if found && at >= 1 {
			q.put(at, strconv.Itoa(pos+1))
		}
		if upOp == "NIN" {
			return !found
		}
		return found
	}
	rv := q.value(right)
	if numeric {
		a, b := atoi(lv), atoi(rv)
		switch upOp {
		case "=", "==":
			return a == b
		case "<>", "!=":
			return a != b
		case ">":
			return a > b
		case "<":
			return a < b
		case ">=", "=>":
			return a >= b
		case "<=", "=<":
			return a <= b
		}
		return false
	}
	lv, rv = pascal.UpCase(lv), pascal.UpCase(rv)
	switch upOp {
	case "=", "==":
		return lv == rv
	case "<>", "!=":
		return lv != rv
	case ">":
		return lv > rv
	case "<":
		return lv < rv
	case ">=", "=>":
		return lv >= rv
	case "<=", "=<":
		return lv <= rv
	}
	return false
}

func (q *vm) get(n int) string {
	if n < 1 || n > maxAnswers {
		return ""
	}
	return q.ans[n]
}

func (q *vm) put(n int, s string) {
	if n < 1 || n > maxAnswers {
		return
	}
	q.ans[n] = s
}

func (q *vm) value(s string) string {
	s = trimRA(s)
	if s == "" {
		return ""
	}
	if s[0] == '#' {
		return q.get(atoi(s[1:]))
	}
	if s[0] == '%' {
		// Pascal Assign %n: value of the answer whose number is stored in #n.
		return q.get(atoi(q.get(atoi(s[1:]))))
	}
	if s[0] == '"' || s[0] == '\'' {
		return unquote(s)
	}
	if n := atoi(s); n > 0 && n <= maxAnswers && !strings.ContainsAny(s, `\/.`) {
		// bare numbers in DISPLAY are answer slots; in ASSIGN RHS literals stay
		return s
	}
	return unquote(s)
}

func trimRA(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func splitQACmd(line string) (cmd, rest string) {
	line = trimRA(line)
	i := 0
	for i < len(line) {
		c := line[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			break
		}
		i++
	}
	j := i
	for j < len(line) {
		c := line[j]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			j++
			continue
		}
		break
	}
	cmd = strings.ToUpper(line[i:j])
	rest = trimRA(line[j:])
	return cmd, rest
}

func firstWord(s string) (word, rest string) {
	s = trimRA(s)
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	return s[:i], trimRA(s[i:])
}

func (q *vm) makeDisplayStr(s string) string {
	s = trimRA(s)
	var out strings.Builder
	quoted := false
	if len(s) > 0 && (s[0] == '"' || s[0] == '\'') {
		quoted = true
		end := s[0]
		s = s[1:]
		i := 0
		for i < len(s) {
			// Pascal MakeDisplayStr: '\' is only an escape for the closer; backtick is literal RADU.
			if s[i] == '\\' && i+1 < len(s) {
				out.WriteByte(s[i+1])
				i += 2
				continue
			}
			if s[i] == end {
				s = trimRA(s[i+1:])
				break
			}
			out.WriteByte(s[i])
			i++
		}
	}
	w, _ := firstWord(s)
	gotVar := false
	if w != "" && isAnswerNum(w) {
		out.WriteString(q.get(atoi(w)))
		gotVar = true
	} else if !quoted && out.Len() == 0 && s != "" {
		out.WriteString(s)
	}
	res := out.String()
	if !gotVar {
		res = q.expandVars(res)
	}
	return expandPipes(res)
}

func (q *vm) expandVars(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if (s[i] == '#' || s[i] == '%') && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			n := atoi(s[i+1 : j])
			if n >= 1 && n <= maxAnswers {
				out.WriteString(q.get(n))
				i = j - 1
				continue
			}
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func isAnswerNum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	n := atoi(s)
	return n >= 1 && n <= maxAnswers
}

func unquote(s string) string {
	s = trimRA(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		end := s[0]
		out := strings.Builder{}
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				out.WriteByte(s[i+1])
				i++
				continue
			}
			if s[i] == end {
				return out.String()
			}
			out.WriteByte(s[i])
		}
		return out.String()
	}
	return s
}

func expandPipes(s string) string {
	s = strings.ReplaceAll(s, "|", "\r\n")
	return s
}

func atoi(s string) int {
	s = strings.TrimSpace(strings.TrimLeft(s, "#%"))
	i := 0
	sign := 1
	if i < len(s) && s[i] == '-' {
		sign = -1
		i++
	}
	n := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return sign * n
}
