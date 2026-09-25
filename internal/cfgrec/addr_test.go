package cfgrec

import "testing"

func TestParseAddr(t *testing.T) {
	def := Addr{Zone: 2, Net: 280, Node: 1, Point: 5}
	for _, tc := range []struct {
		in, want string
	}{
		{"", "2:280/1.5"},
		{"7", "2:280/7"},
		{"100/7", "2:100/7"},
		{"1:100/7", "1:100/7"},
		{"1:100/7.3", "1:100/7.3"},
		{"3:", "3:0/1"},
		{".9", "2:280/1.9"},
	} {
		if got := ParseAddr(tc.in, def).String(); got != tc.want {
			t.Errorf("ParseAddr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
