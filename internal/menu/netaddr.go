package menu

import (
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/lang"
	"elebbs/internal/mail"
	"elebbs/internal/nodelist"
	"elebbs/internal/pascal"
)

type nlListType int

const (
	nlZones nlListType = iota
	nlNets
	nlNodes
)

func (e *Engine) nodelist() nodelist.List {
	if e.G == nil {
		return nodelist.List{}
	}
	return nodelist.List{Dir: e.G.RaConfig.Nodelist}
}

// askAddress is Pascal AskAddress: ask a netmail address until one is
// given (looked up in the nodelist, or sent anyway) or the input is empty.
func (e *Engine) askAddress(addr string, a cfgrec.MessageArea) string {
	for {
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.Address))
		if addr != "" {
			e.T.WriteRA("`A3:" + addr)
			e.T.Println("")
			return addr
		}
		s, err := e.T.GetString(15, false, true)
		if err != nil {
			return ""
		}
		addr = pascal.Trim(s)
		if addr == "" {
			return ""
		}
		e.T.WriteRA(e.normAttr())
		e.T.Println("")
		if strings.Contains(addr, "?") {
			addr = e.handleNodeListInput(addr, a)
		}
		if addr != "" {
			save := addr
			addr = e.matchNodeList(addr, a)
			if addr == "" && e.T.AskYesNo(lang.NetSend, false) {
				addr = save
			}
		}
		e.T.Println("")
		if addr != "" {
			return addr
		}
	}
}

func (e *Engine) normAttr() string {
	attr := 7
	if e.G != nil {
		attr = int(e.G.RaConfig.NormFore) + int(e.G.RaConfig.NormBack)<<4
	}
	return "`A" + strconv.Itoa(attr) + ":"
}

// splitZoneNet is the Pascal FVal(Copy(...)) pair of MatchNodeList and
// HandleNodeListInput: the zone before ':' and the net before '/'.
func splitZoneNet(s string) (zone, net uint16) {
	c := strings.Index(s, ":")
	if c > 0 {
		zone = uint16(fval(s[:c]))
	}
	if sl := strings.Index(s, "/"); sl > c {
		net = uint16(fval(s[c+1 : sl]))
	}
	return zone, net
}

func fval(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// handleNodeListInput is Pascal HandleNodeListInput: "?" lists the zones,
// "z:?" the nets of zone z, "z:n/?" the nodes of net n. A listing
// empties the input.
func (e *Engine) handleNodeListInput(s string, a cfgrec.MessageArea) string {
	if strings.HasPrefix(s, "?") {
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA("`A15:" + raduCenter(e.T.RalGet(lang.AvailZone), 80))
		e.T.Println("")
		e.T.Println("")
		e.listNodeList(nlZones, 0, 0)
		s = ""
	}
	aka := mail.AreaAka(e.G, a)
	zone, nets := aka.Zone, aka.Net
	if s != "" && strings.Contains(s, ":") && !strings.Contains(s, "/") && strings.Contains(s, "?") {
		if z, _ := splitZoneNet(s); z > 0 {
			zone = z
		}
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA("`A15:" + raduCenter(e.T.RalGet(lang.NetsRegs)+" "+strconv.Itoa(int(zone)), 80))
		e.T.Println("")
		e.T.Println("")
		e.listNodeList(nlNets, zone, 0)
		s = ""
	}
	if s != "" && strings.Contains(s, "/") {
		z, n := splitZoneNet(s)
		if z > 0 {
			zone = z
		}
		if n > 0 {
			nets = n
		}
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA("`A15:" + raduCenter(e.T.RalGet(lang.NodesIn)+" "+e.T.RalGet(lang.Net)+" "+strconv.Itoa(int(nets)), 80))
		e.T.Println("")
		e.T.Println("")
		e.listNodeList(nlNodes, zone, nets)
		s = ""
	}
	return s
}

// nodeIncSearch is Pascal NodeIncSearch, including its "not listed" line.
func (e *Engine) nodeIncSearch(zone, net uint16) int {
	nl := e.nodelist()
	if !nl.HasIndex() {
		return 0
	}
	pos := nl.Search(zone, net)
	if pos == 0 {
		e.T.Println("")
		e.T.WriteRA("`A11:" + e.T.RalGet(lang.SorryNo) + " " + e.T.RalGet(lang.Net) + " " + strconv.Itoa(int(net)) + " " +
			e.T.RalGet(lang.Listed) + " " + e.T.RalGet(lang.Zone) + " in " + strconv.Itoa(int(zone)))
		e.T.Println("")
	}
	return pos
}

// listNodeList is Pascal ListNodeList.
func (e *Engine) listNodeList(kind nlListType, zone, net uint16) {
	nl := e.nodelist()
	if kind == nlZones && !nl.HasIndex() {
		e.T.WriteRA("`A12:")
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.NoNLhelp))
		e.T.Println("")
		e.T.PressEnter()
		return
	}
	var list []nodelist.Entry
	switch kind {
	case nlZones:
		list = nl.Zones()
	case nlNets:
		pos := e.nodeIncSearch(zone, net)
		if pos == 0 {
			return
		}
		list = nl.Nets(pos)
	case nlNodes:
		pos := e.nodeIncSearch(zone, net)
		if pos == 0 {
			return
		}
		list = nl.Nodes(pos)
	}
	hdr := e.T.RalGet(lang.NrName)
	e.T.WriteRA("`A10:" + hdr)
	e.T.Println("")
	e.T.WriteRA("`A02:" + strings.Repeat("─", noColorLen(hdr)))
	e.T.Println("")
	for _, n := range list {
		if e.T.StopMore {
			break
		}
		e.showNodeInfo(n)
	}
	e.T.Println("")
	e.T.Println("")
}

