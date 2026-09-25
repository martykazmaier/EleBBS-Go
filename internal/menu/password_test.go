package menu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/mail"
	"elebbs/internal/term"
	"elebbs/internal/userbase"
)

func TestFilePostWatchDog(t *testing.T) {
	dir := t.TempDir()
	a := jamArea(t, dir, "watch")
	a.AreaNum = 7
	if err := os.WriteFile(filepath.Join(dir, "watchdog.msg"), []byte("Someone tried your password.\r\nRegards\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	line := &cfgrec.LineCfg{RaNodeNr: 1, User: cfgrec.User{Name: "Joe User", Record: -1}}
	eng := &Engine{T: term.New(&seqStream{}, g, line), G: g, Line: line, Msgs: []cfgrec.MessageArea{a}}

	if !eng.FilePost(7, "The Sysop", "Joe User", "Incorrect password logon attempt", "watchdog.msg", "!!! WARNING !!!") {
		t.Fatal("watchdog.msg not found")
	}
	arts, err := mail.ReadJAM(a.JAMBase)
	if err != nil || len(arts) != 1 {
		t.Fatalf("arts=%d err=%v", len(arts), err)
	}
	m := arts[0]
	if m.From != "The Sysop" || m.To != "Joe User" || !m.Private || m.Subject != "Incorrect password logon attempt" {
		t.Fatalf("header %+v", m)
	}
	body := strings.ReplaceAll(m.Body, "\r\n", "\n")
	if !strings.HasPrefix(body, "!!! WARNING !!!\nSomeone tried your password.\nRegards\n") {
		t.Fatalf("body %q", m.Body)
	}
	if eng.FilePost(7, "The Sysop", "Joe User", "x", "missing.msg", "") {
		t.Fatal("missing file reported as posted")
	}
}

func TestPostingSetsMailEnteredFlags(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	for _, tc := range []struct {
		typ       byte
		net, echo bool
	}{
		{cfgrec.MsgLocal, false, false},
		{cfgrec.MsgNetMail, true, false},
		{cfgrec.MsgEchoMail, false, true},
	} {
		a := jamArea(t, dir, "area"+string(rune('0'+tc.typ)))
		a.Typ = tc.typ
		line := &cfgrec.LineCfg{RaNodeNr: 1, User: cfgrec.User{Name: "Joe User", Record: -1}}
		eng := &Engine{T: term.New(&seqStream{}, g, line), G: g, Line: line}
		if _, err := eng.saveArticle(a, "Joe User", "Sysop", "Hi", []string{"hello"}, false, false); err != nil {
			t.Fatal(err)
		}
		if line.NetMailEntered != tc.net || line.EchoMailEntered != tc.echo {
			t.Fatalf("type %d: net=%v echo=%v", tc.typ, line.NetMailEntered, line.EchoMailEntered)
		}
	}
}

func TestChangePasswordStrictKeepsCaseAndRejectsName(t *testing.T) {
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MinPwdLen = 4
	g.RaConfig.StrictPwdChecking = true
	g.RaConfig.SysPath = t.TempDir()
	newLine := func() *cfgrec.LineCfg {
		line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Name: "Joe User", Record: -1}}
		userbase.SetPassword(&line.User, "OldPass", true)
		return line
	}

	line := newLine()
	st := &seqStream{in: []byte("oldpass\r\r")}
	eng := &Engine{T: term.New(st, g, line), G: g, Line: line}
	eng.ExecType(17, "")
	if line.User.Password != "OldPass" {
		t.Fatalf("wrong-case current password accepted: %q", line.User.Password)
	}

	line = newLine()
	st = &seqStream{in: []byte("OldPass\ruser\r\r")}
	eng = &Engine{T: term.New(st, g, line), G: g, Line: line}
	eng.ExecType(17, "")
	if line.User.Password != "OldPass" {
		t.Fatalf("last name accepted as password: %q", line.User.Password)
	}

	line = newLine()
	st = &seqStream{in: []byte("OldPass\rMixedCase\rMixedCase\r\r")}
	eng = &Engine{T: term.New(st, g, line), G: g, Line: line}
	eng.ExecType(17, "")
	if line.User.Password != "MixedCase" || !userbase.CheckPassword(line.User, "MixedCase", true) ||
		userbase.CheckPassword(line.User, "mixedcase", true) {
		t.Fatalf("strict password not stored case-sensitively: %+v", line.User)
	}
}
