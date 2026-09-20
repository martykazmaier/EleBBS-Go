package bbs

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
	"elebbs/internal/comm"
	"elebbs/internal/config"
	"elebbs/internal/door"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/logon"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
	"elebbs/internal/menu"
	"elebbs/internal/quest"
	"elebbs/internal/term"
)

type Session struct {
	G    *cfgrec.GlobalCfg
	Line *cfgrec.LineCfg
	Opt  cmdline.Options
}

func New(g *cfgrec.GlobalCfg, opt cmdline.Options) *Session {
	line := &cfgrec.LineCfg{
		RaNodeNr:        1,
		Snooping:        true,
		InheritedHandle: ^uintptr(0),
		MenuStackPtr:    1,
		DispMorePrompt:  false,
	}
	line.Modem = config.LoadModem(g, opt.Node)
	line.Telnet = config.LoadTelnet(g)
	cmdline.Apply(line, opt)
	if line.RaNodeNr < 1 {
		line.RaNodeNr = 1
	}
	if line.RaNodeNr > cfgrec.MaxNodes {
		line.RaNodeNr = cfgrec.MaxNodes
	}
	line.Language = config.LoadLanguage(g, config.LanguageIndex(g.RaConfig.NewUserLang))
	line.SysInfo = config.ReadSysInfo(g)
	return &Session{G: g, Line: line, Opt: opt}
}

func (s *Session) OpenStream() (comm.Stream, error) {
	if s.Line.InheritedHandle != 0 && s.Line.InheritedHandle != ^uintptr(0) {
		st, err := comm.OpenInherited(s.Line.InheritedHandle)
		if err != nil {
			return nil, err
		}
		if ds, ok := st.(interface{ TakeOwnership() }); ok && !s.Line.ComNoClose {
			ds.TakeOwnership()
		}
		if s.Line.TelnetServ {
			return comm.NewTelnet(st), nil
		}
		return st, nil
	}
	if s.Line.TelnetServ {
		return nil, fmt.Errorf("telnet (-XT) requires -H<windows handle> from the front-end")
	}
	if s.Line.LocalLogon || s.Line.Modem.ComPort == 0 {
		return comm.OpenConsole()
	}
	return comm.OpenConsole()
}

func (s *Session) Run() error {
	st, err := s.OpenStream()
	if err != nil {
		return err
	}
	defer st.Close()
	return s.RunOn(st)
}

func (s *Session) RunOn(st comm.Stream) error {
	if err := door.EnterNodeDir(s.G, s.Line); err != nil && s.G != nil {
		logx.Write(s.G, s.Line.RaNodeNr, '!', "node directory: "+err.Error())
	}
	door.AttachSessionConsole(s.Line)
	t := term.New(st, s.G, s.Line)
	eng := &menu.Engine{
		T:     t,
		G:     s.G,
		Line:  s.Line,
		Files: files.LoadAreas(s.G),
		Msgs:  mail.LoadAreas(s.G),
	}
	t.RunScript = func(name, args string) {
		quest.Run(t, s.G, s.Line, name, args)
	}
	t.RunMenu = func(typ byte, data string) {
		eng.ExecType(typ, data)
	}
	s.Line.AnsiOn = true
	localScreen(s, fmt.Sprintf("%sIncoming session node %d (%s)", cfgrec.SystemMsgPrefix, s.Line.RaNodeNr, mode(s)))
	logx.Write(s.G, s.Line.RaNodeNr, '+', "Node started")
	if !logon.Perform(t, s.G, s.Line) {
		logx.Write(s.G, s.Line.RaNodeNr, '-', "Logon failed")
		return io.EOF
	}
	t.Ral = lang.Load(s.G, s.Line.Language)
	eng.Enter()
	t.Println("")
	t.WriteRA("`A14:Goodbye from " + s.G.RaConfig.SystemName + "`A7:\r\n")
	term.DisplayHotFile(t, s.G.RaConfig.TextPath, "goodbye")
	logx.Write(s.G, s.Line.RaNodeNr, '-', s.Line.User.Name+" logged off")
	return nil
}

func mode(s *Session) string {
	if s.Line.InheritedHandle != 0 && s.Line.InheritedHandle != ^uintptr(0) {
		if s.Line.TelnetFromIP != "" {
			return "inherited socket " + s.Line.TelnetFromIP
		}
		return "inherited socket"
	}
	if s.Line.LocalLogon {
		return "local"
	}
	return "remote"
}

func localScreen(s *Session, msg string) {
	if s.Line.Snooping {
		fmt.Fprintln(os.Stderr, msg)
	}
}

func ListenAndServe(g *cfgrec.GlobalCfg, opt cmdline.Options) error {
	s := New(g, opt)
	port := int(s.Line.Telnet.ServerPort)
	addr := opt.Listen
	if addr == "" {
		if port <= 0 {
			port = 23
		}
		addr = fmt.Sprintf(":%d", port)
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	if err := commInit(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "%s%s listening on %s\n", cfgrec.SystemMsgPrefix, cfgrec.PidName, addr)
	node := s.Line.Telnet.StartNodeWith
	if node < 1 {
		node = 1
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		n := int(node)
		node++
		go func(conn net.Conn, node int) {
			defer conn.Close()
			_ = conn.SetDeadline(time.Time{})
			opt2 := opt
			opt2.Node = node
			opt2.Local = false
			opt2.TelnetServ = true
			sess := New(g, opt2)
			sess.Line.LocalLogon = false
			sess.Line.Baud = 11520
			sess.Line.ConnectStr = "115200/TELNET"
			st := comm.NewTelnet(comm.FromConn(conn))
			_ = sess.RunOn(st)
		}(c, n)
	}
}

func commInit() error { return nil }

func ExeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}
