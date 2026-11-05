package ftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/files"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
	"elebbs/internal/serv"
	"elebbs/internal/userbase"
)

type Config struct {
	G          *cfgrec.GlobalCfg
	Port       int
	Limit      int
	Anonymous  bool
	PasvIP     string
	PasvDyn    string
	PasvLo     int
	PasvHi     int
	PasvOffset int
	IndexName  string
	XferLog    string
	TLS        *tls.Config
}

type Server struct {
	cfg   Config
	areas []cfgrec.FilesArea
	grps  []cfgrec.Group
	alive int32
}

type session struct {
	srv     *Server
	c       net.Conn
	r       *bufio.Reader
	user    cfgrec.User
	gotUser bool
	authed  bool
	anon    bool
	group   int
	area    int
	rest    int64
	typeI   bool
	pasvLn  net.Listener
	portDst string
	mu      sync.Mutex
}

func Listen(cfg Config) error {
	if cfg.Port <= 0 {
		cfg.Port = 990
	}
	if cfg.TLS == nil {
		return fmt.Errorf("FTPS requires a TLS certificate (-CERT)")
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 10
	}
	if cfg.PasvLo <= 0 {
		cfg.PasvLo = 1025
	}
	if cfg.PasvHi <= 0 {
		cfg.PasvHi = 65535
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", cfg.Port), cfg.TLS)
	if err != nil {
		return err
	}
	defer ln.Close()
	s := &Server{
		cfg:   cfg,
		areas: files.LoadAreas(cfg.G),
		grps:  files.LoadGroups(cfg.G),
	}
	fmt.Fprintf(os.Stderr, "%sFTPS listening on :%d\n", cfgrec.SystemMsgPrefix, cfg.Port)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		if int(atomic.LoadInt32(&s.alive)) >= cfg.Limit {
			_, _ = c.Write([]byte("421 Too many connections.\r\n"))
			_ = c.Close()
			continue
		}
		atomic.AddInt32(&s.alive, 1)
		go func(conn net.Conn) {
			defer atomic.AddInt32(&s.alive, -1)
			s.handle(conn)
		}(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Minute))
	ss := &session{srv: s, c: c, r: bufio.NewReader(c), group: -1, area: -1, typeI: true}
	sys := s.cfg.G.RaConfig.SystemName
	if sys == "" {
		sys = "EleBBS"
	}
	ss.reply(220, fmt.Sprintf("EleBBS - FTPS server running at %s", sys))
	logx.Write(s.cfg.G, 0, '>', "[FTPSERV] ["+host(c)+"] FTPS connection opened")
	for {
		_ = c.SetDeadline(time.Now().Add(10 * time.Minute))
		line, err := serv.ReadLine(ss.r)
		if err != nil {
			break
		}
		if line == "" {
			continue
		}
		cmd, arg := serv.SplitCmd(line)
		if !ss.dispatch(cmd, arg) {
			break
		}
	}
	logx.Write(s.cfg.G, 0, '>', "[FTPSERV] ["+host(c)+"] Connection closed")
}

