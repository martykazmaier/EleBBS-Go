//go:build windows

package comm

import (
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type consoleStream struct {
	in      *os.File
	out     *os.File
	restore func()
	rdl     time.Time
	pend    []byte
}

func OpenConsole() (Stream, error) {
	restore, err := setupLocalConsole()
	if err != nil {
		return nil, err
	}
	return &consoleStream{in: os.Stdin, out: os.Stdout, restore: restore}, nil
}

var (
	modKernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInputW             = modKernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInputW             = modKernel32.NewProc("ReadConsoleInputW")
	procGetNumberOfConsoleInputEvents = modKernel32.NewProc("GetNumberOfConsoleInputEvents")
)

type conInputRecord struct {
	EventType       uint16
	_               uint16
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

func consoleHasKey(h windows.Handle) bool {
	var n uint32
	r1, _, _ := procGetNumberOfConsoleInputEvents.Call(uintptr(h), uintptr(unsafe.Pointer(&n)))
	if r1 == 0 || n == 0 {
		return false
	}
	if n > 64 {
		n = 64
	}
	recs := make([]conInputRecord, n)
	var got uint32
	r1, _, _ = procPeekConsoleInputW.Call(uintptr(h), uintptr(unsafe.Pointer(&recs[0])), uintptr(n), uintptr(unsafe.Pointer(&got)))
	if r1 == 0 {
		return false
	}
	for i := 0; i < int(got); i++ {
		if recs[i].EventType == 0x0001 && recs[i].KeyDown != 0 {
			if recs[i].UnicodeChar != 0 {
				return true
			}
			switch recs[i].VirtualKeyCode {
			case 0x0D, 0x1B, 0x08, 0x09, 0x25, 0x26, 0x27, 0x28, 0x21, 0x22, 0x24, 0x23, 0x2E, 0x2D:
				return true
			}
		}
	}
	return false
}

func setupLocalConsole() (func(), error) {
	stdin := windows.Handle(os.Stdin.Fd())
	stdout := windows.Handle(os.Stdout.Fd())
	var inMode, outMode uint32
	if err := windows.GetConsoleMode(stdin, &inMode); err != nil {
		return func() {}, nil
	}
	_ = windows.GetConsoleMode(stdout, &outMode)
	newIn := inMode
	newIn &^= windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT
	const enableVTInput = 0x0200
	newIn |= enableVTInput
	if err := windows.SetConsoleMode(stdin, newIn); err != nil {
		return nil, err
	}
	const enableVT = 0x0004
	const disableNL = 0x0008
	newOut := outMode | windows.ENABLE_PROCESSED_OUTPUT | enableVT | disableNL
	_ = windows.SetConsoleMode(stdout, newOut)
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
	return func() {
		_ = windows.SetConsoleMode(stdin, inMode)
		_ = windows.SetConsoleMode(stdout, outMode)
	}, nil
}

func (c *consoleStream) Local() bool { return true }

func (c *consoleStream) Read(b []byte) (int, error) {
	if len(c.pend) > 0 {
		n := copy(b, c.pend)
		c.pend = c.pend[n:]
		return n, nil
	}
	h := windows.Handle(c.in.Fd())
	for {
		timeout := uint32(windows.INFINITE)
		if !c.rdl.IsZero() {
			ms := time.Until(c.rdl).Milliseconds()
			if ms <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			timeout = uint32(ms)
		}
		st, err := windows.WaitForSingleObject(h, timeout)
		if err != nil {
			return 0, err
		}
		if st == uint32(windows.WAIT_TIMEOUT) {
			return 0, os.ErrDeadlineExceeded
		}
		seq, err := readConsoleKey(h)
		if err != nil {
			return 0, err
		}
		if len(seq) == 0 {
			continue
		}
		n := copy(b, seq)
		if n < len(seq) {
			c.pend = append([]byte{}, seq[n:]...)
		}
		return n, nil
	}
}

func readConsoleKey(h windows.Handle) ([]byte, error) {
	var rec conInputRecord
	var got uint32
	r1, _, err := procReadConsoleInputW.Call(uintptr(h), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&got)))
	if r1 == 0 {
		return nil, err
	}
	if got == 0 || rec.EventType != 0x0001 || rec.KeyDown == 0 {
		return nil, nil
	}
	if rec.UnicodeChar != 0 {
		switch rec.VirtualKeyCode {
		case 0x26, 0x28, 0x27, 0x25, 0x24, 0x23:
			// VT input already put CSI in UnicodeChar; still map arrows from VK
			// so a leftover CR is not delivered as Enter before the sequence.
		default:
			r := rune(rec.UnicodeChar)
			if r < 256 {
				return []byte{byte(r)}, nil
			}
			return []byte(string(r)), nil
		}
	}
	switch rec.VirtualKeyCode {
	case 0x26: // VK_UP
		return []byte{0x1b, '[', 'A'}, nil
	case 0x28: // VK_DOWN
		return []byte{0x1b, '[', 'B'}, nil
	case 0x27: // VK_RIGHT
		return []byte{0x1b, '[', 'C'}, nil
	case 0x25: // VK_LEFT
		return []byte{0x1b, '[', 'D'}, nil
	case 0x24: // VK_HOME
		return []byte{0x1b, '[', 'H'}, nil
	case 0x23: // VK_END
		return []byte{0x1b, '[', 'K'}, nil
	case 0x0D: // VK_RETURN
		return []byte{'\r'}, nil
	case 0x1B:
		return []byte{0x1b}, nil
	case 0x08:
		return []byte{8}, nil
	case 0x09:
		return []byte{'\t'}, nil
	}
	if rec.UnicodeChar != 0 {
		r := rune(rec.UnicodeChar)
		if r < 256 {
			return []byte{byte(r)}, nil
		}
		return []byte(string(r)), nil
	}
	return nil, nil
}

