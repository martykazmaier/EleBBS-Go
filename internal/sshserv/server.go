package sshserv

import (
	"crypto/dsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
	"elebbs/internal/comm"
	"elebbs/internal/config"
	"elebbs/internal/logon"
	"elebbs/internal/logx"
	"elebbs/internal/telsrv"
	"elebbs/internal/userbase"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	G    *cfgrec.GlobalCfg
	Port int
}

func Listen(cfg Config) error {
	if cfg.Port <= 0 {
		cfg.Port = 22
	}
	signers, err := loadHostKeys(cfg.G.RaConfig.SysPath)
	if err != nil {
		return err
	}
	sc := &ssh.ServerConfig{
		Config:        legacyAlgorithms(),
		ServerVersion: "SSH-2.0-EleBBS",
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			u, ok := userbase.Search(cfg.G, conn.User())
			if ok && !userbase.CheckPassword(u, string(pass), cfg.G.RaConfig.StrictPwdChecking) {
				return nil, fmt.Errorf("access denied")
			}
			if !ok && logon.IsTrashcanName(cfg.G, conn.User()) {
				return nil, fmt.Errorf("access denied")
			}
			name := conn.User()
			if ok {
				name = u.Name
			}
			return &ssh.Permissions{Extensions: map[string]string{
				"name": name,
				"pass": string(pass),
			}}, nil
		},
	}
	for _, s := range signers {
		sc.AddHostKey(s)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%sSSH listening on :%d (USERS.BBS logins and new-user signup, starts EleBBS)\n", cfgrec.SystemMsgPrefix, cfg.Port)
	for _, s := range signers {
		fmt.Fprintf(os.Stderr, "%sSSH host key %s %s\n", cfgrec.SystemMsgPrefix, s.PublicKey().Type(), ssh.FingerprintSHA256(s.PublicKey()))
	}

	tn := config.LoadTelnet(cfg.G)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		node := telsrv.AcquireNode(tn)
		if node == 0 {
			_ = c.Close()
			continue
		}
		go func(conn net.Conn, node int) {
			defer telsrv.ReleaseNode(node)
			serve(cfg, conn, sc, node)
		}(c, node)
	}
}

func serve(cfg Config, n net.Conn, sc *ssh.ServerConfig, node int) {
	defer n.Close()
	conn, chans, reqs, err := ssh.NewServerConn(n, sc)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	name, pass := "", ""
	if conn.Permissions != nil && conn.Permissions.Extensions != nil {
		name = conn.Permissions.Extensions["name"]
		pass = conn.Permissions.Extensions["pass"]
	}
	ip := n.RemoteAddr().String()
	if i := strings.LastIndex(ip, ":"); i >= 0 {
		ip = strings.Trim(ip[:i], "[]")
	}
	logx.Write(cfg.G, node, '>', "[SSH] ["+ip+"] "+name+" authenticated")
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unknown")
			continue
		}
		ch, creqs, err := newCh.Accept()
		if err != nil {
			return
		}
		handleSession(cfg, ch, creqs, name, pass, ip, node)
		return
	}
}

