package cfgrec

import (
	"elebbs/internal/pascal"
	"testing"
)

// packedOffset walks CONFIGrecord (struct.250) independently of ParseConfig.
func packedConfigOffset(after func(add func(int) int)) {
	off := 0
	add := func(n int) int {
		cur := off
		off += n
		return cur
	}
	after(add)
}

func TestParseConfigLogonAndPaths(t *testing.T) {
	raw := make([]byte, 4096)
	raw[0] = 0x50
	raw[1] = 0x02 // VersionID $250
	var menuOff, msgOff, sysOff, promptOff, nameOff int
	var killOff, crashAskOff, crashOff int
	packedConfigOffset(func(add func(int) int) {
		add(2)  // VersionID
		add(1)  // ProductID
		add(4)  // xBaud
		add(1)  // xInitTries
		add(71) // xInitStr
		add(71) // xBusyStr
		for i := 0; i < 9; i++ {
			add(41)
		}
		add(1)  // xAnswerPhone
		add(21) // xRing
		add(21) // xAnswerStr
		add(1)  // xFlushBuffer
		add(2)  // xModemDelay
		add(2)  // MinimumBaud
		add(2)  // GraphicsBaud
		add(2)  // TransferBaud
		add(6)
		add(6)
		add(6)
		add(6)
		add(42) // PageStart
		add(42) // PageEnd
		add(23) // SeriNum
		add(23) // CustNum
		add(24) // FreeSpace1
		add(2)  // PwdExpiry
		menuOff = add(61)
		add(61) // TextPath
		add(61) // AttachPath
		add(61) // Nodelist
		msgOff = add(61)
		sysOff = add(61)
		add(61) // ExternalEd
		add(80) // Address[0..9]
		nameOff = add(31)
		add(2)  // NewSecurity
		add(2)  // NewCredit
		add(4)  // NewFlags
		add(61) // OriginLine
		add(16) // QuoteString
		add(36) // Sysop
		add(61) // LogFileName
		add(6)  // 6 booleans
		add(2)  // CreditFactor
		add(2)  // UserTimeOut
		add(2)  // LogonTime
		add(2)  // PassWordTries
		add(2)  // MaxPage
		add(2)  // PageLength
		add(3)  // CheckForMulti, ExcludeSysop, OneWord
		add(1)  // CheckMail
		add(6)  // 6 booleans
		add(3)  // ANSI, Clear, More
		add(1)  // UploadMsgs
		killOff = add(1)
		crashAskOff = add(2)
		add(4) // CrashAskFlags
		crashOff = add(2)
		add(4)     // CrashFlags
		add(2 + 4) // FAttachSec/Flags
		add(8)  // colors
		add(8)  // exit levels
		add(1)  // MultiLine
		add(1)  // MinPwdLen
		add(2)  // MinUpSpace
		add(1)  // HotKeys
		add(7)  // BorderFore/Back, BarFore/Back, LogStyle, MultiTasker, PwdBoard
		add(2)  // xBufferSize
		add(10 * 61) // FKeys
		add(1)  // WhyPage
		add(1)  // LeaveMsg
		add(1)  // ShowMissingFiles
		add(1)  // xLockModem
		add(10) // FreeSpace2
		add(1)  // AllowNetmailReplies
		promptOff = add(41)
	})
	put := func(off, max int, s string) {
		pascal.PutString(raw[off:off+max+1], s)
	}
	put(menuOff, 60, `C:\BBS\MENU\`)
	put(msgOff, 60, `C:\BBS\MSG\`)
	put(sysOff, 60, `C:\BBS\`)
	put(nameOff, 30, "Test Board")
	put(promptOff, 40, "`A14:What is your name? ")
	raw[killOff] = 2
	raw[crashAskOff] = 10
	raw[crashOff+1] = 1 // 256

	c := ParseConfig(raw)
	if c.KillSent != 2 || c.CrashAskSec != 10 || c.CrashSec != 256 {
		t.Fatalf("KillSent %d CrashAskSec %d CrashSec %d", c.KillSent, c.CrashAskSec, c.CrashSec)
	}
	if c.MenuPath != `C:\BBS\MENU\` {
		t.Fatalf("MenuPath %q off=%d", c.MenuPath, c.Off.MenuPath)
	}
	if c.MsgBasePath != `C:\BBS\MSG\` {
		t.Fatalf("MsgBasePath %q off=%d want %d", c.MsgBasePath, c.Off.MsgBasePath, msgOff)
	}
	if c.SysPath != `C:\BBS\` {
		t.Fatalf("SysPath %q", c.SysPath)
	}
	if c.SystemName != "Test Board" {
		t.Fatalf("SystemName %q", c.SystemName)
	}
	if c.LogonPrompt != "`A14:What is your name? " {
		t.Fatalf("LogonPrompt %q off=%d want %d", c.LogonPrompt, c.Off.LogonPrompt, promptOff)
	}
}
