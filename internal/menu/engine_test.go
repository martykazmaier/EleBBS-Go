package menu

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/online"
	"elebbs/internal/term"
)

type seqStream struct {
	out    bytes.Buffer
	in     []byte
	remote bool
}

func (s *seqStream) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	p[0] = s.in[0]
	s.in = s.in[1:]
	return 1, nil
}
func (s *seqStream) Write(p []byte) (int, error)      { return s.out.Write(p) }
func (s *seqStream) Close() error                     { return nil }
func (s *seqStream) SetReadDeadline(time.Time) error  { return nil }
func (s *seqStream) SetWriteDeadline(time.Time) error { return nil }
func (s *seqStream) Local() bool                      { return !s.remote }

func putPS(b []byte, max int, s string) {
	if len(s) > max {
		s = s[:max]
	}
	b[0] = byte(len(s))
	copy(b[1:], s)
}

func encodeMenu(m cfgrec.MenuItem) []byte {
	b := make([]byte, cfgrec.MenuSize)
	b[0] = m.Typ
	putPS(b[131:], 135, m.Display)
	putPS(b[267:], 8, m.HotKey)
	putPS(b[276:], 135, m.MiscData)
	b[412] = m.Foreground
	b[413] = m.Background
	return b
}

func TestTopMenuAutoexecDisplaysFileNotSelect(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	auto := encodeMenu(cfgrec.MenuItem{Typ: 5, HotKey: "\x01", MiscData: "top"})
	key := encodeMenu(cfgrec.MenuItem{Typ: 0, HotKey: "F", Display: ""})
	raw := append(append(prompt, auto...), key...)
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.ans"), []byte("MENUANS"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.String()
	if !bytes.Contains(st.out.Bytes(), []byte("MENUANS")) {
		t.Fatalf("top.ans autoexec missing, out=%q", out)
	}
	if bytes.Contains(st.out.Bytes(), []byte("Select: ")) {
		t.Fatalf("fallback Select prompt leaked: %q", out)
	}
	if bytes.Contains(st.out.Bytes(), []byte("Welcome ")) {
		t.Fatalf("hardcoded welcome leaked: %q", out)
	}
	if !bytes.Contains(st.out.Bytes(), []byte("Your choice: ")) {
		t.Fatalf("menu prompt missing: %q", out)
	}
}

func TestType40AnsiDoesNotWaitEnter(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	auto := encodeMenu(cfgrec.MenuItem{Typ: 40, HotKey: "\x01", MiscData: "top"})
	key := encodeMenu(cfgrec.MenuItem{Typ: 0, HotKey: "F", Display: ""})
	raw := append(append(prompt, auto...), key...)
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	body := append([]byte{0x04, 0x01}, []byte("MENUANS")...)
	for i := 0; i < 40; i++ {
		body = append(body, []byte("X\r\n")...)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.ans"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("MENUANS")) {
		t.Fatalf("top.ans missing, out=%q", out)
	}
	if bytes.Contains(out, []byte("Press (Enter)")) {
		t.Fatalf("display ansi with hotkeys waited for enter: %q", out)
	}
	if bytes.Contains(out, []byte("-- More --")) {
		t.Fatalf("display ansi with hotkeys showed -- more --: %q", out)
	}
	if !bytes.Contains(out, []byte("Your choice: ")) {
		t.Fatalf("menu prompt missing: %q", out)
	}
}

func TestSelectAfterTallAnsiDoesNotPause(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	auto := encodeMenu(cfgrec.MenuItem{Typ: 40, HotKey: "\x01", MiscData: "top"})
	key := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "D"})
	raw := append(append(prompt, auto...), key...)
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	var body []byte
	for i := 0; i < 40; i++ {
		body = append(body, []byte("LINE\r\n")...)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.ans"), body, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("D\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, DispMorePrompt: true}
	line.User.ScreenLength = 24
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	tio.Length = 24
	tio.MorePrompt = true
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("-- More --")) || bytes.Contains(bytes.ToLower(out), []byte("paused")) {
		t.Fatalf("page pause after menu key: %q", out)
	}
	if !bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("hotkey D did not run: %q", out)
	}
}

func encodeLightBar(l cfgrec.LightBar) []byte {
	b := make([]byte, cfgrec.LightBarSize)
	b[0] = l.LightX
	b[1] = l.LightY
	putPS(b[2:], 135, l.LowItem)
	putPS(b[138:], 135, l.SelectItem)
	b[274] = l.Attrib
	return b
}

