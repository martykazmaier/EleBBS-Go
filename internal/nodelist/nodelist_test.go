package nodelist

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeTestList writes a NODEIDX.RA / NODEINC.RA / NODELIST.001 set with
// zone 1 (net 2: nodes 3 and 4) and zone 2 (net 5: node 1).
func writeTestList(t *testing.T, dir string) {
	t.Helper()
	lines := []string{
		"Zone,1,Zone_One,Earth,Ann,1-1,300",
		"Host,2,Net_Two,Town,Bob,1-2,300",
		",3,Joe_BBS,Village,Joe,1-3,300",
		"Hub,4,Hub_BBS,Hamlet,Hal,1-4,300",
		"Zone,2,Zone_Two,Mars,Cy,2-1,300",
		"Host,5,Net_Five,Crater,Di,2-5,300",
		",1,Red_BBS,Dune,Ed,2-6,300",
	}
	var text []byte
	ptr := map[int]int32{}
	for i, l := range lines {
		ptr[i] = int32(len(text))
		text = append(text, l+"\r\n"...)
	}
	if err := os.WriteFile(filepath.Join(dir, "NODELIST.001"), text, 0644); err != nil {
		t.Fatal(err)
	}
	inc := make([]byte, incSize)
	inc[0] = byte(copy(inc[1:], "NODELIST.001"))
	if err := os.WriteFile(filepath.Join(dir, "NODEINC.RA"), inc, 0644); err != nil {
		t.Fatal(err)
	}
	var idx []byte
	rec := func(typ byte, num, cost uint16, line int) {
		r := make([]byte, idxSize)
		r[0] = typ
		binary.LittleEndian.PutUint16(r[1:], num)
		binary.LittleEndian.PutUint16(r[3:], cost)
		r[5] = 1
		binary.LittleEndian.PutUint32(r[6:], uint32(ptr[line]))
		idx = append(idx, r...)
	}
	rec(0, 1, 0, 0)
	rec(2, 2, 7, 1)
	rec(0, 2, 0, 4)
	rec(2, 5, 9, 5)
	if err := os.WriteFile(filepath.Join(dir, "NODEIDX.RA"), idx, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLookup(t *testing.T) {
	dir := t.TempDir()
	writeTestList(t, dir)
	nl := List{Dir: dir}
	e, ok := nl.Lookup(1, 2, 3)
	if !ok || e.Name != "Joe BBS" || e.Location != "Village" || e.Cost != 7 {
		t.Fatalf("1:2/3 = %+v %v", e, ok)
	}
	if e, ok := nl.Lookup(1, 2, 4); !ok || e.Name != "Hub BBS" {
		t.Fatalf("hub 1:2/4 = %+v %v", e, ok)
	}
	if _, ok := nl.Lookup(1, 2, 1); ok {
		t.Fatal("1:2/1 should not match a node of zone 2")
	}
	if e, ok := nl.Lookup(2, 5, 1); !ok || e.Name != "Red BBS" || e.Cost != 9 {
		t.Fatalf("2:5/1 = %+v %v", e, ok)
	}
	if nl.Search(1, 9) != 0 {
		t.Fatal("net 1:9 should not be listed")
	}
}

func TestListings(t *testing.T) {
	dir := t.TempDir()
	writeTestList(t, dir)
	nl := List{Dir: dir}
	zones := nl.Zones()
	if len(zones) != 2 || zones[0].Name != "Zone One" || zones[1].Type != "ZONE" {
		t.Fatalf("zones %+v", zones)
	}
	nets := nl.Nets(nl.Search(1, 0))
	if len(nets) != 1 || nets[0].Type != "NET" || nets[0].Number != "2" {
		t.Fatalf("nets %+v", nets)
	}
	nodes := nl.Nodes(nl.Search(1, 2))
	if len(nodes) != 3 || nodes[2].Name != "Hub BBS" {
		t.Fatalf("nodes %+v", nodes)
	}
}
