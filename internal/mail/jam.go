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

const (
	jamIdxSize    = 8
	jamMsgHdrSize = 76
	jamDeleted    = 0x80000000
	jamLocal      = 0x00000001
	jamPrivate    = 0x00000004
	jamRcvd       = 0x00000008
	jamSent       = 0x00000010
	jamFAttach    = 0x00002000
	jamTypeLocal  = 0x00800000
	jamTypeEcho   = 0x01000000
	jamTypeNet    = 0x02000000
	jamBaseSize   = 1024
	jamJhdSize    = 1024
)

type Article struct {
	Num      int
	From     string
	To       string
	Subject  string
	Date     time.Time
	MsgID    string
	ReplyID  string
	Body     string
	Kludges  []string
	Bytes    int
	Lines    int
	Deleted  bool
	Received bool
	Sent     bool
	Private  bool
	FAttach  bool
	Attr     uint32
}

func jamBase(path string) string {
	path = strings.TrimSpace(path)
	return strings.TrimRight(path, `\/`)
}

func jamIdxOff(slot int) int64 {
	return int64(slot) * jamIdxSize
}

func openJam(base, ext string) ([]byte, error) {
	base = jamBase(base)
	b, err := os.ReadFile(base + ext)
	if err != nil {
		b, err = os.ReadFile(base + strings.ToUpper(ext))
	}
	return b, err
}

func ReadJAM(base string) ([]Article, error) {
	idx, err := openJam(base, ".jdx")
	if err != nil {
		return nil, err
	}
	hdr, err := openJam(base, ".jhr")
	if err != nil {
		return nil, err
	}
	txt, _ := openJam(base, ".jdt")
	n := len(idx) / jamIdxSize
	out := make([]Article, 0, n)
	for i := 0; i < n; i++ {
		loc := int32(binary.LittleEndian.Uint32(idx[i*jamIdxSize+4:]))
		if loc < 0 || int(loc)+jamMsgHdrSize > len(hdr) {
			continue
		}
		h := hdr[int(loc):]
		if string(h[:3]) != "JAM" {
			continue
		}
		attr := binary.LittleEndian.Uint32(h[52:])
		if attr&jamDeleted != 0 {
			continue
		}
		subLen := int(binary.LittleEndian.Uint32(h[8:]))
		textOfs := int(binary.LittleEndian.Uint32(h[60:]))
		textLen := int(binary.LittleEndian.Uint32(h[64:]))
		msgNum := int(binary.LittleEndian.Uint32(h[48:]))
		dateUnix := int64(binary.LittleEndian.Uint32(h[36:]))
		a := Article{
			Num:      msgNum,
			Date:     time.Unix(dateUnix, 0).UTC(),
			Received: attr&jamRcvd != 0,
			Sent:     attr&jamSent != 0,
			Private:  attr&jamPrivate != 0,
			FAttach:  attr&jamFAttach != 0,
			Attr:     attr,
		}
		if a.Num == 0 {
			a.Num = i + 1
		}
		if subLen > 0 && int(loc)+jamMsgHdrSize+subLen <= len(hdr) {
			parseJamSubs(h[jamMsgHdrSize:jamMsgHdrSize+subLen], &a)
		}
		if textOfs >= 0 && textLen > 0 && textOfs+textLen <= len(txt) {
			raw := string(txt[textOfs : textOfs+textLen])
			a.Body, a.Kludges = splitJamText(raw)
		}
		if a.MsgID == "" {
			a.MsgID = fmt.Sprintf("%d@local", a.Num)
		}
		a.Bytes = len(a.Body)
		a.Lines = strings.Count(a.Body, "\n")
		if a.Body != "" && !strings.HasSuffix(a.Body, "\n") {
			a.Lines++
		}
		out = append(out, a)
	}
	return out, nil
}

const (
	jamIdxChunk    = 512
	jamSubMax      = 4000
	jamHighScanMax = 65536
	jamIdxSaneMax  = 2_000_000
)

func jamOpenFile(base, ext string) (*os.File, error) {
	base = jamBase(base)
	f, err := os.Open(base + ext)
	if err != nil {
		f, err = os.Open(base + strings.ToUpper(ext))
	}
	return f, err
}

// CountYours is Pascal DoQuickJamScan + YoursNext: walk .jdx CRCs, then seek
// only matching JHR headers. Never loads .jdt or the whole index/header file.
func CountYours(base, name, handle string, max int) int {
	return len(CollectYours(base, name, handle, max))
}

