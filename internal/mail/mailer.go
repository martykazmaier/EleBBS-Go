package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
	"elebbs/internal/userbase"
)

type MailerOptions struct {
	Get, Toss, Scan, Send bool
	TrashBounces          bool
	ReplyTo               bool
	InsecureTLS           bool
	MsgArea               int
	ScanArea              int
	BounceName            string
	EmailHost             string
	Disclaimer            string
	POP3                  ServerSpec
	SMTP                  ServerSpec
	POP3User, POP3Pass    string
	SMTPUser, SMTPPass    string
	Invalid               string
}

func ParseMailerArgs(args []string) MailerOptions {
	o := MailerOptions{
		POP3: ServerSpec{Port: DefaultPOP3},
		SMTP: ServerSpec{Port: DefaultSMTP},
	}
	for _, a := range args {
		if a == "" {
			continue
		}
		if a[0] != '-' && a[0] != '/' {
			o.Invalid = a
			return o
		}
		if a == "-?" || a == "/?" || a == "?" {
			o.Invalid = "?"
			return o
		}
		key := strings.ToUpper(a[1:2])
		val := ""
		if len(a) > 2 {
			val = a[2:]
		}
		switch key {
		case "R":
			o.Get = true
		case "T":
			o.Toss = true
		case "S":
			o.Scan = true
		case "P":
			o.Send = true
		case "F":
			o.TrashBounces = true
		case "Y":
			o.ReplyTo = true
		case "O":
			o.InsecureTLS = true
		case "E":
			o.EmailHost = val
		case "D":
			o.Disclaimer = val
		case "A":
			o.MsgArea = atoiMail(val)
		case "C":
			o.ScanArea = atoiMail(val)
		case "B":
			o.BounceName = strings.ReplaceAll(val, "_", " ")
		case "U":
			o.POP3User, o.POP3Pass = splitUserPass(val)
		case "W":
			o.SMTPUser, o.SMTPPass = splitUserPass(val)
		case "H":
			o.POP3 = ParseServer(val, DefaultPOP3)
		case "I":
			o.SMTP = ParseServer(val, DefaultSMTP)
		default:
			o.Invalid = a
			return o
		}
	}
	if o.Get && o.POP3.Host == "" {
		o.Invalid = "POP3"
	}
	if o.Send && o.SMTP.Host == "" {
		o.Invalid = "SMTP"
	}
	if o.Get && o.MsgArea == 0 {
		o.Invalid = "AREA"
	}
	if o.Scan && o.ScanArea == 0 {
		o.Invalid = "SCAN"
	}
	return o
}

func MailerHelp() string {
	var b strings.Builder
	p := cfgrec.SystemMsgPrefix
	b.WriteString(" " + p + "Command-line parameters:\n\n")
	b.WriteString("         -R               Collect new email from the server\n")
	b.WriteString("         -T               Toss messages into messagebase\n")
	b.WriteString("         -S               Scan messagebase for new messages\n")
	b.WriteString("         -P               Send messages to mail server\n")
	b.WriteString("         -F               Forward unknown recipients to the bounce user\n")
	b.WriteString("         -Y               Set Reply-To: field at the sent messages\n")
	b.WriteString("         -O               Allow self-signed TLS certificates\n")
	b.WriteString("         -H<name>[:port]  POP3 host (port 995 = POP3S; suffix s = implicit TLS)\n")
	b.WriteString("         -I<name>[:port]  SMTP host (465 = SMTPS, 587 = STARTTLS; suffix s/t)\n")
	b.WriteString("         -D[filename]     Filename specifies a textfile that will be added to\n")
	b.WriteString("                          any posted message\n")
	b.WriteString("         -E<domainname>   Domainname for email address, FTN is used when empty\n")
	b.WriteString("         -A<areanum>      Specify an areanumber where to toss new mail in\n")
	b.WriteString("         -C<areanum>      Specify an areanumber where to scan new mail from\n")
	b.WriteString("         -B<bouncename>   Specify an username to where all bounced e-mail is\n")
	b.WriteString("                          sent to\n")
	b.WriteString("         -U<name>@<pword> Username and password to use for the POP3 connection\n")
	b.WriteString("         -W<name>@<pword> Username and password to use for the SMTP connection\n")
	b.WriteString("                          If used, the SMTP server may set the From: field\n")
	b.WriteString("                          with this Username. -Y recommended in that case.\n")
	return b.String()
}

