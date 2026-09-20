package comm

import "testing"

func TestCP437VTPassesCSI(t *testing.T) {
	in := []byte("\x1b[1;37;40mHi\x1b[0m")
	got := CP437VT(in)
	if string(got) != string(in) {
		t.Fatalf("%q", got)
	}
}

func TestCP437VTBox(t *testing.T) {
	got := string(CP437VT([]byte{0xC9, 0xCD, 0xBB})) // ╔═╗
	if got != "╔═╗" {
		t.Fatalf("%q", got)
	}
}

func TestCP437VTMapsCtrlEToClub(t *testing.T) {
	got := string(CP437VT([]byte{0x05}))
	if got != "♣" {
		t.Fatalf("0x05 -> %q want club", got)
	}
	if bytesContainNUL(got) {
		t.Fatal("raw control leaked")
	}
}

func bytesContainNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0x05 {
			return true
		}
	}
	return false
}
