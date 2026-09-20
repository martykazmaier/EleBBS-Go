package mail

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
)

func TestCountYoursStreamsHeaders(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "test")
	writeTestJam(t, base, []testMsg{
		{to: "Sysop", loc: jamBaseSize},
		{to: "Other", loc: jamBaseSize + 200},
		{to: "sysop", loc: jamBaseSize + 400, rcvd: true},
	})
	if n := CountYours(base, "Sysop", "", 201); n != 1 {
		t.Fatalf("CountYours=%d want 1", n)
	}
	if n := CountYours(base, "Nobody", "", 201); n != 0 {
		t.Fatalf("nobody CountYours=%d want 0", n)
	}
}

func TestCountYoursHandle(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "hdl")
	writeTestJam(t, base, []testMsg{
		{to: "Alias", loc: jamBaseSize},
	})
	if n := CountYours(base, "Real Name", "Alias", 201); n != 1 {
		t.Fatalf("handle CountYours=%d want 1", n)
	}
}

func TestCollectYoursAndSetReceived(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "inbox")
	writeTestJam(t, base, []testMsg{
		{to: "Sysop", loc: jamBaseSize},
		{to: "Other", loc: jamBaseSize + 200},
	})
	got := CollectYours(base, "Sysop", "", 201)
	if len(got) != 1 || got[0] < 1 {
		t.Fatalf("CollectYours=%v", got)
	}
	art, ok := ReadMsg(base, got[0])
	if !ok || !strings.EqualFold(art.To, "Sysop") {
		t.Fatalf("ReadMsg %v ok=%v to=%q", got[0], ok, art.To)
	}
	SetReceived(base, got[0])
	if n := CountYours(base, "Sysop", "", 201); n != 0 {
		t.Fatalf("after SetReceived CountYours=%d", n)
	}
}

func TestHasNewMailUsesLastRead(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "new")
	writeTestJam(t, base, []testMsg{{to: "Sysop", loc: jamBaseSize}})
	a := cfgrec.MessageArea{Attribute: 1 << 7, JAMBase: base}
	if !HasNewMail(a, "Sysop", "") {
		t.Fatal("expected new mail with no .jlr")
	}
	high := uint32(jamHighMsg(base))
	var rec [16]byte
	binary.LittleEndian.PutUint32(rec[0:], uint32(crc.Jam("Sysop")))
	binary.LittleEndian.PutUint32(rec[8:], high)
	binary.LittleEndian.PutUint32(rec[12:], high)
	if err := os.WriteFile(base+".jlr", rec[:], 0644); err != nil {
		t.Fatal(err)
	}
	if HasNewMail(a, "Sysop", "") {
		t.Fatal("did not expect new mail when LastRead >= high")
	}
}

func TestHasNewMailUsesLastReadNotHighRead(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "gap")
	loc := jamBaseSize
	hdr := make([]byte, loc+jamMsgHdrSize)
	copy(hdr[0:4], "JAM\x00")
	h := hdr[loc:]
	copy(h[0:4], "JAM\x00")
	binary.LittleEndian.PutUint16(h[4:], 1)
	binary.LittleEndian.PutUint32(h[48:], 5)
	idx := make([]byte, jamIdxSize)
	binary.LittleEndian.PutUint32(idx[4:], uint32(loc))
	if err := os.WriteFile(base+".jdx", idx, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".jhr", hdr, 0644); err != nil {
		t.Fatal(err)
	}
	a := cfgrec.MessageArea{Attribute: 1 << 7, JAMBase: base}
	var rec [16]byte
	binary.LittleEndian.PutUint32(rec[0:], uint32(crc.Jam("Sysop")))
	binary.LittleEndian.PutUint32(rec[8:], 4)
	binary.LittleEndian.PutUint32(rec[12:], 5)
	if err := os.WriteFile(base+".jlr", rec[:], 0644); err != nil {
		t.Fatal(err)
	}
	if !HasNewMail(a, "Sysop", "") {
		t.Fatal("expected * when LastRead is behind even if HighRead equals high")
	}
}

