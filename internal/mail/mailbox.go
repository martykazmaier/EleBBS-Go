package mail

import (
	"elebbs/internal/cfgrec"
)

const MailBoxMax = 201

type MailHit struct {
	Area cfgrec.MessageArea
	Msgs []int
}

func (h MailHit) Count() int { return len(h.Msgs) }

func ScanMailbox(g *cfgrec.GlobalCfg, u cfgrec.User, checkGroup bool, group uint16, onArea func(a cfgrec.MessageArea, fileIdx int)) []MailHit {
	all := LoadAll(g)
	echoOK := u.Attribute2&cfgrec.User2NoEcho == 0
	var hits []MailHit
	total := 0
	remain := MailBoxMax
	for i, a := range all {
		if a.Name == "" || a.AreaNum == 0 {
			continue
		}
		if onArea != nil {
			onArea(a, i+1)
		}
		if remain <= 0 {
			break
		}
		if !a.IsJAM() || a.JAMBase == "" {
			continue
		}
		if !Accessible(a, u, checkGroup, group) {
			continue
		}
		if a.Typ == cfgrec.MsgEchoMail && !echoOK {
			continue
		}
		nums := collectYoursSafe(a.JAMBase, u.Name, u.Handle, remain)
		if len(nums) == 0 {
			continue
		}
		hits = append(hits, MailHit{Area: a, Msgs: nums})
		total += len(nums)
		remain = MailBoxMax - total
	}
	return hits
}

func collectYoursSafe(base, name, handle string, max int) (nums []int) {
	defer func() {
		if recover() != nil {
			nums = nil
		}
	}()
	return CollectYours(base, name, handle, max)
}
