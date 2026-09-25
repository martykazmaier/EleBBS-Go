package cfgrec

import "strings"

const (
	VersionMajor = 0
	VersionMinor = 10
	VersionID    = "0.11.b1-go"
	FullProgName = "EleBBS/W32"
	PidName      = "EleBBS/W32 v0.11.b1-go"
	PlatformID   = "W32"
	VersIDWord   = 0x250
	EleVersID    = 0x010

	MaxNodes     = 255
	MaxNestMenus = 49
	DefaultMenu  = "top"

	UserBaseName     = "users.bbs"
	UserBaseIdxName  = "usersidx.bbs"
	UserBaseXiName   = "usersxi.bbs"
	UserBaseLastRead = "lastread.bbs"

	// DefaultTransferProto is the PROTOCOL.RA hotkey stored on new user records.
	DefaultTransferProto byte = '@'

	// UsersSize is SizeOf(USERSrecord) with PACKRECORDS 1 (struct.250).
	// It is 1016, not 1024; a 1024 stride misses every name after record 0
	// and a 1024-byte append overwrites the next user.
	UsersSize    = 1016
	UsersIdxSize = 8
	FilesHdrSize = 194
	FilesIdxSize = 41
	FilesRecSize = 168
	MessageSize  = 224
	GroupSize    = 153
	MenuSize     = 464
	// LightBarSize is SizeOf(LightbarRecord) PACKRECORDS 1 (struct.250).
	LightBarSize = 375
	ModemSize    = 1393
	LimitsSize   = 128
	// LanguageSize is SizeOf(LANGUAGErecord) with PACKRECORDS 1 (struct.250).
	LanguageSize = 466
	// LanguageSizeRA is the older RA-sized LANGUAGE.RA record some sites still use.
	LanguageSizeRA = 406

	User3MailScan = 1 << 0
	TagRecSize    = 50
	SysInfoSize   = 168
	TimeLogSize   = 71
	EventSize     = 19
	UsersXiSize   = 200
	// ExitinfoSize is SizeOf(EXITINFOrecord) PACKRECORDS 1 (struct.250).
	ExitinfoSize   = 2371
	LastCallSize   = 118
	LastCallSizeRA = 81
	// ProtocolSize is SizeOf(PROTOCOLrecord) PACKRECORDS 1 (struct.250).
	ProtocolSize = 550
	// ProtocolSizeRA is an older RA protocol.ra stride some sites still use.
	ProtocolSizeRA  = 454
	EleMessageSize  = 287
	NewsServerSize  = 289
	NwsPostNever    = 0
	NwsPostAlways   = 1
	NwsPostSettings = 2
	NwsUseBoth      = 3
	EleMsgUsenet    = 1 << 0

	AttrDeleted  = 1 << 0
	AttrUnlisted = 1 << 1
	AttrFree     = 1 << 2
	AttrNotAvail = 1 << 3
	AttrLocked   = 1 << 4
	AttrMissing  = 1 << 5
	AttrNoTime   = 1 << 6

	UserDeleted = 1 << 0
	UserClrScr  = 1 << 1
	UserMore    = 1 << 2
	UserANSI    = 1 << 3
	UserNoKill  = 1 << 4
	UserXferPri = 1 << 5
	UserFSEd    = 1 << 6
	UserQuiet   = 1 << 7

	User2HotKeys  = 1 << 0
	User2Avatar   = 1 << 1
	User2FSView   = 1 << 2
	User2Hidden   = 1 << 3
	User2PagePri  = 1 << 4
	User2NoEcho   = 1 << 5
	User2Guest    = 1 << 6
	User2PostBill = 1 << 7

	AskYes  = 0
	AskNo   = 1
	AskAsk  = 2
	AskOnly = 3

	MsgLocal    = 0
	MsgNetMail  = 1
	MsgEchoMail = 2
	MsgInternet = 3
	MsgNews     = 4
	MsgForum    = 5

	MsgKindBoth    = 0
	MsgKindPrivate = 1
	MsgKindPublic  = 2
	MsgKindROnly   = 3
	MsgKindNoReply = 4

	SystemMsgPrefix = "* "
)

type Flag [4]byte

func (f Flag) Has(bit int) bool {
	if bit < 0 || bit > 31 {
		return false
	}
	return f[bit/8]&(1<<uint(bit%8)) != 0
}

func flagColRow(letter, digit byte) (col, row int, ok bool) {
	if letter >= 'a' && letter <= 'z' {
		letter -= 32
	}
	if letter < 'A' || letter > 'D' {
		return 0, 0, false
	}
	if digit < '1' || digit > '8' {
		return 0, 0, false
	}
	return int(letter - 'A'), int(digit - '1'), true
}

