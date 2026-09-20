package mail

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type TLSMode int

const (
	TLSOff TLSMode = iota
	TLSImplicit
	TLSStart
)

const (
	DefaultPOP3  = 110
	DefaultPOP3S = 995
	DefaultSMTP  = 25
	DefaultSMTPS = 465
	DefaultSub   = 587
)

// ServerSpec is a POP3/SMTP host, port, and TLS mode.
type ServerSpec struct {
	Host string
	Port int
	TLS  TLSMode
}

func (s ServerSpec) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

func (s ServerSpec) Empty() bool { return s.Host == "" }

// ParseServer parses Pascal -H/-I values: host[:port][s|t]
// Trailing s on the port forces implicit TLS; t forces STARTTLS.
// Well-known ports: 995 POP3S, 465 SMTPS, 587 STARTTLS.
func ParseServer(spec string, defaultPort int) ServerSpec {
	spec = strings.TrimSpace(spec)
	out := ServerSpec{Port: defaultPort}
	if spec == "" {
		return out
	}
	host, portStr, ok := strings.Cut(spec, ":")
	if !ok {
		out.Host = spec
		out.TLS = tlsForPort(defaultPort)
		return out
	}
	out.Host = host
	portStr = strings.TrimSpace(portStr)
	forceImplicit := false
	forceStart := false
	if portStr != "" {
		last := portStr[len(portStr)-1]
		if last == 's' || last == 'S' {
			forceImplicit = true
			portStr = portStr[:len(portStr)-1]
		} else if last == 't' || last == 'T' {
			forceStart = true
			portStr = portStr[:len(portStr)-1]
		}
	}
	if portStr == "" {
		if forceImplicit {
			if defaultPort == DefaultSMTP {
				out.Port = DefaultSMTPS
			} else {
				out.Port = DefaultPOP3S
			}
		} else if forceStart {
			out.Port = DefaultSub
		}
	} else {
		n := 0
		for _, r := range portStr {
			if !unicode.IsDigit(r) {
				break
			}
			n = n*10 + int(r-'0')
		}
		if n > 0 {
			out.Port = n
		}
	}
	switch {
	case forceImplicit:
		out.TLS = TLSImplicit
	case forceStart:
		out.TLS = TLSStart
	default:
		out.TLS = tlsForPort(out.Port)
	}
	return out
}

func tlsForPort(port int) TLSMode {
	switch port {
	case DefaultPOP3S, DefaultSMTPS:
		return TLSImplicit
	case DefaultSub:
		return TLSStart
	}
	return TLSOff
}

func dialMail(spec ServerSpec, insecure bool, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	d := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	network := "tcp"
	if ip := net.ParseIP(spec.Host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}
	c, err := d.Dial(network, spec.Addr())
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", spec.Addr(), err)
	}
	_ = c.SetDeadline(time.Now().Add(2 * time.Minute))
	if spec.TLS == TLSImplicit {
		tc, err := tlsWrap(c, spec.Host, insecure)
		if err != nil {
			c.Close()
			return nil, err
		}
		return tc, nil
	}
	return c, nil
}

func tlsWrap(c net.Conn, serverName string, insecure bool) (*tls.Conn, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS10,
		InsecureSkipVerify: insecure,
	}
	if net.ParseIP(serverName) == nil {
		cfg.ServerName = serverName
	} else if !insecure {
		// Dialed by IP; LAN certs usually have a DNS SAN only.
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = verifyChainNoHostname
	}
	tc := tls.Client(c, cfg)
	if err := tc.Handshake(); err != nil {
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	return tc, nil
}

func verifyChainNoHostname(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no server certificates")
	}
	certs := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		c, err := x509.ParseCertificate(raw)
		if err != nil {
			return err
		}
		certs = append(certs, c)
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	_, err := certs[0].Verify(x509.VerifyOptions{Intermediates: inter})
	return err
}
