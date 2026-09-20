package files

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func GroupsPath(g *cfgrec.GlobalCfg) string {
	p := filepath.Join(g.RaConfig.SysPath, "fgroups.ra")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(g.RaConfig.SysPath, "FGROUPS.RA")
}

func LoadGroups(g *cfgrec.GlobalCfg) []cfgrec.Group {
	b, err := os.ReadFile(GroupsPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.GroupSize
	out := make([]cfgrec.Group, 0, n)
	for i := 0; i < n; i++ {
		gr := cfgrec.ParseGroup(b[i*cfgrec.GroupSize : (i+1)*cfgrec.GroupSize])
		if gr.Name == "" && gr.AreaNum == 0 {
			continue
		}
		out = append(out, gr)
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

func FindGroupDir(groups []cfgrec.Group, dir string) (cfgrec.Group, bool) {
	want := pascal.UpCase(ConvDirName(dir))
	for _, g := range groups {
		if pascal.UpCase(ConvDirName(g.Name)) == want {
			return g, true
		}
	}
	return cfgrec.Group{}, false
}

func FindAreaDir(areas []cfgrec.FilesArea, dir string) (cfgrec.FilesArea, bool) {
	want := pascal.UpCase(ConvDirName(dir))
	for _, a := range areas {
		if pascal.UpCase(ConvDirName(a.Name)) == want {
			return a, true
		}
	}
	return cfgrec.FilesArea{}, false
}

func LoadAllGroups(g *cfgrec.GlobalCfg) []cfgrec.Group {
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

func AreaInGroup(a cfgrec.FilesArea, group uint16) bool {
	if group == 0 || a.Attrib2&1 != 0 {
		return true
	}
	if a.Group == group {
		return true
	}
	for _, g := range a.AltGroup {
		if g == group {
			return true
		}
	}
	return false
}

func GroupAccess(gr cfgrec.Group, u cfgrec.User) bool {
	if gr.Security == 0 {
		return true
	}
	return u.Security >= gr.Security
}

func ListAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	if a.ListSecurity == 0 {
		return true
	}
	return u.Security >= a.ListSecurity
}

func DownloadAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	if a.Security == 0 {
		return true
	}
	return u.Security >= a.Security
}

func UploadAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	if a.UploadSecurity == 0 {
		return u.Security >= a.Security
	}
	return u.Security >= a.UploadSecurity
}

// ConvDirName matches EleBBS FTP: illegal path chars become underscores.
func ConvDirName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == ' ' || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case unicode.IsControl(r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func MatchWild(pat, name string) bool {
	if pat == "" || pat == "*" {
		return true
	}
	return MatchName(pat, name)
}