// SetNamed is Pascal RaSetFlags / RaResetFlags: "B2" is bit 1 of flags[1].
func (f *Flag) SetNamed(which string, on bool) {
	if f == nil {
		return
	}
	for i := 0; i+1 < len(which); i += 2 {
		col, row, ok := flagColRow(which[i], which[i+1])
		if !ok {
			continue
		}
		bit := byte(1 << uint(row))
		if on {
			f[col] |= bit
		} else {
			f[col] &^= bit
		}
	}
}

// HasNamed is Pascal RaCheckFlags: every A1-style pair in which must be on.
func (f Flag) HasNamed(which string) bool {
	for i := 0; i+1 < len(which); i += 2 {
		col, row, ok := flagColRow(which[i], which[i+1])
		if !ok {
			continue
		}
		if f[col]&(1<<uint(row)) == 0 {
			return false
		}
	}
	return true
}

func (f *Flag) ToggleNamed(which string) {
	if f == nil {
		return
	}
	for i := 0; i+1 < len(which); i += 2 {
		col, row, ok := flagColRow(which[i], which[i+1])
		if !ok {
			continue
		}
		bit := byte(1 << uint(row))
		f[col] ^= bit
	}
}

// ChangeMenu is Pascal MenuChangeFlags: tokens like "B2+" "A1-" "C3*".
func (f *Flag) ChangeMenu(misc string) {
	if f == nil {
		return
	}
	s := strings.TrimSpace(misc)
	for s != "" {
		if len(s) < 3 {
			break
		}
		tok := s[:3]
		if len(s) > 4 {
			s = strings.TrimLeft(s[4:], " ")
		} else {
			s = ""
		}
		name := tok[:2]
		switch tok[2] {
		case '+':
			f.SetNamed(name, true)
		case '-':
			f.SetNamed(name, false)
		case '*':
			f.ToggleNamed(name)
		}
	}
}

type Addr struct {
	Zone, Net, Node, Point uint16
}

type Config struct {
	VersionID      uint16
	ProductID      byte
	MinimumBaud    uint16
	MenuPath       string
	TextPath       string
	AttachPath     string
	Nodelist       string
	MsgBasePath    string
	SysPath        string
	ExternalEd     string
	Address        [10]Addr
	SystemName     string
	NewSecurity    uint16
	NewCredit      uint16
	NewFlags       Flag
	OriginLine     string
	QuoteString    string
	Sysop          string
	LogFileName    string
	FastLogon      bool
	// StrictPwdChecking makes logon passwords case-sensitive and rejects
	// trivial new passwords (pwdtrash.ctl, parts of the user's name).
	StrictPwdChecking bool
	PwdBoard          byte // WatchDog area: warn the user of bad password attempts
	BadPwdArea        byte // area for the "leave a message to the sysop" comment
	OneWord        bool
	CheckMail      byte
	ANSI           byte
	ClearScreen    byte
	MorePrompt     byte
	NormFore       byte
	NormBack       byte
	// Local screen colours: sysop windows (user editor, chat, password box).
	HiFore, WindFore, WindBack byte
	BorderFore, BorderBack     byte
	FKeys                      [10]string
	FreezeChat                 bool   // stop the user's clock while chatting
	ChatCommand                string // external chat program instead of the built-in chat
	AutoChatCapture            bool   // log sysop chats to CHAT<node>.LOG
	LimitLocal                 bool   // node-window sysop keys disabled
	SavePasswords              bool
	LogonPrompt    string
	Location       string
	FileBase       string
	EchoChar       string
	HotKeys        byte
	AVATAR         byte
	PasswordTries  uint16
	LogonTime      uint16
	UserTimeOut    uint16
	MinPwdLen      byte
	MultiLine      bool
	AskHandle      bool
	AskBirthDate   bool
	NewUserGroup   byte
	FileLine       string
	LangHdr        string
	LanguagePrompt string
	NewUserLang    byte
	EMSIEnable     byte
	EMSINewUser    bool
	KeyboardPwd    string
	CapLocation    bool
	SemPath        string
	LeftBracket    byte
	RightBracket   byte
	PageLength     uint16
	Raw            []byte
	Off            CfgOff
}

// CfgOff holds byte offsets into CONFIG.RA for fields ELCONFIG can save.
type CfgOff struct {
	SystemName    int
	Sysop         int
	Location      int
	LogonPrompt   int
	KeyboardPwd   int
	MenuPath      int
	TextPath      int
	MsgBasePath   int
	SysPath       int
	FileBase      int
	NewSecurity   int
	PasswordTries int
	MinPwdLen     int
	ANSI          int
	OneWord       int
	PageLength    int
	AskHandle     int
}

