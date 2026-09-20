package logon

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/term"
	"elebbs/internal/userbase"
)

func TestDecodeIEMSIRoundTrip(t *testing.T) {
	in := cfgrec.IEMSIUser{
		Name:         "Test User",
		Alias:        "Handle",
		Location:     "Town",
		DataNr:       "555-1212",
		VoiceNr:      "555-3434",
		Password:     "secret",
		BirthDate:    "1A2B3C4D",
		CrtDef:       "ANSI,AVT0",
		Protocols:    "Z89",
		Capabilities: "CHT,MNU",
		Requests:     "MORE,CLR,HOT",
		Software:     "Terminate 5.00",
	}
	pkt := buildICI(in)
	if !bytes.HasPrefix(pkt, []byte("**EMSI_ICI")) {
		t.Fatalf("frame %q", pkt[:min(20, len(pkt))])
	}
	if pkt[len(pkt)-1] != '\r' {
		t.Fatal("missing CR")
	}
	n64, err := strconv.ParseUint(string(pkt[10:14]), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	n := int(n64)
	got := decodeIEMSI(pkt[14 : 14+n])
	if got.Name != in.Name || got.Password != in.Password || got.Software != in.Software {
		t.Fatalf("decoded %+v", got)
	}
	if got.Requests != in.Requests || got.Alias != in.Alias {
		t.Fatalf("decoded %+v", got)
	}
}

func TestGetBracketEscapedBrace(t *testing.T) {
	s, n := getBracket([]byte("{a}}b}rest"), 0)
	if s != "a}b" {
		t.Fatalf("got %q", s)
	}
	if string([]byte("{a}}b}rest")[n:]) != "rest" {
		t.Fatalf("rest %q", []byte("{a}}b}rest")[n:])
	}
}

func TestStartIEMSISessionTelnetSendsIRQ(t *testing.T) {
	st := &keyStream{}
	line := &cfgrec.LineCfg{Baud: 65529}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.EMSIEnable = cfgrec.AskYes
	tio := term.New(st, g, line)
	startIEMSISession(tio, g, line)
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("**EMSI_IRQ")) {
		t.Fatalf("no IRQ: %q", out)
	}
	if !bytes.Contains(out, []byte{'\r'}) {
		t.Fatal("IRQ missing CR")
	}
}

func TestStartIEMSISessionLocalSilent(t *testing.T) {
	st := &keyStream{}
	line := &cfgrec.LineCfg{Baud: 0}
	g := &cfgrec.GlobalCfg{}
	tio := term.New(st, g, line)
	startIEMSISession(tio, g, line)
	if st.out.Len() != 0 {
		t.Fatalf("local IRQ %q", st.out.Bytes())
	}
}

func TestStartIEMSISessionDisabled(t *testing.T) {
	st := &keyStream{}
	line := &cfgrec.LineCfg{Baud: 65529}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.EMSIEnable = cfgrec.AskNo
	tio := term.New(st, g, line)
	startIEMSISession(tio, g, line)
	if st.out.Len() != 0 {
		t.Fatalf("disabled IRQ %q", st.out.Bytes())
	}
}

func TestPerformIEMSITelnetLogin(t *testing.T) {
	dir := t.TempDir()
	u := cfgrec.User{Name: "Test User", Handle: "Test", PasswordCRC: crc.RA("secret", true), Security: 10, ScreenLength: 24}
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), cfgrec.EncodeUser(u), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.TextPath = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.PasswordTries = 3
	g.RaConfig.EMSIEnable = cfgrec.AskYes
	g.RaConfig.SystemName = "Test Board"
	if err := userbase.RebuildIndex(g); err != nil {
		t.Fatal(err)
	}

	pkt := buildICI(cfgrec.IEMSIUser{
		Name:     "Test User",
		Password: "secret",
		CrtDef:   "ANSI",
		Requests: "MORE,CLR",
		Software: "TestTerm 1.0",
	})
	ack := []byte("**EMSI_ACKA490**EMSI_ACKA490")
	st := &keyStream{in: append(pkt, ack...)}
	line := &cfgrec.LineCfg{AnsiOn: true, Baud: 65529, RaNodeNr: 1, TelnetFromIP: "192.0.2.1"}
	tio := term.New(st, g, line)
	if !Perform(tio, g, line) {
		t.Fatalf("IEMSI logon failed leftover=%q out=%q", st.in, st.out.Bytes())
	}
	if !line.EmsiSession {
		t.Fatal("EMSI session not set")
	}
	if line.User.Name != "Test User" {
		t.Fatalf("name %q", line.User.Name)
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("**EMSI_IRQ")) {
		t.Fatalf("telnet logon missing IRQ: %q", out)
	}
	if !bytes.Contains(out, []byte("**EMSI_ISI")) {
		t.Fatalf("telnet logon missing ISI: %q", out)
	}
	irq := bytes.Index(out, []byte("**EMSI_IRQ"))
	pid := bytes.Index(out, []byte(cfgrec.PidName))
	if irq < 0 || pid < 0 || irq > pid {
		t.Fatalf("IRQ should be under the banner, irq=%d pid=%d out=%q", irq, pid, out)
	}
	if bytes.Contains(out[pid:], []byte("**EMSI_IRQ")) {
		t.Fatalf("IRQ left on screen after banner: %q", out)
	}
}

func TestGetIEMSIUserRejectsBadCRC(t *testing.T) {
	st := &keyStream{in: []byte("ICI0010{xxxxxxxx}DEADBEEF")}
	line := &cfgrec.LineCfg{Baud: 65529}
	g := &cfgrec.GlobalCfg{}
	tio := term.New(st, g, line)
	if getIEMSIUser(tio, g, line) {
		t.Fatal("bad CRC accepted")
	}
	if !bytes.Contains(st.out.Bytes(), []byte("**EMSI_NAK")) {
		t.Fatalf("expected NAK %q", st.out.Bytes())
	}
}

func TestIEMSIBirthDate(t *testing.T) {
	tm := time.Date(1990, 9, 16, 12, 0, 0, 0, time.Local)
	got, ok := iemsiBirthDate(fmt.Sprintf("%X", tm.Unix()))
	if !ok {
		t.Fatal("parse")
	}
	if got != "09-16-90" {
		t.Fatalf("birth %q", got)
	}
}
