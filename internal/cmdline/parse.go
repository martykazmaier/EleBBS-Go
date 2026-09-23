package cmdline

import (
	"regexp"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

// spawnFlag splits a glued front-end command line such as
// -N2-H204-XT-XC-XI192.168.0.10-B65529 into separate EleBBS switches.
var spawnFlag = regexp.MustCompile(`(?i)[-/](?:N-?\d+|H\d+|B\d+(?:/[^\-/]*)?|C\d+|XT|XC|XM|XI[\d.]+)`)

type Options struct {
	ShowHelp        bool
	Full            bool
	Node            int
	AutoNode        bool
	ComPort         int
	Baud            uint16
	ConnectStr      string
	Local           bool
	InheritedHandle uintptr
	ReLogOn         bool
	ReLogMenu       bool
	SnoopOff        bool
	NoClose         bool
	Monitor         bool
	TelnetFromIP    string
	TelnetServ      bool
	Mailer          string
	BatchAtExit     string
	EventMins       int
	SysPath         string
}

func expandSpawnFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, splitSpawnArg(a)...)
	}
	return out
}

func splitSpawnArg(a string) []string {
	if len(a) < 2 || (a[0] != '-' && a[0] != '/') {
		return []string{a}
	}
	matches := spawnFlag.FindAllString(a, -1)
	if len(matches) < 2 {
		return []string{a}
	}
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(m)
	}
	if !strings.EqualFold(b.String(), a) {
		return []string{a}
	}
	return matches
}

func Parse(args []string, full bool) Options {
	o := Options{Node: 1, Full: full, InheritedHandle: ^uintptr(0)}
	for _, a := range expandSpawnFlags(args) {
		if a == "" {
			continue
		}
		if a[0] == '?' || strings.EqualFold(a, "-?") || strings.EqualFold(a, "/?") {
			if full {
				o.ShowHelp = true
			}
			continue
		}
		if a[0] != '-' && a[0] != '/' {
			continue
		}
		key := strings.ToUpper(a[1:])
		if !full {
			if len(key) == 0 || !strings.ContainsAny(key[:1], "?HNMOXS") {
				continue
			}
		}
		if key == "?" {
			o.ShowHelp = true
			continue
		}
		switch {
		case key == "L":
			o.Local = true
			o.Baud = 0
		case strings.HasPrefix(key, "B"):
			o.ConnectStr = a[2:]
			o.Baud = parseBaud(o.ConnectStr)
			if o.Baud == 0 {
				o.Local = true
			}
		case strings.HasPrefix(key, "C") && !strings.HasPrefix(key, "CFG"):
			o.ComPort = atoi(a[2:])
		case strings.HasPrefix(key, "N"):
			if strings.HasPrefix(key, "NOEMS") || strings.HasPrefix(key, "NOXMS") || strings.HasPrefix(key, "NORAW") {
				continue
			}
			rest := a[2:]
			if rest == "-1" {
				o.AutoNode = true
			} else {
				o.Node = atoi(rest)
				if o.Node < 1 {
					o.Node = 1
				}
			}
		case strings.HasPrefix(key, "H"):
			o.InheritedHandle = parseHandle(a[2:])
		case key == "R":
			o.ReLogOn = true
		case key == "G":
			o.ReLogMenu = true
		case key == "S":
			o.SnoopOff = true
		case strings.HasPrefix(key, "M"):
			o.Mailer = a[2:]
		case strings.HasPrefix(key, "FEND"):
			o.BatchAtExit = a[len("-FEND"):]
		case strings.HasPrefix(key, "T"):
			o.EventMins = atoi(a[2:])
		case strings.HasPrefix(key, "X"):
			if len(key) < 2 {
				continue
			}
			switch key[1] {
			case 'C':
				o.NoClose = true
			case 'M':
				o.Monitor = true
			case 'I':
				o.TelnetFromIP = strings.TrimSpace(a[3:])
			case 'T':
				o.TelnetServ = true
			}
		}
	}
	return o
}

func parseBaud(s string) uint16 {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if s == "115200" {
		s = "11520"
	}
	n := atoi(s)
	if n < 0 {
		n = 0
	}
	return uint16(n)
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}

func parseHandle(s string) uintptr {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return uintptr(n)
}

func Apply(line *cfgrec.LineCfg, o Options) {
	line.RaNodeNr = o.Node
	line.LocalLogon = o.Local
	line.ConnectStr = o.ConnectStr
	line.Baud = o.Baud
	line.ReLogOnBBS = o.ReLogOn
	line.ReLogMenu = o.ReLogMenu
	line.Snooping = !o.SnoopOff
	line.ComNoClose = o.NoClose
	line.DoMonitor = o.Monitor
	line.TelnetFromIP = o.TelnetFromIP
	line.TelnetServ = o.TelnetServ
	line.InheritedHandle = o.InheritedHandle
	line.BatchAtExit = o.BatchAtExit
	line.MailerCmdLine = o.Mailer
	if o.ComPort > 0 {
		line.Modem.ComPort = byte(o.ComPort)
	}
	if o.Local || line.Baud == 0 && o.InheritedHandle == ^uintptr(0) && !o.TelnetServ && o.ComPort == 0 {
		line.LocalLogon = true
		line.Baud = 0
		line.CarrierCheck = false
	}
	if o.InheritedHandle != 0 && o.InheritedHandle != ^uintptr(0) {
		line.LocalLogon = false
		line.CarrierCheck = true
		if line.Baud == 0 {
			line.Baud = 11520
		}
	}
	if o.TelnetServ {
		line.LocalLogon = false
		line.CarrierCheck = true
		if line.Baud == 0 {
			line.Baud = 65529
		}
		if line.ConnectStr == "" {
			line.ConnectStr = "65529/TELNET"
		}
	}
}

func HelpText() string {
	var b strings.Builder
	b.WriteString(cfgrec.SystemMsgPrefix)
	b.WriteString(cfgrec.PidName)
	b.WriteString(" (")
	b.WriteString(pascal.Trim("255"))
	b.WriteString(" node version) - Command line parameters\n\n")
	b.WriteString("-Bxxxx                 - Log on to EleBBS using 'xxxx' bps (eg: /B2400, /B65529 telnet)\n")
	b.WriteString("-Cxx                   - Serial COM port (used with -XT for FOSSIL doors)\n")
	b.WriteString("-E                     - Specifies the default exit error-level\n")
	b.WriteString("-G                     - Re-Logon to the last menu instead of the TOP\n")
	b.WriteString("-L                     - Local Logon, same as /B0\n")
	b.WriteString("-M<....>               - Specifies mailer-frontend.\n")
	b.WriteString("-Nxxx                  - Nodenumber to logon to (eg: /N250)\n")
	b.WriteString("-R                     - Re-Logon the user.\n")
	b.WriteString("-S                     - Disables screen-output (disables snooping)\n")
	b.WriteString("-T<....>               - Set time till the next event.\n")
	b.WriteString("-Hxxxx                 - Windows socket handle passed from the front-end\n")
	b.WriteString("-XT                    - Telnet session on the inherited handle (-H)\n")
	b.WriteString("-XC                    - Don't close the communications port upon exit\n")
	b.WriteString("-XI<ip>                - Remote IP address from the front-end\n")
	b.WriteString("-XM                    - Make EleBBS available for EleMON\n")
	b.WriteString("-FEND                  - Batch file to execute after the session\n")
	return b.String()
}
