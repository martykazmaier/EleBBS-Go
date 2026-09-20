package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func TestParseServerTLS(t *testing.T) {
	s := ParseServer("pop.example.com:995", DefaultPOP3)
	if s.Host != "pop.example.com" || s.Port != 995 || s.TLS != TLSImplicit {
		t.Fatalf("POP3S: %+v", s)
	}
	s = ParseServer("smtp.example.com:465", DefaultSMTP)
	if s.Port != 465 || s.TLS != TLSImplicit {
		t.Fatalf("SMTPS: %+v", s)
	}
	s = ParseServer("smtp.example.com:587", DefaultSMTP)
	if s.TLS != TLSStart {
		t.Fatalf("587 STARTTLS: %+v", s)
	}
	s = ParseServer("mail.example.com:110s", DefaultPOP3)
	if s.Port != 110 || s.TLS != TLSImplicit {
		t.Fatalf("suffix s: %+v", s)
	}
	s = ParseServer("mail.example.com:25t", DefaultSMTP)
	if s.Port != 25 || s.TLS != TLSStart {
		t.Fatalf("suffix t: %+v", s)
	}
	s = ParseServer("plain.example.com", DefaultPOP3)
	if s.Port != 110 || s.TLS != TLSOff {
		t.Fatalf("plain: %+v", s)
	}
}

func TestParseMailerArgs(t *testing.T) {
	o := ParseMailerArgs([]string{"-R", "-T", "-Hpop.gmail.com:995", "-Ume@secret", "-A12", "-O"})
	if !o.Get || !o.Toss || o.MsgArea != 12 || !o.InsecureTLS {
		t.Fatalf("flags: %+v", o)
	}
	if o.POP3.Host != "pop.gmail.com" || o.POP3.TLS != TLSImplicit || o.POP3User != "me" || o.POP3Pass != "secret" {
		t.Fatalf("pop3: %+v", o)
	}
	o = ParseMailerArgs([]string{"-P", "-Ismtp.gmail.com:465", "-Wme@gmail.com@secret"})
	if !o.Send || o.SMTP.TLS != TLSImplicit || o.SMTPUser != "me@gmail.com" || o.SMTPPass != "secret" {
		t.Fatalf("smtp: %+v", o)
	}
}

func TestExtractToName(t *testing.T) {
	msg := []byte("From: a@b.c\r\nTo: \"Jane Doe\" <jane.doe@bbs.example>\r\nSubject: hi\r\n\r\nbody\r\n")
	if got := ExtractToName(msg, true, "To:"); got != "jane doe" {
		t.Fatalf("mail addr %q", got)
	}
	if got := ExtractToName(msg, false, "To:"); got != "Jane Doe" {
		t.Fatalf("display %q", got)
	}
	if got := GetRawEmail(headerValue("To:", msg)); got != "jane.doe@bbs.example" {
		t.Fatalf("raw %q", got)
	}
}

func TestNewsArticleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, EmailInFile)
	body := []byte("From: a@b.c\r\nTo: x@y.z\r\n\r\nHello\r\n")
	if err := AddMsgToBase(p, "Sysop", 7, 3, body, true); err != nil {
		t.Fatal(err)
	}
	arts, err := readNewsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("n=%d", len(arts))
	}
	a := arts[0]
	if a.GroupName != "Sysop" || a.AreaNum != 7 || a.ArticleNr != 3 || !a.Email() {
		t.Fatalf("%+v", a)
	}
	if !strings.Contains(string(nulTrim(a.Body)), "Hello") {
		t.Fatalf("body %q", a.Body)
	}
}

func TestMakeHostEmail(t *testing.T) {
	if got := MakeHostEmail("bbs.example", "Jane Doe", cfgrec.Addr{}); got != "jane.doe@bbs.example" {
		t.Fatalf("%q", got)
	}
	if got := MakeHostEmail("", "x", cfgrec.Addr{Zone: 1, Net: 2, Node: 3, Point: 0}); got != "x@p0.f3.n2.z1.fidonet.org" {
		t.Fatalf("%q", got)
	}
}

