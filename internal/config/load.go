package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func FindFile(dirs []string, names ...string) string {
	for _, d := range dirs {
		for _, n := range names {
			p := filepath.Join(d, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func SearchDirs(sysPath, exeDir string) []string {
	var dirs []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		p = strings.TrimRight(p, `\/`)
		for _, e := range dirs {
			if strings.EqualFold(e, p) {
				return
			}
		}
		dirs = append(dirs, p)
	}
	cwd, _ := os.Getwd()
	add(cwd)
	add(sysPath)
	add(exeDir)
	if v := os.Getenv("ELEBBS"); v != "" {
		add(v)
	}
	if v := os.Getenv("RA"); v != "" {
		add(v)
	}
	return dirs
}

func Load(sysHint, exeDir string) (*cfgrec.GlobalCfg, error) {
	dirs := SearchDirs(sysHint, exeDir)
	cfgPath := FindFile(dirs, "config.ra", "CONFIG.RA")
	if cfgPath == "" {
		return nil, fmt.Errorf("CONFIG.RA not found (cwd, ELEBBS, RA, exe dir)")
	}
	base := filepath.Dir(cfgPath)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	g := &cfgrec.GlobalCfg{RaConfig: cfgrec.ParseConfig(raw), CfgPath: cfgPath}
	if g.RaConfig.SysPath == "" {
		g.RaConfig.SysPath = pascal.ForceBack(base)
	} else {
		g.RaConfig.SysPath = pascal.ForceBack(g.RaConfig.SysPath)
	}
	if g.RaConfig.MsgBasePath == "" {
		g.RaConfig.MsgBasePath = g.RaConfig.SysPath
	} else {
		g.RaConfig.MsgBasePath = pascal.ForceBack(g.RaConfig.MsgBasePath)
	}
	g.RaConfig.TextPath = pascal.ForceBack(g.RaConfig.TextPath)
	g.RaConfig.MenuPath = pascal.ForceBack(g.RaConfig.MenuPath)
	g.RaConfig.FileBase = pascal.ForceBack(g.RaConfig.FileBase)
	if ele := FindFile([]string{base, g.RaConfig.SysPath}, "config.ele", "CONFIG.ELE"); ele != "" {
		g.ElePath = ele
		if b, err := os.ReadFile(ele); err == nil {
			g.ElConfig = cfgrec.ParseEleConfig(b)
		}
	}
	return g, nil
}

func LoadModem(g *cfgrec.GlobalCfg, node int) cfgrec.Modem {
	dirs := []string{g.RaConfig.SysPath, "."}
	names := []string{
		fmt.Sprintf("modem%d.ra", node),
		fmt.Sprintf("MODEM%d.RA", node),
		"modem.ra",
		"MODEM.RA",
	}
	if p := FindFile(dirs, names...); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			return cfgrec.ParseModem(b)
		}
	}
	return cfgrec.Modem{ComPort: 0, MaxSpeed: 115200}
}

func LoadTelnet(g *cfgrec.GlobalCfg) cfgrec.TelnetCfg {
	dirs := []string{g.RaConfig.SysPath, "."}
	if p := FindFile(dirs, "telnet.ele", "TELNET.ELE"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			return cfgrec.ParseTelnet(b)
		}
	}
	return cfgrec.TelnetCfg{MaxSessions: 10, ServerPort: 23, StartNodeWith: 1}
}

func SaveTelnet(g *cfgrec.GlobalCfg, t cfgrec.TelnetCfg) error {
	dir := strings.TrimSpace(g.RaConfig.SysPath)
	if dir == "" {
		dir = "."
	}
	p := FindFile([]string{dir, "."}, "telnet.ele", "TELNET.ELE")
	if p == "" {
		p = filepath.Join(dir, "TELNET.ELE")
	}
	return os.WriteFile(p, cfgrec.EncodeTelnet(t), 0644)
}

func LoadNewsServer(g *cfgrec.GlobalCfg) cfgrec.NewsServer {
	dirs := []string{g.RaConfig.SysPath, "."}
	if p := FindFile(dirs, "nwserver.ele", "NWSERVER.ELE"); p != "" {
		if b, err := os.ReadFile(p); err == nil && len(b) >= cfgrec.NewsServerSize {
			return cfgrec.ParseNewsServer(b)
		}
	}
	return cfgrec.NewsServer{MaxSessions: 50, ServerPort: 119, DomainName: "elebbs.bbs"}
}

