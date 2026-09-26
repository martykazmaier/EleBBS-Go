package telsrv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/online"
	"elebbs/internal/pascal"
)

const connectBanner = "CONNECT 115200/TELNET\n\r\n\r"

// handshakeTimeout bounds the TLS handshake and WebSocket upgrade, so an idle
// connection cannot hold a goroutine open.
const handshakeTimeout = 20 * time.Second

func EleBBSPath(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg) string {
	return elebbsExe(g, tn)
}

func elebbsExe(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg) string {
	dir := strings.TrimSpace(tn.ProgramPath)
	if dir == "" {
		dir = bbs.ExeDir()
	}
	dir = strings.TrimRight(dir, `\/`)
	for _, n := range []string{"elebbs.exe", "ELEBBS.EXE"} {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p := filepath.Join(bbs.ExeDir(), "elebbs.exe"); fileExists(p) {
		return p
	}
	return filepath.Join(dir, "elebbs.exe")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func nodeDir(tn cfgrec.TelnetCfg, node int) string {
	p := tn.NodeDirectories
	p = strings.ReplaceAll(p, "*N", strconv.Itoa(node))
	p = strings.ReplaceAll(p, "*n", strconv.Itoa(node))
	p = pascal.ForceBack(p)
	return strings.TrimRight(p, `\/`)
}

// Telnet, telnets, WSS and SSH share TELNET.ELE's node range, so they draw
// from one pool.
var pool = struct {
	sync.Mutex
	inUse map[int]bool
}{inUse: map[int]bool{}}

// AcquireNode returns the first free node of TELNET.ELE's range, or 0.
func AcquireNode(tn cfgrec.TelnetCfg) int {
	max := int(tn.MaxSessions)
	if max <= 0 {
		max = 10
	}
	pool.Lock()
	defer pool.Unlock()
	node := online.FirstFree(int(tn.StartNodeWith), max, pool.inUse)
	if node != 0 {
		pool.inUse[node] = true
	}
	return node
}

func ReleaseNode(node int) {
	pool.Lock()
	delete(pool.inUse, node)
	pool.Unlock()
}

// ListenAndSpawn is the plain telnet server on TELNET.ELE's port.
func ListenAndSpawn(g *cfgrec.GlobalCfg) error {
	tn := config.LoadTelnet(g)
	port := int(tn.ServerPort)
	if port <= 0 {
		port = 23
	}
	return listen(g, tn, "TELNET", "TELSRV", port, func(c net.Conn) (net.Conn, error) { return c, nil })
}

// ListenTLS is the telnets server: telnet over implicit TLS.
func ListenTLS(g *cfgrec.GlobalCfg, port int, cfg *tls.Config) error {
	if port <= 0 {
		port = 992
	}
	return listen(g, config.LoadTelnet(g), "TELNETS", "TELNETS", port, func(c net.Conn) (net.Conn, error) {
		return tlsHandshake(c, cfg)
	})
}

// ListenWSS is the secure WebSocket server for web terminals (fTelnet and
// the like): telnet carried in WebSocket messages over TLS.
func ListenWSS(g *cfgrec.GlobalCfg, port int, cfg *tls.Config) error {
	if port <= 0 {
		port = 11235
	}
	return listen(g, config.LoadTelnet(g), "WSS", "WSS", port, func(c net.Conn) (net.Conn, error) {
		tc, err := tlsHandshake(c, cfg)
		if err != nil {
			return nil, err
		}
		_ = tc.SetDeadline(time.Now().Add(handshakeTimeout))
		ws, err := upgradeWebSocket(tc)
		_ = tc.SetDeadline(time.Time{})
		return ws, err
	})
}

func tlsHandshake(c net.Conn, cfg *tls.Config) (net.Conn, error) {
	tc := tls.Server(c, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	if err := tc.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	return tc, nil
}

func listen(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, name, tag string, port int, prepare func(net.Conn) (net.Conn, error)) error {
	exe := elebbsExe(g, tn)
	if !fileExists(exe) {
		return fmt.Errorf("ELEBBS.EXE not found (TELNET.ELE ProgramPath / exe dir): %s", exe)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%s%s spawning %s on :%d\n", cfgrec.SystemMsgPrefix, name, exe, port)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go func(raw net.Conn) {
			ip := host(raw)
			conn, err := prepare(raw)
			if err != nil {
				_ = raw.Close()
				logx.Write(g, 0, '!', fmt.Sprintf("[%s] [%s] %s", tag, ip, err.Error()))
				return
			}
			defer conn.Close()
			node := AcquireNode(tn)
			if node == 0 {
				_, _ = conn.Write([]byte("BUSY\r\n"))
				return
			}
			defer ReleaseNode(node)
			logx.Write(g, 0, '>', fmt.Sprintf("[%s] [%s] Connection opened node %d", tag, ip, node))
			_, _ = conn.Write([]byte(connectBanner))
			if err := spawnEleBBS(g, tn, exe, conn, node, ip); err != nil {
				fmt.Fprintf(os.Stderr, "%s%s spawn: %s\n", cfgrec.SystemMsgPrefix, name, err.Error())
				logx.Write(g, 0, '!', fmt.Sprintf("[%s] spawn failed: %s", tag, err.Error()))
			}
			logx.Write(g, 0, '>', fmt.Sprintf("[%s] [%s] Connection closed", tag, ip))
		}(c)
	}
}

func host(c net.Conn) string {
	a := c.RemoteAddr()
	if a == nil {
		return "0.0.0.0"
	}
	s := a.String()
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "[]")
}
