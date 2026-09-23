package logon

import (
	"fmt"
	"os"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/crc"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
	"elebbs/internal/quest"
	"elebbs/internal/term"
	"elebbs/internal/userbase"
)

func Perform(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) bool {
	ral := lang.Load(g, line.Language)
	t.Ral = ral
	tries := int(g.RaConfig.PasswordTries)
	if tries <= 0 {
		tries = 3
	}
	line.AnsiOn = true
	line.DispMorePrompt = false
	t.ResetLines(0)

	if line.AutoUser == "" {
		line.AutoUser = os.Getenv("ELEBBS_AUTOUSER")
		line.AutoPass = os.Getenv("ELEBBS_AUTOPASS")
	}
	if line.AutoUser != "" {
		u, ok := userbase.Search(g, line.AutoUser)
		if !ok || (line.AutoPass != "" && !userbase.CheckPassword(u, line.AutoPass)) {
			t.Println("Login rejected.")
			return false
		}
		if line.TelnetFromIP != "" {
			logx.Write(g, line.RaNodeNr, '>', u.Name+" on-line using ssh ("+line.TelnetFromIP+")")
		}
		return finishLogon(t, g, line, u)
	}

	t.Println("")
	startIEMSISession(t, g, line)
	// IRQ is CR-terminated on this line; overwrite it so the user never sees
	// **EMSI_IRQ and the name prompt stays at column 1 (Pascal `X1:`).
	t.WriteRaw([]byte("\r\x1b[K"))
	t.WriteRA("`X1:")
	t.Println(cfgrec.PidName)

	for {
		// Pascal: UserRecord is -1 until SearchUser hits. Q-A SETFLAG must
		// not Write record 0 (SysOp) while the name is being collected.
		line.LoggedOn = false
		line.UserRecord = -1
		line.User.Record = -1
		name, psw, err := getUserName(t, g, ral)
		if err != nil {
			return false
		}
		if name == "" {
			continue
		}

		t.Println("")
		t.WriteRA(ral.Get(lang.ScanUsers))

		u, ok := userbase.Search(g, name)
		if ok {
			line.User = u
			config.ApplyUserLanguage(g, line)
			ral = lang.Load(g, line.Language)
			t.Ral = ral
			if line.Baud > 0 && !allowSysopRemote(g) {
				if foldEq(u.Name, g.RaConfig.Sysop) || foldEq(u.Handle, g.RaConfig.Sysop) {
					t.Println("")
					t.WriteRA(ral.Get(lang.NoSysop))
					t.Println("")
					return false
				}
			}
			if line.Baud == 65529 && line.TelnetFromIP != "" {
				logx.Write(g, line.RaNodeNr, '>', u.Name+" on-line using telnet ("+line.TelnetFromIP+")")
			} else if line.Baud > 0 {
				logx.Write(g, line.RaNodeNr, '>', fmt.Sprintf("%s on-line at %d BPS", u.Name, line.Baud))
			} else {
				logx.Write(g, line.RaNodeNr, '>', u.Name+" on-line")
			}
			t.Println("")
			if !getPassword(t, g, line, ral, &u, psw, tries) {
				return false
			}
			return finishLogon(t, g, line, u)
		}

		if !g.RaConfig.OneWord && !strings.Contains(pascal.Trim(name), " ") {
			t.Println("")
			t.Println("")
			t.WriteRA(ral.Get(lang.BothNames))
			continue
		}

		logx.Write(g, line.RaNodeNr, '!', "Name not in user file")
		if n, p := userbase.Count(g); n >= 0 {
			logx.Write(g, line.RaNodeNr, '!', fmt.Sprintf("Search %q missed %s (%d users)", name, p, n))
		}
		if !newUser(t, g, line, ral, name) {
			continue
		}
		return true
	}
}

func allowSysopRemote(g *cfgrec.GlobalCfg) bool {
	return true
}

func foldEq(a, b string) bool {
	return pascal.UpCase(pascal.Trim(a)) == pascal.UpCase(pascal.Trim(b)) && pascal.Trim(a) != ""
}

func getUserName(t *term.IO, g *cfgrec.GlobalCfg, ral *lang.File) (name, psw string, err error) {
	_ = ral
	prompt := g.RaConfig.LogonPrompt
	if prompt == "" {
		prompt = "Please enter your full name: "
	}
	iemsi := iemsiEnabled(g, t.Line)
	for {
		t.WriteRA("`A" + attr(g) + ":")
		t.WriteRA(prompt)
		var s string
		if iemsi {
			s, err = t.GetStringIEMSI(35, true)
		} else {
			s, err = t.GetString(35, false, true)
		}
		if err != nil {
			return "", "", err
		}
		if isIEMSIProbe(s) {
			if !getIEMSIUser(t, g, t.Line) {
				continue
			}
			s = t.Line.EmsiUser.Name
			psw = t.Line.EmsiUser.Password
			s = pascal.NoDoubleSpace(pascal.Trim(s))
			if isIEMSIProbe(s) {
				s = ""
			}
			if s == "" {
				continue
			}
			return s, psw, nil
		}
		s = pascal.NoDoubleSpace(pascal.Trim(s))
		if i := strings.IndexByte(s, ';'); i >= 0 {
			psw = s[i+1:]
			s = pascal.Trim(s[:i])
		}
		if s != "" {
			return s, psw, nil
		}
	}
}

