package quest

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/term"
)

type memStream struct{ out bytes.Buffer }

func (m *memStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *memStream) Write(p []byte) (int, error)      { return m.out.Write(p) }
func (m *memStream) Close() error                     { return nil }
func (m *memStream) SetReadDeadline(time.Time) error  { return nil }
func (m *memStream) SetWriteDeadline(time.Time) error { return nil }
func (m *memStream) Local() bool                      { return true }

func TestDisplayKeepsLiteralRaduBackticks(t *testing.T) {
	q := &vm{}
	got := q.makeDisplayStr("\"`B0:`F15:\"")
	if got != "`B0:`F15:" {
		t.Fatalf("makeDisplayStr=%q want `B0:`F15:", got)
	}
}

func TestGotoAnswerLabel(t *testing.T) {
	src := []string{
		"; randwelc",
		"RANDOM 3 1",
		"GOTO WELC#1",
		":WELC0",
		`DISPLAY "zero"`,
		"QUIT",
		":WELC1",
		`DISPLAY "one"`,
		"QUIT",
		":WELC2",
		`DISPLAY "two"`,
		"QUIT",
	}
	q := &vm{labels: map[string]int{}, lines: src}
	for i, ln := range q.lines {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, ":") {
			lab := strings.ToUpper(strings.TrimSpace(s[1:]))
			q.labels[lab] = i
		}
	}
	q.put(1, "2")
	q.gotoLabel("WELC#1")
	if q.pc != q.labels["WELC2"] {
		t.Fatalf("goto pc %d want %d labels=%+v", q.pc, q.labels["WELC2"], q.labels)
	}
}

func TestUnquoteAndPipes(t *testing.T) {
	if unquote(`"hello|world"`) != "hello|world" {
		t.Fatal(unquote(`"hello|world"`))
	}
	if expandPipes("a|b") != "a\r\nb" {
		t.Fatal(expandPipes("a|b"))
	}
}

func TestRunRandwelcStyle(t *testing.T) {
	dir := t.TempDir()
	script := "RANDOM 1 1\r\nGOTO WELC#1\r\n:WELC0\r\nDISPLAY \"picked\"\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "randwelc.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "randwelc", "")
	if !bytes.Contains(st.out.Bytes(), []byte("picked")) {
		t.Fatalf("script output %q", st.out.Bytes())
	}
}

func TestEmulateInputSemicolonStuffsCR(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "enter.q-a"), []byte("EmulateInput ;\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "enter", "")
	ch, ok := tio.PeekKey()
	if !ok || ch != '\r' {
		t.Fatalf("EmulateInput ; should stuff CR, peek=%q ok=%v", ch, ok)
	}
}

func TestSetFlagB2OnOff(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "quickon.q-a"), []byte("Setflag B2 ON\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quickoff.q-a"), []byte("SetFlag B2 OFF\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "quickon", "")
	if !line.User.Flags.HasNamed("B2") {
		t.Fatalf("SETFLAG B2 ON did not set B2: %#v", line.User.Flags)
	}
	Run(tio, g, line, "quickoff", "")
	if line.User.Flags[1]&(1<<1) != 0 {
		t.Fatalf("SETFLAG B2 OFF did not clear B2: %#v", line.User.Flags)
	}
}

func TestGetFlagB2(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chk.q-a"), []byte("GETFLAG 1 B2\r\nDISPLAY #1\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	line.User.Flags.SetNamed("B2", true)
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "chk", "")
	if !bytes.Contains(st.out.Bytes(), []byte("YES")) {
		t.Fatalf("GETFLAG 1 B2 want YES, got %q", st.out.Bytes())
	}
}

func TestIncludeQAStandaloneRunsRandtag(t *testing.T) {
	dir := t.TempDir()
	// INCLUDEQA randtag is its own command; Display is a separate line.
	parent := "INCLUDEQA randtag\r\nDisplay \"`B0:`F15:\"\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "randwelc.q-a"), []byte(parent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "randtag.q-a"), []byte("DISPLAY \"tagline\"\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "randwelc", "")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("tagline")) {
		t.Fatalf("INCLUDEQA randtag did not run: %q", out)
	}
	plain := stripCSI(out)
	if bytes.Contains(plain, []byte("15")) {
		t.Fatalf("Display printed 15 instead of setting color: %q", out)
	}
}

