package cfgrec

import "elebbs/internal/pascal"

// ParseConfig walks the packed RA 2.50 CONFIGrecord from struct.250.
func ParseConfig(b []byte) Config {
	r := pascal.NewBuf(b)
	c := Config{Raw: append([]byte(nil), b...)}
	c.VersionID = r.U16()
	c.ProductID = r.U8()
	r.I32()       // xBaud
	r.U8()        // xInitTries
	r.PString(70) // xInitStr
	r.PString(70) // xBusyStr
	// xInitResp, xBusyResp, Connect300/1200/2400/4800/9600/19k/38k — 9× String[40]
	for i := 0; i < 9; i++ {
		r.PString(40)
	}
	r.Bool()
	r.PString(20)
	r.PString(20)
	r.Bool()
	r.I16()
	c.MinimumBaud = r.U16()
	r.U16() // GraphicsBaud
	r.U16() // TransferBaud
	r.PString(5)
	r.PString(5)
	r.PString(5)
	r.PString(5)
	for i := 0; i < 7; i++ {
		r.PString(5)
	}
	for i := 0; i < 7; i++ {
		r.PString(5)
	}
	r.PString(22)
	r.PString(22)
	r.Skip(24)
	r.U16() // PwdExpiry
	c.Off.MenuPath = r.I
	c.MenuPath = r.PString(60)
	c.Off.TextPath = r.I
	c.TextPath = r.PString(60)
	c.AttachPath = r.PString(60)
	c.Nodelist = r.PString(60)
	c.Off.MsgBasePath = r.I
	c.MsgBasePath = r.PString(60)
	c.Off.SysPath = r.I
	c.SysPath = r.PString(60)
	c.ExternalEd = r.PString(60)
	for i := 0; i < 10; i++ {
		c.Address[i] = Addr{r.U16(), r.U16(), r.U16(), r.U16()}
	}
	c.Off.SystemName = r.I
	c.SystemName = r.PString(30)
	c.Off.NewSecurity = r.I
	c.NewSecurity = r.U16()
	c.NewCredit = r.U16()
	c.NewFlags = Flag(r.Flags())
	c.OriginLine = r.PString(60)
	c.QuoteString = r.PString(15)
	c.Off.Sysop = r.I
	c.Sysop = r.PString(35)
	c.LogFileName = r.PString(60)
	c.FastLogon = r.Bool()
	r.Bool() // AllowSysRem
	r.Bool() // MonoMode
	c.StrictPwdChecking = r.Bool()
	r.Bool() // DirectWrite
	r.Bool() // SnowCheck
	r.I16()  // CreditFactor
	c.UserTimeOut = r.U16()
	c.LogonTime = r.U16()
	c.Off.PasswordTries = r.I
	c.PasswordTries = r.U16()
	r.U16() // MaxPage
	c.Off.PageLength = r.I
	c.PageLength = r.U16()
	r.Bool()
	r.Bool()
	c.Off.OneWord = r.I
	c.OneWord = r.Bool()
	c.CheckMail = r.U8()
	r.Bool()
	r.Bool()
	r.Bool()
	r.Bool()
	r.Bool()
	r.Bool()
	c.Off.ANSI = r.I
	c.ANSI = r.U8()
	c.ClearScreen = r.U8()
	c.MorePrompt = r.U8()
	r.Bool()
	r.U8()
	r.U16()
	r.Flags()
	r.U16()
	r.Flags()
	r.U16()
	r.Flags()
	c.NormFore = r.U8()
	c.NormBack = r.U8()
	r.Skip(6) // stat/hi/wind colors
	r.Skip(8) // exit levels
	c.MultiLine = r.Bool()
	c.Off.MinPwdLen = r.I
	c.MinPwdLen = r.U8()
	r.U16()
	c.HotKeys = r.U8()
	r.Skip(6) // BorderFore/Back, BarFore/Back, LogStyle, MultiTasker
	c.PwdBoard = r.U8()
	r.U16()   // xBufferSize
	for i := 0; i < 10; i++ {
		r.PString(60)
	}
	r.Bool()
	r.U8()
	r.Bool()
	r.Bool()
	r.Skip(10)
	r.Bool()
	c.Off.LogonPrompt = r.I
	c.LogonPrompt = r.PString(40)
	r.U8() // CheckNewFiles
	r.PString(60)
	r.U8()
	r.Skip(6)
	r.PString(15)
	r.Skip(25)
	r.U16()
	c.LeftBracket = r.Char()
	c.RightBracket = r.Char()
	c.Off.AskHandle = r.I
	c.AskHandle = r.Bool()
	c.AskBirthDate = r.Bool()
	r.U16()
	r.Bool()
	r.Skip(30)
	r.PString(60)
	r.U8()
	r.Skip(3)
	r.U16()
	r.U16()
	r.U16()
	r.PString(60)
	r.Bool()
	c.NewUserGroup = r.U8()
	c.AVATAR = r.U8()
	c.BadPwdArea = r.U8()
	c.Off.Location = r.I
	c.Location = r.PString(40)
	r.U8()
	r.PString(40)
	r.U8()
	r.U8()
	c.LangHdr = r.PString(40)
	r.Bool()
	r.PString(60)
	r.U8() // FullMsgView
	c.EMSIEnable = r.U8()
	c.EMSINewUser = r.Bool()
	c.EchoChar = r.PString(1)
	r.PString(40)
	r.PString(40)
	r.PString(40)
	r.Skip(3)
	r.PString(60)
	r.U8()
	c.NewUserLang = r.U8()
	c.LanguagePrompt = r.PString(40)
	r.U8()
	r.Bool()
	r.Bool()
	r.U8()
	c.Off.KeyboardPwd = r.I
	c.KeyboardPwd = r.PString(15)
	c.CapLocation = r.Bool()
	r.U8()
	r.PString(4)
	r.U8()
	r.U8()
	r.PString(70)
	r.Bool()
	c.SemPath = r.PString(60)
	r.Bool()
	c.Off.FileBase = r.I
	c.FileBase = r.PString(60)
	r.Bool()
	r.Bool()
	r.PString(60)
	r.U8()
	r.U8()
	r.PString(40)
	r.U8()
	r.U8()
	c.FileLine = r.PString(200)
	r.PString(200)
	r.U8()
	r.U16()
	for i := 0; i < 10; i++ {
		r.PString(3)
		r.PString(60)
		r.PString(60)
	}
	for i := 0; i < 5; i++ {
		r.PString(60)
	}
	r.PString(60)
	r.PString(40)
	r.U8()
	r.Bool()
	r.Bool()
	r.U8()
	r.U8()
	r.U16()
	r.U16()
	r.Bool()
	r.Skip(400) // DefaultCombined
	r.Bool()
	r.Bool()
	r.U8()
	r.Bool()
	r.Skip(6)
	r.Bool()
	r.Bool()
	r.Bool()
	r.U8()
	r.PString(60)
	r.Skip(2)
	r.Bool()
	r.U8()
	// remaining FutureExpansion ignored
	if c.PageLength == 0 {
		c.PageLength = 24
	}
	if c.PasswordTries == 0 {
		c.PasswordTries = 3
	}
	if c.LogonTime == 0 {
		c.LogonTime = 15
	}
	if c.LogonPrompt == "" {
		c.LogonPrompt = "Please enter your full name: "
	}
	if c.EchoChar == "" {
		c.EchoChar = "*"
	}
	return c
}

