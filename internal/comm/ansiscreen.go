package comm

import (
	"strconv"
	"strings"
)

// screenOps is a text-mode screen driven by ansiScreen. Coordinates are
// 0-based within the visible window.
type screenOps interface {
	Text(b []byte) // CP437 text plus CR/LF/BS/TAB/BEL
	SetAttr(a uint16)
	Cursor() (x, y int)
	SetCursor(x, y int)
	Size() (w, h int)
	Fill(x, y, n int) // blank n cells from (x,y) in the current attribute
}

// ansiScreen is the Pascal local screen's ANSI emulation: it interprets the
// session's ANSI.SYS subset itself instead of relying on the console host
// (legacy consoles print ESC as a glyph).
type ansiScreen struct {
	ops              screenOps
	st               int
	csi              []byte
	text             []byte
	fg, bg           byte
	bold, blink, rev bool
	sx, sy           int
}

const (
	ansiText = iota
	ansiEsc
	ansiCSI
)

// ANSI color index to console attribute bits (blue=1, green=2, red=4).
var ansiConColor = [8]uint16{0, 4, 2, 6, 1, 5, 3, 7}

func newANSIScreen(ops screenOps) *ansiScreen {
	a := &ansiScreen{ops: ops, fg: 7}
	ops.SetAttr(a.attr())
	return a
}

func (a *ansiScreen) Write(b []byte) (int, error) {
	for _, c := range b {
		switch a.st {
		case ansiText:
			switch c {
			case 0x1b:
				a.flush()
				a.st = ansiEsc
			case 0x0c:
				a.flush()
				a.erase(2)
			case 0:
			default:
				a.text = append(a.text, c)
			}
		case ansiEsc:
			a.st = ansiText
			if c == '[' {
				a.st = ansiCSI
				a.csi = a.csi[:0]
			}
		case ansiCSI:
			if c >= 0x40 && c <= 0x7e {
				a.st = ansiText
				a.do(c)
			} else if len(a.csi) < 32 {
				a.csi = append(a.csi, c)
			} else {
				a.st = ansiText
			}
		}
	}
	a.flush()
	return len(b), nil
}

func (a *ansiScreen) flush() {
	if len(a.text) > 0 {
		a.ops.Text(a.text)
		a.text = a.text[:0]
	}
}

func (a *ansiScreen) params() []int {
	var ps []int
	if len(a.csi) == 0 {
		return ps
	}
	for _, s := range strings.Split(string(a.csi), ";") {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		ps = append(ps, n)
	}
	return ps
}

func param(ps []int, i, def int) int {
	if i < len(ps) && ps[i] > 0 {
		return ps[i]
	}
	return def
}

func (a *ansiScreen) do(cmd byte) {
	if len(a.csi) > 0 && (a.csi[0] == '?' || a.csi[0] == '=') {
		return
	}
	ps := a.params()
	x, y := a.ops.Cursor()
	n := param(ps, 0, 1)
	switch cmd {
	case 'H', 'f':
		a.moveTo(param(ps, 1, 1)-1, n-1)
	case 'A':
		a.moveTo(x, y-n)
	case 'B':
		a.moveTo(x, y+n)
	case 'C':
		a.moveTo(x+n, y)
	case 'D':
		a.moveTo(x-n, y)
	case 'G':
		a.moveTo(n-1, y)
	case 'd':
		a.moveTo(x, n-1)
	case 'J':
		a.erase(param(ps, 0, 0))
	case 'K':
		w, _ := a.ops.Size()
		switch param(ps, 0, 0) {
		case 0:
			a.ops.Fill(x, y, w-x)
		case 1:
			a.ops.Fill(0, y, x+1)
		case 2:
			a.ops.Fill(0, y, w)
		}
	case 's':
		a.sx, a.sy = x, y
	case 'u':
		a.moveTo(a.sx, a.sy)
	case 'm':
		a.sgr(ps)
	}
}

func (a *ansiScreen) moveTo(x, y int) {
	w, h := a.ops.Size()
	x = max(0, min(x, w-1))
	y = max(0, min(y, h-1))
	a.ops.SetCursor(x, y)
}

// erase is ED; like ANSI.SYS, a full clear also homes the cursor.
func (a *ansiScreen) erase(mode int) {
	w, h := a.ops.Size()
	x, y := a.ops.Cursor()
	switch mode {
	case 0:
		a.ops.Fill(x, y, (w-x)+(h-y-1)*w)
	case 1:
		a.ops.Fill(0, 0, y*w+x+1)
	default:
		a.ops.Fill(0, 0, w*h)
		a.ops.SetCursor(0, 0)
	}
}

func (a *ansiScreen) sgr(ps []int) {
	if len(ps) == 0 {
		ps = []int{0}
	}
	for _, p := range ps {
		switch {
		case p == 0:
			a.fg, a.bg, a.bold, a.blink, a.rev = 7, 0, false, false, false
		case p == 1:
			a.bold = true
		case p == 2 || p == 22:
			a.bold = false
		case p == 5 || p == 6:
			a.blink = true
		case p == 25:
			a.blink = false
		case p == 7:
			a.rev = true
		case p == 27:
			a.rev = false
		case p >= 30 && p <= 37:
			a.fg = byte(p - 30)
		case p == 39:
			a.fg = 7
		case p >= 40 && p <= 47:
			a.bg = byte(p - 40)
		case p == 49:
			a.bg = 0
		case p >= 90 && p <= 97:
			a.fg, a.bold = byte(p-90), true
		case p >= 100 && p <= 107:
			a.bg, a.blink = byte(p-100), true
		}
	}
	a.ops.SetAttr(a.attr())
}

// attr is the console attribute; blink shows as a bright background, as on
// the Pascal local screen.
func (a *ansiScreen) attr() uint16 {
	f, b := a.fg, a.bg
	if a.rev {
		f, b = b, f
	}
	v := ansiConColor[f&7] | ansiConColor[b&7]<<4
	if a.bold {
		v |= 0x08
	}
	if a.blink {
		v |= 0x80
	}
	return v
}
