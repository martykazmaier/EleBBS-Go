//go:build windows

package door

import (
	"fmt"
	"os"
	"unsafe"

	"elebbs/internal/cfgrec"
	"golang.org/x/sys/windows"
)

var (
	procAllocConsole     = windows.NewLazySystemDLL("kernel32.dll").NewProc("AllocConsole")
	procGetConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	procSetConsoleTitleW = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTitleW")
)

// AttachSessionConsole is Pascal eleserv CREATE_NEW_CONSOLE + SetWindowTitle:
// one console for this EleBBS node; doors inherit it instead of opening cmd windows.
func AttachSessionConsole(line *cfgrec.LineCfg) {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		_, _, _ = procAllocConsole.Call()
		bindConsoleStd()
	}
	in, _ := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	out, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	errh, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	setInherit(uintptr(in), true)
	setInherit(uintptr(out), true)
	setInherit(uintptr(errh), true)
	n := 1
	if line != nil && line.RaNodeNr > 0 {
		n = line.RaNodeNr
	}
	title, err := windows.UTF16PtrFromString(fmt.Sprintf("EleBBS - Node #%d", n))
	if err == nil {
		_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(title)))
	}
}

func bindConsoleStd() {
	in, err1 := os.OpenFile("CONIN$", os.O_RDWR, 0)
	out, err2 := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err1 != nil || err2 != nil {
		return
	}
	_ = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(in.Fd()))
	_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(out.Fd()))
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(out.Fd()))
	os.Stdin, os.Stdout, os.Stderr = in, out, out
}
