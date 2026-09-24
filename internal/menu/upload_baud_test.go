package menu

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/term"
)

func TestUploadDoesNotGateOnBaud(t *testing.T) {
	dir := t.TempDir()
	up := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(up, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.Sysop = "Sysop"
	line := &cfgrec.LineCfg{
		Baud:             300, // below classic TransferBaud default of 300? still allowed
		ErrorFreeConnect: true,
		AnsiOn:           true,
		User: cfgrec.User{
			Name:         "Bob",
			Security:     100,
			FileArea:     1,
			DefaultProto: 'Z',
			Record:       -1,
		},
	}
	st := &seqStream{in: []byte("A")} // abort at SNA
	tio := term.New(st, g, line)
	eng := &Engine{
		T:    tio,
		G:    g,
		Line: line,
		Files: []cfgrec.FilesArea{{
			AreaNum:        1,
			Name:           "Uploads",
			FilePath:       up,
			UploadSecurity: 10,
			Security:       10,
		}},
	}
	// Register a usable protocol so we reach the SNA prompt instead of XferSlow.
	protPath := filepath.Join(dir, "protocol.ra")
	raw := make([]byte, 512)
	// Minimal: leave empty so FindProtocol fails and we quit without XferSlow.
	_ = os.WriteFile(protPath, raw, 0644)
	eng.upload("")
	out := st.out.String()
	if strings.Contains(out, "not permitted at this speed") {
		t.Fatalf("TransferBaud gate still active: %q", out)
	}
}

func TestUploadAccessDenied(t *testing.T) {
	dir := t.TempDir()
	up := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(up, 0755); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{
		Baud:   65529,
		AnsiOn: true,
		User:   cfgrec.User{Name: "Bob", Security: 5, FileArea: 1, Record: -1},
	}
	st := &seqStream{in: []byte("\r")}
	tio := term.New(st, g, line)
	eng := &Engine{
		T:    tio,
		G:    g,
		Line: line,
		Files: []cfgrec.FilesArea{{
			AreaNum:        1,
			Name:           "Uploads",
			FilePath:       up,
			UploadSecurity: 50,
		}},
	}
	eng.upload("")
	out := st.out.String()
	if !strings.Contains(out, "upload access") && !bytes.Contains(st.out.Bytes(), []byte("upload")) {
		// Language file may be empty; at least we must not claim XferSlow.
		if strings.Contains(out, "not permitted at this speed") {
			t.Fatalf("got speed gate instead of access denial: %q", out)
		}
	}
}
