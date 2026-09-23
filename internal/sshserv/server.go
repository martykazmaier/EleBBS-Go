package sshserv

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
	"elebbs/internal/comm"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/online"
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
	signer, err := loadHostKey(cfg.G.RaConfig.SysPath)
	if err != nil {
		return err
	}
	sc := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-EleBBS",
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			u, ok := userbase.Search(cfg.G, conn.User())
			if !ok || !userbase.CheckPassword(u, string(pass)) {
				return nil, fmt.Errorf("access denied")
			}
			return &ssh.Permissions{Extensions: map[string]string{
				"name": u.Name,
				"pass": string(pass),
			}}, nil
		},
	}
	sc.AddHostKey(signer)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%sSSH listening on :%d (USERS.BBS logins, starts EleBBS)\n", cfgrec.SystemMsgPrefix, cfg.Port)

	tn := config.LoadTelnet(cfg.G)
	max := int(tn.MaxSessions)
	if max <= 0 {
		max = 10
	}
	startNode := int(tn.StartNodeWith)
	if startNode < 1 {
		startNode = 1
	}
	var alive int32
	var mu sync.Mutex
	inUse := map[int]bool{}

	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		if int(atomic.LoadInt32(&alive)) >= max {
			_ = c.Close()
			continue
		}
		mu.Lock()
		node := online.FirstFree(startNode, max, inUse)
		if node == 0 {
			mu.Unlock()
			_ = c.Close()
			continue
		}
		inUse[node] = true
		mu.Unlock()
		atomic.AddInt32(&alive, 1)
		go func(conn net.Conn, node int) {
			defer atomic.AddInt32(&alive, -1)
			defer func() {
				mu.Lock()
				delete(inUse, node)
				mu.Unlock()
			}()
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

func loadHostKey(sysPath string) (ssh.Signer, error) {
	p := filepath.Join(strings.TrimRight(strings.TrimSpace(sysPath), `\/`), "eleserv_hostkey")
	if b, err := os.ReadFile(p); err == nil {
		return ssh.ParsePrivateKey(b)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	_ = os.WriteFile(p, pemBytes, 0600)
	return ssh.NewSignerFromKey(priv)
}