func RunMailer(g *cfgrec.GlobalCfg, o MailerOptions) error {
	if o.BounceName == "" {
		o.BounceName = pascal.Trim(g.RaConfig.Sysop)
	}
	lock := filepath.Join(g.RaConfig.SysPath, MailLockFile)
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		logx.Write(g, 0, '!', "EleMAIL is already running ("+MailLockFile+" exists), aborting.")
		return fmt.Errorf("EleMAIL is already running (%s exists), aborting", MailLockFile)
	}
	_, _ = f.WriteString("elemail\n")
	_ = f.Close()
	defer os.Remove(lock)

	logx.Write(g, 0, '>', "EleMAIL; POP3/SMTP mail utility fired up")

	if o.Scan {
		fmt.Printf(" %sScanning message area for new articles (%d)\n", cfgrec.SystemMsgPrefix, o.ScanArea)
		if err := ScanMsgArea(g, o.ScanArea, EmailOutPath(g), o.EmailHost); err != nil {
			return err
		}
	}
	if o.Send {
		if _, err := os.Stat(EmailOutPath(g)); err != nil {
			o.Send = false
		}
	}
	if o.Get {
		if err := collectPOP3(g, o); err != nil {
			return err
		}
	}
	if o.Send {
		if err := sendSMTP(g, o); err != nil {
			return err
		}
	}
	if o.Toss {
		if err := ProcessNewArticles(g, EmailInPath(g)); err != nil {
			return err
		}
	}
	return nil
}

func collectPOP3(g *cfgrec.GlobalCfg, o MailerOptions) error {
	fmt.Printf(" %sConnecting to %s,", cfgrec.SystemMsgPrefix, o.POP3.Addr())
	cli, err := newPOP3(o.POP3, o.POP3User, o.POP3Pass, o.InsecureTLS)
	if err != nil {
		fmt.Println(" failed.")
		return err
	}
	defer cli.Close()
	if err := cli.Logon(); err != nil {
		fmt.Println(" failed.")
		logx.Write(g, 0, '!', "POP3 server ("+o.POP3.Host+"), refused authentication, aborting.")
		return err
	}
	fmt.Println(" success!")

	n, _, err := cli.Stat()
	if err != nil {
		return err
	}
	fmt.Printf(" %sCollecting mail for '%s' (%d messages)", cfgrec.SystemMsgPrefix, o.POP3User, n)
	if n <= 0 {
		fmt.Println()
		fmt.Printf(" %sClosing connection to %s\n", cfgrec.SystemMsgPrefix, o.POP3.Host)
		return nil
	}
	fmt.Println()
	fmt.Printf(" %sRetrieving message #", cfgrec.SystemMsgPrefix)
	inPath := EmailInPath(g)
	for i := 1; i <= n; i++ {
		body, err := cli.Retr(i)
		if err != nil {
			continue
		}
		fmt.Print(i, strings.Repeat("\b", len(strconvI(i))))
		user, leave, post := routeInbound(g, o, body)
		if post && user != "" {
			_ = AddMsgToBase(inPath, user, o.MsgArea, i, body, true)
		}
		if !leave {
			cli.Dele(i)
		} else {
			logx.Write(g, 0, '>', fmt.Sprintf("Leaving message #%d on server (To: %q)", i, ExtractToName(body, true, "To:")))
		}
	}
	fmt.Println()
	fmt.Printf(" %sClosing connection to %s\n", cfgrec.SystemMsgPrefix, o.POP3.Host)
	return nil
}

func strconvI(n int) string {
	return fmt.Sprintf("%d", n)
}

func routeInbound(g *cfgrec.GlobalCfg, o MailerOptions, body []byte) (user string, leave, post bool) {
	user = ExtractToName(body, true, "To:")
	save := user
	leave = tryLeaveOnServer(g, body)
	if leave {
		return save, true, false
	}
	if user == "" || !knownUser(g, user) {
		user = ExtractToName(body, false, "To:")
		if user == "" || !knownUser(g, user) {
			if tryAliases(g, save, body, o.MsgArea, EmailInPath(g)) || tryAliases(g, user, body, o.MsgArea, EmailInPath(g)) {
				return "", false, false
			}
			if tryAlternateField(g, body, o.MsgArea, EmailInPath(g)) {
				return "", false, false
			}
			if o.TrashBounces {
				who := user
				if who == "" {
					who = save
				}
				stamp := time.Now()
				body = prependLines(body, []string{
					"** EleMAIL v" + cfgrec.VersionID + ".",
					"** Message forwarded at " + stamp.Format("15:04:05") + " - " + stamp.Format("01-02-2006"),
					"** Message originally to: " + who,
					"** Forward reason: Unknown username",
					"",
				})
				user = o.BounceName
			} else {
				user = ""
			}
		}
	}
	return user, false, user != ""
}

