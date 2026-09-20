package mail

import (
	"os"
	"path/filepath"

	"elebbs/internal/cfgrec"
)

func GroupsPath(g *cfgrec.GlobalCfg) string {
	p := filepath.Join(g.RaConfig.SysPath, "mgroups.ra")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(g.RaConfig.SysPath, "MGROUPS.RA")
}

func LoadGroups(g *cfgrec.GlobalCfg) []cfgrec.Group {
	b, err := os.ReadFile(GroupsPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.GroupSize
	out := make([]cfgrec.Group, n)
	for i := 0; i < n; i++ {
		out[i] = cfgrec.ParseGroup(b[i*cfgrec.GroupSize : (i+1)*cfgrec.GroupSize])
	}
	return out
}

func FindGroup(groups []cfgrec.Group, num uint16) (cfgrec.Group, bool) {
	for _, g := range groups {
		if g.AreaNum == num {
			return g, true
		}
	}
	return cfgrec.Group{}, false
}

func GroupIndex(all []cfgrec.Group, num uint16) int {
	for i, g := range all {
		if g.AreaNum == num {
			return i + 1
		}
	}
	return 0
}

func GroupAccess(gr cfgrec.Group, u cfgrec.User) bool {
	if gr.Name == "" {
		return false
	}
	if gr.Security == 0 {
		return true
	}
	return u.Security >= gr.Security
}
