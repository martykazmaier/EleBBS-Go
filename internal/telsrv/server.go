package telsrv

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
)

const connectBanner = "CONNECT 115200/TELNET\n\r\n\r"

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

func ListenAndSpawn(g *cfgrec.GlobalCfg) error {
	tn := config.LoadTelnet(g)
	port := int(tn.ServerPort)
	if port <= 0 {
		port = 23
	}
	exe := elebbsExe(g, tn)
	if !fileExists(exe) {
		return fmt.Errorf("ELEBBS.EXE not found (TELNET.ELE ProgramPath / exe dir): %s", exe)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%sTELNET spawning %s on :%d\n", cfgrec.SystemMsgPrefix, exe, port)
	var alive int32
	var mu sync.Mutex
	inUse := map[int]bool{}
	next := int(tn.StartNodeWith)
	if next < 1 {
		next = 1
	}
	max := int(tn.MaxSessions)
	if max <= 0 {
		max = 10
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		if int(atomic.LoadInt32(&alive)) >= max {
			_, _ = c.Write([]byte("BUSY\r\n"))
			_ = c.Close()
			continue
		}
		mu.Lock()
		node := next
		for inUse[node] {
			node++
			if node > 255 {
				node = int(tn.StartNodeWith)
				if node < 1 {
					node = 1
				}
			}
			if node == next {
				break
			}
		}
		inUse[node] = true
		next = node + 1
		mu.Unlock()
		atomic.AddInt32(&alive, 1)
		go func(conn net.Conn, node int) {
			defer atomic.AddInt32(&alive, -1)
			defer func() {
				mu.Lock()
				delete(inUse, node)
				mu.Unlock()
			}()
			ip := host(conn)
			logx.Write(g, 0, '>', fmt.Sprintf("[TELSRV] [%s] Connection opened node %d", ip, node))
			_, _ = conn.Write([]byte(connectBanner))
			err := spawnEleBBS(g, tn, exe, conn, node, ip)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%sTELNET spawn: %s\n", cfgrec.SystemMsgPrefix, err.Error())
				logx.Write(g, 0, '!', "[TELSRV] spawn failed: "+err.Error())
			}
			_ = conn.Close()
			logx.Write(g, 0, '>', fmt.Sprintf("[TELSRV] [%s] Connection closed", ip))
		}(c, node)
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
