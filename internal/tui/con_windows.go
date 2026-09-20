//go:build windows

package tui

import (
	"os"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const keyEvent = 0x0001

var (
	modkernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procFillConsoleOutputCharacterW = modkernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute  = modkernel32.NewProc("FillConsoleOutputAttribute")
	procSetConsoleTextAttribute     = modkernel32.NewProc("SetConsoleTextAttribute")
	procReadConsoleInputW           = modkernel32.NewProc("ReadConsoleInputW")
)

type inputRecord struct {
	EventType       uint16
	_               uint16
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

func New() *Screen {
	s := &Screen{w: 80, h: 25}
	s.in = uintptr(os.Stdin.Fd())
	s.out = uintptr(os.Stdout.Fd())
	s.restore = setupConsole(windows.Handle(s.in), windows.Handle(s.out))
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(s.out), &info); err == nil {
		s.w = int(info.Window.Right-info.Window.Left) + 1
		s.h = int(info.Window.Bottom-info.Window.Top) + 1
		if s.w < 80 {
			s.w = 80
		}
		if s.h < 25 {
			s.h = 25
		}
	}
	s.Cls()
	return s
}

func setupConsole(in, out windows.Handle) func() {
	var inMode, outMode uint32
	_ = windows.GetConsoleMode(in, &inMode)
	_ = windows.GetConsoleMode(out, &outMode)
	raw := inMode
	raw &^= windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT
	raw |= windows.ENABLE_WINDOW_INPUT
	_ = windows.SetConsoleMode(in, raw)
	// Keep VT off so SetConsoleTextAttribute (BIOS/RA colors) is honored in Windows Terminal.
	_ = windows.SetConsoleMode(out, outMode|windows.ENABLE_PROCESSED_OUTPUT)
	return func() {
		_ = windows.SetConsoleMode(in, inMode)
		_ = windows.SetConsoleMode(out, outMode)
	}
}

func setup() func() { return func() {} }

func (s *Screen) Close() {
	if s.restore != nil {
		s.restore()
	}
	s.Cls()
	s.At(1, 1, Norm, "")
}

func (s *Screen) Cls() {
	h := windows.Handle(s.out)
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return
	}
	size := uint32(info.Size.X) * uint32(info.Size.Y)
	var written uint32
	origin := windows.Coord{X: 0, Y: 0}
	_ = fillConsoleOutputCharacter(h, ' ', size, origin, &written)
	_ = fillConsoleOutputAttribute(h, uint16(Back), size, origin, &written)
	_ = windows.SetConsoleCursorPosition(h, origin)
}

func (s *Screen) At(x, y int, attr byte, msg string) {
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}
	h := windows.Handle(s.out)
	_ = windows.SetConsoleCursorPosition(h, windows.Coord{X: int16(x - 1), Y: int16(y - 1)})
	_ = setConsoleTextAttribute(h, uint16(attr))
	if msg == "" {
		return
	}
	u := utf16.Encode([]rune(msg))
	if len(u) == 0 {
		return
	}
	var n uint32
	_ = windows.WriteConsole(h, &u[0], uint32(len(u)), &n, nil)
}

func (s *Screen) Bar(y int, attr byte, ch rune, width int) {
	if width <= 0 {
		width = s.w
	}
	runes := make([]rune, width)
	for i := range runes {
		runes[i] = ch
	}
	s.At(1, y, attr, string(runes))
}

func (s *Screen) ReadKey() Key {
	h := windows.Handle(s.in)
	var rec inputRecord
	var n uint32
	for {
		if err := readConsoleInput(h, &rec, &n); err != nil || n == 0 {
			return Key{Name: "eof"}
		}
		if rec.EventType != keyEvent {
			continue
		}
		if rec.KeyDown == 0 {
			continue
		}
		switch rec.VirtualKeyCode {
		case 0x26:
			return Key{Esc: true, Name: "up"}
		case 0x28:
			return Key{Esc: true, Name: "down"}
		case 0x25:
			return Key{Esc: true, Name: "left"}
		case 0x27:
			return Key{Esc: true, Name: "right"}
		case 0x21:
			return Key{Esc: true, Name: "pgup"}
		case 0x22:
			return Key{Esc: true, Name: "pgdn"}
		case 0x24:
			return Key{Esc: true, Name: "home"}
		case 0x23:
			return Key{Esc: true, Name: "end"}
		case 0x2D:
			return Key{Esc: true, Name: "ins"}
		case 0x2E:
			return Key{Esc: true, Name: "del"}
		case 0x1B:
			return Key{Ch: 27, Name: "esc"}
		case 0x0D:
			return Key{Ch: 13, Name: "enter"}
		case 0x08:
			return Key{Ch: 8, Name: "bs"}
		}
		ch := rune(rec.UnicodeChar)
		if ch == 0 {
			continue
		}
		if ch == 13 {
			return Key{Ch: 13, Name: "enter"}
		}
		if ch == 27 {
			return Key{Ch: 27, Name: "esc"}
		}
		if ch == 8 {
			return Key{Ch: 8, Name: "bs"}
		}
		return Key{Ch: ch, Name: string(ch)}
	}
}

func coordWord(c windows.Coord) uintptr {
	return uintptr(uint32(uint16(c.X)) | uint32(uint16(c.Y))<<16)
}

func fillConsoleOutputCharacter(h windows.Handle, ch uint16, n uint32, origin windows.Coord, written *uint32) error {
	r1, _, err := procFillConsoleOutputCharacterW.Call(uintptr(h), uintptr(ch), uintptr(n), coordWord(origin), uintptr(unsafe.Pointer(written)))
	if r1 == 0 {
		return err
	}
	return nil
}

func fillConsoleOutputAttribute(h windows.Handle, attr uint16, n uint32, origin windows.Coord, written *uint32) error {
	r1, _, err := procFillConsoleOutputAttribute.Call(uintptr(h), uintptr(attr), uintptr(n), coordWord(origin), uintptr(unsafe.Pointer(written)))
	if r1 == 0 {
		return err
	}
	return nil
}

func setConsoleTextAttribute(h windows.Handle, attr uint16) error {
	r1, _, err := procSetConsoleTextAttribute.Call(uintptr(h), uintptr(attr))
	if r1 == 0 {
		return err
	}
	return nil
}

func readConsoleInput(h windows.Handle, rec *inputRecord, n *uint32) error {
	r1, _, err := procReadConsoleInputW.Call(uintptr(h), uintptr(unsafe.Pointer(rec)), 1, uintptr(unsafe.Pointer(n)))
	if r1 == 0 {
		return err
	}
	return nil
}
