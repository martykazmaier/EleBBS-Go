package logon

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/term"
	"elebbs/internal/userbase"
)

func passwordTestSetup(t *testing.T, keys string, strict bool) (*term.IO, *keyStream, *cfgrec.GlobalCfg, *cfgrec.LineCfg, *lang.File, cfgrec.User) {
	t.Helper()
	dir := t.TempDir()
	st := &keyStream{in: []byte(keys)}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.Sysop = "The Sysop"
	g.RaConfig.StrictPwdChecking = strict
	line := &cfgrec.LineCfg{AnsiOn: true, RaNodeNr: 1}
	tio := term.New(st, g, line)
	ral := lang.Load(g, line.Language)
	tio.Ral = ral
	u := cfgrec.User{Name: "Joe User"}
	userbase.SetPassword(&u, "Secret", strict)
	return tio, st, g, line, ral, u
}

func TestPasswordCaseInsensitiveUnlessStrict(t *testing.T) {
	tio, _, g, line, ral, u := passwordTestSetup(t, "sECRET\r", false)
	if !getPassword(tio, g, line, ral, &u, "", 3) {
		t.Fatal("non-strict logon must ignore case")
	}

	tio, st, g, line, ral, u := passwordTestSetup(t, "secret\rSecret\r", true)
	if !getPassword(tio, g, line, ral, &u, "", 3) {
		t.Fatalf("strict exact password rejected: %q", st.out.Bytes())
	}
	if !bytes.Contains(st.out.Bytes(), []byte(ral.Get(lang.IncPsw))) {
		t.Fatal("strict wrong-case attempt should be incorrect")
	}
}

func TestPasswordTriesExceededNotifiesAndOffersSysopMessage(t *testing.T) {
	tio, st, g, line, ral, u := passwordTestSetup(t, "a\rb\rc\rY", false)
	g.RaConfig.PwdBoard = 7
	g.RaConfig.BadPwdArea = 9
	type post struct {
		area                            int
		from, to, subj, file, addText string
	}
	var posts []post
	tio.FilePost = func(area int, from, to, subj, file, addText string) bool {
		posts = append(posts, post{area, from, to, subj, file, addText})
		return true
	}
	var wrote []string
	tio.WriteMessage = func(area int, toWho, from string) bool {
		wrote = append(wrote, toWho+"<"+from)
		if area != 9 {
			t.Fatalf("comment area %d", area)
		}
		return true
	}
	if getPassword(tio, g, line, ral, &u, "", 3) {
		t.Fatal("wrong passwords accepted")
	}
	out := string(st.out.Bytes())
	if n := strings.Count(out, ral.Get(lang.IncPsw)); n != 2 {
		t.Fatalf("IncPsw shown %d times, want 2 (not after the last try): %q", n, out)
	}
	if !strings.Contains(out, ral.Get(lang.NoAccess)) {
		t.Fatalf("no access message missing: %q", out)
	}
	if len(posts) != 1 {
		t.Fatalf("watchdog posts %v", posts)
	}
	p := posts[0]
	if p.area != 7 || p.from != "The Sysop" || p.to != "Joe User" || p.file != "watchdog.msg" ||
		p.subj != "Incorrect password logon attempt" || !strings.Contains(p.addText, "INVALID PASSWORD") {
		t.Fatalf("watchdog post %+v", p)
	}
	if len(wrote) != 1 || wrote[0] != "The Sysop<Joe User" {
		t.Fatalf("sysop comment %v", wrote)
	}
	log, _ := os.ReadFile(g.RaConfig.LogFileName)
	for _, want := range []string{
		`Incorrect password : "A"`,
		"Exceeded maximum password attempts, user disconnected",
		"WatchDog: User notified of password attempt",
	} {
		if !strings.Contains(string(log), want) {
			t.Fatalf("log missing %q:\n%s", want, log)
		}
	}
}

func TestPerformGivesLogonTimeBeforeLevelLimits(t *testing.T) {
	st := &memStream{}
	line := &cfgrec.LineCfg{AnsiOn: true}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.LogonTime = 15
	tio := term.New(st, g, line)
	Perform(tio, g, line)
	if line.TimeLimit != 15 {
		t.Fatalf("time limit during logon = %d, want LogonTime 15", line.TimeLimit)
	}
}

func TestPasswordTriesExceededSkipsDisabledAreas(t *testing.T) {
	tio, _, g, line, ral, u := passwordTestSetup(t, "a\rb\rc\r", false)
	tio.FilePost = func(int, string, string, string, string, string) bool {
		t.Fatal("FilePost with PwdBoard 0")
		return false
	}
	tio.WriteMessage = func(int, string, string) bool {
		t.Fatal("WriteMessage with BadPwdArea 0")
		return false
	}
	if getPassword(tio, g, line, ral, &u, "", 3) {
		t.Fatal("wrong passwords accepted")
	}
}
