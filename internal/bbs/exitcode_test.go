package bbs

import (
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/cmdline"
)

func TestExitCodeReportsMailEntered(t *testing.T) {
	for _, tc := range []struct {
		net, echo bool
		def, want int
	}{
		{false, false, 0, 0},
		{false, false, 10, 10},
		{true, false, 0, 3},
		{false, true, 0, 4},
		{true, true, 0, 5},
		{true, true, 10, 5},
	} {
		s := &Session{
			Line: &cfgrec.LineCfg{NetMailEntered: tc.net, EchoMailEntered: tc.echo},
			Opt:  cmdline.Options{ExitCode: tc.def},
		}
		if got := s.ExitCode(); got != tc.want {
			t.Fatalf("net=%v echo=%v -E%d: errorlevel %d, want %d", tc.net, tc.echo, tc.def, got, tc.want)
		}
	}
}
