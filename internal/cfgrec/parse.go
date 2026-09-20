package cfgrec

import "elebbs/internal/pascal"

func ParseUsersIdx(b []byte) UsersIdx {
	r := pascal.NewBuf(b)
	return UsersIdx{NameCRC32: r.I32(), HandleCRC32: r.I32()}
}

func ParseUser(b []byte, rec int) User {
	r := pascal.NewBuf(b)
	u := User{Record: rec}
	u.Name = r.PString(35)
	u.Location = r.PString(25)
	u.Organisation = r.PString(50)
	u.Address1 = r.PString(50)
	u.Address2 = r.PString(50)
	u.Address3 = r.PString(50)
	u.Handle = r.PString(35)
	u.Comment = r.PString(80)
	u.PasswordCRC = r.I32()
	u.DataPhone = r.PString(15)
	u.VoicePhone = r.PString(15)
	u.LastTime = r.PString(5)
	u.LastDate = r.PString(8)
	u.Attribute = r.U8()
	u.Attribute2 = r.U8()
	u.Flags = Flag(r.Flags())
	u.Credit = r.I32()
	u.Pending = r.I32()
	u.MsgsPosted = r.U16()
	u.Security = r.U16()
	u.LastRead = r.I32()
	u.NoCalls = r.I32()
	u.Uploads = r.I32()
	u.Downloads = r.I32()
	u.UploadsK = r.I32()
	u.DownloadsK = r.I32()
	u.TodayK = r.I32()
	u.Elapsed = r.I16()
	u.ScreenLength = r.U16()
	u.LastPwdChange = r.U8()
	u.Group = r.U16()
	for i := 0; i < 200; i++ {
		u.Combined[i] = r.U16()
	}
	u.FirstDate = r.PString(8)
	u.BirthDate = r.PString(8)
	u.SubDate = r.PString(8)
	u.ScreenWidth = r.U8()
	u.Language = r.U8()
	u.DateFormat = r.U8()
	u.ForwardTo = r.PString(35)
	u.MsgArea = r.U16()
	u.FileArea = r.U16()
	u.DefaultProto = r.Char()
	u.FileGroup = r.U16()
	u.LastDOBCheck = r.U8()
	u.Sex = r.U8()
	u.XIRecord = r.I32()
	u.MsgGroup = r.U16()
	u.Attribute3 = r.U8()
	u.Password = r.PString(15)
	if u.ScreenLength == 0 {
		u.ScreenLength = 24
	}
	if u.ScreenWidth == 0 {
		u.ScreenWidth = 80
	}
	return u
}

func EncodeUser(u User) []byte {
	w := &pascal.Writer{}
	w.PString(35, u.Name)
	w.PString(25, u.Location)
	w.PString(50, u.Organisation)
	w.PString(50, u.Address1)
	w.PString(50, u.Address2)
	w.PString(50, u.Address3)
	w.PString(35, u.Handle)
	w.PString(80, u.Comment)
	w.I32(u.PasswordCRC)
	w.PString(15, u.DataPhone)
	w.PString(15, u.VoicePhone)
	w.PString(5, u.LastTime)
	w.PString(8, u.LastDate)
	w.U8(u.Attribute)
	w.U8(u.Attribute2)
	w.Flags([4]byte(u.Flags))
	w.I32(u.Credit)
	w.I32(u.Pending)
	w.U16(u.MsgsPosted)
	w.U16(u.Security)
	w.I32(u.LastRead)
	w.I32(u.NoCalls)
	w.I32(u.Uploads)
	w.I32(u.Downloads)
	w.I32(u.UploadsK)
	w.I32(u.DownloadsK)
	w.I32(u.TodayK)
	w.I16(u.Elapsed)
	w.U16(u.ScreenLength)
	w.U8(u.LastPwdChange)
	w.U16(u.Group)
	for i := 0; i < 200; i++ {
		w.U16(u.Combined[i])
	}
	w.PString(8, u.FirstDate)
	w.PString(8, u.BirthDate)
	w.PString(8, u.SubDate)
	w.U8(u.ScreenWidth)
	w.U8(u.Language)
	w.U8(u.DateFormat)
	w.PString(35, u.ForwardTo)
	w.U16(u.MsgArea)
	w.U16(u.FileArea)
	w.Char(u.DefaultProto)
	w.U16(u.FileGroup)
	w.U8(u.LastDOBCheck)
	w.U8(u.Sex)
	w.I32(u.XIRecord)
	w.U16(u.MsgGroup)
	w.U8(u.Attribute3)
	w.PString(15, u.Password)
	if len(w.B) < UsersSize {
		w.Pad(UsersSize - len(w.B))
	}
	if len(w.B) > UsersSize {
		return w.B[:UsersSize]
	}
	return w.B
}

