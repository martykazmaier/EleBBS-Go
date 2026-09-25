package cfgrec

import (
	"elebbs/internal/pascal"
	"testing"
)

// Offsets relative to KeyboardPwd, walked from struct.250 by hand.
const (
	chatCommandFromKbd     = -(61 + 1 + 1 + 41 + 1 + 1 + 1 + 1)
	autoChatCaptureFromKbd = 16 + 1 + 1 + 5 + 1 + 1 + 71 + 1 + 61
	limitLocalFromKbd      = autoChatCaptureFromKbd + 1 + 2662
)

func TestParseConfigSysopKeySettings(t *testing.T) {
	raw := make([]byte, 8192)
	kbd := ParseConfig(raw).Off.KeyboardPwd
	pascal.PutString(raw[kbd+chatCommandFromKbd:kbd+chatCommandFromKbd+61], "CHAT.BAT")
	pascal.PutString(raw[kbd:kbd+16], "lockme")
	raw[kbd+autoChatCaptureFromKbd] = 1
	raw[kbd+limitLocalFromKbd] = 1
	raw[kbd+limitLocalFromKbd+1] = 1

	c := ParseConfig(raw)
	if c.KeyboardPwd != "lockme" {
		t.Fatalf("KeyboardPwd %q", c.KeyboardPwd)
	}
	if c.ChatCommand != "CHAT.BAT" {
		t.Fatalf("ChatCommand %q", c.ChatCommand)
	}
	if !c.AutoChatCapture {
		t.Fatal("AutoChatCapture not read")
	}
	if !c.LimitLocal || !c.SavePasswords {
		t.Fatalf("LimitLocal=%v SavePasswords=%v", c.LimitLocal, c.SavePasswords)
	}
}
