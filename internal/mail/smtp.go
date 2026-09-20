package mail

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	smtpDialTimeout = 30 * time.Second
	smtpCmdTimeout  = 60 * time.Second
	smtpIdleGrace   = 5 * time.Second
)

type smtpClient struct {
	c     net.Conn
	r     *bufio.Reader
	last  string
	caps  string
	host  string
	insec bool
	start bool
}

type prefixConn struct {
	net.Conn
	prefix []byte
}

func (p *prefixConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}

func newSMTP(spec ServerSpec, insecure bool, domain, user, pass string) (*smtpClient, error) {
	c, err := dialMail(spec, insecure, smtpDialTimeout)
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
	s := &smtpClient{c: c, r: bufio.NewReaderSize(c, 64*1024), host: spec.Host, insec: insecure, start: spec.TLS == TLSStart}
	if _, err := s.readGreeting(); err != nil {
		c.Close()
		return nil, fmt.Errorf("SMTP banner from %s: %w", spec.Addr(), err)
	}
	if err := s.session(domain, user, pass); err != nil {
		c.Close()
		return nil, err
	}
	return s, nil
}

func (s *smtpClient) Close() {
	if s == nil || s.c == nil {
		return
	}
	_, _ = s.cmd("QUIT")
	_ = s.c.Close()
	s.c = nil
}

func (s *smtpClient) setTo(d time.Duration) {
	_ = s.c.SetDeadline(time.Now().Add(d))
}

func (s *smtpClient) readLine() (string, error) {
	s.setTo(smtpCmdTimeout)
	ln, err := s.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(ln, "\r\n"), nil
}

func smtpComplete(ln string) bool {
	if len(ln) < 4 {
		return true
	}
	return ln[3] != '-'
}

func (s *smtpClient) readGreeting() (int, error) {
	s.setTo(smtpCmdTimeout)
	first, err := s.r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	first = strings.TrimRight(first, "\r\n")
	all := []string{first}
	for !smtpComplete(first) {
		s.setTo(smtpIdleGrace)
		ln, err := s.r.ReadString('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return 0, err
		}
		ln = strings.TrimRight(ln, "\r\n")
		all = append(all, ln)
		first = ln
	}
	s.last = strings.Join(all, "\n")
	s.caps += "\n" + s.last
	if len(all[0]) < 3 {
		return 0, fmt.Errorf("empty SMTP greeting")
	}
	code, _ := strconv.Atoi(all[0][:3])
	if code >= 400 {
		return code, fmt.Errorf("%s", s.last)
	}
	return code, nil
}

func (s *smtpClient) readResp() (code int, err error) {
	var all []string
	for {
		ln, err := s.readLine()
		if err != nil {
			return 0, err
		}
		all = append(all, ln)
		if smtpComplete(ln) {
			break
		}
	}
	s.last = strings.Join(all, "\n")
	s.caps += "\n" + s.last
	if len(all) == 0 || len(all[0]) < 3 {
		return 0, fmt.Errorf("empty SMTP response")
	}
	code, _ = strconv.Atoi(all[0][:3])
	if code >= 400 {
		return code, fmt.Errorf("%s", all[len(all)-1])
	}
	return code, nil
}

func (s *smtpClient) cmd(line string) (int, error) {
	s.setTo(smtpCmdTimeout)
	if _, err := io.WriteString(s.c, line+"\r\n"); err != nil {
		return 0, err
	}
	return s.readResp()
}

func (s *smtpClient) hasCap(name string) bool {
	return strings.Contains(strings.ToUpper(s.caps), strings.ToUpper(name))
}

func (s *smtpClient) alreadyTLS() bool {
	_, ok := s.c.(*tls.Conn)
	return ok
}

func (s *smtpClient) session(domain, user, pass string) error {
	if err := s.hello(domain); err != nil {
		return err
	}
	if !s.alreadyTLS() && (s.start || s.hasCap("STARTTLS")) {
		if err := s.startTLS(); err != nil {
			return err
		}
		s.caps = ""
		if err := s.hello(domain); err != nil {
			return err
		}
	}
	if user != "" {
		if err := s.Auth(user, pass); err != nil {
			return err
		}
	}
	return nil
}