func TestHasNewMailZeroMsgNumUsesSlot(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "local")
	loc := jamBaseSize
	hdr := make([]byte, loc+jamMsgHdrSize)
	copy(hdr[0:4], "JAM\x00")
	h := hdr[loc:]
	copy(h[0:4], "JAM\x00")
	binary.LittleEndian.PutUint16(h[4:], 1)
	idx := make([]byte, jamIdxSize)
	binary.LittleEndian.PutUint32(idx[4:], uint32(loc))
	if err := os.WriteFile(base+".jdx", idx, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".jhr", hdr, 0644); err != nil {
		t.Fatal(err)
	}
	if jamHighMsg(base) < 1 {
		t.Fatal("MsgNum 0 should fall back to BaseMsgNum+slot")
	}
	a := cfgrec.MessageArea{Attribute: 1 << 7, JAMBase: base}
	if !HasNewMail(a, "Sysop", "") {
		t.Fatal("local JAM with MsgNum 0 should still show new mail")
	}
}

func TestJamHighMsgUsesHeaderNumNotSlotCount(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "echo")
	loc := jamBaseSize
	hdr := make([]byte, loc+jamMsgHdrSize+16)
	copy(hdr[0:4], "JAM\x00")
	h := hdr[loc:]
	copy(h[0:4], "JAM\x00")
	binary.LittleEndian.PutUint16(h[4:], 1)
	binary.LittleEndian.PutUint32(h[48:], 50000)
	nUnused := 20000
	idx := make([]byte, (1+nUnused)*jamIdxSize)
	binary.LittleEndian.PutUint32(idx[4:], uint32(loc))
	for i := 1; i <= nUnused; i++ {
		binary.LittleEndian.PutUint32(idx[i*jamIdxSize+4:], 0xFFFFFFFF)
	}
	if err := os.WriteFile(base+".jdx", idx, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".jhr", hdr, 0644); err != nil {
		t.Fatal(err)
	}
	got := jamHighMsg(base)
	if got != 50000 {
		t.Fatalf("jamHighMsg=%d want 50000 (FidoNet header MsgNum, not 1+slots-1)", got)
	}
	a := cfgrec.MessageArea{Attribute: 1 << 7, JAMBase: base}
	if !HasNewMail(a, "Sysop", "") {
		t.Fatal("echo should show new mail when HighRead is missing")
	}
}

func TestReadMsgByEchoHeaderNum(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "echo")
	loc := jamBaseSize
	to := []byte("Sysop")
	subLen := 8 + len(to)
	hdr := make([]byte, loc+jamMsgHdrSize+subLen)
	copy(hdr[0:4], "JAM\x00")
	h := hdr[loc:]
	copy(h[0:4], "JAM\x00")
	binary.LittleEndian.PutUint16(h[4:], 1)
	binary.LittleEndian.PutUint32(h[8:], uint32(subLen))
	binary.LittleEndian.PutUint32(h[48:], 50000)
	sub := h[jamMsgHdrSize:]
	binary.LittleEndian.PutUint16(sub[0:], 3)
	binary.LittleEndian.PutUint32(sub[4:], uint32(len(to)))
	copy(sub[8:], to)
	idx := make([]byte, jamIdxSize)
	binary.LittleEndian.PutUint32(idx[0:], uint32(crc.Jam("Sysop")))
	binary.LittleEndian.PutUint32(idx[4:], uint32(loc))
	if err := os.WriteFile(base+".jdx", idx, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".jhr", hdr, 0644); err != nil {
		t.Fatal(err)
	}
	got := CollectYours(base, "Sysop", "", 10)
	if len(got) != 1 || got[0] != 50000 {
		t.Fatalf("CollectYours=%v want [50000]", got)
	}
	art, ok := ReadMsg(base, got[0])
	if !ok {
		t.Fatal("ReadMsg failed on echo header MsgNum (mailbox Yes -> End of Messages)")
	}
	if !strings.EqualFold(art.To, "Sysop") || art.Num != 50000 {
		t.Fatalf("art %+v", art)
	}
	SetReceived(base, got[0])
	if n := CollectYours(base, "Sysop", "", 10); len(n) != 0 {
		t.Fatalf("SetReceived missed header MsgNum: %v", n)
	}
}

