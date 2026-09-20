package mail

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/pascal"
)

type AreaStats struct {
	Active int
	First  int
	High   int
}

func jamOpenRW(base, ext string) (*os.File, error) {
	base = jamBase(base)
	f, err := os.OpenFile(base+ext, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		f, err = os.OpenFile(base+pascal.UpCase(ext), os.O_RDWR|os.O_CREATE, 0644)
	}
	return f, err
}

func jamBaseMsg(base string) int32 {
	f, err := jamOpenFile(base, ".jhd")
	if err != nil {
		return 1
	}
	defer f.Close()
	var b [24]byte
	if n, _ := f.Read(b[:]); n < 24 {
		return 1
	}
	n := int32(binary.LittleEndian.Uint32(b[20:]))
	if n < 1 {
		return 1
	}
	return n
}

func idxSlots(idx *os.File) int {
	st, err := idx.Stat()
	if err != nil || st.Size() < jamIdxSize {
		return 0
	}
	return int(st.Size() / jamIdxSize)
}

// Stats is Pascal GetMsgAreaStats for JAM: active, first, high — streamed.
func Stats(base string) AreaStats {
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return AreaStats{}
	}
	defer idx.Close()
	n := idxSlots(idx)
	if n == 0 {
		return AreaStats{}
	}
	baseNum := jamBaseMsg(base)
	hdr, err := jamOpenFile(base, ".jhr")
	if err != nil {
		return AreaStats{High: int(baseNum) + n - 1}
	}
	defer hdr.Close()
	var rec [8]byte
	st := AreaStats{}
	for i := 0; i < n; i++ {
		if _, err := idx.ReadAt(rec[:], jamIdxOff(i)); err != nil {
			break
		}
		loc := int32(binary.LittleEndian.Uint32(rec[4:]))
		if loc < 0 {
			continue
		}
		var h [jamMsgHdrSize]byte
		if _, err := hdr.ReadAt(h[:], int64(loc)); err != nil {
			continue
		}
		if string(h[:3]) != "JAM" {
			continue
		}
		attr := binary.LittleEndian.Uint32(h[52:])
		if attr&jamDeleted != 0 {
			continue
		}
		num := int(binary.LittleEndian.Uint32(h[48:]))
		if num <= 0 {
			num = int(baseNum) + i
		}
		st.Active++
		if st.First == 0 || num < st.First {
			st.First = num
		}
		if num > st.High {
			st.High = num
		}
	}
	return st
}

func LastRead(base, name, handle string) int {
	return int(jamLastRead(base, name, handle))
}

// jamFindMsg locates a JAM header by message number. Local bases store
// MsgNum as BaseMsgNum+slot; FidoNet echo often stores a water-mark MsgNum
// that is not a JDX slot index (mailbox scan returns that header number).
func jamFindMsg(idx, hdr *os.File, baseNum, num, n int) (loc int64, fallback int, ok bool) {
	if num < 1 || n <= 0 || idx == nil || hdr == nil {
		return 0, 0, false
	}
	match := func(slot int) (int64, int, bool) {
		if slot < 0 || slot >= n {
			return 0, 0, false
		}
		var rec [8]byte
		if _, err := idx.ReadAt(rec[:], jamIdxOff(slot)); err != nil {
			return 0, 0, false
		}
		off := int32(binary.LittleEndian.Uint32(rec[4:]))
		if off < 0 {
			return 0, 0, false
		}
		var h [jamMsgHdrSize]byte
		if _, err := hdr.ReadAt(h[:], int64(off)); err != nil {
			return 0, 0, false
		}
		if string(h[:3]) != "JAM" {
			return 0, 0, false
		}
		if binary.LittleEndian.Uint32(h[52:])&jamDeleted != 0 {
			return 0, 0, false
		}
		msgNum := int(binary.LittleEndian.Uint32(h[48:]))
		fb := baseNum + slot
		if msgNum <= 0 {
			msgNum = fb
		}
		if msgNum != num {
			return 0, 0, false
		}
		return int64(off), fb, true
	}
	if loc, fb, ok := match(num - baseNum); ok {
		return loc, fb, true
	}
	buf := make([]byte, jamIdxChunk*jamIdxSize)
	for start := 0; start < n; start += jamIdxChunk {
		chunk := jamIdxChunk
		if chunk > n-start {
			chunk = n - start
		}
		nb := chunk * jamIdxSize
		nr, err := idx.ReadAt(buf[:nb], jamIdxOff(start))
		if nr < jamIdxSize {
			break
		}
		got := nr / jamIdxSize
		for i := 0; i < got; i++ {
			off := i * jamIdxSize
			idxLoc := int32(binary.LittleEndian.Uint32(buf[off+4:]))
			if idxLoc < 0 {
				continue
			}
			if loc, fb, ok := match(start + i); ok {
				return loc, fb, true
			}
		}
		if err != nil {
			break
		}
	}
	return 0, 0, false
}

