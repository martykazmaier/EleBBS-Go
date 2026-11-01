package files

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	if strings.TrimSpace(gr.Name) == "" || gr.AreaNum == 0 {
		return false
	}
	if !cfgrec.FlagAccess(u.Flags, gr.Flags, gr.NotFlags) {
		return false
	}
	return u.Security >= gr.Security
}

func ListAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	return CheckFileAreaAccess(a, u, false, false, false, true, 0)
}

func DownloadAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	return CheckFileAreaAccess(a, u, true, false, false, false, 0)
}

func UploadAccess(a cfgrec.FilesArea, u cfgrec.User) bool {
	return CheckFileAreaAccess(a, u, false, false, true, false, 0)
}

// CheckFileAreaAccess is Pascal access_u.CheckFileAreaAccess.
func CheckFileAreaAccess(a cfgrec.FilesArea, u cfgrec.User, download, group, upload, list bool, groupNr uint16) bool {
	if download {
		if a.Security > u.Security {
			return false
		}
		if !cfgrec.FlagAccess(u.Flags, a.Flags, a.NotFlags) {
			return false
		}
	}
	if upload {
		if a.UploadSecurity > u.Security {
			return false
		}
		if !cfgrec.FlagAccess(u.Flags, a.UploadFlags, a.UploadNotFlags) {
			return false
		}
	}
	if list {
		if a.ListSecurity > u.Security {
			return false
		}
		if !cfgrec.FlagAccess(u.Flags, a.ListFlags, a.ListNotFlags) {
			return false
		}
	}
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.FilePath) == "" || a.AreaNum == 0 {
		return false
	}
	if a.Age > userAge(u) {
		return false
	}
	if group && !AreaInGroup(a, groupNr) {
		return false
	}
	return true
}

func userAge(u cfgrec.User) byte {
	bd := strings.TrimSpace(u.BirthDate)
	if len(bd) < 8 {
		return 0
	}
	month, _ := strconv.Atoi(bd[0:2])
	day, _ := strconv.Atoi(bd[3:5])
	year, _ := strconv.Atoi(bd[6:8])
	if year > 10 {
		year += 1900
	} else {
		year += 2000
	}
	now := time.Now()
	age := now.Year() - year
	if month > int(now.Month()) || (month == int(now.Month()) && now.Day() < day) {
		age--
	}
	if age < 0 {
		return 0
	}
	if age > 255 {
		return 255
	}
	return byte(age)
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