func stripCSI(b []byte) []byte {
	plain := append([]byte(nil), b...)
	for {
		i := bytes.IndexByte(plain, 0x1b)
		if i < 0 {
			break
		}
		j := i + 1
		for j < len(plain) && ((plain[j] >= '0' && plain[j] <= '9') || plain[j] == '[' || plain[j] == ';' || plain[j] == '?') {
			j++
		}
		if j < len(plain) {
			j++
		}
		plain = append(plain[:i:i], plain[j:]...)
	}
	return plain
}

func TestIncludeQARunsSameDirAndSharesAnswers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "randwelc.q-a"), []byte("ASSIGN 1 tagged\r\nINCLUDEQA randtag\r\nDISPLAY \"after\"\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "randtag.q-a"), []byte("DISPLAY \"#1\"\r\nDISPLAY \"tagline\"\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "randwelc", "")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("tagline")) {
		t.Fatalf("INCLUDEQA did not run randtag: %q", out)
	}
	if !bytes.Contains(out, []byte("after")) {
		t.Fatalf("parent did not continue after include: %q", out)
	}
}

func TestIncludeQAFindsUppercaseQAFile(t *testing.T) {
	dir := t.TempDir()
	ques := filepath.Join(dir, "ques")
	if err := os.Mkdir(ques, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "randwelc.q-a"), []byte("INCLUDEQA randtag\r\nDISPLAY \"after\"\r\nQUIT\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ques, "RANDTAG.Q-A"), []byte("DISPLAY \"fromtag\"\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = ques
	line.Language.TextPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "randwelc", "")
	if !bytes.Contains(st.out.Bytes(), []byte("fromtag")) {
		t.Fatalf("INCLUDEQA randtag did not run: %q", st.out.Bytes())
	}
}

func TestDisplayLiteralBacktickRADU(t *testing.T) {
	dir := t.TempDir()
	// Display "`B0:`F15:Hi"  — literal backticks, no backslash
	script := "Display \"`B0:`F15:Hi\"\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "radu.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "radu", "")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("Hi")) {
		t.Fatalf("DISPLAY text missing: %q", out)
	}
	if bytes.Contains(out, []byte("`B")) || bytes.Contains(out, []byte("`F")) {
		t.Fatalf("RADU leaked: %q", out)
	}
	plain := out
	for {
		i := bytes.IndexByte(plain, 0x1b)
		if i < 0 {
			break
		}
		j := i + 1
		for j < len(plain) && ((plain[j] >= '0' && plain[j] <= '9') || plain[j] == '[' || plain[j] == ';' || plain[j] == '?') {
			j++
		}
		if j < len(plain) {
			j++
		}
		plain = append(plain[:i:i], plain[j:]...)
	}
	if bytes.Contains(plain, []byte("15")) {
		t.Fatalf("DISPLAY printed 15 instead of setting color: %q (plain %q)", out, plain)
	}
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Fatalf("expected color CSI, got %q", out)
	}
}