func getPassword(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, ral *lang.File, u *cfgrec.User, preset string, tries int) bool {
	if crc.RA("", true) == u.PasswordCRC && u.Password == "" {
		return true
	}
	pw := preset
	if line.EmsiSession && line.EmsiUser.Password != "" {
		pw = line.EmsiUser.Password
	}
	for n := 1; n <= tries; n++ {
		if pw == "" {
			t.WriteRA("`A" + attr(g) + ":")
			t.WriteRA(ral.Get(lang.Password))
			var err error
			pw, err = t.GetString(15, true, true)
			if err != nil {
				return false
			}
		}
		t.WriteRA("`A7:")
		if userbase.CheckPassword(*u, pw) {
			return true
		}
		pw = ""
		t.Println("")
		t.WriteRA(ral.Get(lang.IncPsw))
		t.Println("")
		logx.Write(g, line.RaNodeNr, '!', "Bad password for "+u.Name)
	}
	t.WriteRA(ral.Get(lang.NoAccess))
	t.Println("")
	term.DisplayHotFile(t, g.RaConfig.TextPath, "BADPWD")
	return false
}

func writeRal(t *term.IO, ral *lang.File, nr int) {
	t.WriteRA(ral.Get(nr))
}

func writeRalLn(t *term.IO, ral *lang.File, nr int) {
	t.WriteRA(ral.Get(nr) + "\r\n")
}

func ralYesNo(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, ral *lang.File, nr int) bool {
	_ = g
	_ = line
	save := t.Ral
	t.Ral = ral
	defer func() { t.Ral = save }()
	return t.AskYesNo(nr, false)
}

