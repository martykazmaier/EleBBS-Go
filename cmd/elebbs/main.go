package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
	"elebbs/internal/config"
)

func main() {
	opt := cmdline.Parse(os.Args[1:], true)
	if opt.ShowHelp {
		fmt.Print(cmdline.HelpText())
		os.Exit(255)
	}

	g, err := config.Load(opt.SysPath, bbs.ExeDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s%s\n", cfgrec.SystemMsgPrefix, err.Error())
		fmt.Fprintf(os.Stderr, "%sPlace CONFIG.RA in the current directory or set ELEBBS=\n", cfgrec.SystemMsgPrefix)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%s%s\n", cfgrec.SystemMsgPrefix, cfgrec.PidName)
	if g.RaConfig.SystemName != "" {
		fmt.Fprintf(os.Stderr, "%sSystem: %s  Sysop: %s\n", cfgrec.SystemMsgPrefix, g.RaConfig.SystemName, g.RaConfig.Sysop)
	}

	sess := bbs.New(g, opt)
	if err := sess.Run(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "%s%s\n", cfgrec.SystemMsgPrefix, err.Error())
		os.Exit(1)
	}
	os.Exit(sess.ExitCode())
}
