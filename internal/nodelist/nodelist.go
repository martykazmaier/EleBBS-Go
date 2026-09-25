// Package nodelist reads the RemoteAccess nodelist index (NODEIDX.RA,
// NODEINC.RA) and the nodelists it points into (Pascal nodelst.pas).
package nodelist

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"strconv"
	"strings"

	"elebbs/internal/config"
)

const (
	idxSize = 10 // NodeIdxRecord
	incSize = 17 // NodeIncRecord = String[16]
)

// Idx is NodeIdxRecord.
type Idx struct {
	Type     byte // 0 zone, 1 region, 2 host
	Number   uint16
	Cost     uint16
	IncEntry byte // 1-based entry in NODEINC.RA
	Pointer  int32
}

// Entry is one nodelist line as GetNodeInfo splits it.
type Entry struct {
	Type, Number, Name, Location string
	Cost                         uint16
}

// List is a nodelist directory (RaConfig NodeListPath).
type List struct{ Dir string }

func (l List) find(name string) string {
	return config.FindFile([]string{l.Dir}, name, strings.ToLower(name), strings.ToUpper(name))
}

// HasIndex reports whether NODEIDX.RA can be opened.
func (l List) HasIndex() bool { return l.find("NODEIDX.RA") != "" }

// Index reads all of NODEIDX.RA.
func (l List) Index() ([]Idx, error) {
	p := l.find("NODEIDX.RA")
	if p == "" {
		return nil, os.ErrNotExist
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	out := make([]Idx, 0, len(raw)/idxSize)
	for off := 0; off+idxSize <= len(raw); off += idxSize {
		r := raw[off:]
		out = append(out, Idx{
			Type:     r[0],
			Number:   binary.LittleEndian.Uint16(r[1:]),
			Cost:     binary.LittleEndian.Uint16(r[3:]),
			IncEntry: r[5],
			Pointer:  int32(binary.LittleEndian.Uint32(r[6:])),
		})
	}
	return out, nil
}

// incEntry is GetIncEntry: the nodelist file name for a NODEINC.RA entry.
func (l List) incEntry(n byte) string {
	p := l.find("NODEINC.RA")
	if p == "" || n == 0 {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	var rec [incSize]byte
	if _, err := f.ReadAt(rec[:], int64(n-1)*incSize); err != nil {
		return ""
	}
	ln := int(rec[0])
	if ln > incSize-1 {
		ln = incSize - 1
	}
	return string(rec[1 : 1+ln])
}

// lines reads the nodelist of idx from its Pointer on; fn returns false to stop.
func (l List) lines(idx Idx, fn func(Entry) bool) {
	name := l.incEntry(idx.IncEntry)
	if name == "" {
		return
	}
	p := l.find(name)
	if p == "" {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(int64(idx.Pointer), io.SeekStart); err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<16)
	for sc.Scan() {
		e := parseLine(strings.TrimRight(sc.Text(), "\r\x1a"))
		e.Cost = idx.Cost
		if !fn(e) {
			return
		}
	}
}

func parseLine(s string) Entry {
	e := Entry{
		Type:     strings.ToUpper(under2Norm(commaArg(s, 1))),
		Number:   under2Norm(commaArg(s, 2)),
		Name:     under2Norm(commaArg(s, 3)),
		Location: under2Norm(commaArg(s, 4)),
	}
	if e.Type == "HOST" {
		e.Type = "NET"
	}
	return e
}

// commaArg is GetCommaArgument: the n-th (1-based) comma-separated field.
func commaArg(s string, n int) string {
	f := strings.Split(s, ",")
	if n < 1 || n > len(f) {
		return ""
	}
	return f[n-1]
}

func under2Norm(s string) string { return strings.ReplaceAll(s, "_", " ") }

func fval(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// segmentEnd returns a check for the ZONE/REGION/NET line after the
// segment header of idx, where its listing stops.
func segmentEnd(idx Idx) func(Entry) bool {
	first := true
	return func(e Entry) bool {
		head := first
		first = false
		switch e.Type {
		case "NET", "REGION", "ZONE":
			return !head || fval(e.Number) != int(idx.Number)
		}
		return false
	}
}

// Search is NodeIncSearch: the 1-based index record of zone (net 0) or of
// zone:net, or 0 when it is not listed.
func (l List) Search(zone, net uint16) int {
	idx, err := l.Index()
	if err != nil {
		return 0
	}
	var curZone, curNet uint16
	for i, r := range idx {
		if r.Type == 0 {
			curZone = r.Number
		}
		if (r.Type == 1 || r.Type == 2) && curZone == zone {
			curNet = r.Number
		}
		if curZone == zone && (net == 0 || net == curNet) {
			return i + 1
		}
	}
	return 0
}

// Node is GetNodeNumber: node in the segment of index record pos.
func (l List) Node(pos, node int) (Entry, bool) {
	idx, err := l.Index()
	if err != nil || pos < 1 || pos > len(idx) {
		return Entry{}, false
	}
	var found Entry
	ok := false
	end := segmentEnd(idx[pos-1])
	l.lines(idx[pos-1], func(e Entry) bool {
		if end(e) {
			return false
		}
		switch e.Type {
		case "", "HUB", "PVT", "HOLD", "DOWN":
			if fval(e.Number) == node {
				found, ok = e, true
				return false
			}
		}
		return true
	})
	return found, ok
}

// Lookup is SearchNodeList.
func (l List) Lookup(zone, net uint16, node int) (Entry, bool) {
	pos := l.Search(zone, net)
	if pos == 0 {
		return Entry{}, false
	}
	return l.Node(pos, node)
}

// info is GetNodeInfo: the nodelist line an index record points at.
func (l List) info(idx Idx) Entry {
	var out Entry
	l.lines(idx, func(e Entry) bool {
		out = e
		return false
	})
	return out
}

// Zones is ListIDX(nlZones): every zone line.
func (l List) Zones() []Entry {
	idx, _ := l.Index()
	var out []Entry
	for _, r := range idx {
		if r.Type == 0 {
			out = append(out, l.info(r))
		}
	}
	return out
}

// Nets is ListIDX(nlNets): the regions and nets after the zone record pos,
// up to the next zone.
func (l List) Nets(pos int) []Entry {
	idx, _ := l.Index()
	if pos < 1 || pos > len(idx) {
		return nil
	}
	curZone := idx[pos-1].Number
	var out []Entry
	for _, r := range idx[pos:] {
		if r.Type == 0 && r.Number != curZone {
			break
		}
		if r.Type != 0 {
			out = append(out, l.info(r))
		}
	}
	return out
}

// Nodes is ListAllNodes: every line in the segment of index record pos.
func (l List) Nodes(pos int) []Entry {
	idx, _ := l.Index()
	if pos < 1 || pos > len(idx) {
		return nil
	}
	var out []Entry
	end := segmentEnd(idx[pos-1])
	l.lines(idx[pos-1], func(e Entry) bool {
		if end(e) {
			return false
		}
		out = append(out, e)
		return true
	})
	return out
}
