package term

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
	"elebbs/internal/config"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
)

type IO struct {
	S          comm.Stream
	Cfg        *cfgrec.GlobalCfg
	Line       *cfgrec.LineCfg
	MorePrompt bool
	Lines      int
	StopMore   bool
	Width      int
	Length     int
	Attr       byte
	mu         sync.Mutex
	push       []byte
	pushSysop  []bool // parallel to push: Pascal KeyBuffer[].IsSysOp
	RunScript  func(name, args string)
	RunMenu    func(typ byte, data string)
	// FilePost is Pascal FilePost: post a text file privately to area. It
	// returns false when the file cannot be opened.
	FilePost func(area int, from, to, subj, file, addText string) bool
	// WriteMessage is Pascal WriteMessage: the interactive editor for a new
	// message to toWho in area.
	WriteMessage func(area int, toWho, from string) bool
	// YesNoQuest is Pascal YesNoAsk's YESNO.Q-A path. handled true means
	// the script returned YES or NO; otherwise AskYesNo falls back to brackets.
	YesNoQuest    func(defYes bool) (handled bool, yes bool)
	Ral           *lang.File
	hotDisplay    bool
	hotKeys       map[byte]struct{}
	abortKey      byte
	hotHaveKey    bool
	CurX          int
	CurY          int
	csiPend       []byte
	raduPend      []byte
	skipWaitEnter bool
	// Idle timer, Pascal tControlObj.TimeInfo + LineCfg.CheckInactivity.
	// Local logons (Baud 0) are never timed out.
	idleTime   time.Duration
	idleLimit  time.Time
	idleOn     bool
	idleWarned bool
	idleDone   bool
	// Local is the node window keyboard of a remote session; its keys are
	// input as if the caller typed them (Pascal Crt.ReadKey in ReadKey).
	Local comm.LocalKeys
	// LocalCommand is Pascal LocalCommand: extended node-window keys
	// (Alt-C, Alt-E, Alt-H, ...) by scan code.
	LocalCommand func(scan byte)
	// FromSysop is Pascal LineCfg.SysOpKey: the last key came from Local.
	FromSysop bool
	hung      bool
	hangC     chan struct{}
	lost      bool // the caller's stream failed (carrier lost)
}

// ErrIdle is Pascal CheckIdle's inactivity hangup.
var ErrIdle = errors.New("inactivity timeout")

// ErrHangup is Pascal HangUp from the node window (Alt-H).
var ErrHangup = errors.New("sysop hung up")

// localPoll is how often GetKey looks at the node window keyboard while it
// waits for the caller.
const localPoll = 50 * time.Millisecond

// HangUp is Pascal HangUp: input fails and output is dropped, so the session
// unwinds as if carrier was lost; the caller is disconnected when it ends.
func (t *IO) HangUp() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hung {
		return
	}
	t.hung = true
	if t.hangC == nil {
		t.hangC = make(chan struct{})
	}
	close(t.hangC)
}

// HungUp reports that HangUp ended this session.
func (t *IO) HungUp() bool {
	return t != nil && t.hung
}

// HangUpC is closed by HangUp.
func (t *IO) HangUpC() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hangC == nil {
		t.hangC = make(chan struct{})
	}
	return t.hangC
}

// Gone is Pascal ProgTerminated: the caller was hung up, timed out or lost
// carrier, so no more input will come.
func (t *IO) Gone() bool {
	return t != nil && (t.hung || t.idleDone || t.lost)
}

// PushKey is Pascal PutInBuffer of one raw key (#255 redraws the menu).
func (t *IO) PushKey(b byte) {
	t.pushBytes([]byte{b})
}

// pollLocal returns the next node-window key, running LocalCommand for
// extended keys. Cursor keys become the caller's ANSI sequences.
func (t *IO) pollLocal() (byte, bool) {
	for t.Local != nil && !t.hung {
		k, ok := t.Local.Poll()
		if !ok {
			return 0, false
		}
		if k.Ch != 0 {
			return k.Ch, true
		}
		if seq := localCursorKey(k.Scan); seq != "" {
			t.pushFrom([]byte(seq[1:]), true)
			return seq[0], true
		}
		if t.LocalCommand != nil {
			t.LocalCommand(k.Scan)
		}
	}
	return 0, false
}

// localCursorKey is Pascal LocalCommand's cursor-key table.
func localCursorKey(scan byte) string {
	switch scan {
	case 75:
		return "\x1b[D"
	case 77:
		return "\x1b[C"
	case 72:
		return "\x1b[A"
	case 80:
		return "\x1b[B"
	case 82:
		return "\x16\x09"
	case 83:
		return "\x7f"
	case 71:
		return "\x1b[H"
	case 79:
		return "\x1b[K"
	}
	return ""
}

func New(s comm.Stream, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) *IO {
	length := int(line.User.ScreenLength)
	if length == 0 {
		length = int(g.RaConfig.PageLength)
	}
	if length == 0 {
		length = 25
	}
	w := int(line.User.ScreenWidth)
	if w == 0 {
		w = 80
	}
	return &IO{
		S:          s,
		Cfg:        g,
		Line:       line,
		MorePrompt: true,
		Width:      w,
		Length:     length,
		Attr:       makeAttr(g.RaConfig.NormFore, g.RaConfig.NormBack),
		Ral:        lang.Load(g, line.Language),
		CurX:       1,
		CurY:       1,
	}
}

func makeAttr(fore, back byte) byte { return (back << 4) | (fore & 0x0F) }

func (t *IO) WriteRaw(p []byte) error {
	if t.hung {
		return nil
	}
	t.trackCursor(p)
	_, err := t.S.Write(p)
	return err
}

func (t *IO) WhereX() int {
	if t.CurX < 1 {
		return 1
	}
	return t.CurX
}

func (t *IO) WhereY() int {
	if t.CurY < 1 {
		return 1
	}
	return t.CurY
}

func (t *IO) GotoXY(x, y int) {
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}
	t.CurX, t.CurY = x, y
	if t.hung {
		return
	}
	_, _ = t.S.Write([]byte(fmt.Sprintf("\x1b[%d;%dH", y, x)))
}

// raduCoord is Pascal PositionX/PositionY: "+n"/"-n" are relative to cur, else absolute.
func raduCoord(arg string, cur int) int {
	s := strings.TrimSpace(arg)
	if s == "" {
		return 1
	}
	rel := 0
	switch s[0] {
	case '+':
		rel = 1
		s = s[1:]
	case '-':
		rel = -1
		s = s[1:]
	}
	n := 0
	fmt.Sscanf(s, "%d", &n)
	if rel != 0 {
		n = cur + rel*n
	}
	if n < 1 {
		n = 1
	}
	return n
}

// setX is Pascal MakeXYStr: column 1 is CR on this line; otherwise relative CSI C/D.
func (t *IO) setX(n int) {
	if n < 1 {
		n = 1
	}
	if n == 1 {
		_ = t.WriteRaw([]byte{'\r'})
		return
	}
	cur := t.WhereX()
	if n == cur {
		return
	}
	ansi := t.Line == nil || t.Line.AnsiOn
	if ansi {
		if n < cur {
			_ = t.WriteRaw([]byte(fmt.Sprintf("\x1b[%dD", cur-n)))
		} else {
			_ = t.WriteRaw([]byte(fmt.Sprintf("\x1b[%dC", n-cur)))
		}
	} else if n < cur {
		_ = t.WriteRaw(bytes.Repeat([]byte{8}, cur-n))
	} else {
		_ = t.WriteRaw(bytes.Repeat([]byte{' '}, n-cur))
	}
	t.CurX = n
}

