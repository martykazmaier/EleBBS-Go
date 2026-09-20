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