func ParseMenuItem(b []byte) MenuItem {
	r := pascal.NewBuf(b)
	m := MenuItem{}
	m.Typ = r.U8()
	m.Security = r.U16()
	m.MaxSec = r.U16()
	m.NotFlags = Flag(r.Flags())
	m.Flags = Flag(r.Flags())
	m.TimeLeft = r.U16()
	m.TimeUsed = r.U16()
	m.Age = r.U8()
	m.TermAttrib = r.U8()
	m.MinSpeed = r.I32()
	m.MaxSpeed = r.I32()
	m.Credit = r.I32()
	m.OptionCost = r.I32()
	m.PerMinCost = r.I32()
	copy(m.Node[:], r.Bytes(32))
	copy(m.Group[:], r.Bytes(32))
	for i := 0; i < 7; i++ {
		m.StartTime[i] = r.U16()
	}
	for i := 0; i < 7; i++ {
		m.StopTime[i] = r.U16()
	}
	m.Display = r.PString(135)
	m.HotKey = r.PString(8)
	m.MiscData = r.PString(135)
	m.Foreground = r.U8()
	m.Background = r.U8()
	return m
}

func ParseLightBar(b []byte) LightBar {
	r := pascal.NewBuf(b)
	l := LightBar{}
	l.LightX = r.U8()
	l.LightY = r.U8()
	l.LowItem = r.PString(135)
	l.SelectItem = r.PString(135)
	l.Attrib = r.U8()
	return l
}

func ParseSysInfo(b []byte) SysInfo {
	r := pascal.NewBuf(b)
	s := SysInfo{}
	s.TotalCalls = r.I32()
	s.LastCaller = r.PString(35)
	s.LastHandle = r.PString(35)
	return s
}

func EncodeSysInfo(s SysInfo) []byte {
	w := &pascal.Writer{}
	w.I32(s.TotalCalls)
	w.PString(35, s.LastCaller)
	w.PString(35, s.LastHandle)
	w.Pad(92)
	if len(w.B) < SysInfoSize {
		w.Pad(SysInfoSize - len(w.B))
	}
	return w.B[:SysInfoSize]
}

func ParseProtocol(b []byte) Protocol {
	r := pascal.NewBuf(b)
	p := Protocol{}
	p.Name = r.PString(15)
	p.ActiveKey = r.U8()
	p.OpusType = r.Bool()
	p.Batch = r.Bool()
	p.Attribute = r.U8()
	p.LogFileName = r.PString(80)
	p.CtlFileName = r.PString(80)
	p.DnCmdString = r.PString(80)
	p.DnCtlString = r.PString(80)
	p.UpCmdString = r.PString(80)
	p.UpCtlString = r.PString(80)
	p.UpLogKeyWord = r.PString(20)
	p.DnLogKeyWord = r.PString(20)
	p.XferDescWord = r.U8()
	p.XferNameWord = r.U8()
	return p
}

func EncodeProtocol(p Protocol) []byte {
	w := &pascal.Writer{}
	w.PString(15, p.Name)
	w.U8(p.ActiveKey)
	w.Bool(p.OpusType)
	w.Bool(p.Batch)
	w.U8(p.Attribute)
	w.PString(80, p.LogFileName)
	w.PString(80, p.CtlFileName)
	w.PString(80, p.DnCmdString)
	w.PString(80, p.DnCtlString)
	w.PString(80, p.UpCmdString)
	w.PString(80, p.UpCtlString)
	w.PString(20, p.UpLogKeyWord)
	w.PString(20, p.DnLogKeyWord)
	w.U8(p.XferDescWord)
	w.U8(p.XferNameWord)
	if len(w.B) < ProtocolSize {
		w.Pad(ProtocolSize - len(w.B))
	}
	return w.B[:ProtocolSize]
}

func ParseFilesArea(b []byte) FilesArea {
	r := pascal.NewBuf(b)
	a := FilesArea{}
	a.AreaNum = r.U16()
	r.U16() // unused
	a.Name = r.PString(40)
	a.Attrib = r.U8()
	a.FilePath = r.PString(40)
	a.KillDaysDL = r.U16()
	a.KillDaysFD = r.U16()
	a.Password = r.PString(15)
	a.MoveArea = r.U16()
	a.Age = r.U8()
	a.ConvertExt = r.U8()
	a.Group = r.U16()
	a.Attrib2 = r.U8()
	a.DefCost = r.U16()
	a.UploadArea = r.U16()
	a.UploadSecurity = r.U16()
	r.Skip(8) // upload flags + not flags
	a.Security = r.U16()
	r.Skip(8) // flags + not flags
	a.ListSecurity = r.U16()
	r.Skip(8)
	a.AltGroup[0] = r.U16()
	a.AltGroup[1] = r.U16()
	a.AltGroup[2] = r.U16()
	a.Device = r.U8()
	return a
}