func TestMenuHotKeyIsHonored(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	key := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "F"})
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), append(prompt, key...), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("F\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	if !bytes.Contains(st.out.Bytes(), []byte("EleBBS")) {
		t.Fatalf("hotkey F not executed: %q", st.out.Bytes())
	}
}

func TestLightBarMenuDrawAndEnter(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	auto := encodeMenu(cfgrec.MenuItem{Typ: 40, HotKey: "\x01", MiscData: "top"})
	filesItem := encodeMenu(cfgrec.MenuItem{Typ: 0, HotKey: "F"})
	verItem := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "G"})
	raw := append(append(append(prompt, auto...), filesItem...), verItem...)
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.ans"), []byte("MENUANS"), 0644); err != nil {
		t.Fatal(err)
	}
	empty := encodeLightBar(cfgrec.LightBar{})
	onF := encodeLightBar(cfgrec.LightBar{LightX: 10, LightY: 5, LowItem: "files", SelectItem: "*FILES*", Attrib: 1})
	onG := encodeLightBar(cfgrec.LightBar{LightX: 10, LightY: 6, LowItem: "goodbye", SelectItem: "*GOODBYE*", Attrib: 1})
	mlb := append(append(append(empty, empty...), onF...), onG...)
	if err := os.WriteFile(filepath.Join(dir, "top.mlb"), mlb, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\x1b[B\r\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("MENUANS")) {
		t.Fatalf("menu art missing: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[5;10H")) || !bytes.Contains(out, []byte("files")) {
		t.Fatalf("inactive lightbar missing: %q", out)
	}
	if !bytes.Contains(out, []byte("*FILES*")) {
		t.Fatalf("active lightbar missing: %q", out)
	}
	if !bytes.Contains(out, []byte("\x1b[6;10H")) || !bytes.Contains(out, []byte("*GOODBYE*")) {
		t.Fatalf("arrow+enter lightbar missing: %q", out)
	}
	if !bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("enter did not run highlighted G: %q", out)
	}
}

func TestSyncTERMLogonEnterDoesNotFireLightBar(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	verItem := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "G"})
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), append(prompt, verItem...), 0644); err != nil {
		t.Fatal(err)
	}
	empty := encodeLightBar(cfgrec.LightBar{})
	onG := encodeLightBar(cfgrec.LightBar{LightX: 3, LightY: 8, LowItem: "lowG", SelectItem: "hiG", Attrib: 1})
	if err := os.WriteFile(filepath.Join(dir, "top.mlb"), append(empty, onG...), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\r\n\x00"), remote: true}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("SyncTERM leftover enter fired lightbar: %q", out)
	}
}

func TestRemoteLogonEnterThenLetterHotKey(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	filesItem := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "F"})
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), append(prompt, filesItem...), 0644); err != nil {
		t.Fatal(err)
	}
	empty := encodeLightBar(cfgrec.LightBar{})
	onF := encodeLightBar(cfgrec.LightBar{LightX: 3, LightY: 8, LowItem: "lowF", SelectItem: "hiF", Attrib: 1})
	if err := os.WriteFile(filepath.Join(dir, "top.mlb"), append(empty, onF...), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\r\nF"), remote: true}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("letter after leftover enter not honored: %q", out)
	}
}

func TestLightBarLetterHotKey(t *testing.T) {
	dir := t.TempDir()
	prompt := encodeMenu(cfgrec.MenuItem{Display: "Your choice: "})
	filesItem := encodeMenu(cfgrec.MenuItem{Typ: 8, HotKey: "F"})
	if err := os.WriteFile(filepath.Join(dir, "top.mnu"), append(prompt, filesItem...), 0644); err != nil {
		t.Fatal(err)
	}
	empty := encodeLightBar(cfgrec.LightBar{})
	onF := encodeLightBar(cfgrec.LightBar{LightX: 3, LightY: 8, LowItem: "lowF", SelectItem: "hiF", Attrib: 1})
	if err := os.WriteFile(filepath.Join(dir, "top.mlb"), append(empty, onF...), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("f\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MenuPath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true}
	line.Language.MenuPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.Enter()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("hiF")) {
		t.Fatalf("lightbar not drawn: %q", out)
	}
	if !bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("letter hotkey not honored: %q", out)
	}
}

