package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func AreasPath(g *cfgrec.GlobalCfg) string {
	p := filepath.Join(g.RaConfig.SysPath, "messages.ra")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(g.RaConfig.SysPath, "MESSAGES.RA")
}

func LoadAreas(g *cfgrec.GlobalCfg) []cfgrec.MessageArea {
	b, err := os.ReadFile(AreasPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.MessageSize
	out := make([]cfgrec.MessageArea, 0, n)
	for i := 0; i < n; i++ {
		a := cfgrec.ParseMessageArea(b[i*cfgrec.MessageSize : (i+1)*cfgrec.MessageSize])
		if a.Name == "" && a.AreaNum == 0 {
			continue
		}
		out = append(out, a)
	}
	return out
}

func LoadAll(g *cfgrec.GlobalCfg) []cfgrec.MessageArea {
	b, err := os.ReadFile(AreasPath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.MessageSize
	out := make([]cfgrec.MessageArea, n)
	for i := 0; i < n; i++ {
		out[i] = cfgrec.ParseMessageArea(b[i*cfgrec.MessageSize : (i+1)*cfgrec.MessageSize])
	}
	return out
}

func FindArea(areas []cfgrec.MessageArea, num uint16) (cfgrec.MessageArea, bool) {
	for _, a := range areas {
		if a.AreaNum == num {
			return a, true
		}
	}
	return cfgrec.MessageArea{}, false
}

func InGroup(a cfgrec.MessageArea, group uint16) bool {
	if group == 0 || a.Attribute2&1 != 0 {
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

func Accessible(a cfgrec.MessageArea, u cfgrec.User, checkGroup bool, group uint16) bool {
	if a.Name == "" || a.AreaNum == 0 {
		return false
	}
	if a.ReadSecurity > 0 && u.Security < a.ReadSecurity {
		return false
	}
	if checkGroup && !InGroup(a, group) {
		return false
	}
	return true
}

func WriteAccessible(a cfgrec.MessageArea, u cfgrec.User) bool {
	if a.Name == "" || a.AreaNum == 0 {
		return false
	}
	if a.MsgKinds == cfgrec.MsgKindROnly && u.Security != a.SysopSecurity {
		return false
	}
	if a.WriteSecurity > 0 && u.Security < a.WriteSecurity {
		return false
	}
	return true
}

func SearchNext(all []cfgrec.MessageArea, group uint16, cur uint16) uint16 {
	for _, a := range all {
		if a.Name == "" || a.AreaNum == 0 {
			continue
		}
		if a.Group == group || a.Attribute2&1 != 0 {
			return a.AreaNum
		}
		for _, g := range a.AltGroup {
			if g == group {
				return a.AreaNum
			}
		}
	}
	return cur
}

func FileIndex(all []cfgrec.MessageArea, areaNum uint16) int {
	for i, a := range all {
		if a.AreaNum == areaNum {
			return i + 1
		}
	}
	return 0
}

func SelectArea(t *term.IO, areas []cfgrec.MessageArea, cur uint16) uint16 {
	t.Println("")
	t.WriteRA("`A15:" + t.RalStr(lang.MsgAreas) + "`A7:")
	t.Println("")
	for _, a := range areas {
		if a.Name == "" {
			continue
		}
		kind := "Local"
		switch a.Typ {
		case cfgrec.MsgNetMail:
			kind = "Netmail"
		case cfgrec.MsgEchoMail:
			kind = "Echo"
		case cfgrec.MsgInternet:
			kind = "Internet"
		case cfgrec.MsgNews:
			kind = "News"
		}
		base := "Hudson"
		if a.IsJAM() {
			base = "JAM"
		}
		mark := " "
		if a.AreaNum == cur {
			mark = ">"
		}
		t.Println(fmt.Sprintf("%s %3d  %-8s %-7s  %s", mark, a.AreaNum, kind, base, a.Name))
	}
	t.WriteRA("`A15:" + t.RalStr(lang.SelArea))
	s, _ := t.GetString(5, false, false)
	s = pascal.Trim(s)
	if s == "" {
		return cur
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if _, ok := FindArea(areas, uint16(n)); ok {
		return uint16(n)
	}
	return cur
}

func ListHeaders(t *term.IO, area cfgrec.MessageArea) {
	t.WriteRA(fmt.Sprintf("`A15:Message area %d: %s`A7:\r\n", area.AreaNum, area.Name))
	if area.IsJAM() && area.JAMBase != "" {
		listJAM(t, area.JAMBase)
		return
	}
	t.Println("Hudson/Squish message reading is not ported yet in this Go node.")
	t.Println("Area base: " + area.JAMBase)
}

func listJAM(t *term.IO, base string) {
	base = strings.TrimSuffix(base, ".")
	idx, err := os.ReadFile(base + ".jdx")
	if err != nil {
		idx, err = os.ReadFile(base + ".JDX")
	}
	if err != nil {
		t.Println("Cannot open JAM index: " + base)
		return
	}
	n := len(idx) / 8
	t.Println(fmt.Sprintf("%d index slots.", n))
	active := 0
	for i := 0; i < n; i++ {
		off := pascal.I32(idx, i*8+4)
		if off != -1 {
			active++
		}
	}
	t.Println(fmt.Sprintf("%d active messages (JAM). Full reader still uses EleBBS JAM units.", active))
}
