package online

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
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

// ResolveSemaFile puts bare NODE<n>.RA / userdoes.<n> / rabusy.<n> names
// in SemPath (else SysPath), matching Pascal CheckAll / WHONLINE / DoRaBusy.
func ResolveSemaFile(g *cfgrec.GlobalCfg, spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" || g == nil {
		return spec
	}
	if filepath.IsAbs(spec) || strings.ContainsAny(spec, `\/`) {
		return spec
	}
	if !isSemaBaseName(spec) {
		return spec
	}
	return filepath.Join(SemPath(g), spec)
}

func isSemaBaseName(name string) bool {
	low := strings.ToLower(filepath.Base(name))
	if strings.HasPrefix(low, "node") && strings.HasSuffix(low, ".ra") {
		n := strings.TrimSuffix(strings.TrimPrefix(low, "node"), ".ra")
		if n == "" {
			return false
		}
		for _, c := range n {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	return strings.HasPrefix(low, "userdoes.") || strings.HasPrefix(low, "rabusy.")
}

func RaBusyPath(g *cfgrec.GlobalCfg, lineNr int) string {
	return filepath.Join(SemPath(g), "rabusy."+strconv.Itoa(lineNr))
}

// SetRaBusy is Pascal DoRaBusy: create or erase rabusy.<n> in SemPath.
func SetRaBusy(g *cfgrec.GlobalCfg, lineNr int, busy bool) {
	if g == nil || lineNr < 1 {
		return
	}
	p := RaBusyPath(g, lineNr)
	if busy {
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		_ = os.WriteFile(p, nil, 0666)
		return
	}
	_ = os.Remove(p)
}

func NodePath(g *cfgrec.GlobalCfg, lineNr int) string {
	return filepath.Join(SemPath(g), "NODE"+strconv.Itoa(lineNr)+".RA")
}

func nodePathCandidates(g *cfgrec.GlobalCfg, lineNr int) []string {
	n := strconv.Itoa(lineNr)
	names := []string{"NODE" + n + ".RA", "node" + n + ".ra"}
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		d = strings.TrimRight(strings.TrimSpace(d), `\/`)
		if d == "" {
			return
		}
		key := strings.ToLower(d)
		if seen[key] {
			return
		}
		seen[key] = true
		dirs = append(dirs, d)
	}
	add(SemPath(g))
	if g != nil {
		add(g.RaConfig.SysPath)
	}
	var out []string
	for _, d := range dirs {
		for _, name := range names {
			out = append(out, filepath.Join(d, name))
		}
	}
	return out
}

// FindNodeFile is Pascal CheckAll: NODE<n>.RA in SemPath, else SysPath.
func FindNodeFile(g *cfgrec.GlobalCfg, lineNr int) string {
	for _, p := range nodePathCandidates(g, lineNr) {
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() && st.Size() > 0 {
			return p
		}
	}
	return ""
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
	if _, err := f.Write(pascal.ToCP437("\r\n\r\n")); err != nil {
		return err
	}
	hdr := "\x0b]497 " + sender + " \x0b]496 " + strconv.Itoa(fromNode) + ":\r\n"
	if _, err := f.Write(pascal.ToCP437(hdr)); err != nil {
		return err
	}
	for _, ln := range lines {
		if _, err := f.Write(append(pascal.ToCP437(ln), '\r', '\n')); err != nil {
			return err
		}
	}
	_, err = f.Write(pascal.ToCP437("\r\n\x0b]258\x01\r\n"))
	return err
}

func NodeMsgReady(g *cfgrec.GlobalCfg, lineNr int) bool {
	return FindNodeFile(g, lineNr) != ""
}

func ClearNodeMsg(g *cfgrec.GlobalCfg, lineNr int) {
	for _, p := range nodePathCandidates(g, lineNr) {
		if err := os.Remove(p); err != nil {
			_ = os.WriteFile(p, nil, 0666)
		}
	}
}