func (c *Config) SyncRaw() {
	if len(c.Raw) == 0 {
		return
	}
	putS := func(off, max int, s string) {
		if off <= 0 || off+max+1 > len(c.Raw) {
			return
		}
		pascal.PutString(c.Raw[off:off+max+1], s)
	}
	putU8 := func(off int, v byte) {
		if off > 0 && off < len(c.Raw) {
			c.Raw[off] = v
		}
	}
	putU16 := func(off int, v uint16) {
		if off > 0 && off+1 < len(c.Raw) {
			c.Raw[off] = byte(v)
			c.Raw[off+1] = byte(v >> 8)
		}
	}
	putS(c.Off.SystemName, 30, c.SystemName)
	putS(c.Off.Sysop, 35, c.Sysop)
	putS(c.Off.Location, 40, c.Location)
	putS(c.Off.LogonPrompt, 40, c.LogonPrompt)
	putS(c.Off.KeyboardPwd, 15, c.KeyboardPwd)
	putS(c.Off.MenuPath, 60, c.MenuPath)
	putS(c.Off.TextPath, 60, c.TextPath)
	putS(c.Off.MsgBasePath, 60, c.MsgBasePath)
	putS(c.Off.SysPath, 60, c.SysPath)
	putS(c.Off.FileBase, 60, c.FileBase)
	putU16(c.Off.NewSecurity, c.NewSecurity)
	putU16(c.Off.PasswordTries, c.PasswordTries)
	putU16(c.Off.PageLength, c.PageLength)
	putU8(c.Off.MinPwdLen, c.MinPwdLen)
	putU8(c.Off.ANSI, c.ANSI)
	if c.OneWord {
		putU8(c.Off.OneWord, 1)
	} else {
		putU8(c.Off.OneWord, 0)
	}
	if c.AskHandle {
		putU8(c.Off.AskHandle, 1)
	} else {
		putU8(c.Off.AskHandle, 0)
	}
}