func (c *consoleStream) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	u := utf16.Encode([]rune(string(CP437VT(b))))
	if len(u) == 0 {
		return len(b), nil
	}
	h := windows.Handle(c.out.Fd())
	var n uint32
	if err := windows.WriteConsole(h, &u[0], uint32(len(u)), &n, nil); err != nil {
		_, err2 := c.out.Write(CP437VT(b))
		if err2 != nil {
			return 0, err2
		}
	}
	return len(b), nil
}

var (
	procFillConsoleOutputCharacterW = modKernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute  = modKernel32.NewProc("FillConsoleOutputAttribute")
	procSetConsoleTextAttribute     = modKernel32.NewProc("SetConsoleTextAttribute")
)

// ConsoleScreen is the node window as a snoop target: CP437/ANSI session
// output rendered on this process's console with Win32 calls, so it works on
// legacy console hosts without VT processing. Nil when there is no console.
func ConsoleScreen() io.Writer {
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	h := windows.Handle(out.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		out.Close()
		return nil
	}
	_ = windows.SetConsoleMode(h, mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_WRAP_AT_EOL_OUTPUT)
	return newANSIScreen(&conOps{out: out, h: h})
}

type conOps struct {
	out  *os.File
	h    windows.Handle
	attr uint16
}

func coordArg(x, y int) uintptr { return uintptr(uint16(x)) | uintptr(uint16(y))<<16 }

func (c *conOps) info() windows.ConsoleScreenBufferInfo {
	var bi windows.ConsoleScreenBufferInfo
	_ = windows.GetConsoleScreenBufferInfo(c.h, &bi)
	return bi
}

func (c *conOps) Text(b []byte) {
	u := utf16.Encode([]rune(string(vtGlyphs(b))))
	if len(u) == 0 {
		return
	}
	var n uint32
	_ = windows.WriteConsole(c.h, &u[0], uint32(len(u)), &n, nil)
}

func (c *conOps) SetAttr(a uint16) {
	c.attr = a
	procSetConsoleTextAttribute.Call(uintptr(c.h), uintptr(a))
}

func (c *conOps) Cursor() (int, int) {
	bi := c.info()
	return int(bi.CursorPosition.X - bi.Window.Left), int(bi.CursorPosition.Y - bi.Window.Top)
}

func (c *conOps) SetCursor(x, y int) {
	bi := c.info()
	_ = windows.SetConsoleCursorPosition(c.h, windows.Coord{X: bi.Window.Left + int16(x), Y: bi.Window.Top + int16(y)})
}

func (c *conOps) Size() (int, int) {
	bi := c.info()
	return int(bi.Window.Right-bi.Window.Left) + 1, int(bi.Window.Bottom-bi.Window.Top) + 1
}

func (c *conOps) Fill(x, y, n int) {
	if n <= 0 {
		return
	}
	bi := c.info()
	at := coordArg(int(bi.Window.Left)+x, int(bi.Window.Top)+y)
	var done uint32
	procFillConsoleOutputCharacterW.Call(uintptr(c.h), ' ', uintptr(n), at, uintptr(unsafe.Pointer(&done)))
	procFillConsoleOutputAttribute.Call(uintptr(c.h), uintptr(c.attr), uintptr(n), at, uintptr(unsafe.Pointer(&done)))
}

func (c *consoleStream) Close() error {
	if c.restore != nil {
		c.restore()
	}
	return nil
}

func (c *consoleStream) SetReadDeadline(t time.Time) error { c.rdl = t; return nil }
func (c *consoleStream) SetWriteDeadline(time.Time) error  { return nil }

func OpenInherited(handle uintptr) (Stream, error) {
	if handle == 0 || handle == ^uintptr(0) {
		return nil, fmt.Errorf("invalid inherited socket handle")
	}
	return FromDoorHandle(handle)
}
