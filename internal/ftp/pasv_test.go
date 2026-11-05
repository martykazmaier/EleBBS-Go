package ftp

import "testing"

func TestPickPasvPortStaysInRangeOn386(t *testing.T) {
	lo, hi := 65000, 65535
	span := hi - lo + 1
	for _, nano := range []int64{timeNanoOverflow32(), 1_736_000_000_000_000_000, -1} {
		for i := 0; i < 3; i++ {
			p := pickPasvPort(lo, span, nano, i)
			if p < lo || p > hi {
				t.Fatalf("port %d outside %d-%d nano=%d i=%d", p, lo, hi, nano, i)
			}
		}
	}
}

func timeNanoOverflow32() int64 {
	return 1_736_000_000_000_000_123
}
