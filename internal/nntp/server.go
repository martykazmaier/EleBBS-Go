package nntp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/serv"
	"elebbs/internal/userbase"
)

type Config struct {
	G     *cfgrec.GlobalCfg
	Port  int
	TLS   *tls.Config
	Limit int
}

func Listen(cfg Config) error {
	if cfg.Port <= 0 {
		cfg.Port = 563
	}
	if cfg.TLS == nil {
		return fmt.Errorf("NNTPS requires a TLS certificate (-CERT)")
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 50
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", cfg.Port), cfg.TLS)
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%sNNTPS listening on :%d\n", cfgrec.SystemMsgPrefix, cfg.Port)
	ns := config.LoadNewsServer(cfg.G)
	groups := mail.NewsGroups(cfg.G)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(cfg, ns, groups, c)
	}
}

type session struct {
	cfg     Config
	ns      cfgrec.NewsServer
	groups  []mail.NewsGroup
	c       net.Conn
	r       *bufio.Reader
	user    cfgrec.User
	authed  bool
	pending string
	cur     mail.NewsGroup
	arts    []mail.Article
	curNum  int
}

func handle(cfg Config, ns cfgrec.NewsServer, groups []mail.NewsGroup, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Minute))
	ss := &session{cfg: cfg, ns: ns, groups: groups, c: c, r: bufio.NewReader(c)}
	sys := cfg.G.RaConfig.SystemName
	if sys == "" {
		sys = "EleBBS"
	}
	ss.line("200 EleBBS news server running at %s ready - posting allowed", sys)
	logx.Write(cfg.G, 0, '>', "[NEWSSRV] ["+remote(c)+"] NNTPS connection opened")
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
	logx.Write(cfg.G, 0, '>', "[NEWSSRV] ["+remote(c)+"] Connection closed")
}

func (ss *session) dispatch(cmd, arg string) bool {
	switch cmd {
	case "QUIT":
		ss.line("205 Closing connection - don't forget to hug your Sysop today!")
		return false
	case "MODE":
		ss.line("200 Mode reader")
	case "HELP":
		ss.line("100 Help text follows")
		for _, h := range []string{"ARTICLE", "BODY", "HEAD", "STAT", "GROUP", "HELP", "LAST", "LIST", "NEXT", "XOVER", "POST", "AUTHINFO", "QUIT"} {
			ss.line("%s", h)
		}
		ss.line(".")
	case "AUTHINFO":
		ss.cmdAuth(arg)
	case "LIST":
		ss.cmdList(arg)
	case "GROUP":
		ss.cmdGroup(arg)
	case "ARTICLE", "HEAD", "BODY", "STAT":
		ss.cmdArticle(cmd, arg)
	case "NEXT", "LAST":
		ss.cmdMove(cmd)
	case "XOVER":
		ss.cmdXover(arg)
	case "POST":
		ss.cmdPost()
	case "DATE":
		ss.line("111 %s", time.Now().UTC().Format("20060102150405"))
	case "CAPABILITIES":
		ss.line("101 Capability list:")
		ss.line("VERSION 2")
		ss.line("READER")
		ss.line("POST")
		ss.line("OVER")
		ss.line("AUTHINFO USER")
		ss.line(".")
	default:
		ss.line("500 Unknown command")
	}
	return true
}

func (ss *session) cmdAuth(arg string) {
	kind, rest := serv.SplitCmd(arg)
	switch kind {
	case "USER":
		ss.pending = strings.TrimSpace(rest)
		ss.line("381 More authentication information required")
	case "PASS":
		u, ok := userbase.Search(ss.cfg.G, ss.pending)
		if !ok || !userbase.CheckPassword(u, rest, ss.cfg.G.RaConfig.StrictPwdChecking) {
			ss.line("481 Authentication rejected")
			return
		}
		ss.user, ss.authed = u, true
		ss.line("281 Authentication accepted")
	default:
		ss.line("500 Unknown AUTHINFO")
	}
}

func (ss *session) cmdList(arg string) {
	if strings.EqualFold(strings.TrimSpace(arg), "OVERVIEW.FMT") {
		ss.line("215 Order of fields in overview database")
		ss.line("Subject:")
		ss.line("From:")
		ss.line("Date:")
		ss.line("Message-ID:")
		ss.line("References:")
		ss.line("Bytes:")
		ss.line("Lines:")
		ss.line(".")
		return
	}
	if ss.ns.AuthForList() && !ss.authed {
		ss.line("480 Authentication required")
		return
	}
	ss.line("215 List of newsgroups follow")
	for _, g := range ss.groups {
		arts, _ := mail.ReadJAM(g.Area.JAMBase)
		lo, hi := mail.HighLow(arts)
		post := "n"
		if g.Posting {
			post = "y"
		}
		ss.line("%s %d %d %s", g.Name, hi, lo, post)
	}
	ss.line(".")
}

func (ss *session) cmdGroup(name string) {
	g, ok := mail.FindNewsGroup(ss.groups, name)
	if !ok {
		ss.line("411 No such newsgroup")
		return
	}
	arts, err := mail.ReadJAM(g.Area.JAMBase)
	if err != nil {
		arts = nil
	}
	ss.cur, ss.arts = g, arts
	lo, hi := mail.HighLow(arts)
	ss.curNum = lo
	ss.line("211 %d %d %d %s", len(arts), lo, hi, g.Name)
}

