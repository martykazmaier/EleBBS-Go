package config

import (
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
)

// PathCandidates lists places to open an RA-style path such as
// C:\ELE\TXTFILES\WELCOME\WELCOME.TXT when SysPath is the real BBS root.
func PathCandidates(g *cfgrec.GlobalCfg, spec string, extra ...string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		for _, e := range out {
			if strings.EqualFold(e, p) {
				return
			}
		}
		out = append(out, p)
	}

	add(spec)
	if (strings.HasPrefix(spec, `\`) && !strings.HasPrefix(spec, `\\`)) ||
		(strings.HasPrefix(spec, `/`) && !strings.HasPrefix(spec, `//`)) {
		if cwd, err := os.Getwd(); err == nil {
			if vol := filepath.VolumeName(cwd); vol != "" {
				add(vol + spec)
			}
		}
	}

	rest, ok := afterEleRoot(spec)
	roots := extra
	if g != nil {
		roots = append(roots,
			g.RaConfig.SysPath,
			g.RaConfig.TextPath,
			g.RaConfig.MenuPath,
			filepath.Dir(g.CfgPath),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	if v := os.Getenv("ELEBBS"); v != "" {
		roots = append(roots, v)
	}
	if v := os.Getenv("RA"); v != "" {
		roots = append(roots, v)
	}

	for _, root := range roots {
		root = strings.TrimSpace(strings.TrimRight(root, `\/`))
		if root == "" {
			continue
		}
		if ok {
			add(filepath.Join(root, rest))
		}
		if !filepath.IsAbs(spec) {
			add(filepath.Join(root, spec))
		}
	}
	return out
}

func afterEleRoot(spec string) (string, bool) {
	norm := strings.ReplaceAll(spec, `/`, `\`)
	low := strings.ToLower(norm)
	const needle = `\ele\`
	i := strings.Index(low, needle)
	if i < 0 {
		if strings.HasSuffix(low, `\ele`) {
			return "", true
		}
		return "", false
	}
	return strings.TrimLeft(norm[i+len(needle):], `\/`), true
}

// ExistingFile returns the first PathCandidates entry that is a regular file.
func ExistingFile(g *cfgrec.GlobalCfg, spec string, extra ...string) string {
	for _, p := range PathCandidates(g, spec, extra...) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
