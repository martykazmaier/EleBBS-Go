//go:build !windows

package telsrv

import (
	"fmt"
	"net"
	"os"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
	"elebbs/internal/comm"
)

func spawnEleBBS(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, exe string, conn net.Conn, node int, ip string) error {
	_ = tn
	_ = exe
	opt := cmdline.Parse(nil, true)
	opt.Node = node
	opt.TelnetServ = true
	opt.TelnetFromIP = ip
	opt.NoClose = true
	opt.Baud = 65529
	sess := bbs.New(g, opt)
	sess.Line.LocalLogon = false
	sess.Line.Baud = 65529
	sess.Line.TelnetFromIP = ip
	st := comm.NewTelnet(comm.FromConn(conn))
	return sess.RunOn(st)
}

func SpawnEleBBSHandle(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, exe string, handle uintptr, node int, ip string, extraEnv map[string]string) error {
	_ = tn
	_ = exe
	for k, v := range extraEnv {
		_ = os.Setenv(k, v)
	}
	st, err := comm.FromDoorHandle(handle)
	if err != nil {
		return fmt.Errorf("socket handle: %w", err)
	}
	opt := cmdline.Parse(nil, true)
	opt.Node = node
	opt.TelnetServ = true
	opt.TelnetFromIP = ip
	opt.NoClose = true
	opt.Baud = 65529
	sess := bbs.New(g, opt)
	sess.Line.LocalLogon = false
	sess.Line.Baud = 65529
	sess.Line.TelnetFromIP = ip
	sess.Line.InheritedHandle = handle
	sess.Line.ComNoClose = true
	return sess.RunOn(comm.NewTelnet(st))
}