// HasKeyboardPwd is true only when CONFIG.RA actually has a keyboard password.
// Pascal AskForPassword exits immediately when S is empty. Misaligned CONFIG.RA
// parses often yield path fragments or binary junk, which must not count.
func HasKeyboardPwd(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 15 {
		return false
	}
	letters := 0
	for _, r := range s {
		if r < 33 || r > 126 {
			return false
		}
		switch r {
		case '\\', '/', ':', '<', '>', '|', '*', '?', '"':
			return false
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			letters++
		}
	}
	return letters > 0
}

type EleConfig struct {
	VersionID          uint16
	UtilityLog         string
	CapitalizeUsername bool
	AttachPassword     string
	WebHTMLPath        string
	WebELMPath         string
}

type Modem struct {
	ComPort         byte
	InitTries       byte
	BufferSize      uint16
	ModemDelay      uint16
	MaxSpeed        uint32
	LockModem       bool
	ErrorFreeString string
}

type TelnetCfg struct {
	MaxSessions     int32
	ServerPort      int32
	StartNodeWith   int32
	Attrib          int32
	ProgramPath     string
	NodeDirectories string
}

type NewsServer struct {
	MaxSessions int32
	ServerPort  int32
	Attribute   int32
	DomainName  string
}

func (n NewsServer) AuthForList() bool { return n.Attribute&(1<<3) != 0 }

type EleMessage struct {
	AreaNum        int32
	GroupName      string
	Attribute      byte
	AccessSettings byte
	AttachArea     int32
}

func (e EleMessage) Usenet() bool { return e.Attribute&EleMsgUsenet != 0 }

func (e EleMessage) CanPost() bool {
	return e.AccessSettings == NwsPostAlways || e.AccessSettings == NwsPostSettings || e.AccessSettings == NwsUseBoth
}

type UsersIdx struct {
	NameCRC32   int32
	HandleCRC32 int32
}

type User struct {
	Name          string
	Location      string
	Organisation  string
	Address1      string
	Address2      string
	Address3      string
	Handle        string
	Comment       string
	PasswordCRC   int32
	DataPhone     string
	VoicePhone    string
	LastTime      string
	LastDate      string
	Attribute     byte
	Attribute2    byte
	Flags         Flag
	Credit        int32
	Pending       int32
	MsgsPosted    uint16
	Security      uint16
	LastRead      int32
	NoCalls       int32
	Uploads       int32
	Downloads     int32
	UploadsK      int32
	DownloadsK    int32
	TodayK        int32
	Elapsed       int16
	ScreenLength  uint16
	LastPwdChange byte
	Group         uint16
	Combined      [200]uint16
	FirstDate     string
	BirthDate     string
	SubDate       string
	ScreenWidth   byte
	Language      byte
	DateFormat    byte
	ForwardTo     string
	MsgArea       uint16
	FileArea      uint16
	DefaultProto  byte
	FileGroup     uint16
	LastDOBCheck  byte
	Sex           byte
	XIRecord      int32
	MsgGroup      uint16
	Attribute3    byte
	Password      string
	Record        int
}

func (u User) Deleted() bool { return u.Attribute&UserDeleted != 0 }
func (u User) ANSI() bool    { return u.Attribute&UserANSI != 0 }
func (u User) More() bool    { return u.Attribute&UserMore != 0 }
func (u User) Guest() bool   { return u.Attribute2&User2Guest != 0 }
func (u User) HotKeys() bool { return u.Attribute2&User2HotKeys != 0 }

type MenuItem struct {
	Typ        byte
	Security   uint16
	MaxSec     uint16
	NotFlags   Flag
	Flags      Flag
	TimeLeft   uint16
	TimeUsed   uint16
	Age        byte
	TermAttrib byte
	MinSpeed   int32
	MaxSpeed   int32
	Credit     int32
	OptionCost int32
	PerMinCost int32
	Node       [32]byte
	Group      [32]byte
	StartTime  [7]uint16
	StopTime   [7]uint16
	Display    string
	HotKey     string
	MiscData   string
	Foreground byte
	Background byte
}

// LightBar is LightbarRecord: one .mlb record aligned with one .mnu item.
type LightBar struct {
	LightX     byte
	LightY     byte
	LowItem    string
	SelectItem string
	Attrib     byte
}

func (l LightBar) Enabled() bool { return l.Attrib&1 != 0 }

type Protocol struct {
	Name         string
	ActiveKey    byte
	OpusType     bool
	Batch        bool
	Attribute    byte
	LogFileName  string
	CtlFileName  string
	DnCmdString  string
	DnCtlString  string
	UpCmdString  string
	UpCtlString  string
	UpLogKeyWord string
	DnLogKeyWord string
	XferDescWord byte
	XferNameWord byte
}

func ProtocolRecSize(fileLen int) int {
	if fileLen <= 0 {
		return ProtocolSize
	}
	for _, sz := range []int{ProtocolSize, ProtocolSizeRA} {
		if fileLen%sz == 0 {
			return sz
		}
	}
	return ProtocolSize
}

