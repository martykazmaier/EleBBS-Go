package mail

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type pop3Client struct {
	c    net.Conn
	r    *bufio.Reader
	last string
	user string
	pass string
}

func newPOP3(spec ServerSpec, user, pass string, insecure bool) (*pop3Client, error) {
	c, err := dialMail(spec, insecure, 30*time.Second)
	if err != nil {
		return nil, err
	}
	p := &pop3Client{c: c, r: bufio.NewReaderSize(c, 64*1024), user: user, pass: pass}
	if _, err := p.readResp(); err != nil {
		c.Close()
		return nil, err
	}
	return p, nil
}

func (p *pop3Client) Close() {
	if p == nil || p.c == nil {
		return
	}
	_, _ = p.cmd("QUIT")
	_ = p.c.Close()
	p.c = nil
}

func (p *pop3Client) readLine() (string, error) {
	_ = p.c.SetDeadline(time.Now().Add(2 * time.Minute))
	s, err := p.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func (p *pop3Client) readResp() (string, error) {
	s, err := p.readLine()
	if err != nil {
		return "", err
	}
	p.last = s
	if strings.HasPrefix(s, "+") {
		return s, nil
	}
	return s, fmt.Errorf("%s", s)
}

func (p *pop3Client) cmd(line string) (string, error) {
	_ = p.c.SetDeadline(time.Now().Add(2 * time.Minute))
	if _, err := io.WriteString(p.c, line+"\r\n"); err != nil {
		return "", err
	}
	return p.readResp()
}

func (p *pop3Client) Logon() error {
	if p.user != "" {
		if _, err := p.cmd("USER " + p.user); err != nil {
			return fmt.Errorf("USER: %w", err)
		}
	}
	if p.pass != "" {
		if _, err := p.cmd("PASS " + p.pass); err != nil {
			return fmt.Errorf("PASS: %w", err)
		}
	}
	return nil
}

func (p *pop3Client) Stat() (n, octets int, err error) {
	s, err := p.cmd("STAT")
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(s)
	if len(f) >= 2 {
		n, _ = strconv.Atoi(f[1])
	}
	if len(f) >= 3 {
		octets, _ = strconv.Atoi(f[2])
	}
	return n, octets, nil
}

func (p *pop3Client) Retr(nr int) ([]byte, error) {
	if _, err := p.cmd("RETR " + strconv.Itoa(nr)); err != nil {
		return nil, err
	}
	var b strings.Builder
	for {
		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if line == "." {
			break
		}
		if strings.HasPrefix(line, ".") {
			line = line[1:]
		}
		b.WriteString(line)
		b.WriteString("\r\n")
		if b.Len() >= MaxMsgBytes {
			break
		}
	}
	return []byte(b.String()), nil
}

func (p *pop3Client) Dele(nr int) {
	_, _ = p.cmd("DELE " + strconv.Itoa(nr))
}
