package main

import (
	"fmt"
	"os"
	"strings"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/logx"
	"elebbs/internal/mail"
)

func main() {
	fmt.Println()
	fmt.Println("ELEMAIL; EleBBS POP3-mail retrieval utility, Version " + cfgrec.VersionID)
	fmt.Println("         Copyright 1997-2003 Maarten Bekers, All rights reserved.")
	fmt.Println("         Go/Win32 port (POP3S/SMTPS).")
	fmt.Println()

	g, err := config.Load("", bbs.ExeDir())
	if err != nil {
		fatal(err.Error())
	}

	o := mail.ParseMailerArgs(os.Args[1:])
	if o.Invalid != "" || (!o.Get && !o.Toss && !o.Scan && !o.Send) {
		if o.Invalid != "" && o.Invalid != "?" {
			fmt.Println(" "+cfgrec.SystemMsgPrefix+"Invalid option :", o.Invalid)
		}
		fmt.Print(mail.MailerHelp())
		fmt.Println()
		if o.Invalid == "POP3" || o.Invalid == "SMTP" {
			fmt.Println(" " + cfgrec.SystemMsgPrefix + "You need to specify a server for this action!")
		}
		if o.Invalid == "AREA" {
			fmt.Println(" " + cfgrec.SystemMsgPrefix + "You need to specify an areanumber when retrieving messages!")
		}
		if o.Invalid == "SCAN" {
			fmt.Println(" " + cfgrec.SystemMsgPrefix + "You need to specify an areanumber when scanning for messages!")
		}
		fmt.Println(" " + cfgrec.SystemMsgPrefix + "Please refer to the documentation for a more complete command summary")
		fmt.Println()
		os.Exit(255)
	}

	fmt.Println(" " + cfgrec.SystemMsgPrefix + "Active options:")
	if o.Scan {
		fmt.Println("   - Scanning messagebase for new mail")
	} else {
		fmt.Println("   - Not scanning messagebase")
	}
	if o.Send {
		fmt.Println("   - Posting articles to mail server")
	} else {
		fmt.Println("   - Not posting articles to mail server")
	}
	if o.Get {
		fmt.Println("   - Collecting messages from", o.POP3.Host)
	} else {
		fmt.Println("   - Not collecting messages")
	}
	if o.Toss {
		fmt.Println("   - Tossing messages into messagebase")
	} else {
		fmt.Println("   - Not tossing messages")
	}
	fmt.Println()

	logx.Write(g, 0, ' ', "")
	if err := mail.RunMailer(g, o); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "auth") || strings.Contains(err.Error(), "USER") || strings.Contains(err.Error(), "PASS") {
			fmt.Println(" " + cfgrec.SystemMsgPrefix + "Fatal error occured!")
			fmt.Println(" "+cfgrec.SystemMsgPrefix, err.Error())
			os.Exit(250)
		}
		fatal(err.Error())
	}
	fmt.Println(" " + cfgrec.SystemMsgPrefix + "Done")
}

func fatal(s string) {
	fmt.Println()
	fmt.Println(" " + cfgrec.SystemMsgPrefix + "Fatal error occured!")
	fmt.Println(" " + cfgrec.SystemMsgPrefix + s)
	os.Exit(255)
}
