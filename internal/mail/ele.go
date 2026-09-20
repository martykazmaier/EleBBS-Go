package mail

import (
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func ElePath(g *cfgrec.GlobalCfg) string {
	p := filepath.Join(g.RaConfig.SysPath, "messages.ele")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(g.RaConfig.SysPath, "MESSAGES.ELE")
}

func LoadEleMessages(g *cfgrec.GlobalCfg) []cfgrec.EleMessage {
	b, err := os.ReadFile(ElePath(g))
	if err != nil {
		return nil
	}
	n := len(b) / cfgrec.EleMessageSize
	out := make([]cfgrec.EleMessage, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, cfgrec.ParseEleMessage(b[i*cfgrec.EleMessageSize:(i+1)*cfgrec.EleMessageSize]))
	}
	return out
}

func FindEle(eles []cfgrec.EleMessage, area uint16) (cfgrec.EleMessage, bool) {
	for _, e := range eles {
		if uint16(e.AreaNum) == area {
			return e, true
		}
	}
	return cfgrec.EleMessage{}, false
}

func Space2Dot(s string) string {
	s = pascal.Trim(s)
	s = strings.ReplaceAll(s, " ", ".")
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return strings.Trim(s, ".")
}

type NewsGroup struct {
	Name    string
	Area    cfgrec.MessageArea
	Ele     cfgrec.EleMessage
	Posting bool
}

func NewsGroups(g *cfgrec.GlobalCfg) []NewsGroup {
	areas := LoadAreas(g)
	eles := LoadEleMessages(g)
	out := make([]NewsGroup, 0, len(areas))
	if len(eles) > 0 {
		for _, a := range areas {
			el, ok := FindEle(eles, a.AreaNum)
			if !ok || !el.Usenet() || el.GroupName == "" || a.Name == "" {
				continue
			}
			out = append(out, NewsGroup{
				Name:    el.GroupName,
				Area:    a,
				Ele:     el,
				Posting: el.CanPost(),
			})
		}
		return out
	}
	for _, a := range areas {
		if a.Name == "" {
			continue
		}
		if !a.IsJAM() && a.Typ != cfgrec.MsgNews {
			continue
		}
		out = append(out, NewsGroup{
			Name:    Space2Dot(a.Name),
			Area:    a,
			Posting: true,
		})
	}
	return out
}

func FindNewsGroup(groups []NewsGroup, name string) (NewsGroup, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, g := range groups {
		if strings.ToLower(g.Name) == want {
			return g, true
		}
	}
	return NewsGroup{}, false
}
