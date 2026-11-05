package serv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	ShowHelp    bool
	FTP         bool
	News        bool
	Telnet      bool
	TelnetS     bool
	TelnetSPort int
	WSS         bool
	WSSPort     int
	SSH         bool
	SSHPort     int
	Ident       bool
	Anonymous   bool
	FTPPort     int
	NewsPort    int
	FTPLimit    int
	FTPNode     int
	PasvIP      string
	PasvDyn     string
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
	o := Options{FTPPort: 990, NewsPort: 563, FTPLimit: 10, FTPNode: 256, PasvLo: 1025, PasvHi: 65535, SSHPort: 22, TelnetSPort: 992, WSSPort: 11235}
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
		case key == "TELNETS":
			o.TelnetS = true
		case strings.HasPrefix(key, "TELNETSPORT:"):
			o.TelnetSPort = atoi(strip(a, "TELNETSPORT:"))
			o.TelnetS = true
		case key == "WSS":
			o.WSS = true
		case strings.HasPrefix(key, "WSSPORT:"):
			o.WSSPort = atoi(strip(a, "WSSPORT:"))
			o.WSS = true
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
		case strings.HasPrefix(key, "PASVDYNIP:"):
			o.PasvDyn = strip(a, "PASVDYNIP:")
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
		"-TELNETS           - Enable telnets (telnet over TLS, default port 992)\n" +
		"-TELNETSPORT:<x>   - Telnets listen port (default 992)\n" +
		"-WSS               - Enable secure WebSocket server for web terminals\n" +
		"                     (default port 11235)\n" +
		"-WSSPORT:<x>       - WSS listen port (default 11235)\n" +
		"-SSH               - Enable SSH server (USERS.BBS login, starts EleBBS)\n" +
		"-SSHPORT:<x>       - SSH listen port (default 22)\n" +
		"                     Host keys eleserv_hostkey (Ed25519), eleserv_hostkey_rsa\n" +
		"                     and eleserv_hostkey_dsa are created in the system path.\n" +
		"-XA                - Enable anonymous FTPS logins\n\n" +
		"-CERT:<file>       - PEM certificate (or combined cert+key PEM)\n" +
		"-KEY:<file>        - PEM private key (optional if -CERT is combined)\n" +
		"                     FTPS, NNTPS, telnets and WSS share the certificate.\n" +
		"                     If omitted, EleSERV looks for eleserv.pem or\n" +
		"                     eleserv.crt + eleserv.key in the system path.\n\n" +
		"-FTPPORT:<x>       - FTPS listen port (default 990)\n" +
		"-NNTSPORT:<x>      - NNTPS listen port (default 563)\n" +
		"-FTPLIMIT:<x>      - Maximum FTPS sessions\n" +
		"-FTPNODE:<x>       - Starting node number for FTPS sessions\n" +
		"-PASVSRVIP:<addr>  - Reported IP for PASV data connections\n" +
		"-PASVDYNIP:<file>  - PASV IPv4 file, or a hostname (looked up like ping)\n" +
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

func ipv4Host(host string) string {
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return host
}

// PasvAdvertisedIP is Pascal cmdPasv: use -PASVSRVIP unless it is unset or the
// control socket is loopback. Private clients keep the LAN address so PASV
// still works inside NAT without hairpin.
func PasvAdvertisedIP(local, client, pasvSrv string) string {
	local = ipv4Host(local)
	client = ipv4Host(client)
	pasvSrv = ipv4Host(pasvSrv)
	if pasvSrv == "" || pasvSrv == "0.0.0.0" || strings.HasPrefix(local, "127.") {
		return local
	}
	if ip := net.ParseIP(client); ip != nil && ip.IsPrivate() {
		return local
	}
	return pasvSrv
}

var (
	dynLookMu sync.Mutex
	dynLookIP string
	dynLookAt time.Time
	dynLookH  string
)

// ReadPasvDynIP is Pascal ftp_DynIp (one-line IPv4 file). A hostname is
// resolved with the Windows DNS client, the same lookup ping uses; ICMP is not sent.
func ReadPasvDynIP(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if ip := parseIPv4(path); ip != "" {
		return ip
	}
	if strings.ContainsAny(path, `/\`) {
		return readPasvIPFile(path)
	}
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return readPasvIPFile(path)
	}
	return lookupPasvIPv4(path)
}

func readPasvIPFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return parseIPv4(line)
}

func lookupPasvIPv4(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	dynLookMu.Lock()
	defer dynLookMu.Unlock()
	if dynLookH == host && dynLookIP != "" && time.Since(dynLookAt) < 5*time.Minute {
		return dynLookIP
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return dynLookIP
	}
	var first string
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		s := v4.String()
		if first == "" {
			first = s
		}
		if !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			dynLookH, dynLookIP, dynLookAt = host, s, time.Now()
			return s
		}
	}
	if first != "" {
		dynLookH, dynLookIP, dynLookAt = host, first, time.Now()
	}
	return first
}

func parseIPv4(s string) string {
	ip := net.ParseIP(ipv4Host(strings.TrimSpace(s)))
	if ip == nil || ip.To4() == nil {
		return ""
	}
	return ip.To4().String()
}