func TestParseAutoexecHotKey(t *testing.T) {
	it := cfgrec.ParseMenuItem(encodeMenu(cfgrec.MenuItem{Typ: 5, HotKey: "\x01", MiscData: "top"}))
	if !isAutoexec(it) {
		t.Fatalf("hotkey %q not autoexec", it.HotKey)
	}
	if it.Typ != 5 || it.MiscData != "top" {
		t.Fatalf("parsed %+v", it)
	}
}

func TestMenuType8UsesVersionAndType16Location(t *testing.T) {
	st := &seqStream{in: []byte("\rTownsville\r")}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1}}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(8, "") {
		t.Fatal("type 8")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("\x1b[2J")) {
		t.Fatalf("type 8 did not clear screen: %q", out)
	}
	if !bytes.Contains(out, []byte("EleBBS")) {
		t.Fatalf("version missing: %q", out)
	}
	eng.ExecType(16, "")
	if line.User.Location != "Townsville" {
		t.Fatalf("location %q", line.User.Location)
	}
}

func TestMenuType16LocationRejectsBlank(t *testing.T) {
	st := &seqStream{in: []byte("\r\n  \rCalgary\r")}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1}}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(16, "") {
		t.Fatal("type 16")
	}
	if line.User.Location != "Calgary" {
		t.Fatalf("blank location accepted: %q", line.User.Location)
	}
}

func TestMenuType60BlankHandleUsesRealName(t *testing.T) {
	st := &seqStream{in: []byte("\r")}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{
		AnsiOn: true,
		User:   cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", Record: -1},
	}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(60, "") {
		t.Fatal("type 60")
	}
	if line.User.Handle != "Martin Kazmaier" {
		t.Fatalf("blank handle %q want real name", line.User.Handle)
	}
}

func TestMenuType16DoesNotPauseAfterLocation(t *testing.T) {
	st := &seqStream{in: []byte("Calgary\rX")}
	g := &cfgrec.GlobalCfg{}
	line := &cfgrec.LineCfg{
		AnsiOn:         true,
		DispMorePrompt: true,
		User:           cfgrec.User{Record: -1, ScreenLength: 6},
	}
	tio := term.New(st, g, line)
	tio.Length = 6
	tio.MorePrompt = true
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(16, "") {
		t.Fatal("type 16")
	}
	if line.User.Location != "Calgary" {
		t.Fatalf("location %q", line.User.Location)
	}
	ch, err := tio.GetKey(0)
	if err != nil || ch != 'X' {
		t.Fatalf("more prompt ate the next key, got %q err=%v rest=%q", ch, err, st.in)
	}
}

func TestSelectProtocolListsNameNotCtlString(t *testing.T) {
	dir := t.TempDir()
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:        "Zmodem",
		ActiveKey:   'Z',
		Batch:       true,
		Attribute:   1,
		DnCmdString: `c:\ele\prot\sexyz.exe /P*P /B115200 /Ufos`,
		UpCmdString: `c:\ele\prot\sexyz.exe *W -8 sz @DSZ.CTL`,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("Z")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1}}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(72, "") {
		t.Fatal("type 72")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("Zmodem")) {
		t.Fatalf("protocol name missing: %q", out)
	}
	if bytes.Contains(out, []byte("sexyz")) || bytes.Contains(out, []byte("DSZ.CTL")) || bytes.Contains(out, []byte("115200")) {
		t.Fatalf("dumped protocol ctl strings: %q", out)
	}
	if line.User.DefaultProto != 'Z' {
		t.Fatalf("proto %q", line.User.DefaultProto)
	}
}

func TestSelectProtocolShowsXferProtNotMenu(t *testing.T) {
	dir := t.TempDir()
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:      "Zmodem",
		ActiveKey: 'Z',
		Batch:     true,
		Attribute: 1,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "XFERPROT.ANS"), []byte("XFERSCREEN"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("Z")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1}}
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(72, "") {
		t.Fatal("type 72")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("XFERSCREEN")) {
		t.Fatalf("XFERPROT.ANS missing: %q", out)
	}
	if bytes.Contains(out, []byte("(Z)")) {
		t.Fatalf("generated protocol menu shown despite XFERPROT.ANS: %q", out)
	}
	if line.User.DefaultProto != 'Z' {
		t.Fatalf("proto %q", line.User.DefaultProto)
	}
}