func (ss *session) dispatch(cmd, arg string) bool {
	switch cmd {
	case "USER":
		ss.cmdUser(arg)
	case "PASS":
		ss.cmdPass(arg)
	case "QUIT":
		ss.reply(221, "Goodbye.")
		return false
	case "NOOP", "ALLO":
		ss.reply(200, "OK")
	case "SYST":
		ss.reply(215, "UNIX Type: L8")
	case "TYPE":
		ss.typeI = strings.ToUpper(arg) != "A"
		ss.reply(200, "Type set")
	case "MODE":
		if strings.EqualFold(strings.TrimSpace(arg), "S") {
			ss.reply(200, "mode S ok")
		} else {
			ss.reply(504, "Please use S(tream) mode")
		}
	case "STRU":
		ss.reply(200, "F ok")
	case "PWD", "XPWD":
		ss.needAuth()
		if ss.authed {
			ss.reply(257, fmt.Sprintf("\"%s\" is your current location", ss.pwd()))
		}
	case "CWD", "XCWD":
		if ss.needAuth() {
			ss.cmdCwd(arg)
		}
	case "CDUP", "XCUP":
		if ss.needAuth() {
			if ss.area > 0 {
				ss.area = -1
			} else {
				ss.group = -1
			}
			ss.reply(257, fmt.Sprintf("\"%s\" is your current location", ss.pwd()))
		}
	case "PASV":
		if ss.needAuth() {
			ss.cmdPasv()
		}
	case "EPSV":
		if ss.needAuth() {
			ss.cmdEpsv()
		}
	case "PORT":
		if ss.needAuth() {
			ss.cmdPort(arg)
		}
	case "LIST":
		if ss.needAuth() {
			ss.cmdList(arg, false)
		}
	case "NLST":
		if ss.needAuth() {
			ss.cmdList(arg, true)
		}
	case "RETR":
		if ss.needAuth() {
			ss.cmdRetr(arg)
		}
	case "STOR":
		if ss.needAuth() {
			ss.cmdStor(arg)
		}
	case "REST":
		ss.rest = int64(servAtoi(arg))
		ss.reply(350, fmt.Sprintf("Restarting at %d. Send STORE or RETRIEVE to initiate transfer.", ss.rest))
	case "SIZE":
		if ss.needAuth() {
			ss.cmdSize(arg)
		}
	case "MDTM":
		if ss.needAuth() {
			ss.cmdMdtm(arg)
		}
	case "DELE":
		if ss.needAuth() {
			ss.cmdDele(arg)
		}
	case "HELP":
		ss.cmdHelp()
	case "AUTH":
		ss.reply(234, "Already using TLS")
	case "PBSZ":
		ss.reply(200, "PBSZ=0")
	case "PROT":
		ss.reply(200, "Data channel protected")
	case "FEAT":
		_ = serv.WriteLine(ss.c, "211-Features:")
		_ = serv.WriteLine(ss.c, " PASV")
		_ = serv.WriteLine(ss.c, " EPSV")
		_ = serv.WriteLine(ss.c, " SIZE")
		_ = serv.WriteLine(ss.c, " MDTM")
		_ = serv.WriteLine(ss.c, " REST STREAM")
		_ = serv.WriteLine(ss.c, " AUTH TLS")
		_ = serv.WriteLine(ss.c, " PBSZ")
		_ = serv.WriteLine(ss.c, " PROT")
		ss.reply(211, "End")
	case "MKD", "XMKD", "RMD", "XRMD", "RNFR", "RNTO", "APPE", "ABOR", "SITE", "SMNT":
		ss.reply(502, "Command not implemented")
	default:
		ss.reply(500, "Unknown command")
	}
	return true
}

func (ss *session) reply(code int, msg string) {
	_ = serv.WriteLine(ss.c, fmt.Sprintf("%d %s", code, msg))
}

func (ss *session) needAuth() bool {
	if ss.authed {
		return true
	}
	ss.reply(530, "Please login with USER and PASS first")
	return false
}

func (ss *session) cmdUser(name string) {
	ss.user = cfgrec.User{}
	ss.gotUser = false
	ss.authed = false
	ss.anon = false
	name = pascal.Trim(name)
	if strings.EqualFold(name, "ftp") {
		name = "anonymous"
	}
	anonName := strings.EqualFold(name, "anonymous")
	u, ok := userbase.Search(ss.srv.cfg.G, name)
	if ok {
		if !anonName && !ss.srv.cfg.Anonymous && u.Guest() {
			ss.reply(331, "User name, okay. Please send password.")
			return
		}
		ss.user = u
		ss.gotUser = true
	}
	if anonName {
		ss.anon = true
		if !ss.gotUser {
			ss.user.Name = "Anonymous"
			ss.user.Handle = "Anonymous"
			ss.gotUser = true
		}
		ss.reply(331, "User name, okay. Please send email address.")
		return
	}
	ss.reply(331, "User name, okay. Please send password.")
}