func (t *IO) trackCursor(p []byte) {
	if t.CurX < 1 {
		t.CurX = 1
	}
	if t.CurY < 1 {
		t.CurY = 1
	}
	if len(t.csiPend) > 0 {
		p = append(append([]byte{}, t.csiPend...), p...)
		t.csiPend = nil
	}
	i := 0
	for i < len(p) {
		if p[i] == 0x1b {
			if i+1 >= len(p) {
				t.csiPend = []byte{0x1b}
				return
			}
			if p[i+1] == '[' {
				j := i + 2
				for j < len(p) && (p[j] < 0x40 || p[j] > 0x7E) {
					j++
				}
				if j >= len(p) {
					t.csiPend = append([]byte{}, p[i:]...)
					return
				}
				t.applyCSI(string(p[i+2:j]), p[j])
				i = j + 1
				continue
			}
		}
		switch p[i] {
		case '\r':
			t.CurX = 1
		case '\n':
			t.CurY++
		case 8:
			if t.CurX > 1 {
				t.CurX--
			}
		default:
			if p[i] >= 32 {
				t.CurX++
			}
		}
		i++
	}
}

func (t *IO) applyCSI(params string, cmd byte) {
	ps := []int{}
	if params != "" {
		for _, part := range strings.Split(params, ";") {
			n := 0
			if part != "" {
				fmt.Sscanf(part, "%d", &n)
			}
			ps = append(ps, n)
		}
	}
	n := 1
	if len(ps) > 0 && ps[0] > 0 {
		n = ps[0]
	}
	switch cmd {
	case 'H', 'f':
		row, col := 1, 1
		if len(ps) > 0 && ps[0] > 0 {
			row = ps[0]
		}
		if len(ps) > 1 && ps[1] > 0 {
			col = ps[1]
		}
		t.CurY, t.CurX = row, col
	case 'G':
		t.CurX = n
	case 'A':
		t.CurY -= n
		if t.CurY < 1 {
			t.CurY = 1
		}
	case 'B':
		t.CurY += n
	case 'C':
		t.CurX += n
	case 'D':
		t.CurX -= n
		if t.CurX < 1 {
			t.CurX = 1
		}
	}
}

func (t *IO) Print(s string) {
	// Language prompts store RA color as ^K (0x0B), often as Unicode ♂ after
	// CP437 decode. Send those through WriteRA so they become ANSI, not a glyph.
	if strings.ContainsRune(s, 0x0B) || strings.Contains(s, "\u2642") {
		t.WriteRA(s)
		return
	}
	t.WriteRaw(pascal.ToCP437(s))
}

// SetNoPause skips -- more -- and ^A wait-for-enter (language Q-A prompts).
func (t *IO) SetNoPause(on bool) {
	if t != nil {
		t.skipWaitEnter = on
	}
}

func (t *IO) NoPause() bool {
	return t != nil && t.skipWaitEnter
}

func (t *IO) Println(s string) {
	t.Print(s + "\r\n")
	t.Lines++
	t.checkMore()
}

// ResetLines is Pascal OutputObj.ResetLines: restart the more-prompt line count.
func (t *IO) ResetLines(n int) {
	if n < 0 {
		n = 0
	}
	t.Lines = n
}

func (t *IO) WriteRA(s string) {
	s = strings.ReplaceAll(s, "\u2642", "\x0b") // Unicode ♂ → RA ^K
	s = strings.ReplaceAll(s, "\u2660", "\x06") // Unicode ♠ → RA ^F
	t.writeRA(pascal.ToCP437(s), nil)
}

// raDelay is time.Sleep; tests may replace it.
var raDelay = time.Sleep

func (t *IO) checkMore() {
	if t.skipWaitEnter || t.hotDisplay || t.Line == nil || !t.MorePrompt || !t.Line.DispMorePrompt || t.StopMore {
		return
	}
	limit := t.Length
	if limit <= 0 {
		limit = 24
	}
	if t.Lines < limit-1 {
		return
	}
	t.Lines = 0
	prompt := "-- More --"
	keys := "YN="
	if t.Ral != nil && t.Ral.Entries() > 0 {
		if s := t.Ral.GetStr(lang.More); s != "" {
			prompt = s
		}
		if k := t.Ral.GetKeys(lang.More); k != "" {
			keys = pascal.UpCase(k)
		}
	}
	t.WriteRaw([]byte("\r\n"))
	t.WriteRA(prompt)
	ch, _ := t.GetKey(0)
	t.WriteRaw([]byte("\r          \r"))
	up := pascal.UpCase(string(ch))
	if up == "" {
		return
	}
	c := up[0]
	if strings.ContainsRune(keys, 'N') && (c == 'N' || c == 'S') {
		t.StopMore = true
		return
	}
	if strings.ContainsRune(keys, '=') && c == '=' {
		t.StopMore = true
		t.Line.DispMorePrompt = false
	}
}

func (t *IO) ClearScreen() {
	t.raduPend = nil
	if t.Line != nil && !t.Line.AnsiOn {
		t.WriteRaw([]byte("\r\n\r\n"))
	} else {
		t.WriteRaw([]byte("\x1b[2J\x1b[H"))
	}
	t.Lines = 0
}

func (t *IO) GetKey(timeout time.Duration) (byte, error) {
	if t != nil && t.idleDone {
		return 0, ErrIdle
	}
	var callEnd time.Time
	timed := timeout > 0
	if timed {
		callEnd = time.Now().Add(timeout)
	}
	defer func() { _ = t.S.SetReadDeadline(time.Time{}) }()
	for {
		if t.hung {
			return 0, ErrHangup
		}
		if err := t.checkIdle(); err != nil {
			return 0, err
		}
		if b, sysop, ok := t.popPush(); ok {
			t.FromSysop = sysop
			t.touchIdle()
			return b, nil
		}
		if b, ok := t.pollLocal(); ok {
			t.FromSysop = true
			t.touchIdle()
			return b, nil
		}
		if t.hung {
			return 0, ErrHangup
		}

		wait := time.Duration(-1)
		if timed {
			wait = time.Until(callEnd)
			if wait <= 0 {
				return 0, idleTimeout{}
			}
		}
		if slice := t.idleSlice(); slice >= 0 && (wait < 0 || slice < wait) {
			wait = slice
		}
		if t.Local != nil && (wait < 0 || wait > localPoll) {
			wait = localPoll
		}
		if wait == 0 {
			if err := t.checkIdle(); err != nil {
				return 0, err
			}
			if t.idleSlice() == 0 {
				time.Sleep(20 * time.Millisecond)
			}
			continue
		}
		if wait > 0 {
			_ = t.S.SetReadDeadline(time.Now().Add(wait))
		} else {
			_ = t.S.SetReadDeadline(time.Time{})
		}
		var buf [1]byte
		n, err := t.S.Read(buf[:])
		if n == 1 {
			t.FromSysop = false
			t.touchIdle()
			return buf[0], nil
		}
		if err == nil {
			err = io.EOF
		}
		if readTimeout(err) {
			if timed && !time.Now().Before(callEnd) {
				return 0, err
			}
			continue
		}
		if !t.S.Local() {
			t.lost = true
		}
		return 0, err
	}
}

