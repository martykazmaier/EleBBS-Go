package comm

import "elebbs/internal/pascal"

// CP437VT converts a CP437 BBS write (CSI color sequences + OEM glyphs) to UTF-8
// for Windows Terminal / SSH clients. CSI (`ESC ... letter`) is passed through.
func CP437VT(p []byte) []byte {
	if len(p) == 0 {
		return nil
	}
	out := make([]byte, 0, len(p)+8)
	i := 0
	for i < len(p) {
		if p[i] == 0x1b {
			start := i
			i++
			for i < len(p) && p[i] != 0x1b && !((p[i] >= 'A' && p[i] <= 'Z') || (p[i] >= 'a' && p[i] <= 'z')) {
				i++
			}
			if i < len(p) {
				i++
			}
			out = append(out, p[start:i]...)
			continue
		}
		j := i
		for j < len(p) && p[j] != 0x1b {
			j++
		}
		out = append(out, vtGlyphs(p[i:j])...)
		i = j
	}
	return out
}

// vtGlyphs maps CP437 including C0 picture glyphs. FromCP437 is left
// unchanged so menu hotkeys and other packed records still see byte 0x01 as ^A.
func vtGlyphs(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, 0, len(b)+8)
	start := 0
	for i, c := range b {
		if g, ok := cp437C0[c]; ok {
			out = append(out, []byte(pascal.FromCP437(b[start:i]))...)
			out = append(out, []byte(string(g))...)
			start = i + 1
		}
	}
	out = append(out, []byte(pascal.FromCP437(b[start:]))...)
	return out
}

// IBM CP437 pictures for C0 bytes. CR/LF/TAB/ESC/BS/BEL stay as controls.
var cp437C0 = map[byte]rune{
	0x01: 0x263A, // ☺
	0x02: 0x263B, // ☻
	0x03: 0x2665, // ♥
	0x04: 0x2666, // ♦
	0x05: 0x2663, // ♣
	0x06: 0x2660, // ♠
	0x0B: 0x2642, // ♂
	0x0C: 0x2640, // ♀
	0x0E: 0x266B, // ♫
	0x0F: 0x263C, // ☼
	0x10: 0x25BA, // ►
	0x11: 0x25C4, // ◄
	0x12: 0x2195, // ↕
	0x13: 0x203C, // ‼
	0x14: 0x00B6, // ¶
	0x15: 0x00A7, // §
	0x16: 0x25AC, // ▬
	0x18: 0x2191, // ↑
	0x19: 0x2193, // ↓
	0x1C: 0x221F, // ∟
	0x1D: 0x2194, // ↔
	0x1E: 0x25B2, // ▲
	0x1F: 0x25BC, // ▼
}
