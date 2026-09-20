package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestPathCandidatesRemapsEleDrive(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "txtfiles", "welcome", "WELCOME.TXT")
	if err := os.MkdirAll(filepath.Dir(want), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = root
	got := ExistingFile(g, `C:\ELE\TXTFILES\WELCOME\WELCOME.TXT`)
	if !strings.EqualFold(got, want) {
		t.Fatalf("ExistingFile=%q want %q", got, want)
	}
}

func TestAfterEleRoot(t *testing.T) {
	rest, ok := afterEleRoot(`c:\ele\txtfiles\welcome\Demiurge.ans`)
	if !ok {
		t.Fatal("expected ele root")
	}
	if !strings.EqualFold(rest, `txtfiles\welcome\Demiurge.ans`) &&
		!strings.EqualFold(rest, `txtfiles/welcome/Demiurge.ans`) {
		t.Fatalf("rest=%q", rest)
	}
}