// showNodeInfo is Pascal ShowNodeInfo.
func (e *Engine) showNodeInfo(n nodelist.Entry) {
	if n.Name == "" {
		return
	}
	e.T.WriteRA("`X01:`A10:" + makeLen(n.Type, 6) +
		"`X11:`A15:" + makeLen(n.Number, 8) +
		"`X19:`A02:" + makeLen(n.Name, 34) +
		"`X54:`A02:" + makeLen(n.Location, 15) +
		"`X73:`A14:" + makeLen(strconv.Itoa(int(n.Cost)), 4))
	e.T.Println("")
}

// matchNodeList is Pascal MatchNodeList: look the address up (zone and
// net default to the area's AKA) and let the caller confirm it. It
// returns the full address, or "" when unknown or not confirmed.
func (e *Engine) matchNodeList(s string, a cfgrec.MessageArea) string {
	orig := s
	aka := mail.AreaAka(e.G, a)
	zone, nets := aka.Zone, aka.Net
	z, n := splitZoneNet(s)
	if z > 0 {
		zone = z
	}
	if n > 0 {
		nets = n
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	point := ""
	if i := strings.Index(s, "."); i >= 0 {
		if p := fval(s[i+1:]); p > 0 {
			point = "." + strconv.Itoa(p)
		}
		s = s[:i]
	}
	node := fval(s)
	e.T.WriteRA("`A3:")
	e.T.Println("")

	var entry nodelist.Entry
	found := false
	if pos := e.nodeIncSearch(zone, nets); pos > 0 {
		entry, found = e.nodelist().Node(pos, node)
	}
	if !found {
		e.T.WriteRA(e.T.RalGet(lang.NoNode) + " " + orig + ".")
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.NodelHdr))
		e.T.Println("")
		return ""
	}
	addr := strconv.Itoa(int(zone)) + ":" + strconv.Itoa(int(nets)) + "/" + strconv.Itoa(node) + point
	e.T.WriteRA(addr + ", " + entry.Name + ", " + entry.Location + ", " + strconv.Itoa(int(entry.Cost)) + " credits")
	e.T.Println("")
	if !e.T.AskYesNo(lang.Correct, false) {
		return ""
	}
	return addr
}

// raduCenter centres s (colour codes not counted) in a width-wide line.
func raduCenter(s string, width int) string {
	if n := noColorLen(s); n < width {
		return strings.Repeat(" ", (width-n)/2) + s
	}
	return s
}