func (ss *session) cmdArticle(kind, arg string) {
	if ss.cur.Name == "" {
		ss.line("412 No newsgroup selected")
		return
	}
	var a mail.Article
	var ok bool
	arg = strings.TrimSpace(arg)
	if arg == "" {
		a, ok = mail.FindArticle(ss.arts, ss.curNum)
	} else if strings.Contains(arg, "@") || strings.HasPrefix(arg, "<") {
		a, ok = mail.FindArticleID(ss.arts, arg)
	} else {
		n, _ := strconv.Atoi(arg)
		a, ok = mail.FindArticle(ss.arts, n)
	}
	if !ok {
		ss.line("430 No such article")
		return
	}
	ss.curNum = a.Num
	id := strings.Trim(a.MsgID, "<>")
	switch kind {
	case "STAT":
		ss.line("223 %d <%s>", a.Num, id)
	case "HEAD":
		ss.line("221 %d <%s>", a.Num, id)
		ss.writeHead(a)
		ss.line(".")
	case "BODY":
		ss.line("222 %d <%s>", a.Num, id)
		ss.writeBody(a.Body)
		ss.line(".")
	default:
		ss.line("220 %d <%s>", a.Num, id)
		ss.writeHead(a)
		ss.line("")
		ss.writeBody(a.Body)
		ss.line(".")
	}
}

func (ss *session) writeHead(a mail.Article) {
	dom := ss.ns.DomainName
	ss.line("X-News-Server: EleBBS NNTPS")
	ss.line("Date: %s", mail.NNTPDate(a.Date))
	ss.line("Message-ID: <%s>", strings.Trim(a.MsgID, "<>"))
	if a.ReplyID != "" {
		ss.line("References: <%s>", strings.Trim(a.ReplyID, "<>"))
	}
	ss.line("From: %s", mail.HostEmail(a.From, dom))
	ss.line("Newsgroups: %s", ss.cur.Name)
	ss.line("Subject: %s", a.Subject)
	for _, k := range a.Kludges {
		if strings.HasPrefix(strings.ToUpper(k), "RFC850-") {
			ss.line("%s", k[7:])
		}
	}
}

func (ss *session) writeBody(body string) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	for _, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(ln, ".") {
			ln = "." + ln
		}
		ss.line("%s", ln)
	}
}

func (ss *session) cmdMove(dir string) {
	if ss.cur.Name == "" {
		ss.line("412 No newsgroup selected")
		return
	}
	step := 1
	if dir == "LAST" {
		step = -1
	}
	next := ss.curNum + step
	a, ok := mail.FindArticle(ss.arts, next)
	if !ok {
		for _, art := range ss.arts {
			if step > 0 && art.Num > ss.curNum && (!ok || art.Num < a.Num) {
				a, ok = art, true
			}
			if step < 0 && art.Num < ss.curNum && (!ok || art.Num > a.Num) {
				a, ok = art, true
			}
		}
	}
	if !ok {
		if dir == "LAST" {
			ss.line("422 No previous article")
		} else {
			ss.line("421 No next article")
		}
		return
	}
	ss.curNum = a.Num
	ss.line("223 %d <%s>", a.Num, strings.Trim(a.MsgID, "<>"))
}

func (ss *session) cmdXover(arg string) {
	if ss.cur.Name == "" {
		ss.line("412 No newsgroup selected")
		return
	}
	lo, hi := mail.HighLow(ss.arts)
	if arg != "" {
		if i := strings.IndexByte(arg, '-'); i >= 0 {
			lo, _ = strconv.Atoi(arg[:i])
			if rest := arg[i+1:]; rest != "" {
				hi, _ = strconv.Atoi(rest)
			}
		} else {
			lo, _ = strconv.Atoi(arg)
			hi = lo
		}
	}
	ss.line("224 Overview information follows")
	dom := ss.ns.DomainName
	for _, a := range ss.arts {
		if a.Num < lo || a.Num > hi {
			continue
		}
		ss.line("%d\t%s\t%s\t%s\t<%s>\t%s\t%d\t%d",
			a.Num, a.Subject, mail.HostEmail(a.From, dom), mail.NNTPDate(a.Date),
			strings.Trim(a.MsgID, "<>"), "", a.Bytes, a.Lines)
	}
	ss.line(".")
}

func (ss *session) cmdPost() {
	if ss.cur.Name != "" && !ss.cur.Posting {
		ss.line("440 Posting not allowed")
		return
	}
	ss.line("340 Send article to be posted. End with <CR-LF>.<CR-LF>")
	var buf strings.Builder
	for {
		ln, err := serv.ReadLine(ss.r)
		if err != nil {
			ss.line("441 Posting failed")
			return
		}
		if ln == "." {
			break
		}
		if strings.HasPrefix(ln, "..") {
			ln = ln[1:]
		}
		buf.WriteString(ln)
		buf.WriteByte('\n')
	}
	_ = buf.String()
	ss.line("441 Posting to JAM is not enabled in this build")
}

func (ss *session) line(format string, args ...any) {
	_ = serv.WriteLine(ss.c, fmt.Sprintf(format, args...))
}

func remote(c net.Conn) string {
	h, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	return h
}
