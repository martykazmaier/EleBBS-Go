package cfgrec

import "testing"

func TestSetNamedB2(t *testing.T) {
	var f Flag
	f.SetNamed("B2", true)
	if !f.HasNamed("B2") {
		t.Fatal("B2 should be on")
	}
	if f[1]&(1<<1) == 0 {
		t.Fatalf("B2 is bit 1 of flags[1], got %#v", f)
	}
	f.SetNamed("B2", false)
	if f.HasNamed("B2") && f[1]&(1<<1) != 0 {
		t.Fatal("B2 should be off")
	}
	f.ChangeMenu("B2+")
	if !f.HasNamed("B2") {
		t.Fatal("B2+ should set")
	}
	f.ChangeMenu("B2-")
	if f[1]&(1<<1) != 0 {
		t.Fatal("B2- should clear")
	}
}
