//go:build windows

package door

import "testing"

func TestWindowsCmdLineKeepsFlags(t *testing.T) {
	got := windowsCmdLine(`c:\doors\lord.exe`, `-H204 -XT`)
	if got != `c:\doors\lord.exe -H204 -XT` {
		t.Fatalf("cmdline %q", got)
	}
	args := splitArgs(`-H204 -XT /N`)
	if len(args) != 3 || args[0] != "-H204" || args[1] != "-XT" || args[2] != "/N" {
		t.Fatalf("args %#v", args)
	}
}
