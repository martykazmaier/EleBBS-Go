package mail

import (
	"bufio"
	"os"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/pascal"
	"elebbs/internal/userbase"
)

type fwdRule struct {
	cmd   string
	to    string
	alias string
	area  int
}

func loadMailFwd(g *cfgrec.GlobalCfg) []fwdRule {
	p := config.FindFile([]string{g.RaConfig.SysPath, "."}, MailFwdFile, "MAILFWD.CTL")
	if p == "" {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rules []fwdRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		w := strings.Fields(line)
		if len(w) < 2 {
			continue
		}
		r := fwdRule{cmd: pascal.UpCase(w[0]), to: w[1]}
		if len(w) >= 3 {
			r.alias = w[2]
		}
		if len(w) >= 4 {
			r.area = atoiMail(w[3])
		}
		rules = append(rules, r)
	}
	return rules
}

func tryAliases(g *cfgrec.GlobalCfg, username string, body []byte, defArea int, inPath string) bool {
	username = pascal.UpCase(pascal.Trim(username))
	if username == "" {
		return false
	}
	ok := false
	for _, r := range loadMailFwd(g) {
		if r.cmd != "FWD" {
			continue
		}
		if pascal.UpCase(pascal.Trim(r.to)) != username {
			continue
		}
		area := defArea
		if r.area > 0 {
			area = r.area
		}
		alias := r.alias
		if alias == "" {
			alias = r.to
		}
		_ = AddMsgToBase(inPath, alias, area, 999, body, true)
		ok = true
	}
	return ok
}

func tryLeaveOnServer(g *cfgrec.GlobalCfg, body []byte) bool {
	n0 := ExtractToName(body, false, "To:")
	n1 := ExtractToName(body, true, "To:")
	for _, r := range loadMailFwd(g) {
		if r.cmd != "LEAVE" {
			continue
		}
		want := pascal.UpCase(pascal.Trim(r.to))
		if want == pascal.UpCase(pascal.Trim(n0)) || want == pascal.UpCase(pascal.Trim(n1)) {
			return true
		}
	}
	return false
}

func tryAlternateField(g *cfgrec.GlobalCfg, body []byte, defArea int, inPath string) bool {
	hasAlt := false
	for _, r := range loadMailFwd(g) {
		if r.cmd == "ALTFIELD" {
			hasAlt = true
			break
		}
	}
	if !hasAlt {
		return false
	}
	user := ExtractToName(body, true, "To:")
	addr := ExtractToName(body, false, "To:")
	if _, ok := userbase.Search(g, user); ok {
		_ = AddMsgToBase(inPath, user, defArea, 1, body, true)
		return true
	}
	if _, ok := userbase.Search(g, addr); ok {
		_ = AddMsgToBase(inPath, addr, defArea, 1, body, true)
		return true
	}
	return false
}

func knownUser(g *cfgrec.GlobalCfg, name string) bool {
	if pascal.Trim(name) == "" {
		return false
	}
	_, ok := userbase.Search(g, name)
	return ok
}