// ReadMsg loads one JAM message by number without reading the whole base.
func ReadMsg(base string, num int) (Article, bool) {
	if num < 1 {
		return Article{}, false
	}
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return Article{}, false
	}
	defer idx.Close()
	hdr, err := jamOpenFile(base, ".jhr")
	if err != nil {
		return Article{}, false
	}
	defer hdr.Close()
	txt, _ := jamOpenFile(base, ".jdt")
	if txt != nil {
		defer txt.Close()
	}
	baseNum := int(jamBaseMsg(base))
	loc, fb, ok := jamFindMsg(idx, hdr, baseNum, num, idxSlots(idx))
	if !ok {
		return Article{}, false
	}
	return articleAt(hdr, txt, loc, fb)
}

func NextActive(base string, from int, forward bool) (Article, bool) {
	if from < 1 {
		from = 1
	}
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return Article{}, false
	}
	defer idx.Close()
	baseNum := int(jamBaseMsg(base))
	n := idxSlots(idx)
	if n == 0 {
		return Article{}, false
	}
	hdr, err := jamOpenFile(base, ".jhr")
	if err != nil {
		return Article{}, false
	}
	defer hdr.Close()
	txt, _ := jamOpenFile(base, ".jdt")
	if txt != nil {
		defer txt.Close()
	}
	start := from - baseNum
	if forward {
		if start < 0 || start >= n {
			start = 0
		}
		var rec [8]byte
		try := func(i int) (Article, bool) {
			if _, err := idx.ReadAt(rec[:], jamIdxOff(i)); err != nil {
				return Article{}, false
			}
			loc := int32(binary.LittleEndian.Uint32(rec[4:]))
			if loc < 0 {
				return Article{}, false
			}
			a, ok := articleAt(hdr, txt, int64(loc), baseNum+i)
			if !ok || a.Num < from {
				return Article{}, false
			}
			return a, true
		}
		for i := start; i < n; i++ {
			if a, ok := try(i); ok {
				return a, true
			}
		}
		if start > 0 {
			for i := 0; i < start; i++ {
				if a, ok := try(i); ok {
					return a, true
				}
			}
		}
		return Article{}, false
	}
	if start >= n {
		start = n - 1
	}
	if start < 0 {
		start = n - 1
	}
	var rec [8]byte
	for i := start; i >= 0; i-- {
		if _, err := idx.ReadAt(rec[:], jamIdxOff(i)); err != nil {
			break
		}
		loc := int32(binary.LittleEndian.Uint32(rec[4:]))
		if loc < 0 {
			continue
		}
		if a, ok := articleAt(hdr, txt, int64(loc), baseNum+i); ok && (from <= 1 || a.Num <= from) {
			return a, true
		}
	}
	return Article{}, false
}

func articleAt(hdr, txt *os.File, loc int64, fallback int) (Article, bool) {
	var h [jamMsgHdrSize]byte
	if _, err := hdr.ReadAt(h[:], loc); err != nil {
		return Article{}, false
	}
	if string(h[:3]) != "JAM" {
		return Article{}, false
	}
	attr := binary.LittleEndian.Uint32(h[52:])
	if attr&jamDeleted != 0 {
		return Article{}, false
	}
	subLen := int(binary.LittleEndian.Uint32(h[8:]))
	textOfs := int64(binary.LittleEndian.Uint32(h[60:]))
	textLen := int(binary.LittleEndian.Uint32(h[64:]))
	msgNum := int(binary.LittleEndian.Uint32(h[48:]))
	dateUnix := int64(binary.LittleEndian.Uint32(h[36:]))
	a := Article{
		Num:      msgNum,
		Date:     time.Unix(dateUnix, 0),
		Received: attr&jamRcvd != 0,
		Sent:     attr&jamSent != 0,
		Private:  attr&jamPrivate != 0,
		Attr:     attr,
	}
	if a.Num == 0 {
		a.Num = fallback
	}
	if subLen > 0 && subLen <= jamSubMax {
		sub := make([]byte, subLen)
		if _, err := hdr.ReadAt(sub, loc+jamMsgHdrSize); err == nil {
			parseJamSubs(sub, &a)
		}
	}
	if txt != nil && textLen > 0 && textOfs >= 0 {
		raw := make([]byte, textLen)
		if n, err := txt.ReadAt(raw, textOfs); err == nil || n > 0 {
			a.Body, a.Kludges = splitJamText(string(raw[:n]))
		}
	}
	a.Bytes = len(a.Body)
	a.Lines = countLines(a.Body)
	return a, true
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	if len(s) > 0 && s[len(s)-1] == '\n' {
		n--
	}
	return n
}

