package nntp

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

func TestNNTPSLoginUsesUsersBBS(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.SysPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
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