func TestChangePasswordChecksCurrentAndMatch(t *testing.T) {
	st := &seqStream{in: []byte("WRONG\r\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MinPwdLen = 4
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, Password: "OLD", PasswordCRC: crc.RA("OLD", true)}}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(17, "") {
		t.Fatal("type 17")
	}
	if line.User.Password != "OLD" {
		t.Fatalf("wrong current password changed user: %q", line.User.Password)
	}

	st = &seqStream{in: []byte("OLD\rNEW1\rNEW2\r\r")}
	line = &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, Password: "OLD", PasswordCRC: crc.RA("OLD", true)}}
	tio = term.New(st, g, line)
	eng = &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(17, "") {
		t.Fatal("type 17 mismatch")
	}
	if line.User.Password != "OLD" {
		t.Fatalf("mismatched new passwords saved: %q", line.User.Password)
	}

	st = &seqStream{in: []byte("OLD\rNEWPASS\rNEWPASS\r\r")}
	line = &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, Password: "OLD", PasswordCRC: crc.RA("OLD", true)}}
	tio = term.New(st, g, line)
	eng = &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(17, "") {
		t.Fatal("type 17 match")
	}
	if line.User.Password != "NEWPASS" || line.User.PasswordCRC != crc.RA("NEWPASS", true) {
		t.Fatalf("password not updated: %q crc=%d", line.User.Password, line.User.PasswordCRC)
	}
}

func TestMenuFlagsRequiredToRun(t *testing.T) {
	eng := &Engine{Line: &cfgrec.LineCfg{User: cfgrec.User{Security: 10}}}
	it := cfgrec.MenuItem{Typ: 8, Security: 0}
	it.Flags[0] = 1 // require flag A1
	if eng.access(it) {
		t.Fatal("item with required flag ran without user flag")
	}
	eng.Line.User.Flags[0] = 1
	if !eng.access(it) {
		t.Fatal("item with matching user flag denied")
	}
	it.NotFlags[0] = 1
	if eng.access(it) {
		t.Fatal("NotFlags ignored — command ran with forbidden flag")
	}
}

func TestStripMenuSwitchesKeepsDoorFlags(t *testing.T) {
	_, _, rest := stripMenuSwitches(`c:\doors\lord.exe -H*W -XT *UDoor`)
	if !strings.Contains(rest, "*W") {
		t.Fatalf("*W stripped: %q", rest)
	}
	if !strings.Contains(rest, "-XT") {
		t.Fatalf("-XT stripped: %q", rest)
	}
	if strings.Contains(strings.ToUpper(rest), "*U") {
		t.Fatalf("*U should be removed: %q", rest)
	}
}