type FilesArea struct {
	AreaNum        uint16
	Name           string
	Attrib         byte
	FilePath       string
	KillDaysDL     uint16
	KillDaysFD     uint16
	Password       string
	MoveArea       uint16
	Age            byte
	ConvertExt     byte
	Group          uint16
	Attrib2        byte
	DefCost        uint16
	UploadArea     uint16
	UploadSecurity uint16
	Security       uint16
	ListSecurity   uint16
	AltGroup       [3]uint16
	Device         byte
}

type FilesHdr struct {
	Name        string
	NameRaw     [13]byte
	Size        uint32
	CRC32       uint32
	Uploader    string
	UploadDate  uint32
	FileDate    uint32
	LastDL      uint32
	TimesDL     uint16
	Attrib      byte
	Password    string
	Keywords    [5]string
	Cost        uint16
	LongDescPtr int32
	LfnPtr      int32
	RecordNum   uint16
}

func (h FilesHdr) Deleted() bool  { return h.Attrib&AttrDeleted != 0 }
func (h FilesHdr) Unlisted() bool { return h.Attrib&AttrUnlisted != 0 }
func (h FilesHdr) Missing() bool  { return h.Attrib&AttrMissing != 0 }
func (h FilesHdr) Locked() bool   { return h.Attrib&AttrLocked != 0 }
func (h FilesHdr) NotAvail() bool { return h.Attrib&AttrNotAvail != 0 }
func (h FilesHdr) Comment() bool  { return h.NameRaw[0] == 0 }

type FilesIdx struct {
	Name        string
	UploadDate  uint32
	KeywordCRC  [5]int32
	LongDescPtr int32
}

type MessageArea struct {
	AreaNum       uint16
	Name          string
	Typ           byte
	MsgKinds      byte
	Attribute     byte
	DaysKill      byte
	RecvKill      byte
	CountKill     uint16
	ReadSecurity  uint16
	WriteSecurity uint16
	SysopSecurity uint16
	OriginLine    string
	AkaAddress    byte
	Age           byte
	JAMBase       string
	Group         uint16
	AltGroup      [3]uint16
	Attribute2    byte
	NetmailArea   uint16
}

func (m MessageArea) IsJAM() bool { return m.Attribute&(1<<7) != 0 }

// AllowsAttach is MESSAGES.RA Attribute bit 2 (file attaches).
func (m MessageArea) AllowsAttach() bool { return m.Attribute&(1<<2) != 0 }

type Group struct {
	AreaNum  uint16
	Name     string
	Security uint16
	Flags    Flag
	NotFlags Flag
}

type Language struct {
	Name     string
	Attrib   byte
	DefName  string
	MenuPath string
	TextPath string
	QuesPath string
	Security uint16
	Flags    Flag
	NotFlags Flag
}

type Limits struct {
	Security uint16
	LTime    uint16
	LLocal   uint16
}

type LineCfg struct {
	RaNodeNr         int
	LocalLogon       bool
	ConnectStr       string
	Baud             uint16
	CarrierCheck     bool
	ReLogOnBBS       bool
	ReLogMenu        bool
	Snooping         bool
	ComNoClose       bool
	DoMonitor        bool
	TelnetFromIP     string
	TelnetServ       bool
	InheritedHandle  uintptr
	AutoUser         string
	AutoPass         string
	BatchAtExit      string
	MailerCmdLine    string
	PrinterLogging   bool
	LoggedOn         bool
	AnsiOn           bool
	AvatarOn         bool
	RipOn            bool
	GuestUser        bool
	DispMorePrompt   bool
	TextfileShells   bool
	TimeFrozen       bool
	NetMailEntered   bool // Pascal Exitinfo: netmail posted (errorlevel 3)
	EchoMailEntered  bool // Pascal Exitinfo: echomail posted (errorlevel 4)
	UserRecord       int
	TimeLimit        uint16
	EventDeducted    uint16
	MenuStack        [50]string
	MenuStackPtr     int
	Modem            Modem
	Telnet           TelnetCfg
	User             User
	Language         Language
	Limits           Limits
	SysInfo          SysInfo
	LoginTime        string
	LoginDate        string
	ErrorFreeConnect bool
	EmsiSession      bool
	EmsiUser         IEMSIUser
}

// IEMSIUser is Pascal IEMSI_USER_Record (fields from the client ICI packet).
type IEMSIUser struct {
	Name         string
	Alias        string
	Location     string
	DataNr       string
	VoiceNr      string
	Password     string
	BirthDate    string
	CrtDef       string
	Protocols    string
	Capabilities string
	Requests     string
	Software     string
	XlatTable    string
}

type SysInfo struct {
	TotalCalls int32
	LastCaller string
	LastHandle string
}

type GlobalCfg struct {
	RaConfig Config
	ElConfig EleConfig
	CfgPath  string
	ElePath  string
}