func handleSession(cfg Config, ch ssh.Channel, reqs <-chan *ssh.Request, name, pass, ip string, node int) {
	defer ch.Close()
	ready := make(chan struct{}, 1)
	go func() {
		for req := range reqs {
			ok := false
			switch req.Type {
			case "pty-req", "shell", "env", "window-change":
				ok = true
				if req.Type == "shell" {
					select {
					case ready <- struct{}{}:
					default:
					}
				}
			}
			if req.WantReply {
				_ = req.Reply(ok, nil)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
	}
	opt := cmdline.Parse([]string{
		fmt.Sprintf("-N%d", node),
		"-B65529",
		"-XI" + ip,
	}, true)
	opt.Local = false
	opt.TelnetServ = false
	sess := bbs.New(cfg.G, opt)
	sess.Line.LocalLogon = false
	sess.Line.AnsiOn = true
	sess.Line.Baud = 65529
	sess.Line.ConnectStr = "65529/SSH"
	sess.Line.TelnetFromIP = ip
	sess.Line.CarrierCheck = true
	sess.Line.AutoUser = name
	sess.Line.AutoPass = pass

	bbsSt, peer, h, hf, err := comm.LocalSocketPair()
	if err != nil {
		sess.Line.TelnetServ = false
		_ = sess.RunOn(comm.PumpDeadlines(&sshStream{ch: ch}))
		return
	}
	defer hf.Close()
	defer bbsSt.Close()
	defer peer.Close()
	go comm.Relay(peer, &sshStream{ch: ch})
	tn := config.LoadTelnet(cfg.G)
	exe := telsrv.EleBBSPath(cfg.G, tn)
	if err := telsrv.SpawnEleBBSHandle(cfg.G, tn, exe, h, node, ip, map[string]string{
		"ELEBBS_AUTOUSER": name,
		"ELEBBS_AUTOPASS": pass,
	}); err != nil {
		logx.Write(cfg.G, node, '!', "[SSH] spawn EleBBS: "+err.Error())
	}
}

type sshStream struct {
	ch ssh.Channel
}

func (s *sshStream) Local() bool { return false }

func (s *sshStream) Read(p []byte) (int, error) { return s.ch.Read(p) }

func (s *sshStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.ch.Write(p)
}

func (s *sshStream) Close() error                     { return s.ch.Close() }
func (s *sshStream) SetReadDeadline(time.Time) error  { return nil }
func (s *sshStream) SetWriteDeadline(time.Time) error { return nil }

var _ io.ReadWriteCloser = (*sshStream)(nil)

// loadHostKeys returns the Ed25519 (eleserv_hostkey), RSA
// (eleserv_hostkey_rsa) and DSA (eleserv_hostkey_dsa) host keys from the
// system path, creating missing ones. The RSA key serves clients without
// Ed25519 as rsa-sha2-256, rsa-sha2-512 or ssh-rsa; the DSA key is ssh-dss
// for the oldest terminal programs.
func loadHostKeys(sysPath string) ([]ssh.Signer, error) {
	dir := strings.TrimRight(strings.TrimSpace(sysPath), `\/`)
	ed, err := loadHostKey(filepath.Join(dir, "eleserv_hostkey"), func() (any, *pem.Block, error) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, nil, err
		}
		return priv, &pem.Block{Type: "PRIVATE KEY", Bytes: der}, nil
	})
	if err != nil {
		return nil, err
	}
	rs, err := loadHostKey(filepath.Join(dir, "eleserv_hostkey_rsa"), func() (any, *pem.Block, error) {
		priv, err := rsa.GenerateKey(rand.Reader, rsaHostKeyBits)
		if err != nil {
			return nil, nil, err
		}
		return priv, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}, nil
	})
	if err != nil {
		return nil, err
	}
	ds, err := loadHostKey(filepath.Join(dir, "eleserv_hostkey_dsa"), func() (any, *pem.Block, error) {
		priv := &dsa.PrivateKey{}
		if err := dsa.GenerateParameters(&priv.Parameters, rand.Reader, dsa.L1024N160); err != nil {
			return nil, nil, err
		}
		if err := dsa.GenerateKey(priv, rand.Reader); err != nil {
			return nil, nil, err
		}
		der, err := asn1.Marshal(struct {
			Version       int
			P, Q, G, Y, X *big.Int
		}{0, priv.P, priv.Q, priv.G, priv.Y, priv.X})
		if err != nil {
			return nil, nil, err
		}
		return priv, &pem.Block{Type: "DSA PRIVATE KEY", Bytes: der}, nil
	})
	if err != nil {
		return nil, err
	}
	return []ssh.Signer{ed, rs, ds}, nil
}

// rsaHostKeyBits is 2048 because older SSH libraries in terminal programs
// (NetRunner, SyncTERM) do not all accept larger RSA host keys.
const rsaHostKeyBits = 2048

// legacyAlgorithms is the modern algorithm list followed by the SHA-1 key
// exchanges and CBC ciphers that older terminal programs need. Clients that
// support the modern ones never pick these.
func legacyAlgorithms() ssh.Config {
	sup, old := ssh.SupportedAlgorithms(), ssh.InsecureAlgorithms()
	return ssh.Config{
		KeyExchanges: append(sup.KeyExchanges, old.KeyExchanges...),
		Ciphers:      append(sup.Ciphers, ssh.InsecureCipherAES128CBC, ssh.InsecureCipherTripleDESCBC),
		MACs:         append(sup.MACs, old.MACs...),
	}
}

func loadHostKey(p string, generate func() (any, *pem.Block, error)) (ssh.Signer, error) {
	if b, err := os.ReadFile(p); err == nil {
		s, err := ssh.ParsePrivateKey(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		return s, nil
	}
	priv, block, err := generate()
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(p, pem.EncodeToMemory(block), 0600)
	return ssh.NewSignerFromKey(priv)
}
