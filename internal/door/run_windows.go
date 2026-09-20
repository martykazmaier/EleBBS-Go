//go:build windows

package door

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"elebbs/internal/comm"
	"golang.org/x/sys/windows"
)

func dupInheritable(h uintptr) uintptr {
	if h == 0 || h == ^uintptr(0) {
		return 0
	}
	proc := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, windows.Handle(h), proc, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0
	}
	return uintptr(dup)
}

func dupForDoor(h uintptr) uintptr {
	return comm.DupForDoor(h)
}

func setInherit(h uintptr, on bool) {
	if h == 0 || h == ^uintptr(0) {
		return
	}
	flag := uint32(0)
	if on {
		flag = windows.HANDLE_FLAG_INHERIT
	}
	_ = windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, flag)
}

func closeHandle(h uintptr) {
	if h == 0 || h == ^uintptr(0) {
		return
	}
	_ = windows.CloseHandle(windows.Handle(h))
}

func closeSocket(h uintptr) {
	comm.CloseSocket(h)
}

func windowsCmdLine(exe, rest string) string {
	if exe == "" {
		return rest
	}
	quoted := exe
	if strings.ContainsAny(exe, " \t") && !strings.HasPrefix(exe, `"`) {
		quoted = `"` + exe + `"`
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return quoted
	}
	return quoted + " " + rest
}

func spawnDoor(exe, rest, dir string, inherit []uintptr, win32Socket bool, foss uintptr) error {
	_ = foss
	if isBatch(exe) {
		exe, rest = viaComspec(exe, rest)
	}
	if p, err := exec.LookPath(exe); err == nil {
		exe = p
	}
	cmdline := windowsCmdLine(exe, rest)
	exe16, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cmd16, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		return err
	}
	var dir16 *uint16
	if dir != "" {
		dir16, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
	}
	for _, h := range inherit {
		setInherit(h, true)
	}
	si := windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	flags := uint32(windows.NORMAL_PRIORITY_CLASS)
	if win32Socket {
		si.Flags = windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_SHOWNORMAL
	}
	var pi windows.ProcessInformation
	err = windows.CreateProcess(exe16, cmd16, nil, nil, true, flags, nil, dir16, &si, &pi)
	if err != nil {
		return fallbackExec(exe, rest, dir)
	}
	_ = windows.CloseHandle(pi.Thread)
	_, err = windows.WaitForSingleObject(pi.Process, windows.INFINITE)
	var code uint32
	_ = windows.GetExitCodeProcess(pi.Process, &code)
	_ = windows.CloseHandle(pi.Process)
	if err != nil {
		return err
	}
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}

type exitError struct{ code uint32 }

func (e *exitError) Error() string { return "exit status " + itoa(int(e.code)) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func fallbackExec(exe, rest, dir string) error {
	args := splitArgs(rest)
	cmd := exec.Command(exe, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:    windowsCmdLine(exe, rest),
		HideWindow: false,
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func splitArgs(rest string) []string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '"' {
			inQ = !inQ
			continue
		}
		if !inQ && (c == ' ' || c == '\t') {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
