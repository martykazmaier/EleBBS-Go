package tui

import (
	"strings"
)

type Key struct {
	Ch   rune
	Esc  bool
	Name string
}

type Screen struct {
	restore func()
	out     uintptr
	in      uintptr
	w, h    int
}

const (
	Norm     = 0x07
	Hi       = 0x70
	Title    = 0x0E
	Blue     = 0x1F
	Cyan     = 0x0B
	MenuAttr = 0x1E
	BarAttr  = 0x70
	Back     = 0x17
)

func (s *Screen) Box(x1, y1, x2, y2 int, attr byte, title string) {
	w := x2 - x1 + 1
	if w < 2 {
		w = 2
	}
	s.At(x1, y1, attr, "┌"+strings.Repeat("─", w-2)+"┐")
	for y := y1 + 1; y < y2; y++ {
		s.At(x1, y, attr, "│"+strings.Repeat(" ", w-2)+"│")
	}
	s.At(x1, y2, attr, "└"+strings.Repeat("─", w-2)+"┘")
	if title != "" {
		s.At(x1+2, y1, attr, " "+title+" ")
	}
}

func (s *Screen) Prompt(x, y, width int, title, cur string) string {
	s.At(x, y, Cyan, title)
	s.At(x+len(title), y, 0x0F, strings.Repeat(" ", width))
	s.At(x+len(title), y, 0x0F, cur)
	b := []byte(cur)
	for {
		k := s.ReadKey()
		switch k.Name {
		case "enter":
			return string(b)
		case "esc":
			return cur
		case "bs":
			if len(b) > 0 {
				b = b[:len(b)-1]
				pad := width - len(b)
				if pad < 0 {
					pad = 0
				}
				s.At(x+len(title), y, 0x0F, string(b)+strings.Repeat(" ", pad))
			}
		default:
			if k.Ch >= 32 && len(b) < width {
				b = append(b, byte(k.Ch))
				s.At(x+len(title), y, 0x0F, string(b))
			}
		}
	}
}

func Menu(s *Screen, x, y int, title string, items []string, start int) int {
	h := len(items) + 1
	w := 16
	for _, it := range items {
		if len(it)+4 > w {
			w = len(it) + 4
		}
	}
	s.Box(x, y, x+w, y+h, MenuAttr, title)
	sel := start
	if sel < 0 {
		sel = 0
	}
	for {
		for i, it := range items {
			attr := byte(MenuAttr)
			if i == sel {
				attr = Hi
			}
			s.At(x+2, y+1+i, attr, pad(it, w-3))
		}
		k := s.ReadKey()
		switch k.Name {
		case "up":
			if sel > 0 {
				sel--
			} else {
				sel = len(items) - 1
			}
		case "down":
			sel = (sel + 1) % len(items)
		case "enter":
			return sel
		case "esc":
			return -1
		default:
			if k.Ch >= '1' && int(k.Ch-'1') < len(items) {
				return int(k.Ch - '1')
			}
			for i, it := range items {
				if len(it) > 0 && (it[0] == byte(k.Ch) || it[0] == byte(k.Ch)-32 || it[0] == byte(k.Ch)+32) {
					return i
				}
			}
		}
	}
}

func pad(s string, n int) string {
	if n < 0 {
		n = 0
	}
	if len(s) > n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func ansi(attr byte) string {
	fg := attr & 0x0F
	bg := (attr >> 4) & 0x07
	bold := 0
	if fg > 7 {
		bold = 1
		fg -= 8
	}
	mapc := []int{0, 4, 2, 6, 1, 5, 3, 7}
	if int(fg) >= len(mapc) {
		fg = 7
	}
	return "\x1b[" + itoa(bold) + ";3" + itoa(mapc[fg]) + ";4" + itoa(mapc[bg]) + "m"
}

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