func sendSMTP(g *cfgrec.GlobalCfg, o MailerOptions) error {
	logx.Write(g, 0, '>', "Posting articles to your mailserver")
	fmt.Printf(" %sPosting articles to your mailserver\n", cfgrec.SystemMsgPrefix)
	path := EmailOutPath(g)
	arts, err := readNewsFile(path)
	if err != nil {
		return err
	}
	if len(arts) == 0 {
		fmt.Println("   (no articles to process)")
		logx.Write(g, 0, '>', "Posting completed, no articles processed")
		return nil
	}
	fmt.Printf(" %sConnecting to %s,", cfgrec.SystemMsgPrefix, o.SMTP.Addr())
	cli, err := newSMTP(o.SMTP, o.InsecureTLS, o.EmailHost, o.SMTPUser, o.SMTPPass)
	if err != nil {
		fmt.Println(" failed.")
		return err
	}
	defer cli.Close()
	fmt.Println(" success!")

	posted := 0
	for i := range arts {
		body := appendDisclaimer(arts[i].Body, o.Disclaimer)
		if err := postMailArticle(cli, o, body); err != nil {
			fmt.Printf("   Failed to send article #%d (%s)\n", i+1, err.Error())
			logx.Write(g, 0, '!', fmt.Sprintf("Failed to send (SMTP) message #%d (%q)", i+1, err.Error()))
			arts[i].TimesSent++
			posted--
		} else {
			arts[i].Attribute |= ArtTossed
		}
		posted++
	}
	logx.Write(g, 0, '>', fmt.Sprintf("Posting completed (%d articles processed)", posted))
	fmt.Printf("   (%d messages processed)\n", posted)
	if err := rewriteNewsFile(path, arts); err != nil {
		return err
	}
	kept, err := PurgeSentBase(path, MaxSentTries)
	if err != nil {
		return err
	}
	logx.Write(g, 0, '>', fmt.Sprintf("Purging completed (%d articles left)", kept))
	fmt.Printf(" %sPurging outbound articlesfile\n", cfgrec.SystemMsgPrefix)
	fmt.Printf("   (%d messages left in outbound)\n", kept)
	fmt.Printf(" %sClosing connection to %s\n", cfgrec.SystemMsgPrefix, o.SMTP.Host)
	return nil
}

func postMailArticle(cli *smtpClient, o MailerOptions, body []byte) error {
	from := GetRawEmail(headerValue("From:", body))
	if err := cli.MailFrom(from); err != nil {
		return err
	}
	to := GetRawEmail(headerValue("To:", body))
	if err := cli.RcptTo(to); err != nil {
		return err
	}
	reply := ""
	if o.ReplyTo {
		reply = from
	}
	return cli.Data(body, reply)
}

func ScanMsgArea(g *cfgrec.GlobalCfg, areaNum int, outPath, emailHost string) error {
	areas := LoadAll(g)
	a, ok := FindArea(areas, uint16(areaNum))
	if !ok || a.Name == "" {
		return nil
	}
	eles := LoadEleMessages(g)
	el, _ := FindEle(eles, a.AreaNum)
	base := jamBase(a.JAMBase)
	st := Stats(base)
	if st.High < 1 {
		return nil
	}
	first := st.First
	if first < 1 {
		first = 1
	}
	var orig cfgrec.Addr
	if int(a.AkaAddress) < len(g.RaConfig.Address) {
		orig = g.RaConfig.Address[a.AkaAddress]
	}
	for n := first; n <= st.High; n++ {
		msg, ok := ReadMsg(base, n)
		if !ok || msg.Sent {
			continue
		}
		host := MakeHostEmail(emailHost, msg.From, orig)
		var b strings.Builder
		b.WriteString("Date: " + MakeNntpDate(msg.Date) + "\r\n")
		b.WriteString(`From: "` + msg.From + `" <` + host + ">\r\n")
		b.WriteString("To: " + msg.To + "\r\n")
		b.WriteString("X-Mailer: " + cfgrec.PidName + "\r\n")
		b.WriteString("Subject: " + msg.Subject + "\r\n\r\n")
		for _, k := range msg.Kludges {
			up := pascal.UpCase(k)
			skip := strings.HasPrefix(up, "MSGID:") || strings.HasPrefix(up, "CHRS:") ||
				strings.HasPrefix(up, "PID:") || strings.HasPrefix(up, "TID:") ||
				strings.HasPrefix(up, "CODEPAGE:") || strings.HasPrefix(up, "TZUTC:") ||
				strings.HasPrefix(up, "REPLY:")
			if skip {
				continue
			}
			b.WriteString("(" + k + ")\r\n")
		}
		body := strings.ReplaceAll(msg.Body, "\n", "\r\n")
		b.WriteString(body)
		raw := []byte(b.String())
		if msg.FAttach {
			if files := listAttachFiles(msg.Subject); len(files) > 0 {
				raw = mimeAttachFiles(raw, files)
			}
		}
		_ = AddMsgToBase(outPath, el.GroupName, int(a.AreaNum), -1, raw, false)
		SetSent(base, n)
	}
	return nil
}