func TestAssignHashCopiesVariableAndKeepsRaColor(t *testing.T) {
	dir := t.TempDir()
	// assign 10 ♂[17  /  assign 11 #8  /  display #10#11
	script := "ASSIGN 8 tagtext\r\nASSIGN 10 \x0b[17\r\nASSIGN 11 #8\r\nDISPLAY #10#11\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "color.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "color", "")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("[17")) || bytes.Contains(out, []byte("[170")) {
		t.Fatalf("RA color ♂[17 leaked as text: %q", out)
	}
	if !bytes.Contains(out, []byte("tagtext")) {
		t.Fatalf("#8 (variable 8) missing: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Fatalf("expected color CSI from ♂[17, got %q", out)
	}
}

func TestRandtagFileReadWhileConcatDisplay(t *testing.T) {
	dir := t.TempDir()
	tags := filepath.Join(dir, "taglines.txt")
	if err := os.WriteFile(tags, []byte("alpha\r\nwanted-tag\r\ngamma\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	script := "ASSIGN 5 " + tags + "\r\n" +
		"ASSIGN 7 0\r\n" +
		"RANDOM 1 7\r\n" +
		"INC 7\r\n" +
		"INC 7\r\n" +
		"ASSIGN 9 1\r\n" +
		"ASSIGN 8 0\r\n" +
		"FILEOPEN 1 #5\r\n" +
		"ASSIGN 9 1\r\n" +
		"ASSIGN 8 0\r\n" +
		"WHILE 9~ <= #7 do\r\n" +
		" FILEREAD 1 8\r\n" +
		" INC 9\r\n" +
		"ENDWHILE\r\n" +
		"FILECLOSE 1\r\n" +
		"SETX 1\r\n" +
		"SETY 25\r\n" +
		"ASSIGN 10 \x0b[17\r\n" +
		"ASSIGN 11 #8\r\n" +
		"DELIMIT 11 79\r\n" +
		"ASSIGN 12 \x0b\\\r\n" +
		"CONCAT 15 10 11\r\n" +
		"CONCAT 15 15 12\r\n" +
		"DISPLAY 15\r\n" +
		"QUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "randtag.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "randtag", "")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("[17")) {
		t.Fatalf("RA color leaked: %q", out)
	}
	if !bytes.Contains(out, []byte("wanted-tag")) {
		t.Fatalf("variable 8 tagline missing: %q", out)
	}
	if bytes.Contains(out, []byte("alpha")) || bytes.Contains(out, []byte("gamma")) {
		t.Fatalf("wrong tagline line: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Fatalf("expected color/cursor CSI, got %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[K")) {
		t.Fatalf("expected clreol from ♂\\, got %q", out)
	}
}

func TestCalcGosubAndFileResult(t *testing.T) {
	dir := t.TempDir()
	script := "" +
		"ASSIGN 1 10\r\n" +
		"ASSIGN 2 3\r\n" +
		"CALC 3 1 * 2\r\n" +
		"GOSUB ADDONE\r\n" +
		"DISPLAY 3\r\n" +
		"QUIT\r\n" +
		":ADDONE\r\n" +
		"INC 3\r\n" +
		"RETURN\r\n"
	if err := os.WriteFile(filepath.Join(dir, "calc.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "calc", "")
	if !bytes.Contains(st.out.Bytes(), []byte("31")) && !bytes.Contains(st.out.Bytes(), []byte("31")) {
		// 10*3=30 then INC -> 31
		if got := st.out.String(); !strings.Contains(got, "31") {
			t.Fatalf("CALC/GOSUB got %q", got)
		}
	}
}

func TestAssignKAExpandsTotalCalls(t *testing.T) {
	dir := t.TempDir()
	script := "Assign 1 \x0bA\r\nDelimit 1 6 ZERO FRONT\r\nDISPLAY 1\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "calls.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, SysInfo: cfgrec.SysInfo{TotalCalls: 42}}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "calls", "")
	if !bytes.Contains(st.out.Bytes(), []byte("000042")) {
		t.Fatalf("Assign ^KA/Delimit want 000042, got %q", st.out.Bytes())
	}
}

func TestAnsiNumberFilesDisplayed(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "txtfiles")
	if err := os.Mkdir(txt, 0755); err != nil {
		t.Fatal(err)
	}
	for d := 0; d <= 9; d++ {
		var body strings.Builder
		for i := 0; i < 6; i++ {
			body.WriteString("DIG")
			body.WriteByte(byte('0' + d))
			body.WriteString("\r\n")
		}
		if err := os.WriteFile(filepath.Join(txt, strconv.Itoa(d)+".ans"), []byte(body.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pathAssign := txt
	if !strings.HasSuffix(pathAssign, `\`) && !strings.HasSuffix(pathAssign, `/`) {
		pathAssign += `\`
	}
	script := "Assign 1 \x0bA\r\n" +
		"Delimit 1 6 ZERO FRONT\r\n" +
		"Assign 2 6\r\n" +
		"Assign 4 1\r\n" +
		"Assign 5 63\r\n" +
		"Assign 7 11\r\n" +
		"ClearScreen\r\n" +
		"While 2~ > 0 Do\r\n" +
		" SubstringVar 3 1 2 4\r\n" +
		" Assign 20 " + pathAssign + "\r\n" +
		" Assign 21 #3\r\n" +
		" Assign 22 .ans\r\n" +
		"Concat 23 20 21\r\n" +
		"Concat 23 23 22\r\n" +
		" FileOpen 1 #23\r\n" +
		" Assign 30 1\r\n" +
		" Assign 46 12\r\n" +
		" While 30~ <= 6 Do\r\n" +
		" FileRead 1 31\r\n" +
		" If 31 = \"\"\r\n" +
		"  Break\r\n" +
		" EndIf\r\n" +
		" Assign 40 \x1b[\r\n" +
		" Assign 47 ;\r\n" +
		" Assign 48 H\r\n" +
		" ConCat 43 40 46\r\n" +
		" ConCat 43 43 47\r\n" +
		" Concat 43 43 5\r\n" +
		" ConCat 43 43 48\r\n" +
		" ConCat 43 43 31\r\n" +
		" Display 43\r\n" +
		" Inc 46\r\n" +
		" Inc 30\r\n" +
		" EndWhile\r\n" +
		" Calc 45 5 - 7\r\n" +
		" Assign 5 #45\r\n" +
		" FileClose 1\r\n" +
		" Dec 2\r\n" +
		"EndWhile\r\n" +
		"Cursor 1 22\r\n" +
		"QUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "digits.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, SysInfo: cfgrec.SysInfo{TotalCalls: 12}}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "digits", "")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("DIG2")) || !bytes.Contains(out, []byte("DIG1")) || !bytes.Contains(out, []byte("DIG0")) {
		t.Fatalf("digit ANSI missing, out=%q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[12;63H")) {
		t.Fatalf("first CUP missing, out=%q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[22;1H")) {
		t.Fatalf("Cursor 1 22 missing, out=%q", out)
	}
}

func TestAssignUserCodeDefaultProtocolSkipsMenu72(t *testing.T) {
	dir := t.TempDir()
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:        "Zmodem",
		ActiveKey:   '@',
		Attribute:   1,
		DnCmdString: `c:\ele\prot\sexyz.exe /P*P /B115200`,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	script := ":start\r\n" +
		"assign 1 \x06!\r\n" +
		"If 1 = \"\"\r\n" +
		" Menucmnd 72\r\n" +
		" goto start\r\n" +
		" Endif\r\n" +
		"If 1 = \"|\"\r\n" +
		" Menucmnd 72\r\n" +
		" goto start\r\n" +
		"Endif\r\n" +
		"QUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "protchk.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{DefaultProto: '@', Record: -1}}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	called := 0
	tio.RunMenu = func(typ byte, _ string) { called++ }
	Run(tio, g, line, "protchk", "")
	if called != 0 {
		t.Fatalf("Menucmnd 72 ran %d times with default protocol set; out=%q", called, st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte("sexyz")) {
		t.Fatalf("protocol.ra dumped: %q", st.out.Bytes())
	}
}

func TestAssignSpadeUserCodeDefaultProtocol(t *testing.T) {
	dir := t.TempDir()
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{Name: "Zmodem", ActiveKey: 'Z', Attribute: 1})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	script := "assign 1 \x06!\r\nDISPLAY 1\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "spade.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{DefaultProto: 'Z', Record: -1}}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	if got := tio.ExpandRA("\u2660!"); got != "Zmodem" {
		t.Fatalf("unicode ♠! ExpandRA = %q want Zmodem", got)
	}
	Run(tio, g, line, "spade", "")
	if !bytes.Contains(st.out.Bytes(), []byte("Zmodem")) {
		t.Fatalf("^F! want Zmodem, got %q", st.out.Bytes())
	}
}

func TestWelcomeMenuCmnd39ShowsConcatFile(t *testing.T) {
	sys := t.TempDir()
	welc := filepath.Join(sys, "txtfiles", "welcome")
	if err := os.MkdirAll(welc, 0755); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(welc, "WELCOME.TXT")
	if err := os.WriteFile(list, []byte("picked.ans\r\nother.ANS\r\n"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(welc, "picked.ans"), []byte("SHOWN-WELCOME"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(welc, "other.ANS"), []byte("WRONG-FILE"), 0644); err != nil {
		t.Fatal(err)
	}
	script := "" +
		"Assign 5 C:\\ELE\\TXTFILES\\WELCOME\\WELCOME.TXT\r\n" +
		"Assign 7 0\r\n" +
		"Random 1 7\r\n" +
		"Inc 7\r\n" +
		"Assign 9 1\r\n" +
		"Assign 8 0\r\n" +
		"FileOpen 1 #5\r\n" +
		"While 9~ <= #7 do\r\n" +
		" FileRead 1 8\r\n" +
		" Inc 9\r\n" +
		"EndWhile\r\n" +
		"FileClose 1\r\n" +
		"Assign 10 c:\\ele\\txtfiles\\welcome\\\r\n" +
		"ConCat 13 10 8\r\n" +
		"MenuCmnd 39 #13\r\n" +
		"QUIT\r\n"
	if err := os.WriteFile(filepath.Join(sys, "randwelc.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = sys
	g.RaConfig.LogFileName = filepath.Join(sys, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = sys
	line.Language.TextPath = filepath.Join(sys, "txtfiles")
	st := &memStream{}
	tio := term.New(st, g, line)
	tio.RunMenu = func(typ byte, data string) {
		if typ != 39 {
			t.Errorf("menu type %d want 39", typ)
			return
		}
		if !strings.Contains(strings.ToLower(data), "picked.ans") {
			t.Errorf("var 13 %q should contain picked.ans", data)
		}
		term.DisplayHotFile(tio, line.Language.TextPath, data)
	}
	Run(tio, g, line, "randwelc", "")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("SHOWN-WELCOME")) {
		t.Fatalf("welcome file in #13 not displayed: %q", out)
	}
	if bytes.Contains(out, []byte("WRONG-FILE")) {
		t.Fatalf("wrong welcome file: %q", out)
	}
}

type keyMem struct {
	out bytes.Buffer
	in  []byte
}

func (s *keyMem) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.in)
	s.in = s.in[n:]
	return n, nil
}
func (s *keyMem) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *keyMem) Close() error                     { return nil }
func (s *keyMem) SetReadDeadline(time.Time) error  { return nil }
func (s *keyMem) SetWriteDeadline(time.Time) error { return nil }
func (s *keyMem) Local() bool                      { return true }

func TestGetRecordInfoHook(t *testing.T) {
	q := &vm{getInfo: func(rec, start int, down bool, put func(int, string)) {
		if rec != 5 || !down || start != 30 {
			t.Fatalf("hook rec=%d start=%d down=%v", rec, start, down)
		}
		put(start, "7")
		put(start+1, "12")
		put(start+2, "General")
		put(start+3, "*")
	}}
	q.put(8, "5")
	q.cmdExtra("GETRECORDINFO", "#8 30 DOWN")
	if q.get(30) != "7" || q.get(31) != "12" || q.get(32) != "General" || q.get(33) != "*" {
		t.Fatalf("answers %q %q %q %q", q.get(30), q.get(31), q.get(32), q.get(33))
	}
}

func TestGetRawKeyArrowsKeepCR(t *testing.T) {
	st := &keyMem{in: []byte{27, '[', 'A', '\r'}}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{}
	q := &vm{t: term.New(st, g, line)}
	q.cmdGetRaw("16")
	if q.get(16) != "UP" {
		t.Fatalf("arrow got %q", q.get(16))
	}
	q.cmdGetRaw("16")
	if q.get(16) != "\r" {
		t.Fatalf("CR got %q", q.get(16))
	}
}

func TestGetRawKeyUnknownCSIIsNotEscape(t *testing.T) {
	st := &keyMem{in: []byte{27, '[', 'I'}}
	q := &vm{t: term.New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})}
	q.cmdGetRaw("16")
	if q.get(16) == "\x1b" {
		t.Fatal("focus CSI must not look like ESC in the changer")
	}
}

func TestGetRawKeyEOFIsEscape(t *testing.T) {
	st := &keyMem{}
	q := &vm{t: term.New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})}
	q.cmdGetRaw("16")
	if q.get(16) != "\x1b" {
		t.Fatalf("EOF got %q", q.get(16))
	}
}

func TestOrdDestSrcAndAsciiTwoArgs(t *testing.T) {
	q := &vm{}
	q.put(16, "\r")
	q.cmdExtra("ORD", "17 16")
	if q.get(17) != "13" {
		t.Fatalf("ORD dest src got %q want 13", q.get(17))
	}
	if !q.testIf("17 = 13") {
		t.Fatal("If 17 = 13 should match Enter")
	}
	q.put(16, "\x1b")
	q.cmdExtra("ORD", "17 16")
	if q.get(17) != "27" {
		t.Fatalf("ORD ESC got %q want 27", q.get(17))
	}
	if !q.testIf("17 = 27") {
		t.Fatal("If 17 = 27 should match ESC")
	}
	q.cmdExtra("ASCII", "33 32")
	if q.get(33) != " " {
		t.Fatalf("ASCII 33 32 got %q", q.get(33))
	}
}

func TestDisplaySplitsRaduXBeforeName(t *testing.T) {
	dir := t.TempDir()
	script := "Assign 10 20\r\nAssign 32 General\r\nDisplay \"|`X\" 10\r\nDisplay \":\" 32\r\nQUIT\r\n"
	if err := os.WriteFile(filepath.Join(dir, "chng.q-a"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	st := &memStream{}
	tio := term.New(st, g, line)
	Run(tio, g, line, "chng", "")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte(":General")) {
		t.Fatalf("colon leaked around name: %q", out)
	}
	if !bytes.Contains(out, []byte("General")) {
		t.Fatalf("name missing: %q", out)
	}
}

func TestAssignPercentIsIndirect(t *testing.T) {
	q := &vm{}
	q.put(17, "28")
	q.put(21, "21")
	q.put(28, "\x1b")
	q.exec("Assign 16 %17")
	if q.get(16) != "\x1b" {
		t.Fatalf("Assign 16 %%17 got %q want ESC (changer Q remap)", q.get(16))
	}
	q.put(16, "Q")
	q.put(20, "824671Q")
	if !q.testIf("16 in #20 at 17") {
		t.Fatal("Q should be in remap list")
	}
	q.exec("Calc 17 17 + 21")
	if q.get(17) != "28" {
		t.Fatalf("Calc remap slot %q want 28", q.get(17))
	}
	q.exec("Assign 16 %17")
	if q.get(16) != "\x1b" {
		t.Fatalf("Q remap got %q want ESC", q.get(16))
	}
}

func TestGetRawKeyUpperQ(t *testing.T) {
	st := &keyMem{in: []byte{'q', '\n'}}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{}
	q := &vm{t: term.New(st, g, line)}
	q.cmdGetRaw("16")
	if q.get(16) != "Q" {
		t.Fatalf("q got %q want Q", q.get(16))
	}
	q.cmdGetRaw("16")
	if q.get(16) != "\r" {
		t.Fatalf("LF as CR got %q", q.get(16))
	}
}

func TestIfInAtAndEquals(t *testing.T) {
	q := &vm{}
	q.put(16, "8")
	q.put(20, "824671Q")
	if !q.testIf("16 in #20 at 17") {
		t.Fatal("IN should match")
	}
	if q.get(17) != "1" {
		t.Fatalf("AT pos %q", q.get(17))
	}
	q.put(7, "16")
	q.put(4, "16")
	if !q.testIf("7~ => #4") {
		t.Fatal("=> should be >=")
	}
	q.put(16, "UP")
	if !q.testIf("16 = UP") {
		t.Fatal("UP compare")
	}
}

func TestDoContinueUsesStopMore(t *testing.T) {
	st := &memStream{}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{}
	tio := term.New(st, g, line)
	q := &vm{t: tio}
	q.cmdExtra("DOCONTINUE", "5")
	if q.get(5) != "YES" {
		t.Fatalf("DoContinue=%q want YES (not AskYesNo)", q.get(5))
	}
	tio.StopMore = true
	q.cmdExtra("DOCONTINUE", "5")
	if q.get(5) != "NO" {
		t.Fatalf("StopMore DoContinue=%q want NO", q.get(5))
	}
	if bytes.Contains(st.out.Bytes(), []byte("correct")) || bytes.Contains(st.out.Bytes(), []byte("Correct")) {
		t.Fatalf("DoContinue asked Is this correct?: %q", st.out.Bytes())
	}
}

func TestFileWriteUsesCP437(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := []byte("Fileopen 1 " + out + "\r\nFilewrite 1 \xC4\r\nFileclose 1\r\nQUIT\r\n")
	if err := os.WriteFile(filepath.Join(dir, "fw.q-a"), script, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.QuesPath = dir
	tio := term.New(st, g, line)
	Run(tio, g, line, "fw", "")
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte{0xC4}) {
		t.Fatalf("want CP437 0xC4, got %q", b)
	}
	if bytes.Contains(b, []byte{0xE2, 0x94, 0x80}) {
		t.Fatalf("Filewrite wrote UTF-8: %q", b)
	}
}