func (ss *session) cmdPass(pw string) {
	if ss.anon {
		ss.authed = true
		ss.reply(230, "Anonymous user logged in")
		logx.Write(ss.srv.cfg.G, 0, '>', "[FTPSERV] ["+host(ss.c)+"] Anonymous ("+pw+") on-line")
		return
	}
	if !ss.gotUser || !userbase.CheckPassword(ss.user, pw, ss.srv.cfg.G.RaConfig.StrictPwdChecking) {
		ss.reply(530, "Login incorrect")
		logx.Write(ss.srv.cfg.G, 0, '!', "[FTPSERV] ["+host(ss.c)+"] Login failed for "+ss.user.Name)
		return
	}
	ss.authed = true
	ss.reply(230, "User logged in")
	logx.Write(ss.srv.cfg.G, 0, '>', "[FTPSERV] ["+host(ss.c)+"] "+ss.user.Name+" on-line")
}

func (ss *session) pwd() string {
	p := "/"
	if ss.group > 0 {
		if g, ok := files.FindGroup(ss.srv.grps, uint16(ss.group)); ok {
			p += files.ConvDirName(g.Name) + "/"
		}
		if ss.area > 0 {
			if a, ok := files.FindArea(ss.srv.areas, uint16(ss.area)); ok {
				p += files.ConvDirName(a.Name) + "/"
			}
		}
	}
	return p
}

func (ss *session) cmdCwd(arg string) {
	arg = strings.TrimSpace(arg)
	if arg == ".." {
		ss.dispatch("CDUP", "")
		return
	}
	listGroup, listArea := ss.group, ss.area
	g, a, _, invalid := ss.partsFromDir(arg, listGroup, listArea)
	if invalid {
		ss.reply(550, "Requested action not taken: file unavailable.")
		return
	}
	ss.group, ss.area = g, a
	ss.reply(250, fmt.Sprintf("Changed directory to \"%s\"", ss.pwd()))
}

func (ss *session) listenPasv() (int, bool) {
	ss.closePasv()
	lo, hi := ss.srv.cfg.PasvLo, ss.srv.cfg.PasvHi
	if lo <= 0 {
		lo = 1025
	}
	if hi < lo {
		hi = 65535
	}
	span := hi - lo + 1
	for i := 0; i < 3; i++ {
		p := pickPasvPort(lo, span, time.Now().UnixNano(), i)
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			ss.pasvLn = ln
			break
		}
	}
	if ss.pasvLn == nil {
		ss.reply(425, "Can't open data connection")
		return 0, false
	}
	_, portStr, _ := net.SplitHostPort(ss.pasvLn.Addr().String())
	return servAtoi(portStr) + ss.srv.cfg.PasvOffset, true
}

func (ss *session) cmdPasv() {
	port, ok := ss.listenPasv()
	if !ok {
		return
	}
	local, _, _ := net.SplitHostPort(ss.c.LocalAddr().String())
	client, _, _ := net.SplitHostPort(ss.c.RemoteAddr().String())
	pasvIP := ss.srv.cfg.PasvIP
	if dyn := serv.ReadPasvDynIP(ss.srv.cfg.PasvDyn); dyn != "" {
		pasvIP = dyn
	}
	ip := serv.PasvAdvertisedIP(local, client, pasvIP)
	ss.portDst = ""
	ss.reply(227, "Entering Passive Mode "+serv.FormatPasv(ip, port))
}

func (ss *session) cmdEpsv() {
	port, ok := ss.listenPasv()
	if !ok {
		return
	}
	ss.portDst = ""
	ss.reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", port))
}

func (ss *session) cmdPort(arg string) {
	ss.closePasv()
	fs := strings.Split(arg, ",")
	if len(fs) < 6 {
		ss.reply(501, "Bad PORT")
		return
	}
	p1, p2 := servAtoi(fs[4]), servAtoi(fs[5])
	ss.portDst = fmt.Sprintf("%s.%s.%s.%s:%d", trim(fs[0]), trim(fs[1]), trim(fs[2]), trim(fs[3]), p1*256+p2)
	ss.reply(200, "PORT command successful")
}