// ArmIdle is Pascal SetIdleTimeLimit plus CheckInactivity. Zero disables it.
func (t *IO) ArmIdle(d time.Duration) {
	if t == nil {
		return
	}
	if d < 0 {
		d = 0
	}
	t.idleTime = d
	t.idleOn = d > 0
	t.idleWarned = false
	t.idleDone = false
	t.SetTimeOut()
}

// SetTimeOut is Pascal SetTimeOut: IdleLimit = now + IdleTime.
func (t *IO) SetTimeOut() {
	if t == nil || t.idleTime <= 0 {
		return
	}
	t.idleLimit = time.Now().Add(t.idleTime)
}

// SuspendIdle is the shell/transfer window where CheckInactivity is false.
// The returned function restores the flag and resets the idle clock.
func (t *IO) SuspendIdle() func() {
	if t == nil {
		return func() {}
	}
	prev := t.idleOn
	t.idleOn = false
	return func() {
		t.idleOn = prev
		t.SetTimeOut()
	}
}

// IdleHung reports that CheckIdle already disconnected this session.
func (t *IO) IdleHung() bool {
	return t != nil && t.idleDone
}

func (t *IO) touchIdle() {
	if t == nil || t.idleTime <= 0 {
		return
	}
	t.idleLimit = time.Now().Add(t.idleTime)
	t.idleWarned = false
}

func (t *IO) idleActive() bool {
	return t != nil && t.Line != nil && t.Line.Baud != 0 && t.idleOn && t.idleTime > 0 && !t.idleDone
}

// idleSlice is how long to block before the next CheckIdle event.
// Negative means the idle timer is not running.
func (t *IO) idleSlice() time.Duration {
	if !t.idleActive() {
		return -1
	}
	now := time.Now()
	remain := t.idleLimit.Sub(now)
	if remain < 0 {
		return 0
	}
	if t.idleTime > 30*time.Second && !t.idleWarned {
		untilWarn := t.idleLimit.Add(-30 * time.Second).Sub(now)
		if untilWarn <= 0 {
			if remain < 30*time.Second {
				return 0
			}
			return time.Millisecond
		}
		if untilWarn < remain {
			return untilWarn
		}
	}
	return remain
}

func (t *IO) checkIdle() error {
	if t == nil {
		return nil
	}
	if t.idleDone {
		return ErrIdle
	}
	if !t.idleActive() {
		return nil
	}
	remain := time.Until(t.idleLimit)
	if t.idleTime > 30*time.Second && !t.idleWarned && remain < 30*time.Second {
		t.idleWarned = true
		t.idleNotice(lang.Inactive2, "You are about to be disconnected for inactivity!", true)
	}
	if !time.Now().Before(t.idleLimit) {
		return t.idleHangup()
	}
	return nil
}

func (t *IO) idleHangup() error {
	t.idleDone = true
	t.idleOn = false
	t.idleNotice(lang.Inactive1, "* User Inactivity Timeout, Disconnecting *", false)
	if t.Cfg != nil && t.Line != nil {
		logx.Write(t.Cfg, t.Line.RaNodeNr, '>', "Inactivity timeout")
	}
	return ErrIdle
}

func (t *IO) idleNotice(id int, fallback string, bells bool) {
	msg := fallback
	if t.Ral != nil {
		if s := t.Ral.Get(id); s != "" {
			msg = s
		}
	}
	save := t.MorePrompt
	t.MorePrompt = false
	t.WriteRaw([]byte("\r\n"))
	t.WriteRA("`A12:" + msg + "\r\n")
	if bells {
		t.WriteRaw([]byte{7, 7})
	}
	t.MorePrompt = save
	t.ResetLines(1)
}

func readTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

type idleTimeout struct{}

func (idleTimeout) Error() string   { return "i/o timeout" }
func (idleTimeout) Timeout() bool   { return true }
func (idleTimeout) Temporary() bool { return true }

func (t *IO) PeekKey() (byte, bool) {
	t.mu.Lock()
	if len(t.push) > 0 {
		b := t.push[0]
		t.mu.Unlock()
		return b, true
	}
	t.mu.Unlock()
	ch, err := t.GetKey(2 * time.Millisecond)
	if err != nil {
		return 0, false
	}
	t.pushFrom([]byte{ch}, t.FromSysop)
	return ch, true
}

// FinishEnter swallows the LF after CR (or CR after LF) so the next prompt
// does not treat that mate as its own Enter.
func (t *IO) FinishEnter(got byte) {
	if t == nil {
		return
	}
	t.eatLineEndMate(got)
}

// eatLineEndMate swallows the LF after CR (or CR after LF) so telnet Enter
// does not leave a second key for the next prompt or the top menu.
func (t *IO) eatLineEndMate(got byte) {
	if got != '\r' && got != '\n' {
		return
	}
	ch, ok := t.PeekKey()
	if !ok {
		return
	}
	if got == '\r' && (ch == '\n' || ch == 0) {
		_, _ = t.GetKey(0)
		return
	}
	if got == '\n' && ch == '\r' {
		_, _ = t.GetKey(0)
	}
}

func (t *IO) eatPushedLineEndMate(got byte) {
	mate := lineEndMate(got)
	if mate == 0 {
		return
	}
	t.mu.Lock()
	if len(t.push) > 0 && t.push[0] == mate {
		t.push = t.push[1:]
		t.pushSysop = t.pushSysop[1:]
	}
	t.mu.Unlock()
}

func lineEndMate(got byte) byte {
	switch got {
	case '\r':
		return '\n'
	case '\n':
		return '\r'
	default:
		return 0
	}
}

// DrainLineEnds drops leftover CR/LF/NUL (password Enter on telnet is often CRLF or CR NUL).
func (t *IO) DrainLineEnds() {
	if t == nil {
		return
	}
	for i := 0; i < 8; i++ {
		ch, ok := t.PeekKey()
		if !ok || !isLineEndKey(ch) {
			return
		}
		_, _ = t.GetKey(0)
		t.eatLineEndMate(ch)
	}
}

func isLineEndKey(ch byte) bool {
	return ch == '\r' || ch == '\n' || ch == 0
}

// GetArrowKeys is Pascal GetArrowKeys: ESC already consumed. CSI A/B/C/D/H/K
// (and SS3 OA..OD). Timeout returns ESC so a lone Escape is not a hotkey.
func (t *IO) GetArrowKeys() byte {
	ch, err := t.GetKey(250 * time.Millisecond)
	if err != nil {
		return 0x1b
	}
	if ch == 'O' {
		ch, err = t.GetKey(time.Second)
		if err != nil {
			return 0x1b
		}
		if ch >= 'a' && ch <= 'z' {
			ch -= 32
		}
		switch ch {
		case 'A', 'B', 'C', 'D', 'H', 'K':
			return ch
		}
		return 0
	}
	if ch != '[' {
		return ch
	}
	final := byte(0)
	for i := 0; i < 24; i++ {
		c, err := t.GetKey(time.Second)
		if err != nil {
			return 0x1b
		}
		if c >= 0x40 && c <= 0x7E {
			final = c
			break
		}
	}
	if final >= 'a' && final <= 'z' {
		final -= 32
	}
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'K':
		return final
	}
	return 0
}

