package online

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

const (
	RecordSize  = 214
	WebNodeBase = 1000

	AttrHidden   = 1 << 0
	AttrWantChat = 1 << 1
	AttrQuiet    = 1 << 3
	AttrReady    = 1 << 6

	StatusBrowsing = 0
	StatusXfer     = 1
	StatusMsgs     = 2
	StatusDoor     = 3
	StatusChat     = 4
	StatusQuest    = 5
	StatusConf     = 6
	StatusLogon    = 7
	StatusCustom   = 255
)

// Record is Pascal USERONrecord (PACKRECORDS 1).
type Record struct {
	Name       string
	Handle     string
	Line       byte
	Baud       uint16
	City       string
	Status     byte
	Attribute  byte
	StatDesc   string
	NoCalls    uint16
	NodeNumber int32
	LastUpdate int32
}

func (r Record) Hidden() bool { return r.Attribute&AttrHidden != 0 }
func (r Record) Quiet() bool  { return r.Attribute&AttrQuiet != 0 }

func (r Record) Visible() bool {
	if r.Hidden() || pascal.Trim(r.Name) == "" {
		return false
	}
	return r.Line > 0 || r.NodeNumber > 0
}

func (r Record) DisplayLine() string {
	if r.NodeNumber != 0 {
		return strconv.FormatInt(int64(r.NodeNumber), 10)
	}
	return strconv.Itoa(int(r.Line))
}

// FirstFree is Pascal FindEmptyNode: lowest unused node from StartNodeWith.
func FirstFree(start, maxSessions int, inUse map[int]bool) int {
	if start < 1 {
		start = 1
	}
	if maxSessions < 1 {
		maxSessions = 1
	}
	end := start + maxSessions - 1
	if end > 255 {
		end = 255
	}
	for n := start; n <= end; n++ {
		if inUse == nil || !inUse[n] {
			return n
		}
	}
	return 0
}

// EmptyNodeNr is Pascal EmptyNodeNr: first USERON.BBS hole, else append.
func EmptyNodeNr(g *cfgrec.GlobalCfg, eleweb bool) int {
	start := 1
	if eleweb {
		start = WebNodeBase
	}
	b, err := os.ReadFile(Path(g))
	if err != nil || len(b) < RecordSize {
		return start
	}
	n := len(b) / RecordSize
	from := 0
	if eleweb {
		from = WebNodeBase - 1
		if from < 0 {
			from = 0
		}
	}
	for i := from; i < n; i++ {
		rec := Decode(b[i*RecordSize : (i+1)*RecordSize])
		if pascal.Trim(rec.Name) == "" || (rec.Line == 0 && rec.NodeNumber == 0) {
			return i + 1
		}
	}
	next := n + 1
	if eleweb && next <= WebNodeBase {
		next = WebNodeBase + 1
	}
	if next < start {
		return start
	}
	return next
}

func Path(g *cfgrec.GlobalCfg) string {
	if g == nil {
		return "useron.bbs"
	}
	return filepath.Join(strings.TrimRight(g.RaConfig.SysPath, `\/`), "useron.bbs")
}

func Encode(r Record) []byte {
	var w pascal.Writer
	w.PString(35, r.Name)
	w.PString(35, r.Handle)
	w.U8(r.Line)
	w.U16(r.Baud)
	w.PString(25, r.City)
	w.U8(r.Status)
	w.U8(r.Attribute)
	w.PString(10, r.StatDesc)
	w.Pad(90)
	w.U16(r.NoCalls)
	w.I32(r.NodeNumber)
	w.I32(r.LastUpdate)
	b := w.B
	if len(b) < RecordSize {
		b = append(b, make([]byte, RecordSize-len(b))...)
	}
	return b[:RecordSize]
}

func Decode(b []byte) Record {
	if len(b) < RecordSize {
		tmp := make([]byte, RecordSize)
		copy(tmp, b)
		b = tmp
	}
	r := pascal.NewBuf(b)
	var rec Record
	rec.Name = r.PString(35)
	rec.Handle = r.PString(35)
	rec.Line = r.U8()
	rec.Baud = r.U16()
	rec.City = r.PString(25)
	rec.Status = r.U8()
	rec.Attribute = r.U8()
	rec.StatDesc = r.PString(10)
	r.Skip(90)
	rec.NoCalls = r.U16()
	rec.NodeNumber = r.I32()
	rec.LastUpdate = r.I32()
	return rec
}

func ReadAll(g *cfgrec.GlobalCfg) []Record {
	b, err := os.ReadFile(Path(g))
	if err != nil {
		return nil
	}
	n := len(b) / RecordSize
	out := make([]Record, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Decode(b[i*RecordSize:(i+1)*RecordSize]))
	}
	return out
}

func ReadSlot(g *cfgrec.GlobalCfg, lineNr int) (Record, bool) {
	if lineNr < 1 {
		return Record{}, false
	}
	b, err := os.ReadFile(Path(g))
	if err != nil {
		return Record{}, false
	}
	off := (lineNr - 1) * RecordSize
	if off < 0 || off+RecordSize > len(b) {
		return Record{}, false
	}
	return Decode(b[off : off+RecordSize]), true
}

func Write(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, statDesc string, status int) error {
	if g == nil || line == nil || pascal.Trim(line.User.Name) == "" {
		return nil
	}
	node := line.RaNodeNr
	if node < 1 {
		node = 1
	}
	nCalls := int(line.User.NoCalls)
	if nCalls > 65535 {
		nCalls = 65535
	}
	rec := Record{
		Name:       line.User.Name,
		Handle:     line.User.Handle,
		Baud:       line.Baud,
		City:       line.User.Location,
		Status:     byte(status),
		StatDesc:   strings.ReplaceAll(statDesc, "_", " "),
		NoCalls:    uint16(nCalls),
		NodeNumber: int32(node),
		LastUpdate: int32(time.Now().Unix()),
	}
	if node <= 255 {
		rec.Line = byte(node)
	}
	if line.User.Attribute2&cfgrec.User2Hidden != 0 {
		rec.Attribute |= AttrHidden
	} else {
		rec.Attribute |= AttrReady
	}
	if line.User.Attribute&cfgrec.UserQuiet != 0 {
		rec.Attribute |= AttrQuiet
	}
	if rec.StatDesc != "" {
		rec.Status = StatusCustom
	}
	return writeSlot(Path(g), node, rec)
}

func Kill(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) {
	if g == nil || line == nil {
		return
	}
	node := line.RaNodeNr
	if node < 1 {
		return
	}
	_ = writeSlot(Path(g), node, Record{})
}

func writeSlot(path string, node int, rec Record) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	need := int64((node - 1) * RecordSize)
	empty := make([]byte, RecordSize)
	for st.Size() < need {
		if _, err := f.Write(empty); err != nil {
			return err
		}
		st, _ = f.Stat()
	}
	if _, err := f.Seek(need, 0); err != nil {
		return err
	}
	_, err = f.Write(Encode(rec))
	return err
}
