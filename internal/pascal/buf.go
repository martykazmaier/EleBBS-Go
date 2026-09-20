package pascal

// Buf is a packed Pascal record cursor (byte-aligned, little-endian).
type Buf struct {
	B []byte
	I int
}

func NewBuf(b []byte) *Buf { return &Buf{B: b} }

func (r *Buf) remain() int {
	if r.I >= len(r.B) {
		return 0
	}
	return len(r.B) - r.I
}

func (r *Buf) slice(n int) []byte {
	if n < 0 {
		n = 0
	}
	if r.I+n > len(r.B) {
		n = r.remain()
	}
	s := r.B[r.I : r.I+n]
	r.I += n
	return s
}

func (r *Buf) Skip(n int) { r.slice(n) }

func (r *Buf) U8() byte {
	s := r.slice(1)
	if len(s) == 0 {
		return 0
	}
	return s[0]
}

func (r *Buf) Bool() bool { return r.U8() != 0 }

func (r *Buf) U16() uint16 {
	s := r.slice(2)
	if len(s) < 2 {
		return 0
	}
	return uint16(s[0]) | uint16(s[1])<<8
}

func (r *Buf) I16() int16 { return int16(r.U16()) }

func (r *Buf) U32() uint32 {
	s := r.slice(4)
	if len(s) < 4 {
		return 0
	}
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}

func (r *Buf) I32() int32 { return int32(r.U32()) }

func (r *Buf) Char() byte { return r.U8() }

func (r *Buf) PString(max int) string {
	return String(r.slice(max + 1))
}

func (r *Buf) Flags() [4]byte {
	var f [4]byte
	copy(f[:], r.slice(4))
	return f
}

func (r *Buf) Bytes(n int) []byte {
	s := r.slice(n)
	out := make([]byte, len(s))
	copy(out, s)
	return out
}

type Writer struct {
	B []byte
}

func (w *Writer) put(p []byte) { w.B = append(w.B, p...) }
func (w *Writer) U8(v byte)    { w.B = append(w.B, v) }
func (w *Writer) Bool(v bool) {
	if v {
		w.U8(1)
	} else {
		w.U8(0)
	}
}
func (w *Writer) U16(v uint16) { w.B = append(w.B, byte(v), byte(v>>8)) }
func (w *Writer) I16(v int16)  { w.U16(uint16(v)) }
func (w *Writer) U32(v uint32) {
	w.B = append(w.B, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
func (w *Writer) I32(v int32) { w.U32(uint32(v)) }
func (w *Writer) Char(v byte) { w.U8(v) }
func (w *Writer) PString(max int, s string) {
	buf := make([]byte, max+1)
	PutString(buf, s)
	w.put(buf)
}
func (w *Writer) Flags(f [4]byte) { w.put(f[:]) }
func (w *Writer) Pad(n int) {
	if n > 0 {
		w.B = append(w.B, make([]byte, n)...)
	}
}
