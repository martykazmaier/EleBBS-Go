package config

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
)

func TestProtocolNameFromFileAndIntern(t *testing.T) {
	dir := t.TempDir()
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:        "Auto",
		ActiveKey:   '@',
		Attribute:   1,
		DnCmdString: `c:\ele\prot\sexyz.exe /B115200`,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	if got := ProtocolName(g, '@'); got != "Auto" {
		t.Fatalf("! for @ = %q want Auto", got)
	}
	if got := ProtocolName(g, 'Z'); got != "Zmodem" {
		t.Fatalf("intern Z = %q", got)
	}
	if got := ProtocolName(g, 0); got != "" {
		t.Fatalf("empty key = %q", got)
	}
	if got := ProtocolName(g, '|'); got != "" {
		t.Fatalf("| key = %q want empty", got)
	}
	p := FindProtocol(g, '@')
	if p.Name != "Auto" || p.DnCmdString == "" {
		t.Fatalf("FindProtocol @ = %+v", p)
	}
	if z := FindProtocol(g, 'Z'); z.Name != "" {
		t.Fatalf("FindProtocol intern Z should be empty, got %+v", z)
	}
}