func (ss *session) openData() (net.Conn, error) {
	var c net.Conn
	var err error
	if ss.pasvLn != nil {
		if tl, ok := ss.pasvLn.(*net.TCPListener); ok {
			_ = tl.SetDeadline(time.Now().Add(30 * time.Second))
		}
		c, err = ss.pasvLn.Accept()
		ss.closePasv()
		if err != nil {
			return nil, err
		}
	} else if ss.portDst != "" {
		d := net.Dialer{Timeout: 30 * time.Second}
		c, err = d.Dial("tcp", ss.portDst)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("no PORT/PASV")
	}
	if ss.srv.cfg.TLS != nil {
		tc := tls.Server(c, ss.srv.cfg.TLS)
		_ = tc.SetDeadline(time.Now().Add(30 * time.Second))
		if err := tc.Handshake(); err != nil {
			_ = c.Close()
			return nil, err
		}
		_ = tc.SetDeadline(time.Time{})
		return tc, nil
	}
	return c, nil
}

func (ss *session) closePasv() {
	if ss.pasvLn != nil {
		_ = ss.pasvLn.Close()
		ss.pasvLn = nil
	}
}

func (ss *session) cmdList(arg string, nlst bool) {
	ss.reply(150, "Opening data connection")
	dc, err := ss.openData()
	if err != nil {
		ss.reply(425, "Unable to open data connection")
		return
	}
	defer dc.Close()
	group, area := ss.group, ss.area
	wild := ""
	arg = ftpListArg(arg)
	if arg != "" {
		g, a, w, _ := ss.partsFromDir(arg, group, area)
		group, area, wild = g, a, w
	}
	if wild == "" {
		wild = "*"
	}
	now := time.Now().Format("Jan 02  2006")
	write := func(s string) { _, _ = io.WriteString(dc, s+"\r\n") }
	if area < 0 && group < 0 {
		for _, g := range ss.srv.grps {
			if !files.GroupAccess(g, ss.user) {
				continue
			}
			if !files.MatchWild(wild, files.ConvDirName(g.Name)) {
				continue
			}
			write(listLine(false, 1024, now, files.ConvDirName(g.Name), nlst))
		}
	} else {
		if area < 0 {
			for _, ar := range ss.srv.areas {
				if !files.CheckFileAreaAccess(ar, ss.user, false, true, false, true, uint16(group)) {
					continue
				}
				if !files.MatchWild(wild, files.ConvDirName(ar.Name)) && !files.MatchWild(wild, ar.Name) {
					continue
				}
				write(listLine(false, 1024, now, files.ConvDirName(ar.Name), nlst))
			}
		}
		if area > 0 && group > 0 {
			a, ok := files.FindArea(ss.srv.areas, uint16(area))
			if ok && files.CheckFileAreaAccess(a, ss.user, false, false, false, true, uint16(group)) {
				if ss.srv.cfg.IndexName != "" {
					write(listLine(true, 0, now, files.ConvDirName(ss.srv.cfg.IndexName), nlst))
				}
				ents, _ := files.ReadFDB(ss.srv.cfg.G, a)
				for _, e := range ents {
					if e.Hdr.Deleted() || e.Hdr.Unlisted() || e.Hdr.Comment() || e.Hdr.Missing() || e.Hdr.NotAvail() || e.Hdr.Password != "" {
						continue
					}
					long := files.LongName(ss.srv.cfg.G, a, e.Hdr)
					disp := files.ConvDirName(long)
					if !files.MatchWild(wild, long) && !files.MatchWild(wild, e.Hdr.Name) && !files.MatchWild(wild, disp) {
						continue
					}
					shown := disp
					if nlst {
						shown = long
					}
					dt := files.UnpackDOSTime(e.Hdr.FileDate)
					ds := now
					if !dt.IsZero() {
						ds = dt.Format("Jan 02  2006")
					}
					write(listLine(true, int64(e.Hdr.Size), ds, shown, nlst))
				}
			}
		}
	}
	ss.reply(226, "Transfer complete")
}

