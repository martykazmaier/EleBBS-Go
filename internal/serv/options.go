package serv

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Options struct {
	ShowHelp    bool
	FTP         bool
	News        bool
	Telnet      bool
	SSH         bool
	SSHPort     int
	Ident       bool
	Anonymous   bool
	FTPPort     int
	NewsPort    int
	FTPLimit    int
	FTPNode     int
	PasvIP      string
	PasvLo      int
	PasvHi      int
	PasvOffset  int
	FTPIndex    string
	TransferLog string
	ImportDIZ   bool
	CertFile    string
	KeyFile     string
	SysPath     string
}

func Parse(args []string) Options {
	o := Options{FTPPort: 990, NewsPort: 563, FTPLimit: 10, FTPNode: 256, PasvLo: 1025, PasvHi: 65535, SSHPort: 22}
	for _, a := range args {
		if a == "" {
			continue
		}
		if a[0] != '-' && a[0] != '/' {
			continue
		}
		key := strings.ToUpper(a[1:])
		switch {
		case key == "?" || key == "H" || key == "HELP":
			o.ShowHelp = true
		case key == "FTP" || key == "FTPS":
			o.FTP = true
		case key == "NEWS" || key == "NNTP" || key == "NNTPS":
			o.News = true
		case key == "TELNET" || key == "XT":
			o.Telnet = true
		case key == "SSH":
			o.SSH = true
		case strings.HasPrefix(key, "SSHPORT:"):
			o.SSHPort = atoi(strip(a, "SSHPORT:"))
			o.SSH = true
		case key == "IDENT":
			o.Ident = true
		case key == "XA":
			o.Anonymous = true
		case strings.HasPrefix(key, "CERT:"):
			o.CertFile = strip(a, "CERT:")
		case strings.HasPrefix(key, "KEY:"):
			o.KeyFile = strip(a, "KEY:")
		case strings.HasPrefix(key, "FTPPORT:"):
			o.FTPPort = atoi(strip(a, "FTPPORT:"))
			o.FTP = true
		case strings.HasPrefix(key, "NNTSPORT:"):
			o.NewsPort = atoi(strip(a, "NNTSPORT:"))
			o.News = true
		case strings.HasPrefix(key, "NEWSPORT:"):
			o.NewsPort = atoi(strip(a, "NEWSPORT:"))
			o.News = true
		case strings.HasPrefix(key, "FTPLIMIT:"):
			o.FTPLimit = atoi(strip(a, "FTPLIMIT:"))
		case strings.HasPrefix(key, "FTPNODE:"):
			o.FTPNode = atoi(strip(a, "FTPNODE:"))
		case strings.HasPrefix(key, "PASVSRVIP:"):
			o.PasvIP = strip(a, "PASVSRVIP:")
		case strings.HasPrefix(key, "PASVPORTS:"):
			rng := strip(a, "PASVPORTS:")
			if i := strings.IndexByte(rng, '-'); i >= 0 {
				o.PasvLo = atoi(rng[:i])
				o.PasvHi = atoi(rng[i+1:])
			}
		case strings.HasPrefix(key, "PASVOFFSET:"):
			o.PasvOffset = atoi(strip(a, "PASVOFFSET:"))
		case strings.HasPrefix(key, "FTPINDEX:"):
			o.FTPIndex = strip(a, "FTPINDEX:")
		case strings.HasPrefix(key, "FTPXLOG:"):
			o.TransferLog = strip(a, "FTPXLOG:")
		case key == "FTPDIZ":
			o.ImportDIZ = true
		}
	}
	return o
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func strip(arg, name string) string {
	u := strings.ToUpper(arg)
	i := strings.Index(u, name)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(arg[i+len(name):])
}

func HelpText(pid string) string {
	return pid + " SERVER - Command line parameters\n\n" +
		"-FTPS              - Enable FTPS (implicit TLS, default port 990)\n" +
		"-NNTPS             - Enable NNTPS (implicit TLS, default port 563)\n" +
		"-TELNET            - Enable telnet server (spawns ELEBBS.EXE -XT -H...)\n" +
		"-XT                - Same as -TELNET (connection is telnet, not in-process BBS)\n" +
		"-SSH               - Enable SSH server (USERS.BBS login, starts EleBBS)\n" +
		"-SSHPORT:<x>       - SSH listen port (default 22)\n" +
		"-XA                - Enable anonymous FTPS logins\n\n" +
		"-CERT:<file>       - PEM certificate (or combined cert+key PEM)\n" +
		"-KEY:<file>        - PEM private key (optional if -CERT is combined)\n" +
		"                     If omitted, EleSERV looks for eleserv.pem or\n" +
		"                     eleserv.crt + eleserv.key in the system path.\n\n" +
		"-FTPPORT:<x>       - FTPS listen port (default 990)\n" +
		"-NNTSPORT:<x>      - NNTPS listen port (default 563)\n" +
		"-FTPLIMIT:<x>      - Maximum FTPS sessions\n" +
		"-FTPNODE:<x>       - Starting node number for FTPS sessions\n" +
		"-PASVSRVIP:<addr>  - Reported IP for PASV data connections\n" +
		"-PASVPORTS:<x-y>   - Passive data port range\n" +
		"-PASVOFFSET:<x>    - Add x to the reported PASV port\n" +
		"-FTPXLOG:<file>    - Transfer summary log\n" +
		"-FTPINDEX:<file>   - Virtual index filename (eg. 00_index.txt)\n" +
		"-FTPDIZ            - Import FILE_ID.DIZ from uploads\n"
}

func ParseInt(s string) int { return atoi(s) }

func ReadLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil && len(s) == 0 {
		return "", err
	}
	s = strings.TrimRight(s, "\r\n")
	return s, err
}

func WriteLine(w io.Writer, s string) error {
	_, err := io.WriteString(w, s+"\r\n")
	return err
}

func SplitCmd(line string) (cmd, arg string) {
	line = strings.TrimSpace(line)
	i := 0
	for i < len(line) && line[i] != ' ' && line[i] != '\t' {
		i++
	}
	cmd = strings.ToUpper(line[:i])
	arg = strings.TrimSpace(line[i:])
	return cmd, arg
}

func FormatPasv(ip string, port int) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		parts = []string{"127", "0", "0", "1"}
	}
	p1 := port / 256
	p2 := port % 256
	return fmt.Sprintf("(%s,%s,%s,%s,%d,%d)", parts[0], parts[1], parts[2], parts[3], p1, p2)
}
