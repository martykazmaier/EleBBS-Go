package cfgrec

import "testing"

func TestHasKeyboardPwd(t *testing.T) {
	if HasKeyboardPwd("") {
		t.Fatal("empty")
	}
	if HasKeyboardPwd("   ") {
		t.Fatal("spaces")
	}
	if HasKeyboardPwd("C:\\BBS\\MENU") {
		t.Fatal("path")
	}
	if HasKeyboardPwd("\x01\x02abc") {
		t.Fatal("control")
	}
	if HasKeyboardPwd(string([]byte{0xC9, 0xCD, 0xBB})) {
		t.Fatal("oem junk")
	}
	if !HasKeyboardPwd("secret") {
		t.Fatal("secret should count")
	}
	if !HasKeyboardPwd("Ab1") {
		t.Fatal("short alnum")
	}
}

func TestParseConfigEmptyKeyboardPwd(t *testing.T) {
	raw := make([]byte, 4096)
	raw[0] = 0x50
	raw[1] = 0x02 // VersionID 0x250
	c := ParseConfig(raw)
	if HasKeyboardPwd(c.KeyboardPwd) {
		t.Fatalf("zeroed CONFIG.RA must not ask for keyboard password, got %q off=%d", c.KeyboardPwd, c.Off.KeyboardPwd)
	}
}

func TestEncodeTelnetRoundTrip(t *testing.T) {
	in := TelnetCfg{MaxSessions: 8, ServerPort: 2323, StartNodeWith: 10, ProgramPath: `C:\BBS`, NodeDirectories: `C:\BBS\NODE*N`}
	out := ParseTelnet(EncodeTelnet(in))
	if out.MaxSessions != 8 || out.ServerPort != 2323 || out.StartNodeWith != 10 {
		t.Fatalf("%+v", out)
	}
	if out.ProgramPath != in.ProgramPath || out.NodeDirectories != in.NodeDirectories {
		t.Fatalf("paths %+v", out)
	}
}
