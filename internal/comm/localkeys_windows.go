//go:build windows

package comm

import (
	"os"
	"unsafe"

	"elebbs/internal/pascal"
	"golang.org/x/sys/windows"
)

var procMapVirtualKeyW = windows.NewLazySystemDLL("user32.dll").NewProc("MapVirtualKeyW")

type conKeys struct {
	in *os.File // owns h; its finalizer would close the handle
	h  windows.Handle
}

// OpenLocalKeys is the node window keyboard for a remote session. Nil when
// the process has no console. Ctrl-C is delivered as a key instead of
// killing the node, like Pascal's Crt with CheckBreak off.
func OpenLocalKeys() LocalKeys {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	h := windows.Handle(in.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		in.Close()
		return nil
	}
	const enableVTInput = 0x0200
	mode &^= windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT | enableVTInput
	_ = windows.SetConsoleMode(h, mode)
	return &conKeys{in: in, h: h}
}

const (
	vkTab   = 0x09
	vkShift = 0x10
	vkCtrl  = 0x11
	vkAlt   = 0x12
	vkCaps  = 0x14
	vkF1    = 0x70
	vkF10   = 0x79
	vkNum   = 0x90
	vkScrl  = 0x91

	rightAlt   = 0x0001
	leftAlt    = 0x0002
	rightCtrl  = 0x0004
	leftCtrl   = 0x0008
	shiftState = 0x0010
)

func (c *conKeys) Poll() (LocalKey, bool) {
	for {
		var n uint32
		r1, _, _ := procGetNumberOfConsoleInputEvents.Call(uintptr(c.h), uintptr(unsafe.Pointer(&n)))
		if r1 == 0 || n == 0 {
			return LocalKey{}, false
		}
		var rec conInputRecord
		var got uint32
		r1, _, _ = procReadConsoleInputW.Call(uintptr(c.h), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&got)))
		if r1 == 0 || got == 0 {
			return LocalKey{}, false
		}
		if k, ok := consoleLocalKey(rec); ok {
			return k, true
		}
	}
}

func consoleLocalKey(rec conInputRecord) (LocalKey, bool) {
	if rec.EventType != 0x0001 || rec.KeyDown == 0 {
		return LocalKey{}, false
	}
	vk := rec.VirtualKeyCode
	switch vk {
	case vkShift, vkCtrl, vkAlt, vkCaps, vkNum, vkScrl:
		return LocalKey{}, false
	}
	alt := rec.ControlKeyState&(rightAlt|leftAlt) != 0
	ctrl := rec.ControlKeyState&(rightCtrl|leftCtrl) != 0
	scan := byte(rec.VirtualScanCode)
	if scan == 0 && vk != 0 {
		r, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0)
		scan = byte(r)
	}
	if alt && !ctrl {
		if vk >= vkF1 && vk <= vkF10 {
			return LocalKey{Scan: 104 + byte(vk-vkF1)}, true
		}
		return LocalKey{Scan: scan}, true
	}
	if vk == vkTab && rec.ControlKeyState&shiftState != 0 {
		return LocalKey{Scan: 15}, true
	}
	if rec.UnicodeChar != 0 {
		if b := pascal.ToCP437(string(rune(rec.UnicodeChar))); len(b) > 0 {
			return LocalKey{Ch: b[0]}, true
		}
		return LocalKey{}, false
	}
	if scan != 0 {
		return LocalKey{Scan: scan}, true
	}
	return LocalKey{}, false
}
