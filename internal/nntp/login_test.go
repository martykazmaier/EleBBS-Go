package nntp

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/mail"
	"elebbs/internal/userbase"
)

func TestNNTPSLoginUsesUsersBBS(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.StrictPwdChecking = true
	u := cfgrec.User{Name: "SysOp", Handle: "Sys", PasswordCRC: crc.RA("hunter", true), Record: 0}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	if err := userbase.RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	srv, cli := net.Pipe()
	defer cli.Close()
	ss := &session{cfg: Config{G: g}, c: srv}
	go func() {
		ss.cmdAuth("USER sysop")
		ss.cmdAuth("PASS Hunter")
		_ = srv.Close()
	}()
	r := bufio.NewReader(cli)
	a, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a, "381") {
		t.Fatalf("USER reply %q", a)
	}
	if !strings.HasPrefix(b, "281") {
		t.Fatalf("PASS reply %q", b)
	}
	if !ss.authed || ss.user.Name != "SysOp" {
		t.Fatalf("session %+v", ss.user)
	}
}

func TestNNTPSOverviewCommands(t *testing.T) {
	for _, cmd := range []string{"OVER", "XOVER"} {
		t.Run(cmd, func(t *testing.T) {
			srv, cli := net.Pipe()
			defer cli.Close()
			ss := &session{c: srv, cur: mail.NewsGroup{Name: "test"}}
			go func() {
				ss.dispatch(cmd, "")
				_ = srv.Close()
			}()

			r := bufio.NewReader(cli)
			status, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(status, "224 ") {
				t.Fatalf("%s reply %q", cmd, status)
			}
			terminator, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if terminator != ".\r\n" {
				t.Fatalf("overview terminator %q", terminator)
			}
		})
	}
}

func TestNNTPBodyByMessageIDWithoutSelectedGroup(t *testing.T) {
	base := filepath.Join(t.TempDir(), "news")
	const msgID = "message-1@shsbbs.net"
	if _, err := mail.AppendMsg(base, mail.Article{
		From:    "Sysop",
		Subject: "Pho News body test",
		MsgID:   msgID,
		Body:    "Message body",
	}); err != nil {
		t.Fatal(err)
	}

	srv, cli := net.Pipe()
	defer cli.Close()
	ss := &session{
		groups: []mail.NewsGroup{{
			Name: "test.group",
			Area: cfgrec.MessageArea{JAMBase: base},
		}},
		c: srv,
	}
	go func() {
		ss.cmdArticle("BODY", "<"+msgID+">")
		_ = srv.Close()
	}()

	r := bufio.NewReader(cli)
	status, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(status, "222 ") {
		t.Fatalf("BODY reply %q", status)
	}
	body, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if body != "Message body\r\n" {
		t.Fatalf("BODY line %q", body)
	}
}

func TestNNTPPostWritesToJAMAndRejectsReadOnlyGroup(t *testing.T) {
	tests := []struct {
		name         string
		posting      bool
		areaType     byte
		expectedAttr uint32
	}{
		{name: "local area", posting: true, areaType: cfgrec.MsgLocal, expectedAttr: 0x00800001},
		{name: "echomail area", posting: true, areaType: cfgrec.MsgEchoMail, expectedAttr: 0x01000001},
		{name: "posting disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "news")
			srv, cli := net.Pipe()
			defer cli.Close()
			ss := &session{
				groups: []mail.NewsGroup{{
					Name:    "test.group",
					Area:    cfgrec.MessageArea{JAMBase: base, Typ: tt.areaType},
					Posting: tt.posting,
				}},
				c: srv,
				r: bufio.NewReader(srv),
			}
			go func() {
				ss.cmdPost()
				_ = srv.Close()
			}()

			r := bufio.NewReader(cli)
			ready, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(ready, "340 ") {
				t.Fatalf("POST ready reply %q", ready)
			}
			article := "From: sysop@example.test\r\n" +
				"Newsgroups: test.group\r\n" +
				"Subject: JAM posting test\r\n" +
				"Date: Thu, 22 Oct 2026 12:00:00 +0000\r\n" +
				"Message-ID: <post-1@example.test>\r\n" +
				"\r\n" +
				"Hello from NNTP\r\n" +
				"..dot-stuffed line\r\n" +
				".\r\n"
			if _, err := io.WriteString(cli, article); err != nil {
				t.Fatal(err)
			}
			reply, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !tt.posting {
				if !strings.HasPrefix(reply, "441 ") {
					t.Fatalf("read-only POST reply %q", reply)
				}
				return
			}
			if !strings.HasPrefix(reply, "240 ") {
				t.Fatalf("POST reply %q", reply)
			}
			arts, err := mail.ReadJAM(base)
			if err != nil {
				t.Fatal(err)
			}
			if len(arts) != 1 {
				t.Fatalf("posted articles %d, want 1", len(arts))
			}
			if arts[0].From != "sysop@example.test" || arts[0].Subject != "JAM posting test" || arts[0].MsgID != "post-1@example.test" {
				t.Fatalf("posted headers %+v", arts[0])
			}
			if arts[0].Attr != tt.expectedAttr {
				t.Errorf("posted JAM attributes %#08x, want %#08x", arts[0].Attr, tt.expectedAttr)
			}
			if !strings.Contains(arts[0].Body, "Hello from NNTP") || !strings.Contains(arts[0].Body, ".dot-stuffed line") {
				t.Fatalf("posted body %q", arts[0].Body)
			}
		})
	}
}