// SetReceived is Pascal SetRcvd + ReWriteHdr: mark one JAM message received.
func SetReceived(base string, num int) {
	jamSetAttrBit(base, num, jamRcvd)
}

// SetSent is Pascal SetSent + ReWriteHdr: mark one JAM message sent (Jam_Sent).
func SetSent(base string, num int) {
	jamSetAttrBit(base, num, jamSent)
}

func jamSetAttrBit(base string, num int, bit uint32) {
	if num < 1 {
		return
	}
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return
	}
	defer idx.Close()
	hdr, err := jamOpenRW(base, ".jhr")
	if err != nil {
		return
	}
	defer hdr.Close()
	loc, _, ok := jamFindMsg(idx, hdr, int(jamBaseMsg(base)), num, idxSlots(idx))
	if !ok {
		return
	}
	var attrBuf [4]byte
	if _, err := hdr.ReadAt(attrBuf[:], loc+52); err != nil {
		return
	}
	attr := binary.LittleEndian.Uint32(attrBuf[:]) | bit
	binary.LittleEndian.PutUint32(attrBuf[:], attr)
	_, _ = hdr.WriteAt(attrBuf[:], loc+52)
}

func CanRead(a Article, u cfgrec.User, sysop bool) bool {
	if !a.Private {
		return true
	}
	if sysop {
		return true
	}
	return yoursHeader(a.To, u.Name, u.Handle) || yoursHeader(a.From, u.Name, u.Handle)
}

func AttrString(a Article) string {
	if a.Received {
		return "Rcvd"
	}
	if a.Private {
		return "Pvt"
	}
	return ""
}

func SetLastRead(base, name, handle string, high int) {
	if high < 0 {
		high = 0
	}
	f, err := jamOpenRW(base, ".jlr")
	if err != nil {
		return
	}
	defer f.Close()
	nameCrc := crc.Jam(name)
	var rec [16]byte
	off := int64(0)
	for {
		n, err := f.ReadAt(rec[:], off)
		if n < 16 {
			break
		}
		if int32(binary.LittleEndian.Uint32(rec[0:])) == nameCrc {
			binary.LittleEndian.PutUint32(rec[8:], uint32(high))
			binary.LittleEndian.PutUint32(rec[12:], uint32(high))
			_, _ = f.WriteAt(rec[:], off)
			return
		}
		off += 16
		if err != nil {
			break
		}
	}
	binary.LittleEndian.PutUint32(rec[0:], uint32(nameCrc))
	binary.LittleEndian.PutUint32(rec[4:], 0)
	binary.LittleEndian.PutUint32(rec[8:], uint32(high))
	binary.LittleEndian.PutUint32(rec[12:], uint32(high))
	st, _ := f.Stat()
	at := int64(0)
	if st != nil {
		at = st.Size()
	}
	_, _ = f.WriteAt(rec[:], at)
}

func jamSub(loid uint16, s string) []byte {
	b := []byte(s)
	out := make([]byte, 8+len(b))
	binary.LittleEndian.PutUint16(out[0:], loid)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(b)))
	copy(out[8:], b)
	return out
}