// CollectYours returns message numbers of unreceived mail to name/handle.
func CollectYours(base, name, handle string, max int) []int {
	if max <= 0 {
		return nil
	}
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return nil
	}
	defer idx.Close()
	nameCrc := crc.Jam(name)
	hdlCrc := crc.Jam(handle)
	wantHdl := pascal.Trim(handle) != "" && hdlCrc != nameCrc
	baseNum := jamBaseMsg(base)
	var hdr *os.File
	defer func() {
		if hdr != nil {
			hdr.Close()
		}
	}()
	var out []int
	buf := make([]byte, jamIdxChunk*jamIdxSize)
	slot := 0
	for len(out) < max {
		n, err := idx.Read(buf)
		if n < jamIdxSize {
			break
		}
		n -= n % jamIdxSize
		for off := 0; off < n && len(out) < max; off += jamIdxSize {
			toCrc := int32(binary.LittleEndian.Uint32(buf[off:]))
			loc := int32(binary.LittleEndian.Uint32(buf[off+4:]))
			this := slot
			slot++
			if toCrc != nameCrc && !(wantHdl && toCrc == hdlCrc) {
				continue
			}
			if loc < 0 {
				continue
			}
			if hdr == nil {
				hdr, err = jamOpenFile(base, ".jhr")
				if err != nil {
					return out
				}
			}
			num, ok := yoursAt(hdr, int64(loc), name, handle)
			if !ok {
				continue
			}
			if num < 1 {
				num = int(baseNum) + this
			}
			out = append(out, num)
		}
		if err != nil {
			break
		}
	}
	return out
}

func yoursAt(hdr *os.File, loc int64, name, handle string) (int, bool) {
	var h [jamMsgHdrSize]byte
	if _, err := hdr.ReadAt(h[:], loc); err != nil {
		return 0, false
	}
	if string(h[:3]) != "JAM" {
		return 0, false
	}
	attr := binary.LittleEndian.Uint32(h[52:])
	if attr&jamDeleted != 0 || attr&jamRcvd != 0 {
		return 0, false
	}
	subLen := binary.LittleEndian.Uint32(h[8:])
	if subLen == 0 || subLen > jamSubMax {
		return 0, false
	}
	sub := make([]byte, subLen)
	if _, err := hdr.ReadAt(sub, loc+jamMsgHdrSize); err != nil {
		return 0, false
	}
	var a Article
	parseJamSubs(sub, &a)
	if !yoursHeader(a.To, name, handle) {
		return 0, false
	}
	num := int(binary.LittleEndian.Uint32(h[48:]))
	return num, true
}

// HasNewMail is Pascal HasNewMail: JAM high message number > last-read pointer.
func HasNewMail(a cfgrec.MessageArea, name, handle string) bool {
	if !a.IsJAM() || strings.TrimSpace(a.JAMBase) == "" {
		return false
	}
	high := jamHighMsg(a.JAMBase)
	if high <= 0 {
		return false
	}
	return high > jamLastRead(a.JAMBase, name, handle)
}

func jamHighMsg(base string) int32 {
	idx, err := jamOpenFile(base, ".jdx")
	if err != nil {
		return 0
	}
	defer idx.Close()
	n := idxSlots(idx)
	if n <= 0 {
		return 0
	}
	baseNum := jamBaseMsg(base)
	var hdr *os.File
	defer func() {
		if hdr != nil {
			hdr.Close()
		}
	}()
	openHdr := func() bool {
		if hdr != nil {
			return true
		}
		var err error
		hdr, err = jamOpenFile(base, ".jhr")
		return err == nil
	}
	numAt := func(slot int, loc int32) (int32, bool) {
		if loc < jamBaseSize {
			return 0, false
		}
		if !openHdr() {
			return 0, false
		}
		var h [jamMsgHdrSize]byte
		if _, err := hdr.ReadAt(h[:], int64(loc)); err != nil {
			return 0, false
		}
		if string(h[:3]) != "JAM" {
			return 0, false
		}
		if binary.LittleEndian.Uint32(h[52:])&jamDeleted != 0 {
			return 0, false
		}
		num := int32(binary.LittleEndian.Uint32(h[48:]))
		if num <= 0 {
			num = baseNum + int32(slot)
		}
		if num <= 0 {
			return 0, false
		}
		return num, true
	}
	scan := func(from, to int, backward bool) int32 {
		if from < 0 {
			from = 0
		}
		if to > n {
			to = n
		}
		if from >= to {
			return 0
		}
		buf := make([]byte, jamIdxChunk*jamIdxSize)
		if backward {
			for end := to; end > from; {
				chunk := jamIdxChunk
				if chunk > end-from {
					chunk = end - from
				}
				start := end - chunk
				nb := chunk * jamIdxSize
				nr, err := idx.ReadAt(buf[:nb], jamIdxOff(start))
				if nr < jamIdxSize {
					break
				}
				got := nr / jamIdxSize
				for i := got - 1; i >= 0; i-- {
					off := i * jamIdxSize
					loc := int32(binary.LittleEndian.Uint32(buf[off+4:]))
					if loc < 0 {
						continue
					}
					if num, ok := numAt(start+i, loc); ok {
						return num
					}
				}
				end = start
				if err != nil {
					break
				}
			}
			return 0
		}
		var last int32
		for start := from; start < to; {
			chunk := jamIdxChunk
			if chunk > to-start {
				chunk = to - start
			}
			nb := chunk * jamIdxSize
			nr, err := idx.ReadAt(buf[:nb], jamIdxOff(start))
			if nr < jamIdxSize {
				break
			}
			got := nr / jamIdxSize
			for i := 0; i < got; i++ {
				off := i * jamIdxSize
				loc := int32(binary.LittleEndian.Uint32(buf[off+4:]))
				if loc < 0 {
					continue
				}
				if num, ok := numAt(start+i, loc); ok {
					last = num
				}
			}
			start += got
			if err != nil {
				break
			}
		}
		return last
	}
	// Last non-deleted header MsgNum (FidoNet echoes keep MSGID numbers).
	// Cap the walk so a 4GB sparse/corrupt .jdx cannot stall MA-CHNG.
	lo := 0
	if n > jamHighScanMax {
		lo = n - jamHighScanMax
	}
	if num := scan(lo, n, true); num > 0 {
		return num
	}
	if lo > 0 {
		if num := scan(0, jamHighScanMax, false); num > 0 {
			return num
		}
	}
	if n <= jamIdxSaneMax {
		high := baseNum + int32(n) - 1
		if high > 0 {
			return high
		}
	}
	return 0
}