func ParseFilesHdr(b []byte, rec uint16) FilesHdr {
	r := pascal.NewBuf(b)
	h := FilesHdr{RecordNum: rec}
	name := r.Bytes(13)
	copy(h.NameRaw[:], name)
	h.Name = pascal.String(name)
	h.Size = r.U32()
	h.CRC32 = r.U32()
	h.Uploader = r.PString(35)
	h.UploadDate = r.U32()
	h.FileDate = r.U32()
	h.LastDL = r.U32()
	h.TimesDL = r.U16()
	h.Attrib = r.U8()
	h.Password = r.PString(15)
	for i := 0; i < 5; i++ {
		h.Keywords[i] = r.PString(15)
	}
	h.Cost = r.U16()
	h.LongDescPtr = r.I32()
	h.LfnPtr = r.I32()
	return h
}

func EncodeFilesHdr(h FilesHdr) []byte {
	w := &pascal.Writer{}
	name := make([]byte, 13)
	copy(name, h.NameRaw[:])
	if name[0] == 0 && h.Name != "" {
		pascal.PutString(name, h.Name)
	}
	w.B = append(w.B, name...)
	w.U32(h.Size)
	w.U32(h.CRC32)
	w.PString(35, h.Uploader)
	w.U32(h.UploadDate)
	w.U32(h.FileDate)
	w.U32(h.LastDL)
	w.U16(h.TimesDL)
	w.U8(h.Attrib)
	w.PString(15, h.Password)
	for i := 0; i < 5; i++ {
		w.PString(15, h.Keywords[i])
	}
	w.U16(h.Cost)
	w.I32(h.LongDescPtr)
	w.I32(h.LfnPtr)
	if len(w.B) < FilesHdrSize {
		w.Pad(FilesHdrSize - len(w.B))
	}
	if len(w.B) > FilesHdrSize {
		return w.B[:FilesHdrSize]
	}
	return w.B
}

func ParseFilesIdx(b []byte) FilesIdx {
	r := pascal.NewBuf(b)
	idx := FilesIdx{}
	idx.Name = r.PString(12)
	idx.UploadDate = r.U32()
	for i := 0; i < 5; i++ {
		idx.KeywordCRC[i] = r.I32()
	}
	idx.LongDescPtr = r.I32()
	return idx
}

func EncodeFilesIdx(idx FilesIdx) []byte {
	w := &pascal.Writer{}
	w.PString(12, idx.Name)
	w.U32(idx.UploadDate)
	for i := 0; i < 5; i++ {
		w.I32(idx.KeywordCRC[i])
	}
	w.I32(idx.LongDescPtr)
	if len(w.B) < FilesIdxSize {
		w.Pad(FilesIdxSize - len(w.B))
	}
	if len(w.B) > FilesIdxSize {
		return w.B[:FilesIdxSize]
	}
	return w.B
}

func HeaderToIdx(h FilesHdr) FilesIdx {
	return FilesIdx{Name: h.Name, UploadDate: h.UploadDate, LongDescPtr: h.LongDescPtr}
}

func ParseMessageArea(b []byte) MessageArea {
	r := pascal.NewBuf(b)
	m := MessageArea{}
	m.AreaNum = r.U16()
	r.U16()
	m.Name = r.PString(40)
	m.Typ = r.U8()
	m.MsgKinds = r.U8()
	m.Attribute = r.U8()
	m.DaysKill = r.U8()
	m.RecvKill = r.U8()
	m.CountKill = r.U16()
	m.ReadSecurity = r.U16()
	r.Skip(8)
	m.WriteSecurity = r.U16()
	r.Skip(8)
	m.SysopSecurity = r.U16()
	r.Skip(8)
	m.OriginLine = r.PString(60)
	m.AkaAddress = r.U8()
	m.Age = r.U8()
	m.JAMBase = r.PString(60)
	m.Group = r.U16()
	m.AltGroup[0] = r.U16()
	m.AltGroup[1] = r.U16()
	m.AltGroup[2] = r.U16()
	m.Attribute2 = r.U8()
	m.NetmailArea = r.U16()
	return m
}