func jamEnsureJhd(base string, active, baseNum int32) {
	f, err := jamOpenRW(base, ".jhd")
	if err != nil {
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	buf := make([]byte, jamJhdSize)
	if st != nil && st.Size() >= 24 {
		_, _ = f.ReadAt(buf[:24], 0)
	}
	copy(buf[0:4], "JAM\x00")
	if binary.LittleEndian.Uint32(buf[4:]) == 0 {
		binary.LittleEndian.PutUint32(buf[4:], uint32(time.Now().Unix()))
	}
	mod := binary.LittleEndian.Uint32(buf[8:]) + 1
	binary.LittleEndian.PutUint32(buf[8:], mod)
	binary.LittleEndian.PutUint32(buf[12:], uint32(active))
	if binary.LittleEndian.Uint32(buf[20:]) == 0 {
		if baseNum < 1 {
			baseNum = 1
		}
		binary.LittleEndian.PutUint32(buf[20:], uint32(baseNum))
	}
	_, _ = f.WriteAt(buf, 0)
}

// AppendMsg writes a new JAM message. Returns the assigned number.
func AppendMsg(base string, a Article) (int, error) {
	idx, err := jamOpenRW(base, ".jdx")
	if err != nil {
		return 0, err
	}
	defer idx.Close()
	hdr, err := jamOpenRW(base, ".jhr")
	if err != nil {
		return 0, err
	}
	defer hdr.Close()
	txt, err := jamOpenRW(base, ".jdt")
	if err != nil {
		return 0, err
	}
	defer txt.Close()

	baseNum := jamBaseMsg(base)
	slots := idxSlots(idx)
	num := int(baseNum) + slots
	if a.Date.IsZero() {
		a.Date = time.Now()
	}
	body := a.Body
	if body != "" && !strings.HasSuffix(body, "\n") && !strings.HasSuffix(body, "\r") {
		body += "\r\n"
	}
	raw := []byte(stringsCRLF(body))
	for _, k := range a.Kludges {
		raw = append(raw, 0x01)
		raw = append(raw, []byte(k)...)
		raw = append(raw, '\r')
	}

	txtSt, _ := txt.Stat()
	textOfs := int64(0)
	if txtSt != nil {
		textOfs = txtSt.Size()
	}
	if _, err := txt.WriteAt(raw, textOfs); err != nil {
		return 0, err
	}

	var subs []byte
	subs = append(subs, jamSub(2, a.From)...)
	subs = append(subs, jamSub(3, a.To)...)
	subs = append(subs, jamSub(6, a.Subject)...)
	if a.MsgID != "" {
		subs = append(subs, jamSub(4, a.MsgID)...)
	}
	if a.ReplyID != "" {
		subs = append(subs, jamSub(5, a.ReplyID)...)
	}

	hdrSt, _ := hdr.Stat()
	loc := int64(0)
	if hdrSt != nil {
		loc = hdrSt.Size()
	}

	h := make([]byte, jamMsgHdrSize+len(subs))
	copy(h[0:4], "JAM\x00")
	binary.LittleEndian.PutUint16(h[4:], 1)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(subs)))
	binary.LittleEndian.PutUint32(h[36:], uint32(a.Date.Unix()))
	binary.LittleEndian.PutUint32(h[48:], uint32(num))
	attr := a.Attr
	if attr == 0 {
		attr = jamLocal | jamTypeLocal
	}
	if a.Private {
		attr |= jamPrivate
	}
	binary.LittleEndian.PutUint32(h[52:], attr)
	binary.LittleEndian.PutUint32(h[60:], uint32(textOfs))
	binary.LittleEndian.PutUint32(h[64:], uint32(len(raw)))
	copy(h[jamMsgHdrSize:], subs)
	if _, err := hdr.WriteAt(h, loc); err != nil {
		return 0, err
	}

	var rec [8]byte
	binary.LittleEndian.PutUint32(rec[0:], uint32(crc.Jam(a.To)))
	binary.LittleEndian.PutUint32(rec[4:], uint32(loc))
	idxSt, _ := idx.Stat()
	at := int64(0)
	if idxSt != nil {
		at = idxSt.Size()
	}
	if _, err := idx.WriteAt(rec[:], at); err != nil {
		return 0, err
	}
	st := Stats(base)
	jamEnsureJhd(base, int32(st.Active), baseNum)
	return num, nil
}

func stringsCRLF(s string) string {
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i == 0 || s[i-1] != '\r' {
				out = append(out, '\r')
			}
			out = append(out, '\n')
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func OriginLine(g *cfgrec.GlobalCfg, a cfgrec.MessageArea) string {
	orig := pascal.Trim(a.OriginLine)
	if orig == "" && g != nil {
		orig = pascal.Trim(g.RaConfig.SystemName)
	}
	addr := ""
	if g != nil && int(a.AkaAddress) < len(g.RaConfig.Address) {
		ak := g.RaConfig.Address[a.AkaAddress]
		if ak.Zone != 0 || ak.Net != 0 {
			if ak.Point != 0 {
				addr = fmt.Sprintf("%d:%d/%d.%d", ak.Zone, ak.Net, ak.Node, ak.Point)
			} else {
				addr = fmt.Sprintf("%d:%d/%d", ak.Zone, ak.Net, ak.Node)
			}
		}
	}
	if addr != "" {
		return fmt.Sprintf(" * Origin: %s (%s)", orig, addr)
	}
	if orig != "" {
		return " * Origin: " + orig
	}
	return ""
}

func TearLine() string {
	return "--- " + cfgrec.PidName
}

func MsgIDKludge(g *cfgrec.GlobalCfg, a cfgrec.MessageArea, num int) string {
	ak := cfgrec.Addr{}
	if g != nil && int(a.AkaAddress) < len(g.RaConfig.Address) {
		ak = g.RaConfig.Address[a.AkaAddress]
	}
	return fmt.Sprintf("MSGID: %d:%d/%d %08x", ak.Zone, ak.Net, ak.Node, uint32(time.Now().Unix())^uint32(num))
}
