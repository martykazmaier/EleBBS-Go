package main

import (
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/ftp"
	"elebbs/internal/nntp"
	"elebbs/internal/serv"
	"elebbs/internal/sshserv"
	"elebbs/internal/telsrv"
)

func main() {
	opt := serv.Parse(os.Args[1:])
	if opt.ShowHelp || (!opt.FTP && !opt.News && !opt.Telnet && !opt.TelnetS && !opt.WSS && !opt.SSH) {
		fmt.Print(serv.HelpText(cfgrec.PidName))
		os.Exit(255)
	}

	g, err := config.Load(opt.SysPath, bbs.ExeDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s%s\n", cfgrec.SystemMsgPrefix, err.Error())
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%s%s SERVER\n", cfgrec.SystemMsgPrefix, cfgrec.PidName)
	if g.RaConfig.SystemName != "" {
		fmt.Fprintf(os.Stderr, "%sSystem: %s\n", cfgrec.SystemMsgPrefix, g.RaConfig.SystemName)
	}

	var tlsCfg *tls.Config
	if opt.FTP || opt.News || opt.TelnetS || opt.WSS {
		var src string
		tlsCfg, src, err = serv.LoadTLS(opt.CertFile, opt.KeyFile, g.RaConfig.SysPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s%s\n", cfgrec.SystemMsgPrefix, err.Error())
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "%sTLS certificate: %s\n", cfgrec.SystemMsgPrefix, src)
	}

	var wg sync.WaitGroup
	start := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				fmt.Fprintf(os.Stderr, "%s%s: %s\n", cfgrec.SystemMsgPrefix, name, err.Error())
			}
		}()
	}
	if opt.FTP {
		cfg := tlsCfg
		start("FTPS", func() error {
			return ftp.Listen(ftp.Config{
				G: g, Port: opt.FTPPort, Limit: opt.FTPLimit, Anonymous: opt.Anonymous,
				PasvIP: opt.PasvIP, PasvLo: opt.PasvLo, PasvHi: opt.PasvHi, PasvOffset: opt.PasvOffset,
				IndexName: opt.FTPIndex, XferLog: opt.TransferLog, TLS: cfg,
			})
		})
	}
	if opt.News {
		cfg := tlsCfg
		start("NNTPS", func() error {
			return nntp.Listen(nntp.Config{G: g, Port: opt.NewsPort, TLS: cfg, Limit: int(config.LoadNewsServer(g).MaxSessions)})
		})
	}
	if opt.Telnet {
		start("TELNET", func() error {
			return telsrv.ListenAndSpawn(g)
		})
	}
	if opt.TelnetS {
		cfg := tlsCfg
		start("TELNETS", func() error {
			return telsrv.ListenTLS(g, opt.TelnetSPort, cfg)
		})
	}
	if opt.WSS {
		cfg := tlsCfg
		start("WSS", func() error {
			return telsrv.ListenWSS(g, opt.WSSPort, cfg)
		})
	}
	if opt.SSH {
		start("SSH", func() error {
			return sshserv.Listen(sshserv.Config{G: g, Port: opt.SSHPort})
		})
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	fmt.Fprintf(os.Stderr, "%sShutting down\n", cfgrec.SystemMsgPrefix)
	os.Exit(0)
}
