package lang

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestRalGetStrStripsKeysAndKeepsColor(t *testing.T) {
	// Two prompts starting at file offset 300/320 (Hi=1 encoding).
	b := make([]byte, 340)
	binary.LittleEndian.PutUint16(b[0:], 2)
	binary.LittleEndian.PutUint16(b[2:], 300)
	binary.LittleEndian.PutUint16(b[4:], 320)
	b[299] = 0x0F
	copy(b[300:], []byte("Y Yes"))
	b[305] = 255
	b[319] = 0x07
	copy(b[320:], []byte("N No"))
	b[324] = 255
	f := parse(b)
	if f == nil || f.n != 2 {
		t.Fatalf("parse n=%v", f)
	}
	if k := f.GetKeys(Yes); k != "Y" {
		t.Fatalf("GetKeys(Yes)=%q", k)
	}
	got := f.GetStr(Yes)
	if got != "\x0b[0FYes" && got != "`[0FYes" {
		// color prefix is CP437 0x0B '[' '0' 'F'
		if len(got) < 4 || got[1] != '[' {
			t.Fatalf("GetStr(Yes)=%q", got)
		}
		if got[len(got)-3:] != "Yes" {
			t.Fatalf("GetStr(Yes) body %q", got)
		}
	}
	if f.GetStr(No)[len(f.GetStr(No))-2:] != "No" {
		t.Fatalf("GetStr(No)=%q", f.GetStr(No))
	}
}

func TestEmptyRalFallsBackToDefaults(t *testing.T) {
	f := &File{}
	if f.Get(Password) != "Password: " {
		t.Fatalf("default Password %q", f.Get(Password))
	}
	if f.GetStr(AskLoc) == "" {
		t.Fatal("AskLoc default empty")
	}
}

func encodeRalOff(pos int) uint16 {
	for hi := 0; hi < 256; hi++ {
		s := pos + 255
		if hi != 0 {
			s = pos + hi - 1
		}
		if s >= 0 && s <= 65535 && s>>8 == hi {
			return uint16(s)
		}
	}
	return uint16(pos)
}

func TestLoadedRalEmpty473IsNotStockLoading(t *testing.T) {
	const n = 473
	header := 2 * (n + 1)
	body := make([]byte, 0, n*2)
	off := make([]uint16, n+1)
	off[0] = n
	pos := header
	for i := 1; i < n; i++ {
		body = append(body, 0x07, 'x')
		off[i] = encodeRalOff(pos + 1)
		pos += 2
	}
	off[n] = encodeRalOff(pos) // #473 is empty (start at EOF)
	b := make([]byte, header+len(body))
	for i, o := range off {
		binary.LittleEndian.PutUint16(b[i*2:], o)
	}
	copy(b[header:], body)
	f := parse(b)
	if f == nil || f.n != n {
		t.Fatalf("parse n=%v", f)
	}
	got := f.Get(Loading)
	if got != "" {
		t.Fatalf("empty RAL #473 should stay empty, got %q", got)
	}
	if f.GetStr(Loading) != "" {
		t.Fatalf("GetStr(#473)=%q, want empty", f.GetStr(Loading))
	}
	if f.Get(1) == "" {
		t.Fatal("prompt 1 missing")
	}
}

func TestLoadedRalCustom473IsUsed(t *testing.T) {
	const n = 473
	header := 2 * (n + 1)
	custom := []byte{0x0B, '@', 'L', 'O', 'A', 'D', '|'}
	body := make([]byte, 0, n*2+len(custom))
	off := make([]uint16, n+1)
	off[0] = n
	pos := header
	for i := 1; i <= n; i++ {
		body = append(body, 0x0E)
		pos++
		start := pos
		if i == Loading {
			body = append(body, custom...)
			pos += len(custom)
		} else {
			body = append(body, 'x')
			pos++
		}
		off[i] = encodeRalOff(start)
	}
	b := make([]byte, header+len(body))
	for i, o := range off {
		binary.LittleEndian.PutUint16(b[i*2:], o)
	}
	copy(b[header:], body)
	f := parse(b)
	if f == nil {
		t.Fatal("parse")
	}
	got := f.Get(Loading)
	if !strings.Contains(got, "\x0b@LOAD|") && !strings.Contains(got, "@LOAD|") {
		t.Fatalf("RAL #473 custom prompt missing, got %q", got)
	}
	if strings.Contains(got, "Loading") {
		t.Fatalf("stock Loading text leaked over RAL #473: %q", got)
	}
}
