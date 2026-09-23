package online

import (
	"testing"

	"elebbs/internal/cfgrec"
)

func TestUserOnRoundTrip(t *testing.T) {
	in := Record{
		Name:       "Martin Kazmaier",
		Handle:     "Shurato",
		Line:       2,
		Baud:       65529,
		City:       "Calgary",
		Status:     StatusBrowsing,
		Attribute:  AttrReady,
		NoCalls:    12,
		NodeNumber: 2,
	}
	got := Decode(Encode(in))
	if got.Name != in.Name || got.Handle != in.Handle || got.Line != in.Line || got.Baud != in.Baud {
		t.Fatalf("got %+v", got)
	}
	if got.City != in.City || got.NodeNumber != in.NodeNumber || got.NoCalls != in.NoCalls {
		t.Fatalf("got %+v", got)
	}
	if len(Encode(in)) != RecordSize {
		t.Fatalf("size %d", len(Encode(in)))
	}
}

func TestFirstFreeRecyclesLowestNode(t *testing.T) {
	inUse := map[int]bool{2: true}
	if n := FirstFree(1, 10, inUse); n != 1 {
		t.Fatalf("got %d want 1", n)
	}
	inUse[1] = true
	if n := FirstFree(1, 10, inUse); n != 3 {
		t.Fatalf("got %d want 3", n)
	}
	delete(inUse, 1)
	if n := FirstFree(1, 10, inUse); n != 1 {
		t.Fatalf("after free got %d want 1", n)
	}
	full := map[int]bool{}
	for i := 1; i <= 3; i++ {
		full[i] = true
	}
	if n := FirstFree(1, 3, full); n != 0 {
		t.Fatalf("full got %d want 0", n)
	}
}

func TestEmptyNodeNrRecyclesHole(t *testing.T) {
	dir := t.TempDir()
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	n1 := &cfgrec.LineCfg{RaNodeNr: 1, User: cfgrec.User{Name: "A"}}
	n2 := &cfgrec.LineCfg{RaNodeNr: 2, User: cfgrec.User{Name: "B"}}
	if err := Write(g, n1, "", StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	if err := Write(g, n2, "", StatusBrowsing); err != nil {
		t.Fatal(err)
	}
	Kill(g, n1)
	if n := EmptyNodeNr(g, false); n != 1 {
		t.Fatalf("EmptyNodeNr=%d want 1", n)
	}
}
