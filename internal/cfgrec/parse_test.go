package cfgrec

import "testing"

func TestUserRoundTrip(t *testing.T) {
	u := User{
		Name:         "Joe User",
		Handle:       "Joe",
		Location:     "Town",
		PasswordCRC:  12345,
		Security:     100,
		ScreenLength: 24,
		ScreenWidth:  80,
		MsgArea:      1,
		FileArea:     2,
		DefaultProto: '@',
		Attribute:    UserANSI | UserMore,
		Record:       0,
	}
	b := EncodeUser(u)
	if len(b) != UsersSize {
		t.Fatalf("size %d want %d", len(b), UsersSize)
	}
	if UsersSize != 1016 {
		t.Fatalf("UsersSize %d want 1016 (Pascal USERSrecord)", UsersSize)
	}
	got := ParseUser(b, 0)
	if got.Name != u.Name || got.Handle != u.Handle || got.PasswordCRC != u.PasswordCRC {
		t.Fatalf("roundtrip %+v", got)
	}
	if got.Security != 100 || got.FileArea != 2 {
		t.Fatalf("fields %+v", got)
	}
	if got.DefaultProto != '@' {
		t.Fatalf("DefaultProto %q", got.DefaultProto)
	}
}

func TestMenuSize(t *testing.T) {
	b := make([]byte, MenuSize)
	b[0] = 9
	m := ParseMenuItem(b)
	if m.Typ != 9 {
		t.Fatalf("typ %d", m.Typ)
	}
}

func TestLightBarSize(t *testing.T) {
	b := make([]byte, LightBarSize)
	b[0] = 10
	b[1] = 5
	b[2] = 4
	copy(b[3:], "Low!")
	b[138] = 6
	copy(b[139:], "High!!")
	b[274] = 1
	l := ParseLightBar(b)
	if l.LightX != 10 || l.LightY != 5 {
		t.Fatalf("xy %d,%d", l.LightX, l.LightY)
	}
	if l.LowItem != "Low!" || l.SelectItem != "High!!" {
		t.Fatalf("text %+v", l)
	}
	if !l.Enabled() {
		t.Fatal("attrib bit 0 should enable")
	}
}

func TestSysInfoRoundTrip(t *testing.T) {
	s := SysInfo{TotalCalls: 12345, LastCaller: "Joe", LastHandle: "Joey"}
	b := EncodeSysInfo(s)
	if len(b) != SysInfoSize {
		t.Fatalf("size %d want %d", len(b), SysInfoSize)
	}
	got := ParseSysInfo(b)
	if got.TotalCalls != 12345 || got.LastCaller != "Joe" || got.LastHandle != "Joey" {
		t.Fatalf("%+v", got)
	}
}

func TestProtocolSize(t *testing.T) {
	p := Protocol{
		Name:        "Zmodem",
		ActiveKey:   'Z',
		Batch:       true,
		Attribute:   1,
		DnCmdString: `c:\ele\prot\sexyz.exe /P*P /B115200`,
	}
	b := EncodeProtocol(p)
	if len(b) != ProtocolSize {
		t.Fatalf("size %d want %d", len(b), ProtocolSize)
	}
	got := ParseProtocol(b)
	if got.Name != "Zmodem" || got.ActiveKey != 'Z' || !got.Batch || got.Attribute != 1 {
		t.Fatalf("%+v", got)
	}
	if got.DnCmdString != p.DnCmdString {
		t.Fatalf("cmd %q", got.DnCmdString)
	}
	if ProtocolRecSize(ProtocolSize*3) != ProtocolSize {
		t.Fatal("rec size 550")
	}
}
