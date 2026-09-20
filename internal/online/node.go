package online

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
)

func SemPath(g *cfgrec.GlobalCfg) string {
	if g == nil {
		return ""
	}
	p := strings.TrimSpace(g.RaConfig.SemPath)
	if p == "" {
		p = g.RaConfig.SysPath
	}
	return strings.TrimRight(p, `\/`)
}

func NodePath(g *cfgrec.GlobalCfg, lineNr int) string {
	return filepath.Join(SemPath(g), "NODE"+strconv.Itoa(lineNr)+".RA")
}

// AppendNodeMsg writes a Pascal NODE<n>.RA internode message (non-web format).
func AppendNodeMsg(g *cfgrec.GlobalCfg, lineNr int, sender string, fromNode int, lines []string) error {
	p := NodePath(g, lineNr)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil && filepath.Dir(p) != "." {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("\r\n\r\n"); err != nil {
		return err
	}
	hdr := "\x0b]497 " + sender + " \x0b]496 " + strconv.Itoa(fromNode) + ":\r\n"
	if _, err := f.WriteString(hdr); err != nil {
		return err
	}
	for _, ln := range lines {
		if _, err := f.WriteString(ln + "\r\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString("\r\n\x0b]258\x01\r\n")
	return err
}

func NodeMsgReady(g *cfgrec.GlobalCfg, lineNr int) bool {
	st, err := os.Stat(NodePath(g, lineNr))
	return err == nil && st.Size() > 0
}

func ClearNodeMsg(g *cfgrec.GlobalCfg, lineNr int) {
	p := NodePath(g, lineNr)
	if err := os.Remove(p); err != nil {
		_ = os.WriteFile(p, nil, 0666)
	}
}
