package online

import "testing"

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