func (s *smtpClient) hello(domain string) error {
	if domain == "" {
		domain = "localhost"
	}
	s.caps = ""
	code, err := s.cmd("EHLO " + domain)
	if err != nil || code != 250 {
		s.caps = ""
		code, err = s.cmd("HELO " + domain)
		if err != nil {
			return fmt.Errorf("HELO: %w", err)
		}
		if code != 250 {
			return fmt.Errorf("HELO: %s", s.last)
		}
	}
	return nil
}

func (s *smtpClient) startTLS() error {
	if s.alreadyTLS() {
		return nil
	}
	code, err := s.cmd("STARTTLS")
	if err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}
	if code != 220 {
		return fmt.Errorf("STARTTLS: %s", s.last)
	}
	raw := s.c
	if n := s.r.Buffered(); n > 0 {
		buf := make([]byte, n)
		_, _ = io.ReadFull(s.r, buf)
		raw = &prefixConn{Conn: s.c, prefix: buf}
	}
	s.setTo(smtpCmdTimeout)
	tc, err := tlsWrap(raw, s.host, s.insec)
	if err != nil {
		return err
	}
	s.c = tc
	s.r = bufio.NewReaderSize(tc, 64*1024)
	return nil
}

func (s *smtpClient) Auth(user, pass string) error {
	if user == "" {
		return nil
	}
	plain := base64.StdEncoding.EncodeToString(append(append(append([]byte{0}, user...), 0), pass...))
	code, err := s.cmd("AUTH PLAIN " + plain)
	if err == nil && (code == 235 || code == 250) {
		return nil
	}
	if code, err = s.cmd("AUTH LOGIN"); err != nil {
		return fmt.Errorf("AUTH: %w", err)
	}
	if code != 334 {
		return fmt.Errorf("AUTH LOGIN: %s", s.last)
	}
	if _, err := s.cmd(base64.StdEncoding.EncodeToString([]byte(user))); err != nil {
		return fmt.Errorf("AUTH user: %w", err)
	}
	code, err = s.cmd(base64.StdEncoding.EncodeToString([]byte(pass)))
	if err != nil {
		return fmt.Errorf("AUTH pass: %w", err)
	}
	if code != 235 && code != 250 {
		return fmt.Errorf("AUTH: %s", s.last)
	}
	return nil
}

func (s *smtpClient) MailFrom(from string) error {
	code, err := s.cmd("MAIL FROM:<" + from + ">")
	if err != nil {
		return err
	}
	if code != 250 {
		return fmt.Errorf("MAIL FROM: %s", s.last)
	}
	return nil
}

func (s *smtpClient) RcptTo(to string) error {
	code, err := s.cmd("RCPT TO:<" + strings.TrimSpace(to) + ">")
	if err != nil {
		return err
	}
	if code != 250 && code != 251 {
		return fmt.Errorf("RCPT TO: %s", s.last)
	}
	return nil
}

func (s *smtpClient) Data(body []byte, replyTo string) error {
	code, err := s.cmd("DATA")
	if err != nil {
		return err
	}
	if code != 354 {
		return fmt.Errorf("DATA: %s", s.last)
	}
	var payload strings.Builder
	if replyTo != "" {
		payload.WriteString("Reply-To: <" + replyTo + ">\r\n")
	}
	text := string(nulTrim(body))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		payload.WriteString(line)
		payload.WriteString("\r\n")
	}
	payload.WriteString(".\r\n")
	s.setTo(smtpCmdTimeout)
	if _, err := io.WriteString(s.c, payload.String()); err != nil {
		return err
	}
	code, err = s.readResp()
	if err != nil {
		return err
	}
	if code != 250 {
		return fmt.Errorf("DATA end: %s", s.last)
	}
	_, _ = s.cmd("RSET")
	return nil
}
