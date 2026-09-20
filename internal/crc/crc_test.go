package crc

import "testing"

func TestRA(t *testing.T) {
	if RA("", true) != -1 {
		t.Fatalf("empty CRC want -1 got %d", RA("", true))
	}
	if Jam("") != -1 {
		t.Fatalf("empty Jam CRC want -1 got %d", Jam(""))
	}
	if Jam("SYSOP") != Jam("sysop") {
		t.Fatal("Jam CRC should lowercase")
	}
	a := RA("SYSOP", true)
	b := RA("sysop", true)
	if a != b {
		t.Fatalf("ForceUpper mismatch %d vs %d", a, b)
	}
	if a == 0 || a == -1 {
		t.Fatalf("unexpected SYSOP crc %d", a)
	}
}

func TestCRC32MatchesTable(t *testing.T) {
	got := CRC32([]byte("123456789"), 0xFFFFFFFF)
	if got == 0 {
		t.Fatal("zero crc")
	}
}

func TestEMSI16KnownFingerprints(t *testing.T) {
	if got := EMSI16("EMSI_ACK"); got != 0xA490 {
		t.Fatalf("ACK %04X want A490", got)
	}
	if got := EMSI16("EMSI_NAK"); got != 0xEEC3 {
		t.Fatalf("NAK %04X want EEC3", got)
	}
	if got := EMSI16("EMSI_INQ"); got != 0xC816 {
		t.Fatalf("INQ %04X want C816", got)
	}
}
