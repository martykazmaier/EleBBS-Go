package cmdline

import (
	"testing"

	"elebbs/internal/cfgrec"
)

func TestParseLocalNode(t *testing.T) {
	o := Parse([]string{"-L", "-N2", "-B2400"}, true)
	if !o.Local || o.Node != 2 {
		t.Fatalf("%+v", o)
	}
	o = Parse([]string{"-H204", "-N3"}, true)
	if o.InheritedHandle != 204 || o.Node != 3 {
		t.Fatalf("handle %+v", o)
	}
	o = Parse([]string{"-XT"}, true)
	if !o.TelnetServ {
		t.Fatalf("telnet %+v", o)
	}
	o = Parse([]string{"-?"}, true)
	if !o.ShowHelp {
		t.Fatal("help")
	}
}

func TestParseDefaultExitCode(t *testing.T) {
	if o := Parse([]string{"-E10", "-N1"}, true); o.ExitCode != 10 {
		t.Fatalf("-E10 gave %d", o.ExitCode)
	}
}

func TestParseFrontEndSpawn(t *testing.T) {
	o := Parse([]string{"-XC", "-XT", "-B65529", "-H204", "-N2", "-XI192.168.0.10"}, true)
	if o.Node != 2 || o.InheritedHandle != 204 || !o.TelnetServ || !o.NoClose {
		t.Fatalf("spawn %+v", o)
	}
	if o.TelnetFromIP != "192.168.0.10" || o.Baud != 65529 || o.ComPort != 0 {
		t.Fatalf("spawn fields %+v", o)
	}
	o = Parse([]string{"-N2-H204-XT-XC-XI10.0.0.1-B65529"}, true)
	if o.Node != 2 || o.InheritedHandle != 204 || !o.TelnetServ || !o.NoClose {
		t.Fatalf("glued %+v", o)
	}
	if o.TelnetFromIP != "10.0.0.1" || o.Baud != 65529 {
		t.Fatalf("glued fields %+v", o)
	}
	line := &cfgrec.LineCfg{InheritedHandle: ^uintptr(0)}
	Apply(line, o)
	if line.LocalLogon || line.RaNodeNr != 2 || line.TelnetFromIP != "10.0.0.1" || !line.ComNoClose || !line.TelnetServ {
		t.Fatalf("apply %+v", line)
	}
	if line.Baud != 65529 || line.InheritedHandle != 204 {
		t.Fatalf("apply handle/baud %+v", line)
	}
	if line.Modem.ComPort != 0 {
		t.Fatalf("spawn without -C should keep COM 0: %+v", line.Modem)
	}
}

func TestPascalTelnetSpawnKeepsCOM1(t *testing.T) {
	o := Parse([]string{"-C1", "-XC", "-XT", "-B65529", "-H204", "-N2"}, true)
	if !o.TelnetServ || o.ComPort != 1 || !o.NoClose {
		t.Fatalf("Pascal -C1 -XT must keep both: %+v", o)
	}
	line := &cfgrec.LineCfg{InheritedHandle: ^uintptr(0), Modem: cfgrec.Modem{ComPort: 2}}
	Apply(line, o)
	if !line.TelnetServ || line.Modem.ComPort != 1 || line.LocalLogon {
		t.Fatalf("apply: telnet=%v com=%d local=%v", line.TelnetServ, line.Modem.ComPort, line.LocalLogon)
	}
}