func jamLastRead(base, name, handle string) int32 {
	f, err := jamOpenFile(base, ".jlr")
	if err != nil {
		return 0
	}
	defer f.Close()
	nameCrc := crc.Jam(name)
	hdlCrc := crc.Jam(handle)
	wantHdl := pascal.Trim(handle) != "" && hdlCrc != nameCrc
	var rec [16]byte
	for {
		n, err := f.Read(rec[:])
		if n < 16 {
			break
		}
		crc32 := int32(binary.LittleEndian.Uint32(rec[0:]))
		if crc32 == nameCrc || (wantHdl && crc32 == hdlCrc) {
			// LastRead (offset 8), not HighRead (12). A header scan can
			// bump HighRead to the water mark while the read pointer is still
			// behind; * and "new messages" follow LastRead.
			return int32(binary.LittleEndian.Uint32(rec[8:]))
		}
		if err != nil {
			break
		}
	}
	return 0
}

func yoursHeader(to, name, handle string) bool {
	to = pascal.UpCase(pascal.Trim(to))
	if to == "" {
		return false
	}
	if to == pascal.UpCase(pascal.Trim(name)) {
		return true
	}
	h := pascal.UpCase(pascal.Trim(handle))
	return h != "" && to == h
}

func parseJamSubs(buf []byte, a *Article) {
	off := 0
	for off+8 <= len(buf) {
		loid := binary.LittleEndian.Uint16(buf[off:])
		rawLen := binary.LittleEndian.Uint32(buf[off+4:])
		off += 8
		if rawLen > jamSubMax {
			break
		}
		dlen := int(rawLen)
		if off+dlen > len(buf) {
			break
		}
		s := strings.TrimRight(string(buf[off:off+dlen]), "\x00")
		off += dlen
		switch loid {
		case 2:
			a.From = s
		case 3:
			a.To = s
		case 4:
			a.MsgID = strings.TrimSpace(s)
		case 5:
			a.ReplyID = strings.TrimSpace(s)
		case 6:
			a.Subject = s
		}
	}
}

func splitJamText(raw string) (body string, kludges []string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	var lines []string
	for _, ln := range strings.Split(raw, "\n") {
		if strings.HasPrefix(ln, "\x01") {
			kludges = append(kludges, ln[1:])
			continue
		}
		lines = append(lines, ln)
	}
	return strings.Join(lines, "\n"), kludges
}

func NNTPDate(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format("02 Jan 2006 15:04:05 -0000")
}

func HostEmail(from, domain string) string {
	from = pascal.Trim(from)
	if from == "" {
		from = "unknown"
	}
	if strings.Contains(from, "@") {
		return from
	}
	local := strings.Map(func(r rune) rune {
		if r == ' ' {
			return '.'
		}
		return r
	}, from)
	if domain == "" {
		domain = "elebbs.bbs"
	}
	return fmt.Sprintf("%s <%s@%s>", from, local, domain)
}

func FindArticle(arts []Article, n int) (Article, bool) {
	for _, a := range arts {
		if a.Num == n {
			return a, true
		}
	}
	return Article{}, false
}

func FindArticleID(arts []Article, id string) (Article, bool) {
	id = strings.Trim(id, "<>")
	for _, a := range arts {
		if strings.EqualFold(strings.Trim(a.MsgID, "<>"), id) {
			return a, true
		}
	}
	return Article{}, false
}

func HighLow(arts []Article) (first, last int) {
	if len(arts) == 0 {
		return 0, 0
	}
	first, last = arts[0].Num, arts[0].Num
	for _, a := range arts[1:] {
		if a.Num < first {
			first = a.Num
		}
		if a.Num > last {
			last = a.Num
		}
	}
	return first, last
}
