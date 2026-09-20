package userbase

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
)

func TestCheckPasswordCaseInsensitive(t *testing.T) {
	u := cfgrec.User{PasswordCRC: crc.RA("Secret", true)}
	for _, pw := range []string{"SECRET", "secret", "Secret", "SeCrEt"} {
		if !CheckPassword(u, pw) {
			t.Fatalf("CRC password rejected %q", pw)
		}
	}
	u.Password = "Secret"
	u.PasswordCRC = 0
	if !CheckPassword(u, "secret") || !CheckPassword(u, "SECRET") {
		t.Fatal("plaintext password not case-insensitive")
	}
	if CheckPassword(u, "wrong") {
		t.Fatal("wrong password accepted")
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	u := cfgrec.User{Name: "SysOp", Handle: "Sys", PasswordCRC: crc.RA("hunter", true)}
	raw := cfgrec.EncodeUser(u)
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sysop", "SYSOP", "SysOp", " sysop "} {
		got, ok := Search(g, name)
		if !ok {
			t.Fatalf("user %q not found", name)
		}
		if got.Name != "SysOp" {
			t.Fatalf("name=%q", got.Name)
		}
		if !CheckPassword(got, "HUNTER") || !CheckPassword(got, "hunter") {
			t.Fatalf("password failed for user %q", name)
		}
	}
	if _, ok := Search(g, "nobody"); ok {
		t.Fatal("unknown user found")
	}
}

func TestSearchFindsUserPastFirstRecord(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	raw := append(
		cfgrec.EncodeUser(cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", PasswordCRC: crc.RA("x", true)}),
		cfgrec.EncodeUser(cfgrec.User{Name: "Anachronist", Handle: "Anachronist", PasswordCRC: crc.RA("y", true)})...,
	)
	if len(raw) != 2*cfgrec.UsersSize {
		t.Fatalf("file %d want %d", len(raw), 2*cfgrec.UsersSize)
	}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Search(g, "Anachronist"); !ok {
		t.Fatal("second user not found — record stride is wrong")
	}
	if _, ok := Search(g, "Shurato"); !ok {
		t.Fatal("sysop handle not found")
	}
}

func TestSearchDriveRootMsgBase(t *testing.T) {
	root := t.TempDir()
	msg := filepath.Join(root, "msgbase")
	if err := os.Mkdir(msg, 0755); err != nil {
		t.Fatal(err)
	}
	u := cfgrec.User{Name: "Martin Kazmaier", Handle: "Shurato", PasswordCRC: crc.RA("x", true)}
	if err := os.WriteFile(filepath.Join(msg, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.CfgPath = filepath.Join(root, "CONFIG.RA")
	if err := os.WriteFile(g.CfgPath, []byte{0}, 0644); err != nil {
		t.Fatal(err)
	}
	g.RaConfig.SysPath = `\ele`
	g.RaConfig.MsgBasePath = `\ele\msgbase`
	got, ok := Search(g, "Shurato")
	if !ok {
		t.Fatalf("missed Shurato path=%s", Path(g, cfgrec.UserBaseName))
	}
	if got.Handle != "Shurato" {
		t.Fatalf("handle %q", got.Handle)
	}
}

func TestWriteUnsetRecordDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	sysop := cfgrec.User{Name: "SysOp", Handle: "Sys", PasswordCRC: crc.RA("hunter", true)}
	other := cfgrec.User{Name: "Joe User", Handle: "Joe", PasswordCRC: crc.RA("secret", true)}
	raw := append(cfgrec.EncodeUser(sysop), cfgrec.EncodeUser(other)...)
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	blank := NewDefaults(g)
	if blank.Record >= 0 {
		t.Fatalf("new user Record=%d, want -1", blank.Record)
	}
	if err := Write(g, blank); err != nil {
		t.Fatal(err)
	}
	if _, ok := Search(g, "SysOp"); !ok {
		t.Fatal("SysOp clobbered by unset-record Write")
	}
	if _, ok := Search(g, "Joe User"); !ok {
		t.Fatal("Joe User missing after unset-record Write")
	}
}

func TestAppendKeepsExistingUsers(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	sysop := cfgrec.User{Name: "SysOp", PasswordCRC: crc.RA("hunter", true)}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(sysop), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(g); err != nil {
		t.Fatal(err)
	}
	nu := NewDefaults(g)
	nu.Name = "New User"
	if _, err := Append(g, nu); err != nil {
		t.Fatal(err)
	}
	if _, ok := Search(g, "SysOp"); !ok {
		t.Fatal("SysOp missing after Append")
	}
	if _, ok := Search(g, "New User"); !ok {
		t.Fatal("new user not found")
	}
	users, err := List(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("users=%d want 2", len(users))
	}
}

func TestWriteRefusesToReplaceSysop(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = dir
	sysop := cfgrec.User{Name: "SysOp", Handle: "Sys", PasswordCRC: crc.RA("hunter", true)}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(sysop), 0644); err != nil {
		t.Fatal(err)
	}
	ghost := NewDefaults(g)
	ghost.Name = "New User"
	ghost.Record = 0
	if err := Write(g, ghost); err == nil {
		t.Fatal("Write must not replace SysOp with a different name")
	}
	got, ok := Search(g, "SysOp")
	if !ok {
		t.Fatal("SysOp missing after refused Write")
	}
	if got.Name != "SysOp" {
		t.Fatalf("SysOp became %q", got.Name)
	}
}

func TestSearchPrefersLargerUsersFile(t *testing.T) {
	msg := t.TempDir()
	sys := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = msg
	g.RaConfig.SysPath = sys
	shadow := cfgrec.User{Name: "Ghost", PasswordCRC: crc.RA("x", true)}
	if err := os.WriteFile(filepath.Join(msg, cfgrec.UserBaseName), cfgrec.EncodeUser(shadow), 0644); err != nil {
		t.Fatal(err)
	}
	real := append(cfgrec.EncodeUser(cfgrec.User{Name: "SysOp", PasswordCRC: crc.RA("hunter", true)}),
		cfgrec.EncodeUser(cfgrec.User{Name: "Joe User", PasswordCRC: crc.RA("secret", true)})...)
	if err := os.WriteFile(filepath.Join(sys, cfgrec.UserBaseName), real, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Search(g, "SysOp"); !ok {
		t.Fatal("should use the larger SysPath users.bbs, not the 1-record shadow")
	}
	if _, ok := Search(g, "Joe User"); !ok {
		t.Fatal("Joe User in larger file")
	}
}

func TestSearchRelativeMsgBaseIgnoresCwd(t *testing.T) {
	sys := t.TempDir()
	msg := filepath.Join(sys, "msg")
	if err := os.Mkdir(msg, 0755); err != nil {
		t.Fatal(err)
	}
	u := cfgrec.User{Name: "SysOp", PasswordCRC: crc.RA("hunter", true)}
	if err := os.WriteFile(filepath.Join(msg, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = sys
	g.RaConfig.MsgBasePath = "msg"
	cwd := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if _, ok := Search(g, "sysop"); !ok {
		t.Fatal("relative MsgBasePath must resolve against SysPath, not node cwd")
	}
}
