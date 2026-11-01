package ftp

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/userbase"
)

func TestFTPSLoginUsesUsersBBS(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	u := cfgrec.User{Name: "Joe User", Handle: "Joe", PasswordCRC: crc.RA("hunter", true), Record: 0}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	if err := userbase.RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	srv, cli := net.Pipe()
	defer cli.Close()
	ss := &session{srv: &Server{cfg: Config{G: g}}, c: srv}
	go func() {
		ss.cmdUser("joe user")
		ss.cmdPass("HUNTER")
		_ = srv.Close()
	}()
	r := bufio.NewReader(cli)
	u331, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	u230, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u331, "331") {
		t.Fatalf("USER reply %q", u331)
	}
	if !strings.HasPrefix(u230, "230") {
		t.Fatalf("PASS reply %q", u230)
	}
	if !ss.authed || ss.user.Name != "Joe User" {
		t.Fatalf("session %+v", ss.user)
	}
}

func writeUser(t *testing.T, dir string, u cfgrec.User) *cfgrec.GlobalCfg {
	t.Helper()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	if err := userbase.RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestFTPSAnonymousAcceptsAnyPassword(t *testing.T) {
	u := cfgrec.User{Name: "Anonymous", Handle: "Anon", PasswordCRC: crc.RA("secret", true), Security: 10, Record: 0}
	u.Flags[0] = 1
	g := writeUser(t, t.TempDir(), u)
	srv, cli := net.Pipe()
	defer cli.Close()
	ss := &session{srv: &Server{cfg: Config{G: g, Anonymous: true}}, c: srv}
	go func() {
		ss.cmdUser("ftp")
		ss.cmdPass("not-the-user-password")
		_ = srv.Close()
	}()
	r := bufio.NewReader(cli)
	if u331, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(u331, "331") {
		t.Fatalf("USER %q %v", u331, err)
	}
	if u230, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(u230, "230") {
		t.Fatalf("PASS %q %v", u230, err)
	}
	if !ss.authed || !ss.anon || ss.user.Name != "Anonymous" || ss.user.Security != 10 || ss.user.Flags[0] != 1 {
		t.Fatalf("anon session %+v anon=%v", ss.user, ss.anon)
	}
}

func TestFTPSRegularUserStillNeedsPassword(t *testing.T) {
	u := cfgrec.User{Name: "Joe User", Handle: "Joe", PasswordCRC: crc.RA("hunter", true), Security: 20, Record: 0}
	g := writeUser(t, t.TempDir(), u)
	srv, cli := net.Pipe()
	defer cli.Close()
	ss := &session{srv: &Server{cfg: Config{G: g}}, c: srv}
	go func() {
		ss.cmdUser("joe user")
		ss.cmdPass("wrong")
		_ = srv.Close()
	}()
	r := bufio.NewReader(cli)
	_, _ = r.ReadString('\n')
	u530, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(u530, "530") {
		t.Fatalf("PASS %q %v", u530, err)
	}
	if ss.authed {
		t.Fatal("wrong password logged in")
	}
}
