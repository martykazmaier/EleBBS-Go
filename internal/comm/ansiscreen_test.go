package comm

import (
	"strings"
	"testing"
)

type fakeScreen struct {
	w, h  int
	x, y  int
	attr  uint16
	chars [][]byte
	attrs [][]uint16
}

func newFakeScreen(w, h int) *fakeScreen {
	f := &fakeScreen{w: w, h: h}
	for i := 0; i < h; i++ {
		f.chars = append(f.chars, []byte(strings.Repeat(" ", w)))
		f.attrs = append(f.attrs, make([]uint16, w))
	}
	return f
}

func (f *fakeScreen) put(c byte) {
	f.chars[f.y][f.x] = c
	f.attrs[f.y][f.x] = f.attr
	f.x++
	if f.x >= f.w {
		f.x, f.y = 0, min(f.y+1, f.h-1)
	}
}

func (f *fakeScreen) Text(b []byte) {
	for _, c := range b {
		switch c {
		case '\r':
			f.x = 0
		case '\n':
			f.y = min(f.y+1, f.h-1)
		case 8:
			f.x = max(f.x-1, 0)
		default:
			f.put(c)
		}
	}
}

func (f *fakeScreen) SetAttr(a uint16)    { f.attr = a }
func (f *fakeScreen) Cursor() (int, int)  { return f.x, f.y }
func (f *fakeScreen) SetCursor(x, y int)  { f.x, f.y = x, y }
func (f *fakeScreen) Size() (int, int)    { return f.w, f.h }
func (f *fakeScreen) row(y int) string    { return strings.TrimRight(string(f.chars[y]), " ") }
func (f *fakeScreen) Fill(x, y, n int) {
	for i := y*f.w + x; i < y*f.w+x+n && i < f.w*f.h; i++ {
		f.chars[i/f.w][i%f.w] = ' '
		f.attrs[i/f.w][i%f.w] = f.attr
	}
}

func TestANSIScreenInterpretsCodes(t *testing.T) {
	f := newFakeScreen(80, 25)
	a := newANSIScreen(f)
	a.Write([]byte("garbage\x1b[2J\x1b[3;5H\x1b[1;33mHi\x1b[0m there"))
	if got := f.row(0); got != "" {
		t.Fatalf("row 0 = %q, want cleared", got)
	}
	if got := f.row(2); got != "    Hi there" {
		t.Fatalf("row 2 = %q", got)
	}
	if f.attrs[2][4] != 0x0E {
		t.Fatalf("Hi attr = %#x, want bright yellow 0x0e", f.attrs[2][4])
	}
	if f.attrs[2][7] != 0x07 {
		t.Fatalf("there attr = %#x, want 0x07", f.attrs[2][7])
	}
	if strings.ContainsRune(f.row(2), 0x1b) {
		t.Fatal("escape leaked to the screen")
	}
}

func TestANSIScreenSplitSequence(t *testing.T) {
	f := newFakeScreen(80, 25)
	a := newANSIScreen(f)
	a.Write([]byte("\x1b"))
	a.Write([]byte("[44;3"))
	a.Write([]byte("1mX\x1b[K"))
	if f.row(0) != "X" || f.attrs[0][0] != 0x14 {
		t.Fatalf("row = %q attr %#x, want X on blue in red", f.row(0), f.attrs[0][0])
	}
	if f.attrs[0][1] != 0x14 {
		t.Fatal("clear to end of line should use the current colour")
	}
}

func TestANSIScreenCursorMoves(t *testing.T) {
	f := newFakeScreen(80, 25)
	a := newANSIScreen(f)
	a.Write([]byte("\x1b[10;10H\x1b[s\x1b[2A\x1b[3DA\x1b[uB\x1b[99;99HZ"))
	if f.row(7) != "      A" {
		t.Fatalf("row 7 = %q", f.row(7))
	}
	if f.row(9) != "         B" {
		t.Fatalf("row 9 = %q", f.row(9))
	}
	if f.chars[24][79] != 'Z' {
		t.Fatal("CUP beyond the screen should clamp to the bottom-right corner")
	}
}

func TestANSIScreenFormFeedClears(t *testing.T) {
	f := newFakeScreen(80, 25)
	a := newANSIScreen(f)
	a.Write([]byte("old\r\n\x0cnew"))
	if f.row(0) != "new" || f.row(1) != "" {
		t.Fatalf("rows = %q / %q", f.row(0), f.row(1))
	}
}
