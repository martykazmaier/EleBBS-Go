package door

import (
	"os"
	"strconv"
	"strings"
	"unicode"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

type starFlags struct {
	useHandle   bool
	oldStyle    bool
	leaveHot    bool
	clearScreen bool
	inherit     bool
	noBatches   bool
	closeHandle bool
	pauseCom    bool
	freezeTime  bool
	memorySwap  bool
	baud        int
	cmd         string
	dupOnCmd    bool
}

func fixBaud(b uint16) int {
	if b == 11520 {
		return 115200
	}
	return int(b)
}

func firstName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}

func lastName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return ""
}

func filePath(areas []cfgrec.FilesArea, area uint16) string {
	if a, ok := files.FindArea(areas, area); ok {
		return a.FilePath
	}
	return ""
}

func expandStars(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, areas []cfgrec.FilesArea, raw, inheritHandle string) starFlags {
	f := starFlags{
		clearScreen: true,
		closeHandle: true,
		pauseCom:    true,
		baud:        fixBaud(line.Baud),
	}
	if line.ComNoClose {
		f.closeHandle = false
	}
	if t != nil {
		raw = t.ExpandRA(raw)
	}
	var out strings.Builder
	i := 0
	for i < len(raw) {
		if raw[i] != '*' || i+1 >= len(raw) {
			out.WriteByte(raw[i])
			i++
			continue
		}
		code := unicode.ToUpper(rune(raw[i+1]))
		i += 2
		switch code {
		case '!':
			f.freezeTime = true
		case 'V':
			f.clearScreen = false
		case 'A':
			f.useHandle = true
		case 'D':
			f.oldStyle = true
		case 'H':
			f.leaveHot = true
			f.closeHandle = false
		case 'W':
			f.inherit = true
			f.dupOnCmd = true
			f.closeHandle = false
			if inheritHandle != "" {
				out.WriteString(inheritHandle)
			}
		case 'Y':
			f.inherit = true
			f.noBatches = true
			f.closeHandle = false
			f.leaveHot = true
			f.pauseCom = false
		case 'Z':
			f.inherit = true
			f.noBatches = true
		case 'B':
			out.WriteString(strconv.Itoa(f.baud))
		case 'C':
			out.WriteString(comspec())
		case 'F':
			out.WriteString(firstName(line.User.Name))
		case 'G':
			if line.AnsiOn {
				out.WriteByte('1')
			} else {
				out.WriteByte('0')
			}
		case 'L':
			out.WriteString(lastName(line.User.Name))
		case 'M':
			// Pascal RaExec *M: MemorySwap. On Win32 the flag is consumed and
			// EleBBS stays in RAM as the waiting parent (no DOS swap).
			f.memorySwap = true
		case 'N':
			out.WriteString(strconv.Itoa(line.RaNodeNr))
		case 'P':
			if line.Baud == 0 {
				out.WriteByte('1')
			} else {
				out.WriteString(strconv.Itoa(int(line.Modem.ComPort)))
			}
		case 'R':
			if line.User.Record >= 0 {
				out.WriteString(strconv.Itoa(line.User.Record))
			} else {
				out.WriteString("-1")
			}
		case 'T':
			lim := int(line.TimeLimit)
			if lim > 32767 {
				lim = 32767
			}
			out.WriteString(strconv.Itoa(lim))
		case '0':
			out.WriteString(filePath(areas, line.User.FileArea))
		case '1':
			out.WriteString(strconv.Itoa(int(line.User.MsgArea)))
		case 'O', 'S', 'U':
			rest := raw[i:]
			val, next := takeStarArg(rest)
			i += next
			if code == 'O' {
				if n, err := strconv.Atoi(val); err == nil && n > 0 {
					f.baud = n
				}
			}
		default:
			out.WriteByte('*')
			out.WriteByte(byte(code))
		}
	}
	f.cmd = strings.TrimSpace(out.String())
	return f
}

func comspec() string {
	if v := os.Getenv("COMSPEC"); v != "" {
		return v
	}
	return "cmd.exe"
}

func takeStarArg(s string) (val string, n int) {
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '*' {
		i++
	}
	return s[:i], i
}

func splitPath(cmd string) (exe, rest string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", ""
	}
	if cmd[0] == '"' {
		if j := strings.Index(cmd[1:], `"`); j >= 0 {
			return cmd[1 : 1+j], strings.TrimSpace(cmd[2+j:])
		}
	}
	i := 0
	for i < len(cmd) && cmd[i] != ' ' && cmd[i] != '\t' {
		i++
	}
	return cmd[:i], strings.TrimSpace(cmd[i:])
}

func isBatch(exe string) bool {
	e := pascal.UpCase(exe)
	return strings.HasSuffix(e, ".BAT") || strings.HasSuffix(e, ".CMD") || strings.HasSuffix(e, ".SH")
}