func ProcessNewArticles(g *cfgrec.GlobalCfg, path string) error {
	logx.Write(g, 0, '>', "Tossing articles into message base")
	fmt.Printf(" %sTossing articles into message base\n", cfgrec.SystemMsgPrefix)
	arts, err := readNewsFile(path)
	if err != nil {
		return err
	}
	if len(arts) == 0 {
		fmt.Println("   (no articles to process)")
		logx.Write(g, 0, '>', "Tossing completed, no articles processed")
		return nil
	}
	areas := LoadAll(g)
	eles := LoadEleMessages(g)
	tossed := 0
	lastName := ""
	areaCnt := 0
	for _, art := range arts {
		a, ok := FindArea(areas, uint16(art.AreaNum))
		if !ok {
			continue
		}
		if lastName != art.GroupName {
			if lastName != "" {
				fmt.Printf("%d)\n", areaCnt)
			}
			fmt.Printf(" %sProcessing mail from %s (#", cfgrec.SystemMsgPrefix, art.GroupName)
			areaCnt = 0
		}
		lastName = art.GroupName
		areaCnt++
		el, _ := FindEle(eles, a.AreaNum)
		if err := tossArticle(g, a, art, el.AttachArea); err != nil {
			logx.Write(g, 0, '!', "Toss: "+err.Error())
		}
		tossed++
	}
	fmt.Printf("%d)\n", areaCnt)
	logx.Write(g, 0, '>', fmt.Sprintf("Tossing completed (%d articles processed)", tossed))
	fmt.Printf("   (%d messages processed)\n", tossed)
	_ = os.Remove(path)
	removeDszLogFile(attachRoot(g))
	return nil
}

func tossArticle(g *cfgrec.GlobalCfg, area cfgrec.MessageArea, art NewsArticle, attachArea int32) error {
	body := nulTrim(art.Body)
	body, attachDir, hasAtt := processInboundAttach(g, attachArea, body)
	to, from, subj, reply, msgid, dateField, text := splitRFC822(body)
	if art.Email() {
		to = art.GroupName
	}
	if to == "" {
		to = "All"
	}
	if from == "" {
		from = "EleBBS (unknown sender)"
	}
	if subj == "" {
		subj = art.GroupName
	}
	if hasAtt && attachDir != "" {
		subj = attachDir
	}
	if u, ok := userbase.Search(g, to); ok && pascal.Trim(u.ForwardTo) != "" {
		to = u.ForwardTo
	}
	when := parseNntpDate(dateField)
	var kl []string
	if msgid != "" {
		msgid = strings.Trim(msgid, "<>")
		kl = append(kl, "MSGID: 0:0/0 "+msgid)
	}
	if reply != "" {
		reply = strings.Trim(reply, "<>")
		kl = append(kl, "REPLY: 0:0/0 "+reply)
	}
	attr := uint32(jamLocal | jamTypeLocal | jamSent)
	if hasAtt {
		attr |= jamFAttach
	}
	_, err := AppendMsg(jamBase(area.JAMBase), Article{
		From:    from,
		To:      to,
		Subject: subj,
		Date:    when,
		MsgID:   msgid,
		ReplyID: reply,
		Body:    text,
		Kludges: kl,
		Private: art.Email(),
		FAttach: hasAtt,
		Attr:    attr,
		Sent:    true,
	})
	return err
}

func splitRFC822(body []byte) (to, from, subj, reply, msgid, date, text string) {
	lines := msgLines(body)
	i := 0
	for ; i < len(lines); i++ {
		ln := lines[i]
		if ln == "" {
			i++
			break
		}
		switch {
		case hasPrefixFold(ln, "From:"):
			from = strings.TrimSpace(ln[5:])
		case hasPrefixFold(ln, "To:"):
			to = strings.TrimSpace(ln[3:])
		case hasPrefixFold(ln, "Subject:"):
			subj = strings.TrimSpace(ln[8:])
		case hasPrefixFold(ln, "Reply-To:"):
			reply = strings.TrimSpace(ln[9:])
		case hasPrefixFold(ln, "Message-ID:"):
			msgid = strings.TrimSpace(ln[11:])
		case hasPrefixFold(ln, "References:"):
			if reply == "" {
				reply = strings.TrimSpace(ln[12:])
			}
		case hasPrefixFold(ln, "Date:"):
			date = strings.TrimSpace(ln[5:])
		}
	}
	text = strings.Join(lines[i:], "\r\n")
	return
}

func hasPrefixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p)
}
