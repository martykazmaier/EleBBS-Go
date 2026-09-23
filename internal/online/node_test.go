package online

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestResolveSemaFileUsesSemPath(t *testing.T) {
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = `c:\ele\`
	g.RaConfig.SemPath = `c:\ele\sem\`
	got := ResolveSemaFile(g, "node2.ra")
	want := filepath.Join(`c:\ele\sem`, "node2.ra")
	if !strings.EqualFold(got, want) {
		t.Fatalf("ResolveSemaFile=%q want %q", got, want)
	}
	abs := `c:\ele\sem\node3.ra`
	if got := ResolveSemaFile(g, abs); got != abs {
		t.Fatalf("abs path rewritten: %q", got)
	}
}

func TestSetRaBusyCreatesAndClears(t *testing.T) {
	sem := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = t.TempDir()
	g.RaConfig.SemPath = sem
	SetRaBusy(g, 2, true)
	p := RaBusyPath(g, 2)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("rabusy.2 missing in SemPath: %v", err)
	}
	if filepath.Dir(p) != sem {
		t.Fatalf("rabusy dir %q want %q", filepath.Dir(p), sem)
	}
	SetRaBusy(g, 2, false)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("rabusy.2 should be erased on logoff")
	}
}

func TestFindNodeFilePrefersSemPath(t *testing.T) {
	sys := t.TempDir()
	sem := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = sys
	g.RaConfig.SemPath = sem
	if err := os.WriteFile(filepath.Join(sem, "node2.ra"), []byte("from sem"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sys, "NODE2.RA"), []byte("from sys"), 0644); err != nil {
		t.Fatal(err)
	}
	p := FindNodeFile(g, 2)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "from sem" {
		t.Fatalf("FindNodeFile read %q from %q, want SemPath", b, p)
	}
}
