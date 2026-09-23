package term

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/pascal"
)

type memStream struct{ out bytes.Buffer }

func (m *memStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *memStream) Write(p []byte) (int, error)      { return m.out.Write(p) }
func (m *memStream) Close() error                     { return nil }
func (m *memStream) SetReadDeadline(time.Time) error  { return nil }
func (m *memStream) SetWriteDeadline(time.Time) error { return nil }
func (m *memStream) Local() bool                      { return true }

type keyStream struct {
	out bytes.Buffer
	in  []byte
}

func (s *keyStream) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.in)
	s.in = s.in[n:]
	return n, nil
}
func (s *keyStream) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *keyStream) Close() error                     { return nil }
func (s *keyStream) SetReadDeadline(time.Time) error  { return nil }
func (s *keyStream) SetWriteDeadline(time.Time) error { return nil }
func (s *keyStream) Local() bool                      { return true }

func TestWriteRARunsQAAtControl(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	g := &cfgrec.GlobalCfg{}
	io := New(st, g, line)
	var gotName, gotArgs string
	io.RunScript = func(name, args string) {
		gotName, gotArgs = name, args
	}
	io.writeRA([]byte{0x0B, '@', 'r', 'a', 'n', 'd', 'w', 'e', 'l', 'c', '\r', '\n', 'x'}, nil)
	if gotName != "randwelc" {
		t.Fatalf("RunScript name=%q args=%q out=%q", gotName, gotArgs, st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x0B}) || bytes.Contains(st.out.Bytes(), []byte("@randwelc")) {
		t.Fatalf("control leaked into output: %q", st.out.Bytes())
	}
	if !bytes.Contains(st.out.Bytes(), []byte{'x'}) {
		t.Fatalf("expected trailing text, got %q", st.out.Bytes())
	}
}

