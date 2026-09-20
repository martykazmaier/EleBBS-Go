package pascal

import (
	"encoding/binary"
	"strings"
	"unicode"
)

// String reads a Pascal ShortString stored as length-prefixed bytes of size max+1.
func String(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	n := int(buf[0])
	if n > len(buf)-1 {
		n = len(buf) - 1
	}
	return strings.TrimRight(FromCP437(buf[1:1+n]), "\x00")
}

func PutString(buf []byte, s string) {
	if len(buf) == 0 {
		return
	}
	raw := ToCP437(s)
	max := len(buf) - 1
	if len(raw) > max {
		raw = raw[:max]
	}
	buf[0] = byte(len(raw))
	copy(buf[1:], raw)
	for i := 1 + len(raw); i < len(buf); i++ {
		buf[i] = 0
	}
}

func U16(b []byte, off int) uint16 {
	if off+1 >= len(b) {
		return 0
	}
	return binary.LittleEndian.Uint16(b[off:])
}

func U32(b []byte, off int) uint32 {
	if off+3 >= len(b) {
		return 0
	}
	return binary.LittleEndian.Uint32(b[off:])
}

func I16(b []byte, off int) int16 { return int16(U16(b, off)) }
func I32(b []byte, off int) int32 { return int32(U32(b, off)) }

func PutU16(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func PutU32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }

func UpCase(s string) string {
	b := ToCP437(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return FromCP437(b)
}

func Trim(s string) string { return strings.TrimSpace(s) }

func NoDoubleSpace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			prevSpace = true
			b.WriteByte(' ')
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func ForceBack(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	if p == "" {
		return p
	}
	if !strings.HasSuffix(p, `\`) {
		p += `\`
	}
	return p
}