func (t *IO) PutBack(s string) {
	t.pushBytes(pascal.ToCP437(s))
}

// PutInBuffer is Pascal InputObj.PutInBuffer: ';' becomes CR, '_' becomes space.
func (t *IO) PutInBuffer(s string) {
	t.putInBuffer(s, false)
}

// PutSysopInBuffer is PutInBuffer with FromSysOp set, so the keys count as
// typed on the node window (Q-A EMULATESYSINPUT / EMULATESYSVAR).
func (t *IO) PutSysopInBuffer(s string) {
	t.putInBuffer(s, true)
}

func (t *IO) putInBuffer(s string, sysop bool) {
	if s == "" {
		return
	}
	b := pascal.ToCP437(s)
	for i, c := range b {
		switch c {
		case ';':
			b[i] = '\r'
		case '_':
			b[i] = ' '
		}
	}
	t.pushFrom(b, sysop)
}

func (t *IO) pushBytes(b []byte) {
	t.pushFrom(b, false)
}

// pushFrom puts b in front of the key buffer; sysop marks node-window keys.
func (t *IO) pushFrom(b []byte, sysop bool) {
	if len(b) == 0 {
		return
	}
	from := make([]bool, len(b))
	for i := range from {
		from[i] = sysop
	}
	t.mu.Lock()
	t.push = append(append([]byte{}, b...), t.push...)
	t.pushSysop = append(from, t.pushSysop...)
	t.mu.Unlock()
}

func (t *IO) popPush() (b byte, sysop, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.push) == 0 {
		return 0, false, false
	}
	b, sysop = t.push[0], t.pushSysop[0]
	t.push, t.pushSysop = t.push[1:], t.pushSysop[1:]
	return b, sysop, true
}

func (t *IO) GetString(max int, hidden, capitalize bool) (string, error) {
	return t.collectString(max, hidden, capitalize, false)
}

// GetStringIEMSI is Pascal GetString(..., IsIEMSI=true): stop as soon as the
// buffer contains **EMSI_ so the rest of the ICI packet stays in the stream.
func (t *IO) GetStringIEMSI(max int, capitalize bool) (string, error) {
	return t.collectString(max, false, capitalize, true)
}

func (t *IO) collectString(max int, hidden, capitalize, iemsi bool) (string, error) {
	var b []byte
	echo := "*"
	if t.Cfg != nil && t.Cfg.RaConfig.EchoChar != "" {
		echo = t.Cfg.RaConfig.EchoChar
	}
	for {
		ch, err := t.GetKey(0)
		if err != nil {
			return string(b), err
		}
		switch ch {
		case '\r':
			t.eatLineEndMate(ch)
			t.WriteRaw([]byte("\r\n"))
			t.Lines++
			s := pascal.FromCP437(b)
			if capitalize {
				s = titleCase(s)
			}
			return s, nil
		case '\n':
			continue
		case 8, 127:
			if len(b) > 0 {
				b = b[:len(b)-1]
				t.WriteRaw([]byte{8, ' ', 8})
			}
		case 3, 24: // ctrl-c / cancel
			return "", io.EOF
		default:
			if ch < 32 {
				continue
			}
			if len(b) >= max {
				continue
			}
			if capitalize {
				ch = titleCaseByte(b, ch)
			}
			b = append(b, ch)
			if hidden {
				t.WriteRaw(pascal.ToCP437(echo))
			} else {
				t.WriteRaw([]byte{ch})
			}
			if iemsi && bytes.Contains(bytes.ToUpper(b), []byte("**EMSI_")) {
				return pascal.FromCP437(b), nil
			}
		}
	}
}

// ReadBytes is Pascal GetFossilBuffer: read n bytes or fail on timeout/EOF.
func (t *IO) ReadBytes(n int, timeout time.Duration) ([]byte, bool) {
	if n <= 0 {
		return nil, true
	}
	b := make([]byte, 0, n)
	deadline := time.Now().Add(timeout)
	for len(b) < n {
		left := time.Until(deadline)
		if left <= 0 {
			return b, false
		}
		ch, err := t.GetKey(left)
		if err != nil {
			return b, false
		}
		b = append(b, ch)
	}
	return b, true
}

func titleCaseByte(prev []byte, ch byte) byte {
	if len(prev) == 0 || isNameWordSep(prev[len(prev)-1]) {
		if ch >= 'a' && ch <= 'z' {
			return ch - 32
		}
		return ch
	}
	if ch >= 'A' && ch <= 'Z' {
		return ch + 32
	}
	return ch
}

func isNameWordSep(ch byte) bool {
	return ch == ' ' || ch == '-' || ch == '.' || ch == ','
}

func titleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	wordStart := true
	for _, r := range s {
		if r == ' ' || r == '-' || r == '.' || r == ',' {
			b.WriteRune(r)
			wordStart = true
			continue
		}
		if wordStart {
			b.WriteRune(unicode.ToUpper(r))
			wordStart = false
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func (t *IO) waitEnter() {
	if t.skipWaitEnter {
		t.Lines = 0
		t.StopMore = false
		return
	}
	for {
		ch, err := t.GetKey(0)
		if err != nil || ch == '\r' || ch == '\n' {
			if err == nil {
				t.eatPushedLineEndMate(ch)
			}
			t.WriteRaw([]byte("\r\n"))
			t.Lines = 0
			t.StopMore = false
			return
		}
	}
}

// WaitEnter is Pascal InputObj.WaitEnter: wait for CR, no prompt.
func (t *IO) WaitEnter() { t.waitEnter() }

// PressEnter is Pascal InputObj.PressEnter: newline, RAL #258 (ralRtnCont), then CR.
func (t *IO) PressEnter() {
	if t.hotDisplay {
		return
	}
	t.Println("")
	fg, bg := 14, 0
	if t.Cfg != nil {
		if t.Cfg.RaConfig.NormFore != 0 {
			fg = int(t.Cfg.RaConfig.NormFore)
		}
		bg = int(t.Cfg.RaConfig.NormBack)
	}
	t.WriteRA(fmt.Sprintf("`F%d:`B%d:%s", fg, bg, t.RalGet(lang.RtnCont)))
	t.waitEnter()
}

func (t *IO) RalGet(nr int) string {
	if t != nil && t.Ral != nil {
		return t.Ral.Get(nr)
	}
	return lang.Default(nr)
}

func (t *IO) RalStr(nr int) string {
	if t != nil && t.Ral != nil {
		return t.Ral.GetStr(nr)
	}
	return lang.Default(nr)
}

func (t *IO) RalKeys(nr int) string {
	if t != nil && t.Ral != nil {
		return t.Ral.GetKeys(nr)
	}
	return ""
}

func (t *IO) AskYesNo(ralNr int, def bool) bool {
	s := t.RalGet(ralNr)
	s, def = stripRalYesNo(s, def)
	t.WriteRA("`A14:" + s)
	if t.Line != nil && (t.Line.AnsiOn || t.Line.AvatarOn) && t.YesNoQuest != nil {
		if ok, yes := t.YesNoQuest(def); ok {
			return yes
		}
	}
	return t.yesNoAsk(def)
}

// stripRalYesNo is Pascal RalStrYesNoAsk: the last Y/N of a yes/no
// language prompt is the default and is not displayed.
func stripRalYesNo(s string, def bool) (string, bool) {
	n := len(s)
	if n < 2 {
		return s, def
	}
	last, prev := s[n-1], s[n-2]
	letter := (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z')
	if letter {
		return s, def
	}
	switch last {
	case 'Y', 'y':
		return s[:n-1], true
	case 'N', 'n':
		return s[:n-1], false
	}
	return s, def
}

func (t *IO) yesNoAsk(def bool) bool {
	y := byte('Y')
	n := byte('N')
	if k := t.RalKeys(lang.Yes); k != "" {
		y = pascal.UpCase(k)[0]
	}
	if k := t.RalKeys(lang.No); k != "" {
		n = pascal.UpCase(k)[0]
	}
	left, right := "[", "]"
	if t.Cfg != nil {
		if t.Cfg.RaConfig.LeftBracket != 0 {
			left = string(t.Cfg.RaConfig.LeftBracket)
		}
		if t.Cfg.RaConfig.RightBracket != 0 {
			right = string(t.Cfg.RaConfig.RightBracket)
		}
	}
	slash := t.RalGet(lang.Slash)
	if slash == "" {
		slash = "/"
	}
	var br string
	if def {
		br = " " + left + string(y) + slash + strings.ToLower(string(n)) + right + "? "
	} else {
		br = " " + left + strings.ToLower(string(y)) + slash + string(n) + right + "? "
	}
	t.Print(br)
	for {
		ch, err := t.GetKey(0)
		if err != nil {
			return def
		}
		up := pascal.UpCase(string(ch))
		if up == "" {
			continue
		}
		c := up[0]
		yes := c == y
		no := c == n
		enter := c == '\r' || c == '\n'
		if !yes && !no && !enter {
			continue
		}
		if enter {
			yes = def
			no = !def
		}
		if yes {
			t.WriteRA(t.RalStr(lang.Yes))
			t.WriteRaw([]byte("\r\n"))
			return true
		}
		t.WriteRA(t.RalStr(lang.No))
		t.WriteRaw([]byte("\r\n"))
		return false
	}
}

func (t *IO) YesNo(prompt string, def bool) bool {
	t.WriteRA(prompt)
	if def {
		t.Print(" [Y/n]? ")
	} else {
		t.Print(" [y/N]? ")
	}
	ch, err := t.GetKey(0)
	t.WriteRaw([]byte("\r\n"))
	if err != nil {
		return def
	}
	switch ch {
	case 'Y', 'y':
		return true
	case 'N', 'n':
		return false
	case '\r', '\n':
		return def
	}
	return def
}

func (t *IO) writeRA(src []byte, hot map[byte]struct{}) byte {
	if t.hotKeys == nil {
		t.hotKeys = hot
	}
	if len(t.raduPend) > 0 {
		src = append(append([]byte{}, t.raduPend...), src...)
		t.raduPend = nil
	}
	buf := make([]byte, 0, 512)
	seenANSI := false
	sawCtrlE := false
	flush := func() {
		if len(buf) == 0 {
			return
		}
		t.WriteRaw(buf)
		buf = buf[:0]
		t.checkHot()
	}
	i := 0
	for i < len(src) {
		if t.StopMore {
			flush()
			return t.abortKey
		}
		c := src[i]
		if c == 0x17 { // ^W one-second delay
			flush()
			raDelay(time.Second)
			i++
			continue
		}
		if next, ok := skipRaF(src, i); ok { // ^F — RA user codes (♠ in CP437)
			flush()
			if next >= len(src) {
				break
			}
			nPad, pad, next := consumeMacroPad(src, next)
			if next >= len(src) {
				break
			}
			i = next
			code := src[i]
			i++
			val := t.userCode(string([]byte{code}))
			t.WriteRaw(pascal.ToCP437(dotPadding(val, nPad, userNumeric(code), pad)))
			continue
		}
		if next, ok := skipRaK(src, i); ok { // ^K — RA system codes (♂ in CP437)
			flush()
			if next >= len(src) {
				break
			}
			nPad, pad, next := consumeMacroPad(src, next)
			if next >= len(src) {
				break
			}
			i = next
			code := src[i]
			i++
			if code == '[' {
				if i+2 <= len(src) {
					t.setColor(hexAttr(src[i], src[i+1]))
					i += 2
				}
				continue
			}
			if code == ']' {
				if i+3 <= len(src) {
					n := int(src[i]-'0')*100 + int(src[i+1]-'0')*10 + int(src[i+2]-'0')
					i += 3
					t.WriteRA(dotPadding(t.RalGet(n), nPad, false, pad))
				}
				continue
			}
			if code == '/' || code == '\\' {
				t.WriteRaw([]byte("\x1b[K"))
				continue
			}
			if code == '@' || code == '!' {
				start := i
				for i < len(src) && !raCmdEnd(src[i]) {
					i++
				}
				token := strings.TrimSpace(string(src[start:i]))
				if i < len(src) && src[i] == '|' {
					i++
				}
				if i < len(src) && src[i] == '\r' {
					i++
				}
				if i < len(src) && src[i] == '\n' {
					i++
				}
				name, args := splitFirst(token)
				if name == "" {
					continue
				}
				if code == '@' {
					if t.RunScript != nil {
						was := t.skipWaitEnter
						t.skipWaitEnter = true
						t.RunScript(name, args)
						t.skipWaitEnter = was
					}
				} else {
					// ^K!<txtfile> — Pascal chrDisplayText / DisplayHotFile
					// from the language TextPath (txtfiles), not a bare ^K name.
					DisplayHotFile(t, t.textPath(), name)
				}
				continue
			}
			val := t.systemCode(string([]byte{code}))
			t.WriteRaw(pascal.ToCP437(dotPadding(val, nPad, systemNumeric(code), pad)))
			continue
		}
		switch c {
		case 0x01: // ^A WaitEnter. Type 40 (hotDisplay) keeps ☺ as a glyph.
			i++
			if t.hotDisplay {
				buf = append(buf, 0x01)
				continue
			}
			flush()
			t.waitEnter()
		case 0x02, 0x03: // ^B abort off / ^C abort on — hearts in ANSI are glyphs
			if seenANSI {
				buf = append(buf, c)
			}
			i++
		case 0x04: // ^D enable more — ♦ in ANSI art must not re-enable paging
			if !seenANSI && !sawCtrlE && t.Line != nil && !t.hotDisplay {
				t.Line.DispMorePrompt = true
			} else {
				buf = append(buf, 0x04)
			}
			i++
		case 0x05: // ^E disable -- more --
			if t.Line != nil {
				t.Line.DispMorePrompt = false
			}
			t.StopMore = false
			sawCtrlE = true
			i++
		case 0x1A: // ^Z ignored
			i++
		case 0x1B: // ANSI/CSI: keep the sequence together (one console write)
			seenANSI = true
			start := i
			i = consumeCSI(src, i)
			seq := src[start:i]
			buf = append(buf, seq...)
			if ansiResetsPager(seq) {
				t.Lines = 0
			}
			if len(buf) >= 4096 {
				flush()
			}
		case '`':
			if i+1 >= len(src) {
				flush()
				t.raduPend = []byte{'`'}
				return t.abortKey
			}
			code := src[i+1]
			if code == '`' {
				buf = append(buf, '`')
				i += 2
				continue
			}
			if !isRaduLetter(code) {
				buf = append(buf, '`')
				i++
				continue
			}
			flush()
			arg, next, ok := readArg(src, i+2)
			if !ok {
				t.raduPend = append([]byte{}, src[i:]...)
				return t.abortKey
			}
			i = next
			t.handleCode(code, arg)
		case '\n':
			buf = append(buf, '\r', '\n')
			flush()
			t.Lines++
			t.checkMore()
			i++
		case '\r':
			buf = append(buf, '\r')
			flush()
			i++
		default:
			buf = append(buf, c)
			i++
			limit := 4096
			if t.hotDisplay {
				limit = 128
			}
			if len(buf) >= limit {
				flush()
			}
		}
	}
	flush()
	return t.abortKey
}

func (t *IO) checkHot() {
	if !t.hotDisplay || len(t.hotKeys) == 0 || t.StopMore || t.hotHaveKey {
		return
	}
	ch, ok := t.PeekKey()
	if !ok {
		return
	}
	hitCh := ch
	if _, hit := t.hotKeys[ch]; !hit {
		alt := ch
		if ch >= 'a' && ch <= 'z' {
			alt = ch - 32
		} else if ch >= 'A' && ch <= 'Z' {
			alt = ch + 32
		}
		if alt == ch {
			t.hotHaveKey = true
			return
		}
		if _, hit := t.hotKeys[alt]; !hit {
			t.hotHaveKey = true
			return
		}
		hitCh = alt
	}
	_, _ = t.GetKey(2 * time.Millisecond)
	t.abortKey = hitCh
	t.StopMore = true
}

func consumeCSI(src []byte, i int) int {
	if i >= len(src) || src[i] != 0x1b {
		return i
	}
	i++
	if i >= len(src) {
		return i
	}
	switch src[i] {
	case '[':
		i++
		for i < len(src) {
			ch := src[i]
			i++
			if ch >= 0x40 && ch <= 0x7E {
				break
			}
		}
	case ']':
		i++
		for i < len(src) {
			ch := src[i]
			i++
			if ch == 0x07 || ch == 0x1b {
				if ch == 0x1b && i < len(src) && src[i] == '\\' {
					i++
				}
				break
			}
		}
	default:
		i++
	}
	return i
}

func ansiResetsPager(seq []byte) bool {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != '[' {
		return false
	}
	s := string(seq)
	if strings.Contains(s, "2J") {
		return true
	}
	if !strings.HasSuffix(s, "H") && !strings.HasSuffix(s, "f") {
		return false
	}
	body := strings.TrimSuffix(strings.TrimSuffix(s[2:], "H"), "f")
	switch body {
	case "", "1", "0", "1;1", "0;0", "1;0", "0;1":
		return true
	}
	return false
}

func isRaduLetter(c byte) bool {
	if c >= 'a' && c <= 'z' {
		c -= 32
	}
	switch c {
	case 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'L', 'X', 'Y', 'S', 'U':
		return true
	}
	return false
}

func readArg(src []byte, i int) (string, int, bool) {
	start := i
	for i < len(src) && src[i] != ':' && src[i] != '`' {
		i++
		if i-start > 40 {
			return string(src[start:i]), i, true
		}
	}
	arg := string(src[start:i])
	if i >= len(src) {
		return arg, i, false
	}
	if src[i] == ':' {
		i++
	}
	return arg, i, true
}

func (t *IO) handleCode(code byte, arg string) {
	switch code {
	case 'A', 'a':
		n := 7
		fmt.Sscanf(arg, "%d", &n)
		t.setColor(byte(n))
	case 'B', 'b': // RADU background (not backspace)
		n := 0
		fmt.Sscanf(arg, "%d", &n)
		fg := t.Attr & 0x0F
		t.setColor((byte(n&0x07) << 4) | fg)
	case 'F', 'f': // RADU foreground
		n := 7
		fmt.Sscanf(arg, "%d", &n)
		bg := t.Attr & 0x70
		t.setColor(bg | byte(n&0x0F))
	case 'C', 'c': // RADU blink
		t.setColor(t.Attr | 0x80)
	case 'E', 'e':
		t.WriteRaw([]byte("\x1b[K"))
	case 'G', 'g':
		x, y := 1, 1
		if i := strings.IndexByte(arg, ','); i >= 0 {
			fmt.Sscanf(arg[:i], "%d", &x)
			fmt.Sscanf(arg[i+1:], "%d", &y)
		} else {
			fmt.Sscanf(arg, "%d", &x)
		}
		t.GotoXY(x, y)
	case 'S', 's':
		t.ClearScreen()
	case 'U', 'u':
		t.WriteRaw([]byte("\x1b[A"))
	case 'X', 'x':
		t.setX(raduCoord(arg, t.WhereX()))
	case 'Y', 'y':
		t.GotoXY(t.WhereX(), raduCoord(arg, t.WhereY()))
	case 'K', 'k':
		t.WriteRaw([]byte("\r\n"))
		t.Lines++
	case 'P', 'p':
		if !t.hotDisplay {
			t.PressEnter()
		}
	case '#':
		t.Print(t.systemCode(arg))
	case '@':
		t.Print(t.userCode(arg))
	}
}

func (t *IO) setColor(attr byte) {
	t.Attr = attr
	t.WriteRaw(ansiColor(attr))
}

func ansiColor(attr byte) []byte {
	fg := attr & 0x0F
	bg := (attr >> 4) & 0x0F
	mapc := []int{0, 4, 2, 6, 1, 5, 3, 7}
	var b strings.Builder
	b.WriteString("\x1b[0;")
	if fg > 7 {
		b.WriteString("1;")
		fg -= 8
	}
	if bg > 7 {
		b.WriteString("5;")
		bg -= 8
	}
	if fg > 7 {
		fg = 7
	}
	if bg > 7 {
		bg = 7
	}
	fmt.Fprintf(&b, "3%d;4%dm", mapc[fg], mapc[bg])
	return []byte(b.String())
}

func (t *IO) systemCode(arg string) string {
	if arg == "" {
		return ""
	}
	c := arg[0]
	if c >= 'a' && c <= 'z' {
		c -= 32
	}
	line := t.Line
	itoa := func(n int) string { return fmt.Sprintf("%d", n) }
	switch c {
	case '#':
		if t.Cfg != nil {
			return t.Cfg.RaConfig.SemPath
		}
	case '$':
		return "0"
	case '%':
		if line != nil {
			return line.SysInfo.LastCaller
		}
	case '&', '\'':
		return "0"
	case '(':
		if line != nil {
			return line.Language.DefName
		}
	case '0', 'C', 'D', 'E':
		return "0"
	case '1':
		if line != nil {
			return itoa(config.MessageAreaIndex(t.Cfg, line.User.MsgArea))
		}
		return "0"
	case '2':
		if line != nil {
			return itoa(config.FileAreaIndex(t.Cfg, line.User.FileArea))
		}
		return "0"
	case 'A':
		if line != nil {
			return itoa(int(line.SysInfo.TotalCalls))
		}
		return "0"
	case 'B':
		if line != nil {
			return line.SysInfo.LastCaller
		}
	case 'F':
		return "0"
	case 'G':
		return time.Now().Weekday().String()
	case 'H':
		return "0"
	case 'I':
		return time.Now().Format("15:04")
	case 'J':
		return time.Now().Format("01-02-06")
	case 'K':
		return "0"
	case 'L', 'N', 'P':
		return "00"
	case 'M':
		if line != nil {
			return itoa(int(line.User.Elapsed))
		}
		return "0"
	case 'O', 'Q':
		if line != nil {
			return itoa(int(line.TimeLimit))
		}
		return "0"
	case 'R':
		if line != nil {
			return itoa(int(line.Baud))
		}
		return "0"
	case 'S':
		s := time.Now().Weekday().String()
		if len(s) > 3 {
			return s[:3]
		}
		return s
	case 'T':
		return "0"
	case 'U':
		return "0"
	case 'V':
		return ""
	case 'W':
		if line != nil {
			return itoa(line.RaNodeNr)
		}
		return "0"
	case 'Y':
		if line != nil {
			return config.MessageAreaName(t.Cfg, line.User.MsgArea)
		}
		return ""
	case 'Z':
		if line != nil {
			return config.FileAreaName(t.Cfg, line.User.FileArea)
		}
		return ""
	}
	return ""
}

// ExpandRA replaces RA ^K system and ^F user codes with their values (Pascal RaCodeStr).
func (t *IO) ExpandRA(s string) string {
	if t == nil || s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\u2642", "\x0b") // ♂ → RA ^K
	s = strings.ReplaceAll(s, "\u2660", "\x06") // ♠ → RA ^F
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x0B && i+1 < len(s) {
			nPad, pad, j := consumeMacroPadStr(s, i+1)
			if j >= len(s) {
				break
			}
			code := s[j]
			switch code {
			case '[':
				if j+2 < len(s) {
					out.Write(ansiColor(hexAttr(s[j+1], s[j+2])))
					i = j + 2
				} else {
					i = j
				}
				continue
			case ']':
				if j+3 < len(s) {
					n := 0
					fmt.Sscanf(s[j+1:j+4], "%d", &n)
					out.WriteString(dotPadding(t.RalGet(n), nPad, false, pad))
					i = j + 3
				} else {
					i = j
				}
				continue
			case '/', '\\':
				out.WriteString("\x1b[K")
				i = j
				continue
			}
			out.WriteString(dotPadding(t.systemCode(string([]byte{code})), nPad, systemNumeric(code), pad))
			i = j
			continue
		}
		if s[i] == 0x06 && i+1 < len(s) {
			nPad, pad, j := consumeMacroPadStr(s, i+1)
			if j >= len(s) {
				break
			}
			code := s[j]
			out.WriteString(dotPadding(t.userCode(string([]byte{code})), nPad, userNumeric(code), pad))
			i = j
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func (t *IO) userCode(arg string) string {
	if arg == "" || t.Line == nil {
		return ""
	}
	u := t.Line.User
	c := arg[0]
	if c >= 'a' && c <= 'z' {
		c -= 32
	}
	yn := func(v bool) string {
		if t.Ral != nil {
			return t.Ral.YesNo(v)
		}
		if v {
			return "Yes"
		}
		return "No"
	}
	itoa := func(n int) string { return fmt.Sprintf("%d", n) }
	switch c {
	case '!':
		return config.ProtocolName(t.Cfg, u.DefaultProto)
	case '@':
		return "0"
	case '~':
		return ""
	case '3':
		return u.Handle
	case '4':
		return u.FirstDate
	case '5':
		return u.BirthDate
	case '6':
		return u.SubDate
	case 'A':
		return u.Name
	case 'B':
		return u.Location
	case 'D':
		return u.DataPhone
	case 'E':
		return u.VoicePhone
	case 'F':
		return u.LastDate
	case 'G':
		return u.LastTime
	case 'H':
		return flagByte(u.Flags[0])
	case 'I':
		return flagByte(u.Flags[1])
	case 'J':
		return flagByte(u.Flags[2])
	case 'K':
		return flagByte(u.Flags[3])
	case 'L':
		return itoa(int(u.Credit - u.Pending))
	case 'M':
		return itoa(int(u.MsgsPosted))
	case 'N':
		return itoa(int(u.LastRead))
	case 'O':
		return itoa(int(u.Security))
	case 'P':
		return itoa(int(u.NoCalls))
	case 'Q':
		return itoa(int(u.Uploads))
	case 'R':
		return itoa(int(u.UploadsK))
	case 'S':
		return itoa(int(u.Downloads))
	case 'T':
		return itoa(int(u.DownloadsK))
	case 'U':
		return itoa(int(u.Elapsed))
	case 'V':
		return itoa(int(u.ScreenLength))
	case 'W':
		return firstWord(u.Name)
	case 'X':
		return yn(u.ANSI())
	case 'Y':
		return yn(u.More())
	case 'Z':
		return yn(u.Attribute&cfgrec.UserClrScr != 0)
	case ']':
		return u.Comment
	case '$':
		return u.Address1
	case '%':
		return u.Address2
	case '&':
		return u.Address3
	case '#':
		return config.FileGroupName(t.Cfg, u.FileGroup)
	case ')':
		return config.MessageGroupName(t.Cfg, u.MsgGroup)
	case '*':
		return itoa(int(u.FileGroup))
	case '+':
		return itoa(int(u.MsgGroup))
	}
	return ""
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}

func flagByte(b byte) string {
	var out [8]byte
	for i := 0; i < 8; i++ {
		if b&(1<<uint(i)) != 0 {
			out[i] = 'X'
		} else {
			out[i] = '-'
		}
	}
	return string(out[:])
}

func (t *IO) textPath() string {
	if t == nil {
		return ""
	}
	if t.Line != nil {
		if p := strings.TrimSpace(t.Line.Language.TextPath); p != "" {
			return p
		}
	}
	if t.Cfg != nil {
		return t.Cfg.RaConfig.TextPath
	}
	return ""
}

func DisplayFile(t *IO, textPath, name string) bool {
	return openAndShow(t, textPath, name)
}

// FindTextFile is Pascal OpenTextFile: resolved ANS/ASC path, if the file exists.
func FindTextFile(t *IO, textPath, name string) (string, bool) {
	p, _, ok := openTextFile(t, textPath, name)
	return p, ok
}

// DisplayHotFile is Pascal DisplayHotFile(name, []): ANS/ASC with ContrCodes
// on, so ^A waits for Enter. Menu type 40 uses DisplayHot instead.
func DisplayHotFile(t *IO, textPath, name string) bool {
	if t != nil {
		t.ResetLines(1)
	}
	return openAndShow(t, textPath, name)
}

// DisplayHot is menu type 40: show ANS/ASC with hotkeys, no -- more --, no wait-for-enter.
// Returns 0 if the file finished, or the hotkey that aborted it. found is false if missing.
func DisplayHot(t *IO, textPath, name string, keys map[byte]struct{}) (abort byte, found bool) {
	saveMore := t.MorePrompt
	saveDisp := true
	if t.Line != nil {
		saveDisp = t.Line.DispMorePrompt
		t.Line.DispMorePrompt = false
	}
	t.MorePrompt = false
	t.hotDisplay = true
	t.hotKeys = keys
	t.abortKey = 0
	t.hotHaveKey = false
	t.Lines = 0
	t.StopMore = false
	defer func() {
		t.hotDisplay = false
		t.hotKeys = nil
		t.hotHaveKey = false
		t.MorePrompt = saveMore
		if t.Line != nil {
			t.Line.DispMorePrompt = saveDisp
		}
		t.StopMore = false
		t.ResetLines(1)
	}()
	if !openAndShow(t, textPath, name) {
		return 0, false
	}
	return t.abortKey, true
}

func openAndShow(t *IO, textPath, name string) bool {
	_, b, ok := openTextFile(t, textPath, name)
	if !ok {
		return false
	}
	saveMore := true
	if t.Line != nil {
		saveMore = t.Line.DispMorePrompt
	}
	t.StopMore = false
	t.Lines = 0
	t.writeRA(b, t.hotKeys)
	if t.Line != nil && t.Line.DispMorePrompt {
		t.Line.DispMorePrompt = saveMore
	}
	t.StopMore = false
	return true
}

func openTextFile(t *IO, textPath, name string) (string, []byte, bool) {
	name = strings.TrimSpace(strings.Trim(name, `"'`))
	if name == "" {
		return "", nil, false
	}
	ansiOn := t != nil && t.Line != nil && t.Line.AnsiOn
	extra := []string{}
	if t != nil && t.Line != nil {
		extra = append(extra, t.Line.Language.TextPath, t.Line.Language.MenuPath, t.Line.Language.QuesPath)
	}
	extra = append(extra, textPath)
	if t != nil && t.Cfg != nil {
		extra = append(extra, t.Cfg.RaConfig.TextPath, t.Cfg.RaConfig.MenuPath, t.Cfg.RaConfig.SysPath)
	}
	var cfg *cfgrec.GlobalCfg
	if t != nil {
		cfg = t.Cfg
	}
	pathed := strings.ContainsAny(name, `\/`) || strings.Contains(name, ".")
	if pathed {
		for _, spec := range config.PathCandidates(cfg, name, extra...) {
			if p, b, ok := readTextName(spec, ansiOn); ok {
				return p, b, true
			}
		}
		return "", nil, false
	}
	dirs := append([]string{}, extra...)
	dirs = append(dirs, ".")
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		base := filepath.Join(strings.TrimRight(d, `\/`), name)
		if p, b, ok := readTextName(base, ansiOn); ok {
			return p, b, true
		}
	}
	return "", nil, false
}

func readTextName(spec string, ansiOn bool) (string, []byte, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", nil, false
	}
	dir := filepath.Dir(spec)
	base := stripTextExt(filepath.Base(spec))
	var cands []string
	if ansiOn {
		cands = append(cands, filepath.Join(dir, base+".ans"), filepath.Join(dir, base+".ANS"))
	}
	cands = append(cands,
		filepath.Join(dir, base+".asc"),
		filepath.Join(dir, base+".ASC"),
		filepath.Join(dir, base+".avt"),
		filepath.Join(dir, base+".AVT"),
		filepath.Join(dir, base+".txt"),
		filepath.Join(dir, base+".TXT"),
		spec,
	)
	seen := map[string]bool{}
	for _, p := range cands {
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return p, b, true
	}
	return "", nil, false
}

func stripTextExt(name string) string {
	ext := filepath.Ext(name)
	switch strings.ToLower(ext) {
	case ".ans", ".asc", ".avt", ".txt", ".rip", ".ri2":
		return name[:len(name)-len(ext)]
	}
	return name
}

func splitFirst(s string) (name, rest string) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	name = s[:i]
	rest = strings.TrimSpace(s[i:])
	return
}

func hexAttr(h1, h2 byte) byte {
	return hexNibble(h1)<<4 | hexNibble(h2)
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return 0
}

// skipRaK consumes CP437 ^K (0x0B) or UTF-8 ♂ (U+2642), the RA system-code lead-in.
func skipRaK(src []byte, i int) (int, bool) {
	if i >= len(src) {
		return i, false
	}
	if src[i] == 0x0B {
		return i + 1, true
	}
	if i+2 < len(src) && src[i] == 0xE2 && src[i+1] == 0x99 && src[i+2] == 0x82 {
		return i + 3, true
	}
	return i, false
}

// skipRaF consumes CP437 ^F (0x06) or UTF-8 ♠ (U+2660), the RA user-code lead-in.
func skipRaF(src []byte, i int) (int, bool) {
	if i >= len(src) {
		return i, false
	}
	if src[i] == 0x06 {
		return i + 1, true
	}
	if i+2 < len(src) && src[i] == 0xE2 && src[i+1] == 0x99 && src[i+2] == 0xA0 {
		return i + 3, true
	}
	return i, false
}

func isMacroPad(c byte) bool {
	return c == '.' || c == ',' || c == '?' || c == '`'
}

func consumeMacroPad(src []byte, i int) (n int, pad byte, next int) {
	pad = '.'
	if i >= len(src) || !isMacroPad(src[i]) {
		return 0, pad, i
	}
	pad = src[i]
	for i < len(src) && n < 59 && isMacroPad(src[i]) {
		n++
		i++
	}
	return n, pad, i
}

func consumeMacroPadStr(s string, i int) (n int, pad byte, next int) {
	pad = '.'
	if i >= len(s) || !isMacroPad(s[i]) {
		return 0, pad, i
	}
	pad = s[i]
	for i < len(s) && n < 59 && isMacroPad(s[i]) {
		n++
		i++
	}
	return n, pad, i
}

func systemNumeric(c byte) bool {
	if c >= 'a' && c <= 'z' {
		c -= 32
	}
	switch c {
	case '$', '&', '\'', '0', '1', '2', 'A', 'D', 'E', 'F', 'H', 'K', 'L', 'M', 'N', 'O', 'Q', 'R', 'T', 'U', 'W':
		return true
	}
	return false
}

func userNumeric(c byte) bool {
	if c >= 'a' && c <= 'z' {
		c -= 32
	}
	switch c {
	case '*', '+', '9', ':', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', '[', '^', '_', '{':
		return true
	}
	return false
}

func dotPadding(s string, n int, numeric bool, pad byte) string {
	if n <= 0 {
		return s
	}
	b := pascal.ToCP437(s)
	switch pad {
	case '.':
		if numeric {
			b = rightJustBytes(b, n)
		} else {
			b = leftJustBytes(b, n)
		}
	case ',':
		b = rightJustBytes(b, n)
	case '?':
		b = leftJustBytes(b, n)
	case '`':
		b = centerJustBytes(b, n)
	}
	if len(b) > n {
		b = b[:n]
	}
	return pascal.FromCP437(b)
}

func leftJustBytes(b []byte, n int) []byte {
	for len(b) < n {
		b = append(b, ' ')
	}
	return b
}

func rightJustBytes(b []byte, n int) []byte {
	for len(b) < n {
		b = append([]byte{' '}, b...)
	}
	return b
}

func centerJustBytes(b []byte, n int) []byte {
	for len(b) < n {
		b = append(b, ' ')
		if len(b) < n {
			b = append([]byte{' '}, b...)
		}
	}
	return b
}

func raCmdEnd(c byte) bool {
	return c == '|' || c == '\r' || c == '\n' || c == 0x1b || c == 0x0B || c == 0x01
}