func languageRA(g *cfgrec.GlobalCfg) []byte {
	dirs := []string{g.RaConfig.SysPath, g.RaConfig.MenuPath, g.RaConfig.TextPath, "."}
	if p := FindFile(dirs, "language.ra", "LANGUAGE.RA"); p != "" {
		b, err := os.ReadFile(p)
		if err == nil {
			return b
		}
	}
	return nil
}

func languageRecSize(n int) int {
	if n <= 0 {
		return cfgrec.LanguageSize
	}
	if n%cfgrec.LanguageSize == 0 {
		return cfgrec.LanguageSize
	}
	if n%cfgrec.LanguageSizeRA == 0 {
		return cfgrec.LanguageSizeRA
	}
	if n >= cfgrec.LanguageSize {
		return cfgrec.LanguageSize
	}
	return cfgrec.LanguageSizeRA
}

func finishLanguage(g *cfgrec.GlobalCfg, l cfgrec.Language) cfgrec.Language {
	if l.MenuPath == "" {
		l.MenuPath = g.RaConfig.MenuPath
	}
	if l.TextPath == "" {
		l.TextPath = g.RaConfig.TextPath
	}
	if l.QuesPath == "" {
		l.QuesPath = l.TextPath
	}
	if l.DefName == "" {
		l.DefName = "english.ral"
	}
	if l.Name == "" {
		l.Name = "English"
	}
	l.MenuPath = pascal.ForceBack(l.MenuPath)
	l.TextPath = pascal.ForceBack(l.TextPath)
	l.QuesPath = pascal.ForceBack(l.QuesPath)
	return l
}

func defaultLanguage(g *cfgrec.GlobalCfg) cfgrec.Language {
	return finishLanguage(g, cfgrec.Language{
		Name:    "English",
		DefName: "english.ral",
	})
}

// LoadLanguage loads LANGUAGE.RA record rec (0-based file index).
func LoadLanguage(g *cfgrec.GlobalCfg, rec int) cfgrec.Language {
	b := languageRA(g)
	if len(b) < 21 {
		return defaultLanguage(g)
	}
	sz := languageRecSize(len(b))
	if rec < 0 {
		rec = 0
	}
	off := rec * sz
	if off+sz > len(b) {
		off = 0
	}
	chunk := b[off:]
	if len(chunk) > cfgrec.LanguageSize {
		chunk = chunk[:cfgrec.LanguageSize]
	}
	return finishLanguage(g, cfgrec.ParseLanguage(chunk))
}

// LanguageIndex converts the 1-based UsersRecord.Language field to a 0-based LANGUAGE.RA index.
func LanguageIndex(userLang byte) int {
	if userLang == 0 {
		return 0
	}
	return int(userLang) - 1
}

func ListLanguages(g *cfgrec.GlobalCfg) []cfgrec.Language {
	b := languageRA(g)
	if len(b) < 21 {
		return []cfgrec.Language{defaultLanguage(g)}
	}
	sz := languageRecSize(len(b))
	n := len(b) / sz
	out := make([]cfgrec.Language, 0, n)
	for i := 0; i < n; i++ {
		off := i * sz
		chunk := b[off:]
		if len(chunk) > cfgrec.LanguageSize {
			chunk = chunk[:cfgrec.LanguageSize]
		}
		l := finishLanguage(g, cfgrec.ParseLanguage(chunk))
		if l.Name == "" && l.DefName == "" {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return []cfgrec.Language{defaultLanguage(g)}
	}
	return out
}

// ApplyUserLanguage sets line.Language from the user's 1-based language number.
func ApplyUserLanguage(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) {
	if g == nil || line == nil {
		return
	}
	idx := LanguageIndex(line.User.Language)
	if line.User.Language == 0 {
		idx = LanguageIndex(g.RaConfig.NewUserLang)
	}
	line.Language = LoadLanguage(g, idx)
}
