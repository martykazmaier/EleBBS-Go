package sysop

import (
	"strings"

	"elebbs/internal/comm"
	"elebbs/internal/pascal"
)

const blockChar = '░' // Pascal mnuBlockChar (#176)

// Editor key results, Pascal SysOpLineEdit.
const (
	keyNone  = 0
	keyDown  = 1 // Tab, Down
	keyUp    = 2 // Shift-Tab, Up
	keyEnter = '\r'
	keyEsc   = 0x1b
)

func attr(fore, back byte) byte {
	if fore&0x0F == back&0x0F {
		fore ^= 0x0F
	}
	return back<<4 | fore&0x0F
}

func (n *Node) write(x, y int, a byte, s string) {
	n.Win.WriteAt(x, y, a, pascal.ToCP437(s))
}

// box is Pascal FastScrn.BoxWindow (style 4).
func (n *Node) box(x1, y1, x2, y2 int, fore, back byte, title string) {
	a := attr(fore, back)
	inner := x2 - x1 - 1
	top := strings.Repeat("═", inner)
	if title != "" && len(title)+2 <= inner {
		t := " " + title + " "
		top = strings.Repeat("═", inner-len(t)) + t
	}
	n.write(x1, y1, a, "╒"+top+"╕")
	for y := y1 + 1; y < y2; y++ {
		n.write(x1, y, a, "│"+strings.Repeat(" ", inner)+"│")
	}
	n.write(x1, y2, a, "╘"+strings.Repeat("═", inner)+"╛")
}

// fieldText pads s with block characters to width, like MakeEditorsField.
func fieldText(s string, width int, hidden bool) string {
	if hidden {
		s = strings.Repeat("*", len(s))
	}
	if len(s) > width {
		s = s[:width]
	}
	return s + strings.Repeat(string(blockChar), width-len(s))
}

// lineEdit is Pascal SysOpLineEdit at (x,y): the sysop edits s in place.
// It returns the edited value and keyEnter, keyEsc, keyUp or keyDown.
func (n *Node) lineEdit(x, y int, a byte, s string, width int, legal func(byte) bool, hidden bool) (string, byte) {
	buf := []byte(pascal.ToCP437(s))
	if len(buf) > width {
		buf = buf[:width]
	}
	pos := len(buf)
	for {
		shown := string(pascal.FromCP437(buf))
		n.write(x, y, a, fieldText(shown, width, hidden))
		n.Win.GotoXY(x+min(pos, width-1), y)
		k := n.readKey()
		if k.Ch == 0 {
			switch k.Scan {
			case 72, 15:
				return string(pascal.FromCP437(buf)), keyUp
			case 80:
				return string(pascal.FromCP437(buf)), keyDown
			case 75:
				pos = max(pos-1, 0)
			case 77:
				pos = min(pos+1, len(buf))
			case 71:
				pos = 0
			case 79:
				pos = len(buf)
			case 83:
				if pos < len(buf) {
					buf = append(buf[:pos], buf[pos+1:]...)
				}
			}
			continue
		}
		switch k.Ch {
		case '\r':
			return string(pascal.FromCP437(buf)), keyEnter
		case 0x1b:
			return string(pascal.FromCP437(buf)), keyEsc
		case '\t':
			return string(pascal.FromCP437(buf)), keyDown
		case 8:
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
			}
		case 25: // Ctrl-Y
			buf, pos = buf[:0], 0
		default:
			if k.Ch >= 32 && len(buf) < width && (legal == nil || legal(k.Ch)) {
				buf = append(buf[:pos], append([]byte{k.Ch}, buf[pos:]...)...)
				pos++
			}
		}
	}
}

// editBox is Pascal EditBox: a titled window with one input line.
func (n *Node) editBox(x1, y1, x2, y2 int, title string, width int, value string, hidden bool) (string, byte) {
	cfg := &n.G.RaConfig
	n.box(x1, y1, x2, y2, cfg.BorderFore, cfg.BorderBack, title)
	x := x1 + (x2-x1+1-width)/2
	return n.lineEdit(x, (y1+y2)/2, attr(cfg.HiFore, cfg.WindBack), value, width, nil, hidden)
}

// navKey maps a key pressed on a non-text field to the editor's movement.
func navKey(k comm.LocalKey) byte {
	if k.Ch == 0 {
		switch k.Scan {
		case 72, 15:
			return keyUp
		case 80:
			return keyDown
		}
		return keyNone
	}
	switch k.Ch {
	case '\r':
		return keyEnter
	case 0x1b:
		return keyEsc
	case '\t':
		return keyDown
	}
	return keyNone
}
