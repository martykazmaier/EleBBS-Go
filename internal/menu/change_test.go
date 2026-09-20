package menu

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/mail"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

type keyMem struct {
	in []byte
}

func (s *keyMem) Read(p []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.in)
	s.in = s.in[n:]
	return n, nil
}
func (s *keyMem) Write(p []byte) (int, error)      { return len(p), nil }
func (s *keyMem) Close() error                     { return nil }
func (s *keyMem) SetReadDeadline(time.Time) error  { return nil }
func (s *keyMem) SetWriteDeadline(time.Time) error { return nil }
func (s *keyMem) Local() bool                      { return true }

func TestChangerPageRightRealMsgs(t *testing.T) {
	ele := `C:\Users\Owner\OneDrive\ele`
	body, err := os.ReadFile(filepath.Join(ele, "QUESTION", "MA-CHNG.Q-A"))
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ma-chng.q-a"), body, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = ele
	all := mail.LoadAll(g)
	if len(all) < 32 {
		t.Skipf("only %d msg areas", len(all))
	}
	u := cfgrec.User{Security: 100, Attribute: cfgrec.UserANSI, Name: "Martin Kazmaier", Handle: "Shurato"}
	total := 0
	for _, a := range all {
		if mail.Accessible(a, u, false, 0) {
			total++
		}
	}
	t.Logf("msg records=%d accessible=%d", len(all), total)
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		msgGetInfo(all, u, false, 0, false, recordNum, start, down, put)
	}
	var keys []byte
	for i := 0; i < 4; i++ {
		keys = append(keys, 27, '[', 'C')
	}
	keys = append(keys, 27)
	g2 := &cfgrec.GlobalCfg{}
	g2.RaConfig.SysPath = dir
	g2.RaConfig.TextPath = dir
	g2.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: u}
	line.Language.QuesPath = dir
	st := &keyMem{in: keys}
	tio := term.New(st, g2, line)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = quest.Exec(tio, g2, line, "MA-CHNG /N", quest.ScriptOpts{
			NoLog:   true,
			Answers: map[int]string{1: strconv.Itoa(total), 2: "1", 3: "1"},
			GetInfo: hook,
		})
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("MA-CHNG hung paging right on real MESSAGES.RA")
	}
}

func TestChangerPageRightRealFiles(t *testing.T) {
	ele := `C:\Users\Owner\OneDrive\ele`
	body, err := os.ReadFile(filepath.Join(ele, "QUESTION", "FA-CHNG.Q-A"))
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fa-chng.q-a"), body, 0644); err != nil {
		t.Fatal(err)
	}
	if ans, err := os.ReadFile(filepath.Join(ele, "txtfiles", "CHANGER.ANS")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "changer.ans"), ans, 0644)
	}

	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = ele
	all := files.LoadAll(g)
	if len(all) < 32 {
		t.Skipf("only %d file areas", len(all))
	}
	u := cfgrec.User{Security: 100, Attribute: cfgrec.UserANSI, Name: "Sysop"}
	total := 0
	for _, a := range all {
		if fileAccessible(a, u, false, 0) {
			total++
		}
	}
	t.Logf("file records=%d accessible=%d", len(all), total)
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		fileGetInfo(all, u, false, 0, recordNum, start, down, put)
	}

	var keys []byte
	for i := 0; i < 4; i++ {
		keys = append(keys, 27, '[', 'C')
	}
	keys = append(keys, 27)

	g2 := &cfgrec.GlobalCfg{}
	g2.RaConfig.SysPath = dir
	g2.RaConfig.TextPath = dir
	g2.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: u}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	st := &keyMem{in: keys}
	tio := term.New(st, g2, line)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = quest.Exec(tio, g2, line, "FA-CHNG /N", quest.ScriptOpts{
			NoLog: true,
			Answers: map[int]string{
				1: strconv.Itoa(total),
				2: "1",
				3: "1",
			},
			GetInfo: hook,
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("FA-CHNG hung paging right on real FILES.RA")
	}
}