func ParseEleMessage(b []byte) EleMessage {
	r := pascal.NewBuf(b)
	m := EleMessage{}
	m.AreaNum = r.I32()
	m.GroupName = r.PString(128)
	m.Attribute = r.U8()
	m.AccessSettings = r.U8()
	m.AttachArea = r.I32()
	return m
}

func ParseNewsServer(b []byte) NewsServer {
	r := pascal.NewBuf(b)
	n := NewsServer{}
	n.MaxSessions = r.I32()
	n.ServerPort = r.I32()
	n.Attribute = r.I32()
	n.DomainName = r.PString(120)
	if n.ServerPort <= 0 {
		n.ServerPort = 119
	}
	if n.MaxSessions <= 0 {
		n.MaxSessions = 50
	}
	if n.DomainName == "" {
		n.DomainName = "elebbs.bbs"
	}
	return n
}

func ParseGroup(b []byte) Group {
	r := pascal.NewBuf(b)
	g := Group{}
	g.AreaNum = r.U16()
	g.Name = r.PString(40)
	g.Security = r.U16()
	g.Flags = Flag(r.Flags())
	g.NotFlags = Flag(r.Flags())
	return g
}

func ParseLanguage(b []byte) Language {
	r := pascal.NewBuf(b)
	l := Language{}
	l.Name = r.PString(20)
	l.Attrib = r.U8()
	l.DefName = r.PString(60)
	l.MenuPath = r.PString(60)
	l.TextPath = r.PString(60)
	l.QuesPath = r.PString(60)
	l.Security = r.U16()
	l.Flags = Flag(r.Flags())
	l.NotFlags = Flag(r.Flags())
	return l
}

func ParseLimits(b []byte) Limits {
	r := pascal.NewBuf(b)
	l := Limits{}
	l.Security = r.U16()
	l.LTime = r.U16()
	r.Skip(22) // baud limits through L14400-ish
	r.Skip(8)
	l.LLocal = r.U16()
	return l
}

func ParseEleConfig(b []byte) EleConfig {
	r := pascal.NewBuf(b)
	e := EleConfig{}
	e.VersionID = r.U16()
	e.UtilityLog = r.PString(250)
	e.CapitalizeUsername = r.Bool()
	e.AttachPassword = r.PString(15)
	e.WebHTMLPath = r.PString(250)
	e.WebELMPath = r.PString(250)
	return e
}

func EncodeEleConfig(e EleConfig) []byte {
	w := &pascal.Writer{}
	w.U16(e.VersionID)
	w.PString(250, e.UtilityLog)
	if e.CapitalizeUsername {
		w.U8(1)
	} else {
		w.U8(0)
	}
	w.PString(15, e.AttachPassword)
	w.PString(250, e.WebHTMLPath)
	w.PString(250, e.WebELMPath)
	return w.B
}

func ParseModem(b []byte) Modem {
	r := pascal.NewBuf(b)
	m := Modem{}
	m.ComPort = r.U8()
	m.InitTries = r.U8()
	m.BufferSize = r.U16()
	m.ModemDelay = r.U16()
	m.MaxSpeed = r.U32()
	r.Bool() // SendBreak
	m.LockModem = r.Bool()
	r.Skip(2) // AnswerPhone, OffHook
	r.PString(70)
	r.PString(70)
	r.PString(70)
	for i := 0; i < 12; i++ {
		r.PString(40)
	}
	r.PString(20)
	r.PString(20)
	m.ErrorFreeString = r.PString(15)
	return m
}

func ParseTelnet(b []byte) TelnetCfg {
	r := pascal.NewBuf(b)
	t := TelnetCfg{}
	t.MaxSessions = r.I32()
	t.ServerPort = r.I32()
	t.StartNodeWith = r.I32()
	t.Attrib = r.I32()
	t.ProgramPath = r.PString(255)
	t.NodeDirectories = r.PString(255)
	if t.ServerPort == 0 {
		t.ServerPort = 23
	}
	if t.MaxSessions == 0 {
		t.MaxSessions = 10
	}
	if t.StartNodeWith == 0 {
		t.StartNodeWith = 1
	}
	return t
}

func EncodeTelnet(t TelnetCfg) []byte {
	w := &pascal.Writer{}
	w.I32(t.MaxSessions)
	w.I32(t.ServerPort)
	w.I32(t.StartNodeWith)
	w.I32(t.Attrib)
	w.PString(255, t.ProgramPath)
	w.PString(255, t.NodeDirectories)
	w.Pad(20 * 4)
	return w.B
}