func askLanguage(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) {
	langs := config.ListLanguages(g)
	if len(langs) == 0 {
		return
	}
	prompt := g.RaConfig.LanguagePrompt
	if prompt == "" {
		prompt = "Select language: "
	}
	t.Println("")
	for i, l := range langs {
		t.Println(fmt.Sprintf("  %2d  %s", i+1, l.Name))
	}
	t.WriteRA("`A15:" + prompt)
	s, err := t.GetString(3, false, false)
	if err != nil {
		return
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if n < 1 || n > len(langs) {
		if g.RaConfig.NewUserLang > 0 {
			n = int(g.RaConfig.NewUserLang)
		} else {
			n = 1
		}
	}
	if n < 1 || n > len(langs) {
		n = 1
	}
	line.User.Language = byte(n)
	config.ApplyUserLanguage(g, line)
	t.Ral = lang.Load(g, line.Language)
}

func newUser(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, ral *lang.File, name string) bool {
	wasPause := t.NoPause()
	t.SetNoPause(true)
	line.DispMorePrompt = false
	t.MorePrompt = false
	t.ResetLines(0)
	defer func() {
		t.SetNoPause(wasPause)
	}()

	term.DisplayHotFile(t, g.RaConfig.TextPath, "NOTFOUND")
	t.Println("")
	writeRalLn(t, ral, lang.NotFound1)
	t.Println("")
	t.WriteRA(ral.Get(lang.NmEntered))
	t.Print(name)
	t.Println(".")
	t.Println("")
	if !ralYesNo(t, g, line, ral, lang.AskNmOk) {
		return false
	}
	if g.RaConfig.NewSecurity == 0 {
		term.DisplayHotFile(t, g.RaConfig.TextPath, "PRIVATE")
		writeRalLn(t, ral, lang.PrivSys)
		return false
	}

	u := userbase.NewDefaults(g)
	u.Name = name
	u.Handle = name
	iemsiNU := iemsiNewUser(g, line)
	if iemsiNU {
		applyIEMSINewUser(&u, line.EmsiUser)
	}
	line.User = u
	line.AnsiOn = u.Attribute&cfgrec.UserANSI != 0

	if g.RaConfig.NewUserLang != 0 {
		askLanguage(t, g, line)
		ral = t.Ral
		if ral == nil {
			ral = lang.Load(g, line.Language)
			t.Ral = ral
		}
	}

	term.DisplayHotFile(t, g.RaConfig.TextPath, "NEWUSER1")
	t.Println("")
	t.Println("")

	if g.RaConfig.ANSI == cfgrec.AskAsk && !iemsiNU {
		if ralYesNo(t, g, line, ral, lang.AskAnsi) {
			u.Attribute |= cfgrec.UserANSI
		} else {
			u.Attribute &^= cfgrec.UserANSI
		}
		line.AnsiOn = u.Attribute&cfgrec.UserANSI != 0
		t.Println("")
	}
	if g.RaConfig.AVATAR == cfgrec.AskAsk && !iemsiNU {
		if ralYesNo(t, g, line, ral, lang.AskAvt) {
			u.Attribute2 |= cfgrec.User2Avatar
		} else {
			u.Attribute2 &^= cfgrec.User2Avatar
		}
		t.Println("")
	}
	t.WriteRA(ral.Get(lang.AskLines))
	ls, err := t.GetString(2, false, false)
	if err != nil {
		return false
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(ls), "%d", &n)
	if n < 10 || n > 66 {
		n = 24
	}
	u.ScreenLength = uint16(n)
	t.Length = n
	t.Println("")

	if g.RaConfig.MorePrompt == cfgrec.AskAsk && !iemsiNU {
		if ralYesNo(t, g, line, ral, lang.AskPause) {
			u.Attribute |= cfgrec.UserMore
		} else {
			u.Attribute &^= cfgrec.UserMore
		}
		t.Println("")
	}
	if g.RaConfig.ClearScreen == cfgrec.AskAsk && !iemsiNU {
		if ralYesNo(t, g, line, ral, lang.AskClr) {
			u.Attribute |= cfgrec.UserClrScr
		} else {
			u.Attribute &^= cfgrec.UserClrScr
		}
		t.Println("")
	}

	if !iemsiNU || pascal.Trim(u.Location) == "" {
		t.WriteRA("`F14:`B0:")
		for {
			writeRal(t, ral, lang.AskLoc)
			loc, err := t.GetString(25, false, g.RaConfig.CapLocation)
			if err != nil {
				return false
			}
			loc = pascal.Trim(loc)
			t.Println("")
			if loc != "" {
				u.Location = loc
				break
			}
			t.WriteRA("`F14:`B0:")
		}
	}

	if pascal.Trim(u.Handle) == "" {
		u.Handle = name
	}
	if g.RaConfig.AskHandle && !iemsiNU {
		t.WriteRA("`A9:")
		writeRal(t, ral, lang.AskHandle)
		h, err := t.GetString(35, false, true)
		if err != nil {
			return false
		}
		h = pascal.Trim(h)
		if h == "" {
			u.Handle = name
		} else {
			u.Handle = h
		}
		t.Println("")
	}
	if g.RaConfig.AskBirthDate && (!iemsiNU || pascal.Trim(u.BirthDate) == "") {
		t.WriteRA("`A3:")
		writeRal(t, ral, lang.AskBirth)
		t.Print(" ")
		bd, err := t.GetString(10, false, false)
		if err != nil {
			return false
		}
		u.BirthDate = pascal.Trim(bd)
		t.Println("")
	}

	minLen := int(g.RaConfig.MinPwdLen)
	if minLen < 1 {
		minLen = 4
	}
	var pw string
	if iemsiNU && pascal.Trim(line.EmsiUser.Password) != "" {
		setIEMSIPassword(&u, line.EmsiUser)
		pw = line.EmsiUser.Password
	} else {
		for {
			t.WriteRA("`A14:")
			writeRal(t, ral, lang.AskPsw1)
			pw, err = t.GetString(15, true, true)
			if err != nil {
				return false
			}
			if len(strings.TrimSpace(pw)) < minLen {
				writeRalLn(t, ral, lang.PswShort1)
				continue
			}
			t.WriteRA("`A14:")
			writeRal(t, ral, lang.AskPsw2)
			pw2, err := t.GetString(15, true, true)
			if err != nil {
				return false
			}
			if pascal.UpCase(pw) != pascal.UpCase(pw2) {
				writeRalLn(t, ral, lang.InvPsw1)
				continue
			}
			break
		}
		u.Password = pascal.UpCase(pascal.Trim(pw))
		u.PasswordCRC = crc.RA(pw, true)
	}
	u.DefaultProto = cfgrec.DefaultTransferProto
	u.Record = -1
	line.User = u
	line.LoggedOn = false
	line.UserRecord = -1

	term.DisplayHotFile(t, g.RaConfig.TextPath, "NEWUSER2")
	if quest.Kind(g, line, "NEWUSER") == "q-a" {
		quest.Run(t, g, line, "NEWUSER", "")
		u = line.User
		u.Name = name
		if pascal.Trim(u.Handle) == "" {
			u.Handle = name
		}
		u.Password = pascal.UpCase(pascal.Trim(pw))
		u.PasswordCRC = crc.RA(pw, true)
		u.Record = -1
	}

	nu, err := userbase.Append(g, u)
	if err != nil {
		t.Println("Could not create user: " + err.Error())
		logx.Write(g, line.RaNodeNr, '!', "New user failed: "+err.Error())
		return false
	}
	logx.Write(g, line.RaNodeNr, '+', fmt.Sprintf("New user %s, protocol %c", nu.Name, nu.DefaultProto))
	t.SetNoPause(wasPause)
	t.MorePrompt = true
	line.DispMorePrompt = false
	return finishLogon(t, g, line, nu)
}

func finishLogon(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, u cfgrec.User) bool {
	line.User = u
	line.UserRecord = u.Record
	line.LoggedOn = true
	line.AnsiOn = u.ANSI() || true
	line.AvatarOn = u.Attribute2&cfgrec.User2Avatar != 0
	line.GuestUser = u.Guest()
	line.DispMorePrompt = u.More()
	config.ApplyUserLanguage(g, line)
	t.Ral = lang.Load(g, line.Language)
	if u.ScreenLength > 0 {
		t.Length = int(u.ScreenLength)
	}
	if line.Limits.LTime == 0 || line.Limits.LTime == 32767 {
		line.TimeLimit = 32767
	} else {
		used := int(u.Elapsed)
		if used < 0 {
			used = 0
		}
		left := int(line.Limits.LTime) - used
		if left < 0 {
			left = 0
		}
		line.TimeLimit = uint16(left)
	}
	now := time.Now()
	prevDate, prevTime := u.LastDate, u.LastTime
	u.LastTime = now.Format("15:04")
	u.LastDate = now.Format("01-02-06")
	line.LoginTime = u.LastTime
	line.LoginDate = u.LastDate
	line.ErrorFreeConnect = !line.LocalLogon
	u.NoCalls++
	line.User = u
	config.BumpSysInfoCalls(g, line)
	_ = userbase.Write(g, u)
	_ = online.Write(g, line, "", online.StatusBrowsing)
	logx.Write(g, line.RaNodeNr, '>', fmt.Sprintf("%s logged on, security %d", u.Name, u.Security))
	t.DrainLineEnds()
	t.ClearScreen()
	t.ResetLines(1)
	showWelcomeFiles(t, g, line, prevDate, prevTime)
	return true
}

func showWelcomeFiles(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, prevDate, prevTime string) {
	tp := g.RaConfig.TextPath
	term.DisplayHotFile(t, tp, "WELCOME")
	term.DisplayHotFile(t, tp, "WELCOME1")
	if line.User.Security > 0 {
		term.DisplayHotFile(t, tp, fmt.Sprintf("SEC%d", line.User.Security))
	}
	if onceOnlyNewer(t, tp, prevDate, prevTime) {
		term.DisplayHotFile(t, tp, "ONCEONLY")
	}
	now := time.Now()
	term.DisplayHotFile(t, tp, fmt.Sprintf("TIME%02d", now.Hour()))
	term.DisplayHotFile(t, tp, "NEWS")
	if birthdayToday(line.User.BirthDate, now) {
		term.DisplayHotFile(t, tp, "BIRTHDAY")
	}
	term.DisplayHotFile(t, tp, fmt.Sprintf("%02d-%02d", int(now.Month()), now.Day()))
}

// onceOnlyNewer is Pascal WelcomeInformation: show ONCEONLY when the file
// is newer than the user's previous logon (LastDate/LastTime before this session).
func onceOnlyNewer(t *term.IO, textPath, lastDate, lastTime string) bool {
	p, ok := term.FindTextFile(t, textPath, "ONCEONLY")
	if !ok {
		return false
	}
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return st.ModTime().After(parseRADateTime(lastDate, lastTime))
}

func parseRADateTime(date, tm string) time.Time {
	date = pascal.Trim(date)
	tm = pascal.Trim(tm)
	if len(date) < 8 {
		return time.Time{}
	}
	mon, day, yr := 0, 0, 0
	fmt.Sscanf(date, "%d-%d-%d", &mon, &day, &yr)
	if yr < 80 {
		yr += 2000
	} else if yr < 100 {
		yr += 1900
	}
	hh, mm := 0, 0
	if tm != "" {
		fmt.Sscanf(tm, "%d:%d", &hh, &mm)
	}
	if mon < 1 || day < 1 {
		return time.Time{}
	}
	return time.Date(yr, time.Month(mon), day, hh, mm, 0, 0, time.Local)
}

func birthdayToday(birth string, now time.Time) bool {
	birth = pascal.Trim(birth)
	if len(birth) < 5 {
		return false
	}
	return birth[:5] == now.Format("01-02")
}

func attr(g *cfgrec.GlobalCfg) string {
	a := (g.RaConfig.NormBack << 4) | (g.RaConfig.NormFore & 0x0F)
	if a == 0 {
		a = 7
	}
	return fmt.Sprintf("%d", a)
}
