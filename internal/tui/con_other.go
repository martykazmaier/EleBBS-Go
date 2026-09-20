//go:build !windows

package tui

import (
	"fmt"
	"os"
	"strings"
)

func New() *Screen {
	s := &Screen{w: 80, h: 25}
	s.restore = func() {}
	return s
}

func setup() func() { return func() {} }

func (s *Screen) Close() {
	fmt.Print("\x1b[0m\x1b[2J\x1b[H")
}

func (s *Screen) Cls() { fmt.Print("\x1b[2J\x1b[H") }

func (s *Screen) At(x, y int, attr byte, msg string) {
	fmt.Printf("\x1b[%d;%dH%s%s\x1b[0m", y, x, ansi(attr), msg)
}

func (s *Screen) Bar(y int, attr byte, ch rune, width int) {
	s.At(1, y, attr, strings.Repeat(string(ch), width))
}

func (s *Screen) ReadKey() Key {
	var b [8]byte
	n, _ := os.Stdin.Read(b[:])
	if n == 0 {
		return Key{Name: "eof"}
	}
	if b[0] == 0x1b && n >= 2 {
		seq := string(b[1:n])
		switch seq {
		case "[A":
			return Key{Esc: true, Name: "up"}
		case "[B":
			return Key{Esc: true, Name: "down"}
		case "[C":
			return Key{Esc: true, Name: "right"}
		case "[D":
			return Key{Esc: true, Name: "left"}
		case "[H", "[1~":
			return Key{Esc: true, Name: "home"}
		case "[F", "[4~":
			return Key{Esc: true, Name: "end"}
		case "[5~":
			return Key{Esc: true, Name: "pgup"}
		case "[6~":
			return Key{Esc: true, Name: "pgdn"}
		case "[2~":
			return Key{Esc: true, Name: "ins"}
		case "[3~":
			return Key{Esc: true, Name: "del"}
		}
		return Key{Ch: 27, Name: "esc"}
	}
	switch b[0] {
	case 13:
		return Key{Ch: 13, Name: "enter"}
	case 27:
		return Key{Ch: 27, Name: "esc"}
	case 8, 127:
		return Key{Ch: 8, Name: "bs"}
	}
	return Key{Ch: rune(b[0]), Name: string(b[0])}
}