func TestPOP3SCollectAndSMTPSSend(t *testing.T) {
	cert, err := selfSigned()
	if err != nil {
		t.Fatal(err)
	}
	msg := "From: alice@example.com\r\nTo: \"Sysop\" <sysop@bbs.example>\r\nSubject: hi\r\n\r\nHello there\r\n"
	popLn := tlsListen(t, cert, func(c net.Conn) {
		br := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		readLine := func() string {
			s, _ := br.ReadString('\n')
			return strings.TrimRight(s, "\r\n")
		}
		w("+OK pop3s ready")
		if !strings.HasPrefix(readLine(), "USER") {
			return
		}
		w("+OK")
		if !strings.HasPrefix(readLine(), "PASS") {
			return
		}
		w("+OK")
		if !strings.HasPrefix(readLine(), "STAT") {
			return
		}
		w("+OK 1 80")
		if !strings.HasPrefix(readLine(), "RETR") {
			return
		}
		w("+OK")
		for _, ln := range strings.Split(strings.TrimRight(strings.ReplaceAll(msg, "\r\n", "\n"), "\n"), "\n") {
			w(ln)
		}
		w(".")
		_ = readLine()
		w("+OK")
		_ = readLine()
		w("+OK bye")
	})
	defer popLn.Close()

	gotDATA := make(chan string, 1)
	smtpLn := tlsListen(t, cert, func(c net.Conn) {
		br := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		readLine := func() string {
			s, err := br.ReadString('\n')
			if err != nil {
				return ""
			}
			return strings.TrimRight(s, "\r\n")
		}
		w("220 smtps ready")
		for {
			ln := readLine()
			if ln == "" {
				return
			}
			switch {
			case strings.HasPrefix(ln, "EHLO"), strings.HasPrefix(ln, "HELO"):
				w("250-hello")
				w("250 AUTH PLAIN LOGIN")
			case strings.HasPrefix(ln, "AUTH"):
				w("235 2.7.0 OK")
			case strings.HasPrefix(ln, "MAIL"):
				w("250 OK")
			case strings.HasPrefix(ln, "RCPT"):
				w("250 OK")
			case strings.HasPrefix(ln, "DATA"):
				w("354 go")
				var b strings.Builder
				for {
					s, err := br.ReadString('\n')
					b.WriteString(s)
					if strings.TrimRight(s, "\r\n") == "." || err != nil {
						break
					}
				}
				gotDATA <- b.String()
				w("250 queued")
			case strings.HasPrefix(ln, "QUIT"):
				w("221 bye")
				return
			default:
				w("250 OK")
			}
		}
	})
	defer smtpLn.Close()

	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = pascal.ForceBack(dir)
	g.RaConfig.Sysop = "Sysop"
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")

	users := make([]byte, cfgrec.UsersSize)
	pascal.PutString(users[0:36], "Sysop")
	if err := os.WriteFile(filepath.Join(dir, cfgrec.UserBaseName), users, 0644); err != nil {
		t.Fatal(err)
	}

	popHost, popPort, _ := net.SplitHostPort(popLn.Addr().String())
	o := MailerOptions{
		Get:         true,
		Toss:        false,
		MsgArea:     1,
		InsecureTLS: true,
		BounceName:  "Sysop",
		POP3:        ServerSpec{Host: popHost, Port: atoiMail(popPort), TLS: TLSImplicit},
		POP3User:    "me",
		POP3Pass:    "pw",
	}
	if err := collectPOP3(g, o); err != nil {
		t.Fatal(err)
	}
	arts, err := readNewsFile(EmailInPath(g))
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("collected %d", len(arts))
	}
	if !strings.Contains(string(arts[0].Body), "Hello there") {
		t.Fatalf("body %q", arts[0].Body)
	}

	smtpHost, smtpPort, _ := net.SplitHostPort(smtpLn.Addr().String())
	out := EmailOutPath(g)
	if err := AddMsgToBase(out, "out", 1, 1, []byte(msg), true); err != nil {
		t.Fatal(err)
	}
	so := MailerOptions{
		Send:        true,
		InsecureTLS: true,
		EmailHost:   "bbs.example",
		SMTP:        ServerSpec{Host: smtpHost, Port: atoiMail(smtpPort), TLS: TLSImplicit},
		SMTPUser:    "me",
		SMTPPass:    "pw",
	}
	if err := sendSMTP(g, so); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-gotDATA:
		if !strings.Contains(data, "Hello there") {
			t.Fatalf("smtp data %q", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no SMTPS DATA")
	}
}

