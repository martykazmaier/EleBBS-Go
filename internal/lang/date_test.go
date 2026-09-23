package lang

import (
	"testing"
	"time"
)

func TestFormatDateUserFormat5(t *testing.T) {
	d := time.Date(2026, 9, 20, 15, 4, 0, 0, time.Local)
	got := FormatDate(d, 5, nil)
	if got != "20-09-2026" {
		t.Fatalf("format 5: %q", got)
	}
	got = FormatDate(d, 2, nil)
	if got != "09-20-26" {
		t.Fatalf("format 2: %q", got)
	}
	got = FormatDate(d, 0, nil)
	if got != "20-09-2026" {
		t.Fatalf("format 0 defaults to 5: %q", got)
	}
}
