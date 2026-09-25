//go:build windows

package comm

import (
	"os"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procWriteConsoleOutputCharacterW = modKernel32.NewProc("WriteConsoleOutputCharacterW")
	procWriteConsoleOutputAttribute  = modKernel32.NewProc("WriteConsoleOutputAttribute")
	procReadConsoleOutputW           = modKernel32.NewProc("ReadConsoleOutputW")
	procWriteConsoleOutputW          = modKernel32.NewProc("WriteConsoleOutputW")
)

type conWindow struct {
	h windows.Handle
}

// OpenLocalWindow is the node console as a sysop drawing surface. Nil when
// there is no console.
func OpenLocalWindow() LocalWindow {
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
	return &conWindow{h: h}
}

func (c *conWindow) info() windows.ConsoleScreenBufferInfo {
	var bi windows.ConsoleScreenBufferInfo
	_ = windows.GetConsoleScreenBufferInfo(c.h, &bi)
	return bi
}

func (c *conWindow) WriteAt(x, y int, attr byte, s []byte) {
	if len(s) == 0 {
		return
	}
	bi := c.info()
	u := utf16.Encode([]rune(string(vtGlyphs(s))))
	attrs := make([]uint16, len(u))
	for i := range attrs {
		attrs[i] = uint16(attr)
	}
	at := coordArg(int(bi.Window.Left)+x-1, int(bi.Window.Top)+y-1)
	var n uint32
	procWriteConsoleOutputCharacterW.Call(uintptr(c.h), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), at, uintptr(unsafe.Pointer(&n)))
	procWriteConsoleOutputAttribute.Call(uintptr(c.h), uintptr(unsafe.Pointer(&attrs[0])), uintptr(len(attrs)), at, uintptr(unsafe.Pointer(&n)))
}

func (c *conWindow) GotoXY(x, y int) {
	bi := c.info()
	_ = windows.SetConsoleCursorPosition(c.h, windows.Coord{X: bi.Window.Left + int16(x-1), Y: bi.Window.Top + int16(y-1)})
}

type charInfo struct {
	Char uint16
	Attr uint16
}

func (c *conWindow) Save() func() {
	bi := c.info()
	w := int(bi.Window.Right-bi.Window.Left) + 1
	h := int(bi.Window.Bottom-bi.Window.Top) + 1
	if w <= 0 || h <= 0 {
		return func() {}
	}
	buf := make([]charInfo, w*h)
	size := coordArg(w, h)
	rect := bi.Window
	r1, _, _ := procReadConsoleOutputW.Call(uintptr(c.h), uintptr(unsafe.Pointer(&buf[0])), size, 0, uintptr(unsafe.Pointer(&rect)))
	cursor, attr := bi.CursorPosition, bi.Attributes
	return func() {
		if r1 != 0 {
			rect := bi.Window
			procWriteConsoleOutputW.Call(uintptr(c.h), uintptr(unsafe.Pointer(&buf[0])), size, 0, uintptr(unsafe.Pointer(&rect)))
		}
		_ = windows.SetConsoleCursorPosition(c.h, cursor)
		procSetConsoleTextAttribute.Call(uintptr(c.h), uintptr(attr))
	}
}