func tlsListen(t *testing.T, cert tls.Certificate, handle func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		handle(c)
	}()
	return ln
}

func selfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func TestRouteUnknownDroppedUnlessForward(t *testing.T) {
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = t.TempDir()
	body := []byte("To: nobody@x\r\n\r\nhi\r\n")
	user, _, post := routeInbound(g, MailerOptions{BounceName: "Sysop"}, body)
	if post || user != "" {
		t.Fatalf("default should drop unknown, got %q post=%v", user, post)
	}
	user, _, post = routeInbound(g, MailerOptions{TrashBounces: true, BounceName: "Sysop"}, body)
	if !post || user != "Sysop" {
		t.Fatalf("forward %q post=%v", user, post)
	}
}

func TestFwdLeave(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, MailFwdFile), []byte("LEAVE spam@example.com\r\nFWD alias Sysop 4\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	leave := []byte("To: spam@example.com\r\n\r\nx\r\n")
	if !tryLeaveOnServer(g, leave) {
		t.Fatal("LEAVE")
	}
	fwd := []byte("To: alias@host\r\n\r\nx\r\n")
	if !tryAliases(g, "alias", fwd, 1, filepath.Join(dir, EmailInFile)) {
		t.Fatal("FWD")
	}
	arts, _ := readNewsFile(filepath.Join(dir, EmailInFile))
	if len(arts) != 1 || arts[0].GroupName != "Sysop" || arts[0].AreaNum != 4 {
		t.Fatalf("%+v", arts)
	}
}

func TestSMTPStartTLSPort587(t *testing.T) {
	cert, err := selfSigned()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	gotDATA := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		w := func(conn net.Conn, s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		read := func() string {
			s, err := br.ReadString('\n')
			if err != nil {
				return ""
			}
			return strings.TrimRight(s, "\r\n")
		}
		w(c, "220-mail.example ESMTP")
		w(c, "220 ready")
		if !strings.HasPrefix(read(), "EHLO") {
			return
		}
		w(c, "250-hello")
		w(c, "250-STARTTLS")
		w(c, "250 AUTH PLAIN")
		if !strings.HasPrefix(read(), "STARTTLS") {
			return
		}
		w(c, "220 go")
		tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
		if err := tc.Handshake(); err != nil {
			return
		}
		defer tc.Close()
		br = bufio.NewReader(tc)
		c2 := net.Conn(tc)
		if !strings.HasPrefix(read(), "EHLO") {
			return
		}
		w(c2, "250-hello")
		w(c2, "250 AUTH PLAIN")
		if !strings.HasPrefix(read(), "AUTH") {
			return
		}
		w(c2, "235 ok")
		for {
			ln := read()
			if ln == "" {
				return
			}
			switch {
			case strings.HasPrefix(ln, "MAIL"):
				w(c2, "250 OK")
			case strings.HasPrefix(ln, "RCPT"):
				w(c2, "250 OK")
			case strings.HasPrefix(ln, "DATA"):
				w(c2, "354 go")
				var b strings.Builder
				for {
					s, err := br.ReadString('\n')
					b.WriteString(s)
					if strings.TrimRight(s, "\r\n") == "." || err != nil {
						break
					}
				}
				gotDATA <- b.String()
				w(c2, "250 queued")
			case strings.HasPrefix(ln, "RSET"):
				w(c2, "250 OK")
			case strings.HasPrefix(ln, "QUIT"):
				w(c2, "221 bye")
				return
			default:
				w(c2, "250 OK")
			}
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	cli, err := newSMTP(ServerSpec{Host: host, Port: atoiMail(port), TLS: TLSStart}, true, "bbs.example", "me", "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	body := []byte("From: a@b.c\r\nTo: c@d.e\r\n\r\nHello 587\r\n")
	if err := cli.MailFrom("a@b.c"); err != nil {
		t.Fatal(err)
	}
	if err := cli.RcptTo("c@d.e"); err != nil {
		t.Fatal(err)
	}
	if err := cli.Data(body, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-gotDATA:
		if !strings.Contains(data, "Hello 587") {
			t.Fatalf("data %q", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no STARTTLS DATA")
	}
}
