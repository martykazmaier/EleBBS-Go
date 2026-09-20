package door

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func DropDir(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) string {
	p := ""
	if line != nil {
		p = strings.TrimSpace(line.Telnet.NodeDirectories)
	}
	if p != "" {
		n := 1
		if line != nil && line.RaNodeNr > 0 {
			n = line.RaNodeNr
		}
		p = strings.ReplaceAll(p, "*N", strconv.Itoa(n))
		p = strings.ReplaceAll(p, "*n", strconv.Itoa(n))
		return strings.TrimRight(p, `\/`)
	}
	if g != nil && !g.RaConfig.MultiLine {
		return strings.TrimRight(g.RaConfig.SysPath, `\/`)
	}
	cwd, _ := os.Getwd()
	return cwd
}

// EnterNodeDir makes the node directory the process current directory, as
// Pascal EleServ / -N-1 ChDir(NodeDirectories). Drop files and doors read
// door32.sys from cwd.
func EnterNodeDir(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) error {
	dir := DropDir(g, line)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.Chdir(dir)
}

func socketHandle(t *term.IO, line *cfgrec.LineCfg) uintptr {
	if line != nil {
		h := line.InheritedHandle
		if h != 0 && h != ^uintptr(0) {
			return h
		}
	}
	if t != nil {
		return comm.SocketOf(t.S)
	}
	return 0
}

func handleNum(h uintptr) int64 {
	if h == 0 || h == ^uintptr(0) {
		return -1
	}
	return int64(h)
}

func yn(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

func writeLines(path string, lines []string) error {
	var b strings.Builder
	for _, s := range lines {
		b.WriteString(s)
		b.WriteString("\r\n")
	}
	return os.WriteFile(path, pascal.ToCP437(b.String()), 0644)
}

// WriteDropFiles creates DORINFO1.DEF, DOOR.SYS, DOOR32.SYS and EXITINFO.BBS.
func WriteDropFiles(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, t *term.IO, dir string, flags starFlags, sock uintptr) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	baud := flags.baud
	if baud <= 0 {
		baud = fixBaud(line.Baud)
	}
	name := line.User.Name
	if flags.useHandle && line.User.Handle != "" {
		name = line.User.Handle
	}
	emul := 0
	if line.AnsiOn {
		emul = 1
	}
	if line.AvatarOn {
		emul = 2
	}
	if line.RipOn {
		emul = 3
	}
	baudStr := "BAUD"
	if line.ErrorFreeConnect && !flags.oldStyle {
		baudStr = "BAUD-R"
	}
	com := int(line.Modem.ComPort)
	if line.Baud == 0 {
		com = 0
	}
	sys := ""
	sysop := ""
	msgBase := ""
	if g != nil {
		sys = g.RaConfig.SystemName
		sysop = g.RaConfig.Sysop
		msgBase = g.RaConfig.MsgBasePath
	}
	lim := int(line.TimeLimit)
	if lim > 32767 {
		lim = 32767
	}
	_ = writeLines(filepath.Join(dir, "dorinfo1.def"), []string{
		pascal.UpCase(sys),
		pascal.UpCase(firstName(sysop)),
		pascal.UpCase(lastName(sysop)),
		fmt.Sprintf("COM%d", com),
		fmt.Sprintf("%d %s,N,8,1", baud, baudStr),
		"0",
		pascal.UpCase(firstName(name)),
		pascal.UpCase(lastName(name)),
		pascal.UpCase(line.User.Location),
		strconv.Itoa(emul),
		strconv.Itoa(int(line.User.Security)),
		strconv.Itoa(lim),
		"",
	})
	sub := line.User.SubDate
	if sub == "" {
		sub = "31-12" + time.Now().Format("06")
	}
	gr := "NG"
	if line.AnsiOn {
		gr = "GR"
	}
	proto := ""
	if line.User.DefaultProto != 0 {
		proto = string([]byte{line.User.DefaultProto})
	}
	userRec := line.User.Record
	scrn := int(line.User.ScreenLength)
	if scrn == 0 {
		scrn = 24
	}
	login := line.LoginTime
	if login == "" {
		login = time.Now().Format("15:04")
	}
	_ = writeLines(filepath.Join(dir, "door.sys"), []string{
		fmt.Sprintf("COM%d:", com),
		strconv.Itoa(baud),
		"8",
		strconv.Itoa(line.RaNodeNr),
		strconv.Itoa(baud),
		yn(line.Snooping),
		yn(line.PrinterLogging),
		"Y",
		"Y",
		line.User.Name,
		line.User.Location,
		line.User.VoicePhone,
		line.User.DataPhone,
		"",
		strconv.Itoa(int(line.User.Security)),
		strconv.Itoa(int(line.User.NoCalls)),
		line.User.LastDate,
		strconv.Itoa(lim * 60),
		strconv.Itoa(lim),
		gr,
		strconv.Itoa(scrn),
		"N",
		"",
		"",
		sub,
		strconv.Itoa(userRec),
		proto,
		strconv.Itoa(int(line.User.Uploads)),
		strconv.Itoa(int(line.User.Downloads)),
		strconv.Itoa(int(line.User.TodayK)),
		"65534",
		line.User.BirthDate,
		msgBase,
		msgBase,
		sysop,
		line.User.Handle,
		"",
		yn(line.ErrorFreeConnect),
		"N",
		"Y",
		"7",
		"0",
		line.User.LastDate,
		login,
		line.User.LastTime,
		"32768",
		"0",
		strconv.Itoa(int(line.User.UploadsK)),
		strconv.Itoa(int(line.User.DownloadsK)),
		line.User.Comment,
		"0",
		strconv.Itoa(int(line.User.MsgsPosted)),
		"",
	})
	comType := "0"
	if sock != 0 && sock != ^uintptr(0) {
		comType = "2"
	} else if line.TelnetServ {
		comType = "2"
	} else if line.Baud != 0 && !line.LocalLogon {
		comType = "1"
	}
	emul32 := "0"
	if line.AnsiOn {
		emul32 = "1"
	}
	if line.AvatarOn {
		emul32 = "2"
	}
	urec := line.User.Record + 1
	if line.User.Record < 0 {
		urec = 1
	}
	_ = writeLines(filepath.Join(dir, "door32.sys"), []string{
		comType,
		strconv.FormatInt(handleNum(sock), 10),
		strconv.Itoa(baud),
		cfgrec.PidName,
		strconv.Itoa(urec),
		line.User.Name,
		line.User.Handle,
		strconv.Itoa(int(line.User.Security)),
		strconv.Itoa(int(line.TimeLimit)),
		emul32,
		strconv.Itoa(line.RaNodeNr),
		"",
	})
	exitPath := filepath.Join(dir, "exitinfo.bbs")
	if g != nil && !g.RaConfig.MultiLine {
		exitPath = filepath.Join(strings.TrimRight(g.RaConfig.SysPath, `\/`), "exitinfo.bbs")
	}
	return os.WriteFile(exitPath, cfgrec.EncodeExitinfo(line), 0644)
}