func (ss *session) findFile(name string) (cfgrec.FilesArea, files.Entry, bool) {
	name = strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
	_, areaNum, wild, invalid := ss.partsFromDir(name, ss.group, ss.area)
	if invalid || areaNum <= 0 || wild == "" {
		return cfgrec.FilesArea{}, files.Entry{}, false
	}
	a, ok := files.FindArea(ss.srv.areas, uint16(areaNum))
	if !ok {
		return cfgrec.FilesArea{}, files.Entry{}, false
	}
	ents, err := files.ReadFDB(ss.srv.cfg.G, a)
	if err != nil {
		return a, files.Entry{}, false
	}
	bjork := strings.ReplaceAll(wild, "_", "?")
	for _, e := range ents {
		if e.Hdr.Deleted() || e.Hdr.Comment() {
			continue
		}
		long := files.LongName(ss.srv.cfg.G, a, e.Hdr)
		cands := []string{e.Hdr.Name, files.ConvDirName(e.Hdr.Name), long, files.ConvDirName(long)}
		for _, cand := range cands {
			if cand == "" {
				continue
			}
			if strings.EqualFold(cand, wild) || files.MatchWild(wild, cand) || files.MatchWild(bjork, cand) {
				return a, e, true
			}
		}
	}
	return a, files.Entry{}, false
}

func (ss *session) diskPath(a cfgrec.FilesArea, e files.Entry) string {
	long := files.LongName(ss.srv.cfg.G, a, e.Hdr)
	if p := files.FileOnDisk(ss.srv.cfg.G, a, long); p != "" {
		return p
	}
	if p := files.FileOnDisk(ss.srv.cfg.G, a, e.Hdr.Name); p != "" {
		return p
	}
	if long != "" && !strings.EqualFold(long, e.Hdr.Name) {
		return files.DiskFile(a, long)
	}
	return files.DiskFile(a, e.Hdr.Name)
}

func (ss *session) cmdRetr(name string) {
	a, e, ok := ss.findFile(name)
	if !ok || !files.DownloadAccess(a, ss.user) || !ftpFileDownloadOK(e) {
		ss.reply(550, "File not found")
		return
	}
	path := ss.diskPath(a, e)
	f, err := os.Open(path)
	if err != nil {
		ss.reply(550, "File not found")
		return
	}
	defer f.Close()
	if ss.rest > 0 {
		_, _ = f.Seek(ss.rest, io.SeekStart)
	}
	ss.reply(150, "Opening data connection")
	dc, err := ss.openData()
	if err != nil {
		ss.reply(425, "Unable to open data connection")
		ss.rest = 0
		return
	}
	n, copyErr := io.Copy(dc, f)
	_ = dc.Close()
	ss.rest = 0
	if copyErr != nil {
		ss.reply(426, "Transfer aborted")
		return
	}
	ss.reply(226, "Transfer complete")
	logx.Write(ss.srv.cfg.G, 0, '>', fmt.Sprintf("[FTPSERV] [%s] Sending %s (%d bytes)", host(ss.c), e.Hdr.Name, n))
	ss.xferLog("RETR", e.Hdr.Name, n)
}

func ftpFileDownloadOK(e files.Entry) bool {
	if e.Hdr.Deleted() || e.Hdr.NotAvail() || e.Hdr.Missing() || e.Hdr.Password != "" {
		return false
	}
	return true
}