func TestChangerPageRightDoesNotHang(t *testing.T) {
	src := `C:\Users\Owner\OneDrive\ele\QUESTION\MA-CHNG.Q-A`
	body, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ma-chng.q-a"), body, 0644); err != nil {
		t.Fatal(err)
	}

	all := make([]cfgrec.MessageArea, 40)
	for i := range all {
		all[i] = cfgrec.MessageArea{AreaNum: uint16(i + 1), Name: "Area" + strconv.Itoa(i+1)}
	}
	u := cfgrec.User{Security: 100, Attribute: cfgrec.UserANSI, Name: "Sysop"}
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		msgGetInfo(all, u, false, 0, false, recordNum, start, down, put)
	}

	var keys []byte
	for i := 0; i < 4; i++ {
		keys = append(keys, 27, '[', 'C') // RIGHT
	}
	keys = append(keys, 27) // ESC → END2

	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: u}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	st := &keyMem{in: keys}
	tio := term.New(st, g, line)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = quest.Exec(tio, g, line, "MA-CHNG /N", quest.ScriptOpts{
			NoLog: true,
			Answers: map[int]string{
				1: "40",
				2: "1",
				3: "1",
			},
			GetInfo: hook,
		})
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("MA-CHNG hung after paging right")
	}
}

func TestChangerQExitsLikeEscape(t *testing.T) {
	src := `C:\Users\Owner\OneDrive\ele\QUESTION\MA-CHNG.Q-A`
	body, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ma-chng.q-a"), body, 0644); err != nil {
		t.Fatal(err)
	}

	all := make([]cfgrec.MessageArea, 8)
	for i := range all {
		all[i] = cfgrec.MessageArea{AreaNum: uint16(i + 1), Name: "Area" + strconv.Itoa(i+1)}
	}
	u := cfgrec.User{Security: 100, Attribute: cfgrec.UserANSI, Name: "Sysop"}
	hook := func(recordNum, start int, down bool, put func(int, string)) {
		msgGetInfo(all, u, false, 0, false, recordNum, start, down, put)
	}

	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{AnsiOn: true, User: u}
	line.Language.QuesPath = dir
	line.Language.TextPath = dir
	st := &keyMem{in: []byte{'q'}}
	tio := term.New(st, g, line)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = quest.Exec(tio, g, line, "MA-CHNG /N", quest.ScriptOpts{
			NoLog: true,
			Answers: map[int]string{
				1: "8",
				2: "1",
				3: "1",
			},
			GetInfo: hook,
		})
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("MA-CHNG did not exit on Q (should remap to Escape)")
	}
}

func TestGetInfoDoesNotWrapPastEOF(t *testing.T) {
	all := []cfgrec.MessageArea{
		{AreaNum: 1, Name: "General"},
		{AreaNum: 2, Name: "Local"},
	}
	u := cfgrec.User{Security: 10}
	got := map[int]string{}
	put := func(n int, s string) { got[n] = s }

	msgGetInfo(all, u, false, 0, true, 3, 30, true, put)
	if got[30] != "2" {
		t.Fatalf("DOWN past last #30=%q want 2 (file length), wrapping to 0 restarts the list", got[30])
	}
	if got[31] != "0" || got[32] != "" {
		t.Fatalf("EOF answers #31=%q #32=%q", got[31], got[32])
	}

	msgGetInfo(all, u, false, 0, true, 3, 30, true, put)
	if got[32] != "" {
		t.Fatalf("second DOWN past EOF should stay empty, got %q", got[32])
	}

	msgGetInfo(all, u, false, 0, true, 0, 30, false, put)
	if got[30] != "0" {
		t.Fatalf("UP past start #30=%q want 0", got[30])
	}

	files := []cfgrec.FilesArea{
		{AreaNum: 1, Name: "Uploads", FilePath: "c:\\files"},
	}
	fileGetInfo(files, u, false, 0, 2, 30, true, put)
	if got[30] != "1" || got[32] != "" {
		t.Fatalf("file EOF #30=%q #32=%q", got[30], got[32])
	}

	groups := []cfgrec.Group{
		{AreaNum: 1, Name: "Main"},
	}
	groupGetInfo(groups, u, 2, 30, true, put)
	if got[30] != "1" || got[32] != "" {
		t.Fatalf("group EOF #30=%q #32=%q", got[30], got[32])
	}
}