func TestWriteRARunsQAFromUTF8MaleSign(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	io := New(st, &cfgrec.GlobalCfg{}, line)
	var got string
	io.RunScript = func(name, _ string) { got = name }
	lead := []byte{0xE2, 0x99, 0x82, '@'}
	io.writeRA(append(lead, []byte("randwelc|\x1b[0m")...), nil)
	if got != "randwelc" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayFileRunsEmbeddedQA(t *testing.T) {
	dir := t.TempDir()
	body := []byte{0x0B, '@'}
	body = append(body, []byte("randwelc\r\nOK")...)
	if err := os.WriteFile(filepath.Join(dir, "WELCOME.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	g.RaConfig.SysPath = dir
	io := New(st, g, line)
	var got string
	io.RunScript = func(name, _ string) { got = name }
	if !DisplayFile(io, dir, "WELCOME") {
		t.Fatal("DisplayFile missed WELCOME.ANS")
	}
	if got != "randwelc" {
		t.Fatalf("script %q", got)
	}
}

func TestCtrlEDisablesMorePrompt(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.MorePrompt = true
	tio.Length = 8
	body := append([]byte{0x05}, bytes.Repeat([]byte("line\n"), 40)...)
	tio.writeRA(body, nil)
	if bytes.Contains(st.out.Bytes(), []byte("-- More --")) {
		t.Fatalf("more prompt still fired after ^E: %q", st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x05}) {
		t.Fatal("ctrl-e leaked into output")
	}
	if line.DispMorePrompt {
		t.Fatal("DispMorePrompt should stay off after writeRA ^E")
	}
}

func TestMorePromptWithoutCtrlE(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.MorePrompt = true
	tio.Length = 8
	tio.writeRA(bytes.Repeat([]byte("line\n"), 20), nil)
	if !bytes.Contains(st.out.Bytes(), []byte("-- More --")) {
		t.Fatalf("expected more prompt, got %q", st.out.Bytes())
	}
}

func TestDisplayFileCtrlEStaysOff(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte{0x05}, []byte("\x1b[2Jhello\n")...)
	if err := os.WriteFile(filepath.Join(dir, "TOP.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	if !DisplayFile(tio, dir, "TOP") {
		t.Fatal("missing TOP.ANS")
	}
	if line.DispMorePrompt {
		t.Fatal("^E in the file should leave DispMorePrompt off")
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x05}) {
		t.Fatal("ctrl-e leaked")
	}
}

func TestDisplayFileCtrlAWaitsForEnter(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte("\x1b[0mHello"), 0x01)
	body = append(body, []byte("World")...)
	if err := os.WriteFile(filepath.Join(dir, "PAUSE.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &keyStream{in: []byte{'X', '\r'}}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	if !DisplayFile(tio, dir, "PAUSE") {
		t.Fatal("missing PAUSE.ANS")
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x01}) {
		t.Fatalf("^A leaked as a glyph instead of waiting: %q", st.out.Bytes())
	}
	if !bytes.Contains(st.out.Bytes(), []byte("Hello")) || !bytes.Contains(st.out.Bytes(), []byte("World")) {
		t.Fatalf("text missing: %q", st.out.Bytes())
	}
	if len(st.in) != 0 {
		t.Fatalf("WaitEnter should consume keys until CR, leftover %q", st.in)
	}
}

func TestDisplayHotFileCtrlAWaitsForEnter(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte("\x1b[1;33mPAUSE"), 0x01)
	body = append(body, []byte("DONE")...)
	if err := os.WriteFile(filepath.Join(dir, "INFO.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &keyStream{in: []byte{'\r'}}
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	if !DisplayHotFile(tio, dir, "INFO") {
		t.Fatal("missing INFO.ANS")
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x01}) {
		t.Fatalf("type 5 ^A leaked as a glyph: %q", st.out.Bytes())
	}
	if !bytes.Contains(st.out.Bytes(), []byte("PAUSE")) || !bytes.Contains(st.out.Bytes(), []byte("DONE")) {
		t.Fatalf("text missing: %q", st.out.Bytes())
	}
	if len(st.in) != 0 {
		t.Fatalf("type 5 ^A must WaitEnter, leftover %q", st.in)
	}
}

func TestDisplayHotFileNoMoreOnAnsiWithCtrlE(t *testing.T) {
	dir := t.TempDir()
	var body []byte
	body = append(body, 0x05) // ^E disable more
	body = append(body, []byte("\x1b[2J\x1b[1;1H")...)
	for i := 0; i < 40; i++ {
		body = append(body, 0x04) // CP437 diamond; must not re-enable more
		body = append(body, []byte("LINE\r\n")...)
	}
	if err := os.WriteFile(filepath.Join(dir, "WELCOME.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	tio.Length = 8
	if !DisplayHotFile(tio, dir, "WELCOME") {
		t.Fatal("missing WELCOME.ANS")
	}
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("-- More --")) {
		t.Fatalf("welcome ansi paged: %q", out)
	}
	if bytes.Contains(out, []byte{0x05}) {
		t.Fatal("ctrl-e leaked to terminal")
	}
}

func TestAnsiDiamondDoesNotReenableMore(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.MorePrompt = true
	tio.Length = 8
	var body []byte
	body = append(body, 0x05)
	body = append(body, []byte("\x1b[0m")...)
	for i := 0; i < 40; i++ {
		body = append(body, 0x04, '\n')
	}
	tio.writeRA(body, nil)
	if bytes.Contains(st.out.Bytes(), []byte("-- More --")) {
		t.Fatalf("diamond 0x04 re-enabled more: %q", st.out.Bytes())
	}
	if line.DispMorePrompt {
		t.Fatal("more should stay off after ^E")
	}
}

type countStream struct {
	memStream
	n int
}

func (c *countStream) Write(p []byte) (int, error) {
	c.n++
	return c.memStream.Write(p)
}

func TestWriteRABatchesANSI(t *testing.T) {
	st := &countStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	var body []byte
	for i := 0; i < 80; i++ {
		body = append(body, []byte("\x1b[1;32mX")...)
	}
	tio.writeRA(body, nil)
	if st.n > 5 {
		t.Fatalf("ANSI written in %d syscalls, want a handful of batches", st.n)
	}
}

func TestRaduBFSetsColors(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.WriteRA("`B0:`F15:Hello")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte{0x08}) {
		t.Fatal("`B was treated as backspace")
	}
	if bytes.Contains(out, []byte("`B")) || bytes.Contains(out, []byte("`F")) {
		t.Fatalf("RADU codes leaked: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[0;1;37;40m")) {
		t.Fatalf("expected bright white on black, got %q", out)
	}
	if !bytes.Contains(out, []byte("Hello")) {
		t.Fatalf("text missing: %q", out)
	}
	plain := out
	for {
		i := bytes.IndexByte(plain, 0x1b)
		if i < 0 {
			break
		}
		j := i + 1
		for j < len(plain) && plain[j] != 'm' {
			j++
		}
		if j < len(plain) {
			plain = append(plain[:i], plain[j+1:]...)
		} else {
			break
		}
	}
	if !bytes.Contains(plain, []byte("Hello")) {
		t.Fatalf("stripped text missing Hello: %q", out)
	}
}

func TestRaduKIsNotACode(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("`K:")
	if !bytes.Contains(st.out.Bytes(), []byte("`K:")) {
		t.Fatalf("Pascal RADU has no `K; WriteRA must not swallow it, got %q", st.out.Bytes())
	}
}

func TestDisplayHotAcceptsHotkeyWithoutEnter(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte{0x01, 0x05}, []byte("MENUANS")...)
	if err := os.WriteFile(filepath.Join(dir, "TOP.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &keyStream{in: []byte{'F'}}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	abort, found := DisplayHot(tio, dir, "TOP", map[byte]struct{}{'F': {}, 'f': {}})
	if !found {
		t.Fatal("missing TOP.ANS")
	}
	if abort != 'F' && abort != 'f' {
		t.Fatalf("hotkey F not accepted, abort=%q out=%q", abort, st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte("Press (Enter)")) {
		t.Fatalf("type 40 waited for enter: %q", st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte("-- More --")) {
		t.Fatalf("type 40 showed -- more --: %q", st.out.Bytes())
	}
}

func TestDisplayHotDoesNotWaitEnterOnSOH(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte{0x01}, []byte("TOPANS")...)
	if err := os.WriteFile(filepath.Join(dir, "TOP.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	abort, found := DisplayHot(tio, dir, "TOP", map[byte]struct{}{'F': {}, 'f': {}})
	if !found {
		t.Fatal("missing TOP.ANS")
	}
	if abort != 0 {
		t.Fatalf("abort=%d", abort)
	}
	if bytes.Contains(st.out.Bytes(), []byte("Press (Enter)")) {
		t.Fatalf("type 40 waited for enter: %q", st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte("-- More --")) {
		t.Fatalf("type 40 showed -- more --: %q", st.out.Bytes())
	}
	if !bytes.Contains(st.out.Bytes(), []byte("TOPANS")) {
		t.Fatalf("ansi missing: %q", st.out.Bytes())
	}
}

func TestDisplayHotNoMorePromptOnCtrlD(t *testing.T) {
	dir := t.TempDir()
	var body []byte
	body = append(body, 0x04) // ^D would re-enable more on a normal file
	for i := 0; i < 40; i++ {
		body = append(body, []byte("LINE\r\n")...)
	}
	body = append(body, []byte("ENDANS")...)
	if err := os.WriteFile(filepath.Join(dir, "TOP.ANS"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = dir
	tio := New(st, g, line)
	tio.Length = 24
	abort, found := DisplayHot(tio, dir, "TOP", map[byte]struct{}{'F': {}, 'f': {}})
	if !found {
		t.Fatal("missing TOP.ANS")
	}
	if abort != 0 {
		t.Fatalf("abort=%d", abort)
	}
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("-- More --")) {
		t.Fatalf("type 40 showed -- more --: %q", out)
	}
	if bytes.Contains(out, []byte("Press (Enter)")) {
		t.Fatalf("type 40 waited for enter: %q", out)
	}
	if !bytes.Contains(out, []byte("ENDANS")) {
		t.Fatalf("ansi truncated: %q", out)
	}
}

func TestCtrlWDelaysOneSecond(t *testing.T) {
	old := raDelay
	defer func() { raDelay = old }()
	var waited time.Duration
	raDelay = func(d time.Duration) { waited += d }
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.writeRA([]byte("AB\x17CD"), nil)
	if waited != time.Second {
		t.Fatalf("Ctrl-W delay %v want 1s", waited)
	}
	if !bytes.Contains(st.out.Bytes(), []byte("AB")) || !bytes.Contains(st.out.Bytes(), []byte("CD")) {
		t.Fatalf("text missing: %q", st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte{0x17}) {
		t.Fatalf("Ctrl-W leaked: %q", st.out.Bytes())
	}
}

func TestRaColorBracketHex(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("\x0b[17Hello")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("[17")) || bytes.Contains(out, []byte("[170")) {
		t.Fatalf("♂[17 leaked: %q", out)
	}
	if !bytes.Contains(out, []byte("Hello")) {
		t.Fatalf("text missing: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Fatalf("expected CSI color, got %q", out)
	}
}

func TestGetArrowKeysConsumesExtendedCSI(t *testing.T) {
	st := &keyStream{in: []byte("\x1b[1;5CF")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	ch, err := tio.GetKey(0)
	if err != nil || ch != 0x1b {
		t.Fatalf("esc: %q %v", ch, err)
	}
	if got := tio.GetArrowKeys(); got != 'C' {
		t.Fatalf("RIGHT CSI got %q", got)
	}
	ch, err = tio.GetKey(0)
	if err != nil || ch != 'F' {
		t.Fatalf("trailing key swallowed: ch=%q err=%v rest=%q", ch, err, st.in)
	}
}

func TestGetKeyDoesNotSwallowFollowingHotKey(t *testing.T) {
	st := &keyStream{in: []byte("\x1b[AF")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	ch, err := tio.GetKey(0)
	if err != nil || ch != 0x1b {
		t.Fatalf("esc: %q %v", ch, err)
	}
	if got := tio.GetArrowKeys(); got != 'A' {
		t.Fatalf("arrow %q", got)
	}
	ch, err = tio.GetKey(0)
	if err != nil || ch != 'F' {
		t.Fatalf("hotkey swallowed: ch=%q err=%v rest=%q", ch, err, st.in)
	}
}

func TestRaduXYKeepsColumn(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("`X10:`Y5:")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("\x1b[5;10H")) {
		t.Fatalf("expected CUP 5;10, got %q", out)
	}
	if bytes.Contains(out, []byte("\x1b[5H")) {
		t.Fatalf("`Y` reset column: %q", out)
	}
}

func TestRaduRelativeYPagesUp(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("`Y19:")
	if tio.WhereY() != 19 {
		t.Fatalf("start Y=%d", tio.WhereY())
	}
	for i := 0; i < 16; i++ {
		tio.WriteRA("`Y-1:`X20:")
	}
	if tio.WhereY() != 3 {
		t.Fatalf("after 16 × Y-1, Y=%d want 3 (LEFT page in MA-CHNG)", tio.WhereY())
	}
	if tio.WhereX() != 20 {
		t.Fatalf("X=%d want 20", tio.WhereX())
	}
}

func TestRaduRelativeXY(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("`X10:`Y8:")
	tio.WriteRA("`X+2:`Y-1:")
	if tio.WhereX() != 12 || tio.WhereY() != 7 {
		t.Fatalf("X,Y=%d,%d want 12,7", tio.WhereX(), tio.WhereY())
	}
}

func TestPressEnterUsesRalPrompt258(t *testing.T) {
	dir := t.TempDir()
	b := make([]byte, 640)
	binary.LittleEndian.PutUint16(b[0:], 258)
	binary.LittleEndian.PutUint16(b[258*2:], 540)
	b[538] = 0x0E
	copy(b[539:], []byte{0x0B, '@', 'E', 'N', 'T', 'E', 'R'})
	if err := os.WriteFile(filepath.Join(dir, "english.ral"), b, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, Language: cfgrec.Language{DefName: "english.ral", TextPath: dir}}
	st := &memStream{}
	tio := New(st, g, line)
	tio.Ral = lang.Load(g, line.Language)
	var got string
	tio.RunScript = func(name, _ string) { got = name }
	tio.PressEnter()
	if got != "ENTER" {
		t.Fatalf("RAL #258 ^K@ENTER should run ENTER.q-a, got %q out=%q", got, st.out.Bytes())
	}
	if bytes.Contains(st.out.Bytes(), []byte("Press (Enter)")) || bytes.Contains(st.out.Bytes(), []byte("Press (Enter) to continue")) {
		t.Fatalf("hardcoded enter prompt leaked: %q", st.out.Bytes())
	}
}

type readCountStream struct {
	memStream
	reads int
}

func (s *readCountStream) Read([]byte) (int, error) {
	s.reads++
	return 0, io.EOF
}

func TestPutInBufferSemicolonIsCR(t *testing.T) {
	st := &memStream{}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	tio := New(st, g, line)
	tio.PutInBuffer(";")
	ch, err := tio.GetKey(0)
	if err != nil || ch != '\r' {
		t.Fatalf("PutInBuffer ; want CR, got %q err=%v", ch, err)
	}
	tio.PutInBuffer("A_B")
	var got []byte
	for i := 0; i < 3; i++ {
		c, err := tio.GetKey(0)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	if string(got) != "A B" {
		t.Fatalf("PutInBuffer _ want space, got %q", got)
	}
}

func TestPressEnterConsumesEmulatedCRFromEnterQA(t *testing.T) {
	dir := t.TempDir()
	b := make([]byte, 640)
	binary.LittleEndian.PutUint16(b[0:], 258)
	binary.LittleEndian.PutUint16(b[258*2:], 540)
	b[538] = 0x0E
	copy(b[539:], []byte{0x0B, '@', 'E', 'N', 'T', 'E', 'R'})
	if err := os.WriteFile(filepath.Join(dir, "english.ral"), b, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, Language: cfgrec.Language{DefName: "english.ral", TextPath: dir}}
	st := &readCountStream{}
	tio := New(st, g, line)
	tio.Ral = lang.Load(g, line.Language)
	// ENTER.q-a ends with EmulateInput ; which stuffs CR for PressEnter's ^A.
	tio.RunScript = func(name, _ string) {
		if name != "ENTER" {
			t.Fatalf("script %q", name)
		}
		tio.PutInBuffer(";")
	}
	tio.PressEnter()
	if st.reads != 0 {
		t.Fatalf("PressEnter waited for a second key after ENTER.q-a (reads=%d)", st.reads)
	}
}

func TestDisplayHotFileAbsoluteEleRemap(t *testing.T) {
	sys := t.TempDir()
	welc := filepath.Join(sys, "txtfiles", "welcome")
	if err := os.MkdirAll(welc, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(welc, "JIJI.ANS"), []byte("JIJI-ART"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.TextPath = filepath.Join(sys, "txtfiles")
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = sys
	tio := New(st, g, line)
	if !DisplayHotFile(tio, line.Language.TextPath, `c:\ele\txtfiles\welcome\JIJI.ANS`) {
		t.Fatal("missing remapped JIJI.ANS")
	}
	if !bytes.Contains(st.out.Bytes(), []byte("JIJI-ART")) {
		t.Fatalf("got %q", st.out.Bytes())
	}
}

func TestWriteRAExpandsSystemAndUserCodes(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{
		AnsiOn:   true,
		RaNodeNr: 3,
		Baud:     65529,
		User: cfgrec.User{
			Name:     "Martin Sysop",
			Handle:   "Marty",
			Location: "Denver",
			Security: 100,
		},
		SysInfo: cfgrec.SysInfo{TotalCalls: 42, LastCaller: "Bob"},
	}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.WriteRA("\x0bA|\x06A|\x06W|\x063|\x0bW|\x06O")
	got := st.out.Bytes()
	for _, want := range []string{"42", "Martin Sysop", "Martin", "Marty", "3", "100"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if bytes.Contains(got, []byte{0x0B}) || bytes.Contains(got, []byte{0x06}) {
		t.Fatalf("control leaked: %q", got)
	}
}

func TestExpandRAUserNameIsSpadeA(t *testing.T) {
	line := &cfgrec.LineCfg{User: cfgrec.User{Name: "Martin Sysop"}}
	tio := New(&memStream{}, &cfgrec.GlobalCfg{}, line)
	if got := tio.ExpandRA("\u2660A"); got != "Martin Sysop" {
		t.Fatalf("ExpandRA ♠A = %q", got)
	}
	if got := tio.ExpandRA("\x0bW"); got != "0" {
		t.Fatalf("ExpandRA ^KW node = %q", got)
	}
	line.RaNodeNr = 4
	if got := tio.ExpandRA("\x0bW"); got != "4" {
		t.Fatalf("ExpandRA ^KW = %q", got)
	}
}

func TestExpandRAAreaAndGroupCodes(t *testing.T) {
	dir := t.TempDir()
	msg := make([]byte, cfgrec.MessageSize)
	w := &pascal.Writer{}
	w.U16(7)
	w.U16(0)
	w.PString(40, "General")
	copy(msg, w.B)
	if err := os.WriteFile(filepath.Join(dir, "MESSAGES.RA"), msg, 0644); err != nil {
		t.Fatal(err)
	}
	fil := make([]byte, cfgrec.FilesRecSize)
	w = &pascal.Writer{}
	w.U16(3)
	w.U16(0)
	w.PString(40, "Uploads")
	copy(fil, w.B)
	if err := os.WriteFile(filepath.Join(dir, "FILES.RA"), fil, 0644); err != nil {
		t.Fatal(err)
	}
	mgr := make([]byte, cfgrec.GroupSize)
	w = &pascal.Writer{}
	w.U16(2)
	w.PString(40, "Mail")
	copy(mgr, w.B)
	if err := os.WriteFile(filepath.Join(dir, "MGROUPS.RA"), mgr, 0644); err != nil {
		t.Fatal(err)
	}
	fgr := make([]byte, cfgrec.GroupSize)
	w = &pascal.Writer{}
	w.U16(4)
	w.PString(40, "Files")
	copy(fgr, w.B)
	if err := os.WriteFile(filepath.Join(dir, "FGROUPS.RA"), fgr, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{User: cfgrec.User{MsgArea: 7, FileArea: 3, MsgGroup: 2, FileGroup: 4}}
	tio := New(&memStream{}, g, line)
	if got := tio.ExpandRA("\x0bY"); got != "General" {
		t.Fatalf("^KY = %q", got)
	}
	if got := tio.ExpandRA("\x0bZ"); got != "Uploads" {
		t.Fatalf("^KZ = %q", got)
	}
	if got := tio.ExpandRA("\x06)"); got != "Mail" {
		t.Fatalf("^F) = %q", got)
	}
	if got := tio.ExpandRA("\x06#"); got != "Files" {
		t.Fatalf("^F# = %q", got)
	}
	st := &memStream{}
	tio = New(st, g, line)
	tio.WriteRA("\x0bY \x06) \x0bZ \x06#")
	got := string(st.out.Bytes())
	if !strings.Contains(got, "General") || !strings.Contains(got, "Mail") || !strings.Contains(got, "Uploads") || !strings.Contains(got, "Files") {
		t.Fatalf("WriteRA expanded %q", got)
	}
}

func TestWriteRADotPaddedCodes(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{
		AnsiOn:  true,
		User:    cfgrec.User{Name: "Martin Sysop"},
		SysInfo: cfgrec.SysInfo{TotalCalls: 42},
	}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	tio.WriteRA("\x0b....A|\x06....A")
	got := string(st.out.Bytes())
	if !strings.Contains(got, "  42") {
		t.Fatalf("^K....A want right-padded 42, got %q", got)
	}
	if !strings.Contains(got, "Mart") {
		t.Fatalf("^F....A want left-clipped Martin, got %q", got)
	}
	if strings.Contains(got, "....") {
		t.Fatalf("pad dots leaked: %q", got)
	}
}

func TestTitleCaseFirstLetterOfEachWord(t *testing.T) {
	cases := []struct{ in, want string }{
		{"shurato", "Shurato"},
		{"SHURATO", "Shurato"},
		{"martin kazmaier", "Martin Kazmaier"},
		{"MARTIN KAZMAIER", "Martin Kazmaier"},
		{"jean-luc picard", "Jean-Luc Picard"},
	}
	for _, c := range cases {
		if got := titleCase(c.in); got != c.want {
			t.Fatalf("titleCase(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestGetStringIEMSIStopsLeavingPacket(t *testing.T) {
	rest := []byte("ICI0004{ab}AABBCCDD")
	st := &keyStream{in: append([]byte("**EMSI_"), rest...)}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	got, err := tio.GetStringIEMSI(35, true)
	if err != nil {
		t.Fatal(err)
	}
	if pascal.UpCase(got) != "**EMSI_" {
		t.Fatalf("got %q", got)
	}
	if string(st.in) != string(rest) {
		t.Fatalf("leftover %q want %q", st.in, rest)
	}
}

func TestGetStringConsumesCRLFLeavingNoEnter(t *testing.T) {
	st := &keyStream{in: []byte("secret\r\nNEXT")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	got, err := tio.GetString(15, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Fatalf("GetString=%q", got)
	}
	ch, err := tio.GetKey(0)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'N' {
		t.Fatalf("leftover enter passed through, next=%q rest=%q", ch, st.in)
	}
}

func TestGetStringIgnoresLeftoverLF(t *testing.T) {
	st := &keyStream{in: []byte("\nhello\r")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	got, err := tio.GetString(40, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("GetString=%q want hello (LF after node Enter aborted message ASK)", got)
	}
}

func TestDrainLineEndsDropsTelnetEnter(t *testing.T) {
	st := &keyStream{in: []byte("\nM")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	tio.DrainLineEnds()
	ch, err := tio.GetKey(0)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'M' {
		t.Fatalf("got %q", ch)
	}
}

func TestGetStringConsumesCRNULLeavingNoEnter(t *testing.T) {
	st := &keyStream{in: []byte("secret\r\x00NEXT")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	got, err := tio.GetString(15, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Fatalf("GetString=%q", got)
	}
	ch, err := tio.GetKey(0)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'N' {
		t.Fatalf("leftover enter passed through, next=%q rest=%q", ch, st.in)
	}
}

func TestDrainLineEndsDropsCRNULBurst(t *testing.T) {
	st := &keyStream{in: []byte("\r\n\x00\rM")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	tio.DrainLineEnds()
	ch, err := tio.GetKey(0)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'M' {
		t.Fatalf("got %q rest=%q", ch, st.in)
	}
}

func TestGetStringCapitalizesWordsLive(t *testing.T) {
	st := &keyStream{in: []byte("martin kazmaier\r")}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{})
	got, err := tio.GetString(35, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Martin Kazmaier" {
		t.Fatalf("GetString=%q", got)
	}
	if !bytes.Contains(st.out.Bytes(), []byte("Martin Kazmaier")) {
		t.Fatalf("echo=%q", st.out.Bytes())
	}
}

func TestWriteRABangDisplaysTxtfileNotBareK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ANSI.ANS"), []byte("FROMANSI"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.TextPath = dir
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.TextPath = t.TempDir() // not where ANSI.ANS lives
	tio := New(st, g, line)
	tio.WriteRA("\x0b!ANSI|after")
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("FROMANSI")) {
		t.Fatalf("^K!ANSI should show txtfiles ANSI.ANS, got %q", out)
	}
	if !bytes.Contains(out, []byte("after")) {
		t.Fatalf("text after | missing: %q", out)
	}

	st.out.Reset()
	tio.WriteRA("\x0bANSI")
	if bytes.Contains(st.out.Bytes(), []byte("FROMANSI")) {
		t.Fatal("^KANSI must not display ANSI.ANS; only ^K!ANSI does")
	}
}

func TestRaduXYSpansSeparateWrites(t *testing.T) {
	st := &memStream{}
	tio := New(st, &cfgrec.GlobalCfg{}, &cfgrec.LineCfg{AnsiOn: true})
	tio.WriteRA("|`X")
	tio.WriteRA("20")
	tio.WriteRA(":General")
	out := st.out.Bytes()
	if bytes.Contains(out, []byte(":General")) {
		t.Fatalf("colon leaked before name: %q", out)
	}
	if !bytes.Contains(out, []byte("General")) {
		t.Fatalf("name missing: %q", out)
	}
}

func TestStripRalYesNo(t *testing.T) {
	s, def := stripRalYesNo("Send files with this message N", true)
	if s != "Send files with this message " || def {
		t.Fatalf("strip N: %q %v", s, def)
	}
	s, def = stripRalYesNo("Pause after each message Y", false)
	if s != "Pause after each message " || !def {
		t.Fatalf("strip Y: %q %v", s, def)
	}
	s, def = stripRalYesNo("Send files with this message", false)
	if s != "Send files with this message" || def {
		t.Fatalf("no trailer: %q %v", s, def)
	}
}

func TestAskYesNoUsesYesNoQuest(t *testing.T) {
	st := &keyStream{in: []byte("x")}
	line := &cfgrec.LineCfg{AnsiOn: true}
	tio := New(st, &cfgrec.GlobalCfg{}, line)
	called := false
	tio.YesNoQuest = func(defYes bool) (bool, bool) {
		called = true
		if defYes {
			t.Fatal("AttFiles1 default is no")
		}
		return true, true
	}
	if !tio.AskYesNo(lang.AttFiles1, false) {
		t.Fatal("expected yes from YESNO.Q-A")
	}
	if !called {
		t.Fatal("YesNoQuest not used")
	}
	if bytes.Contains(st.out.Bytes(), []byte("[y/N]")) || bytes.Contains(st.out.Bytes(), []byte("[Y/n]")) {
		t.Fatalf("fallback brackets used: %q", st.out.Bytes())
	}
}