type testMsg struct {
	to   string
	loc  int
	rcvd bool
}

func writeTestJam(t *testing.T, base string, msgs []testMsg) {
	t.Helper()
	hdr := make([]byte, jamBaseSize+600)
	copy(hdr[0:4], "JAM\x00")
	idx := make([]byte, 0, (len(msgs)+1)*jamIdxSize)
	for _, m := range msgs {
		to := []byte(m.to)
		subLen := 8 + len(to)
		h := hdr[m.loc:]
		copy(h[0:4], "JAM\x00")
		binary.LittleEndian.PutUint16(h[4:], 1)
		binary.LittleEndian.PutUint32(h[8:], uint32(subLen))
		binary.LittleEndian.PutUint32(h[48:], 1)
		attr := uint32(0)
		if m.rcvd {
			attr |= jamRcvd
		}
		binary.LittleEndian.PutUint32(h[52:], attr)
		sub := h[jamMsgHdrSize:]
		binary.LittleEndian.PutUint16(sub[0:], 3) // MsgTo
		binary.LittleEndian.PutUint32(sub[4:], uint32(len(to)))
		copy(sub[8:], to)

		var rec [8]byte
		binary.LittleEndian.PutUint32(rec[0:], uint32(crc.Jam(m.to)))
		binary.LittleEndian.PutUint32(rec[4:], uint32(m.loc))
		idx = append(idx, rec[:]...)
	}
	var unused [8]byte
	binary.LittleEndian.PutUint32(unused[0:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(unused[4:], 0xFFFFFFFF)
	idx = append(idx, unused[:]...)
	if err := os.WriteFile(base+".jdx", idx, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".jhr", hdr, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAppendAndReadMsg(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "post")
	num, err := AppendMsg(base, Article{
		From:    "Sysop",
		To:      "All",
		Subject: "Hello",
		Body:    "Line one\nLine two\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if num < 1 {
		t.Fatalf("num %d", num)
	}
	a, ok := ReadMsg(base, num)
	if !ok {
		t.Fatal("ReadMsg failed")
	}
	if a.From != "Sysop" || a.To != "All" || a.Subject != "Hello" {
		t.Fatalf("hdr %+v", a)
	}
	if !strings.Contains(a.Body, "Line one") {
		t.Fatalf("body %q", a.Body)
	}
	st := Stats(base)
	if st.Active != 1 || st.High != num {
		t.Fatalf("stats %+v want active 1 high %d", st, num)
	}
	SetLastRead(base, "Sysop", "", num)
	if LastRead(base, "Sysop", "") != num {
		t.Fatalf("lastread %d", LastRead(base, "Sysop", ""))
	}
}

func TestNextActiveAfterAppendFindsEarlierUnread(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "more")
	for _, subj := range []string{"A", "B", "C"} {
		if _, err := AppendMsg(base, Article{From: "Sysop", To: "All", Subject: subj, Body: subj}); err != nil {
			t.Fatal(err)
		}
	}
	first, ok := NextActive(base, 1, true)
	if !ok {
		t.Fatal("first")
	}
	if _, err := AppendMsg(base, Article{From: "Bob", To: first.From, Subject: "Re: " + first.Subject, Body: "reply"}); err != nil {
		t.Fatal(err)
	}
	next, ok := NextActive(base, first.Num+1, true)
	if !ok {
		t.Fatal("next missing after reply")
	}
	if next.Subject != "B" {
		t.Fatalf("next subject %q want B", next.Subject)
	}
}