func TestWhosOnlineShowsHandleNotRealName(t *testing.T) {
	dir := t.TempDir()
	st := &seqStream{in: []byte("\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SystemName = "Test Board"
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{
		AnsiOn:   true,
		RaNodeNr: 2,
		Baud:     65529,
		User:     cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", Location: "Calgary", Record: -1},
	}
	if err := online.Write(g, line, "", online.StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	if !eng.ExecType(52, "") {
		t.Fatal("type 52")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("Shurato")) {
		t.Fatalf("handle missing: %q", out)
	}
	if bytes.Contains(out, []byte("Martin Kazmaier")) {
		t.Fatalf("real name shown: %q", out)
	}
	if !bytes.Contains(out, []byte("65529")) {
		t.Fatalf("baud missing: %q", out)
	}
	if !bytes.Contains(out, []byte("Calgary")) {
		t.Fatalf("location missing: %q", out)
	}
	hdr := "Name                         Line   BaudRate   Status     Location"
	if !bytes.Contains(out, []byte(hdr)) {
		t.Fatalf("header missing Location columns: %q", out)
	}
}

func TestBBSSendMessageWritesNodeRA(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.SystemName = "Test Board"
	sender := &cfgrec.LineCfg{
		AnsiOn:   true,
		RaNodeNr: 1,
		Baud:     65529,
		User:     cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", Location: "Calgary", Record: -1},
	}
	dest := &cfgrec.LineCfg{
		AnsiOn:   true,
		RaNodeNr: 2,
		Baud:     2400,
		User:     cfgrec.User{Name: "Other User", Handle: "Other", Location: "Town", Record: -1},
	}
	if err := online.Write(g, sender, "", online.StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	if err := online.Write(g, dest, "", online.StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("2\rhello there\r\rS\r")}
	tio := term.New(st, g, sender)
	eng := &Engine{T: tio, G: g, Line: sender}
	if !eng.ExecType(54, "") {
		t.Fatal("type 54")
	}
	b, err := os.ReadFile(online.NodePath(g, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("hello there")) {
		t.Fatalf("body missing: %q", b)
	}
	if !bytes.Contains(b, []byte("\x0b]497")) {
		t.Fatalf("pascal header missing: %q", b)
	}
}

func TestWhosOnlineNodeEnterAsksForMessage(t *testing.T) {
	dir := t.TempDir()
	src := "" +
		":WHOSONLINE\r\n" +
		"ask 5 34\r\n" +
		"If 34 = \"\"\r\n" +
		`Display "EMPTYNODE"` + "\r\n" +
		"Quit\r\n" +
		"EndIf\r\n" +
		"Assign 5 1\r\n" +
		"Assign 11 X\r\n" +
		"While 11 <> \"\" do\r\n" +
		"GetRecordInfo #5 10 DOWN\r\n" +
		"If 11 = \"\"\r\n" +
		"BREAK\r\n" +
		"EndIf\r\n" +
		"If 12 = #34\r\n" +
		"BREAK\r\n" +
		"EndIf\r\n" +
		"Assign 5 #17\r\n" +
		"Inc 5\r\n" +
		"EndWhile\r\n" +
		"If 12 <> #34\r\n" +
		`Display "NOONE"` + "\r\n" +
		"Quit\r\n" +
		"EndIf\r\n" +
		"ask 40 37\r\n" +
		"Length 36 37\r\n" +
		"If 36 = 0\r\n" +
		`Display "MSGABORT"` + "\r\n" +
		"Quit\r\n" +
		"EndIf\r\n" +
		`Display "GOTMSG "` + "\r\n" +
		"Display 37\r\n" +
		"Quit\r\n"
	if err := os.WriteFile(filepath.Join(dir, "whonline.q-a"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	n1 := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 1, Baud: 65529, User: cfgrec.User{Name: "Sysop", Handle: "Sysop", Location: "Here", Record: -1}}
	n2 := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 2, Baud: 65529, User: cfgrec.User{Name: "Martin", Handle: "Shurato", Location: "Calgary", Record: -1}}
	if err := online.Write(g, n1, "", online.StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	if err := online.Write(g, n2, "", online.StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	line := *n2
	line.Language.QuesPath = dir
	st := &seqStream{in: []byte("2\r\nhello\r")}
	tio := term.New(st, g, &line)
	eng := &Engine{T: tio, G: g, Line: &line}
	if !eng.ExecType(52, "") {
		t.Fatal("type 52")
	}
	out := st.out.Bytes()
	if bytes.Contains(out, []byte("NOONE")) {
		t.Fatalf("node 2 not found: %q", out)
	}
	if bytes.Contains(out, []byte("MSGABORT")) || bytes.Contains(out, []byte("EMPTYNODE")) {
		t.Fatalf("compose aborted after node number: %q", out)
	}
	if !bytes.Contains(out, []byte("GOTMSG")) || !bytes.Contains(out, []byte("hello")) {
		t.Fatalf("message ASK not run after node number, out=%q", out)
	}
}

func TestCheckNodeMsgShowsRAFileFromSemPath(t *testing.T) {
	sys := t.TempDir()
	sem := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = sys
	g.RaConfig.SemPath = sem
	if err := online.AppendNodeMsg(g, 2, "Shurato", 1, []string{"hello remote"}); err != nil {
		t.Fatal(err)
	}
	if p := online.FindNodeFile(g, 2); p == "" || filepath.Dir(p) != sem {
		t.Fatalf("NODE.RA not in SemPath: %q", online.FindNodeFile(g, 2))
	}
	st := &seqStream{in: []byte("\r")}
	line := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 2}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.checkNodeMsg()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("hello remote")) {
		t.Fatalf("inbound node msg not shown: %q", out)
	}
	if online.FindNodeFile(g, 2) != "" {
		t.Fatal("NODE.RA should be cleared after display")
	}
}

func TestCheckNodeMsgShowsRAFile(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	if err := online.AppendNodeMsg(g, 2, "Shurato", 1, []string{"hello remote"}); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\r")}
	line := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 2}
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line}
	eng.checkNodeMsg()
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("hello remote")) {
		t.Fatalf("inbound node msg not shown: %q", out)
	}
	if online.FindNodeFile(g, 2) != "" {
		t.Fatal("NODE.RA should be cleared after display")
	}
}