func (ss *session) cmdStor(name string) {
	if ss.area <= 0 {
		ss.reply(550, "CWD into a file area first")
		return
	}
	a, ok := files.FindArea(ss.srv.areas, uint16(ss.area))
	g, gok := files.FindGroup(ss.srv.grps, uint16(ss.group))
	if !ok || !gok || !files.GroupAccess(g, ss.user) || !files.UploadAccess(a, ss.user) {
		ss.reply(550, "Permission denied")
		return
	}
	if a.UploadArea > 0 {
		ua, uok := files.FindArea(ss.srv.areas, a.UploadArea)
		if !uok {
			ss.reply(550, "Permission denied")
			return
		}
		a = ua
	}
	name = filepath.Base(name)
	dest := files.DiskFile(a, name)
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	flag := os.O_CREATE | os.O_WRONLY
	if ss.rest > 0 {
		flag |= os.O_RDWR
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flag, 0644)
	if err != nil {
		ss.reply(550, "Cannot create file")
		return
	}
	if ss.rest > 0 {
		_, _ = f.Seek(ss.rest, io.SeekStart)
	}
	ss.reply(150, "Opening data connection")
	dc, err := ss.openData()
	if err != nil {
		_ = f.Close()
		ss.reply(425, "Unable to open data connection")
		ss.rest = 0
		return
	}
	n, copyErr := io.Copy(f, dc)
	_ = dc.Close()
	_ = f.Close()
	ss.rest = 0
	if copyErr != nil {
		ss.reply(426, "Transfer aborted")
		return
	}
	up := ss.user.Name
	if up == "" {
		up = "Anonymous"
	}
	_ = files.AfterUpload(ss.srv.cfg.G, a, dest, up)
	ss.reply(226, "Transfer complete")
	logx.Write(ss.srv.cfg.G, 0, '>', fmt.Sprintf("[FTPSERV] [%s] Received %s (%d bytes)", host(ss.c), name, n))
	ss.xferLog("STOR", name, n)
}

func (ss *session) cmdSize(name string) {
	_, e, ok := ss.findFile(name)
	if !ok {
		ss.reply(550, "File not found")
		return
	}
	ss.reply(213, fmt.Sprintf("%d", e.Hdr.Size))
}

func (ss *session) cmdMdtm(name string) {
	_, e, ok := ss.findFile(name)
	if !ok {
		ss.reply(550, "File not found")
		return
	}
	dt := files.UnpackDOSTime(e.Hdr.FileDate)
	if dt.IsZero() {
		dt = time.Now()
	}
	ss.reply(213, dt.UTC().Format("20060102150405"))
}

func (ss *session) cmdDele(name string) {
	a, e, ok := ss.findFile(name)
	if !ok || !files.UploadAccess(a, ss.user) {
		ss.reply(550, "File not found")
		return
	}
	ents, err := files.ReadFDB(ss.srv.cfg.G, a)
	if err != nil {
		ss.reply(550, "Cannot update file list")
		return
	}
	for i := range ents {
		if ents[i].Hdr.RecordNum == e.Hdr.RecordNum {
			h := ents[i].Hdr
			h.Attrib |= cfgrec.AttrDeleted
			_ = files.WriteHdr(ss.srv.cfg.G, a, i, h)
			break
		}
	}
	_ = os.Remove(ss.diskPath(a, e))
	ss.reply(250, "File deleted")
}

func (ss *session) cmdHelp() {
	_ = serv.WriteLine(ss.c, "214-The following commands are recognized")
	for _, c := range []string{"CWD", "DELE", "EPSV", "LIST", "MDTM", "NLST", "NOOP", "PASS", "PASV", "PORT", "PWD", "QUIT", "REST", "RETR", "SIZE", "STOR", "SYST", "TYPE", "USER"} {
		_ = serv.WriteLine(ss.c, "   "+c)
	}
	ss.reply(214, "HELP command successful")
}

func (ss *session) xferLog(op, name string, n int64) {
	if ss.srv.cfg.XferLog == "" {
		return
	}
	f, err := os.OpenFile(ss.srv.cfg.XferLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s %s %d\r\n", time.Now().Format(time.RFC3339), host(ss.c), op, name, n)
}

func listLine(isFile bool, size int64, date, name string, nlst bool) string {
	if nlst {
		return name
	}
	acc := "dr-xr-xr-x"
	if isFile {
		acc = "-r--r--r--"
	}
	return fmt.Sprintf("%s   1 user     group    %10d %s %s", acc, size, date, name)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func host(c net.Conn) string {
	h, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	return h
}

func trim(s string) string { return strings.TrimSpace(s) }

func servAtoi(s string) int { return serv.ParseInt(s) }

// pickPasvPort stays in [lo, lo+span). UnixNano must not be narrowed to 32-bit int.
func pickPasvPort(lo, span int, nano int64, i int) int {
	if span <= 0 {
		return lo
	}
	return lo + int((uint64(nano)+uint64(uint32(i)))%uint64(span))
}
