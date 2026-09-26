//go:build !windows

package door

import (
	"os"
	"os/exec"
	"strings"
)

func dupForDoor(h uintptr) uintptr { return 0 }

func closeHandle(uintptr) {}

func setInherit(uintptr, bool) {}

func spawnDoor(exe, rest, dir string, inherit []uintptr, win32Socket bool, foss uintptr) error {
	_ = inherit
	_ = win32Socket
	_ = foss
	line := exe
	if rest != "" {
		line = exe + " " + rest
	}
	cmd := exec.Command(comspec(), "-c", line)
	if _, err := exec.LookPath(comspec()); err != nil || strings.EqualFold(comspec(), "cmd.exe") {
		cmd = exec.Command(exe)
		if rest != "" {
			cmd = exec.Command(comspec(), "/C", line)
		}
	}
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
