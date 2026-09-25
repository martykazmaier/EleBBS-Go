package sysop

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"elebbs/internal/door"
	"elebbs/internal/lang"
	"elebbs/internal/term"
)

// Pascal global.pas chat colours.
const (
	chatSysopColor = 14 // Yellow
	chatUserColor  = 3  // Cyan
)

// Pascal BeginChatting AbortWrapSet: characters that end a word.
const chatWordBreak = "\r \n.,])/?!"

// Chat is Pascal tChatterObj.BeginChatting (Alt-C): the sysop and the caller
// type to each other until the sysop presses Esc.
func (n *Node) Chat() {
	t, line, cfg := n.T, n.Line, &n.G.RaConfig
	n.chatting = true
	saveFrozen := line.TimeFrozen
	line.TimeFrozen = cfg.FreezeChat
	restoreIdle := t.SuspendIdle()
	saveMore := t.MorePrompt
	t.MorePrompt = false
	defer func() {
		n.closeLog()
		t.MorePrompt = saveMore
		restoreIdle()
		line.TimeFrozen = saveFrozen
		n.chatting = false
	}()

	t.Println("")
	t.Println("")
	t.Println("")
	if !term.DisplayHotFile(t, cfg.TextPath, "startcht") {
		t.WriteRA(t.RalStr(lang.StartCht) + "\r\n")
	}
	if cfg.ChatCommand != "" && line.LoggedOn {
		door.Run(t, n.G, line, nil, cfg.ChatCommand, false)
	} else {
		t.Println("")
		if line.Baud == 0 {
			t.WriteRA("`A14:" + t.RalStr(lang.NoUser1) + "\r\n")
		} else {
			if cfg.AutoChatCapture {
				n.openLog()
			}
			n.chatLoop()
		}
		t.Println("")
		t.Println("")
	}
	n.closeLog()
	if !term.DisplayHotFile(t, cfg.TextPath, "endcht") {
		t.WriteRA("`A7:" + t.RalStr(lang.EndCht) + "\r\n")
	}
	t.PushKey(255)
}

type chatState struct {
	color byte
	word  []byte
}

func (n *Node) chatLoop() {
	t := n.T
	st := &chatState{}
	for !t.HungUp() {
		ch, sysop, ok := n.chatKey()
		if !ok {
			return
		}
		if ch != 0x1b {
			n.logStr = append(n.logStr, ch)
		}
		if bytes.IndexByte([]byte(chatWordBreak), ch) >= 0 {
			st.word = st.word[:0]
		} else {
			st.word = append(st.word, ch)
		}
		if t.WhereX() == 79 && ch != '\r' && len(st.word) < 70 {
			t.WriteRA(fmt.Sprintf("`X%d:", t.WhereX()-len(st.word)))
			t.WriteRaw(bytes.Repeat([]byte{' '}, 80-t.WhereX()))
			t.WriteRaw([]byte("\r\n"))
			st.word = st.word[:len(st.word)-1]
			t.WriteRaw(st.word)
		}
		n.chatInput(ch, sysop, st)
		if ch == 0x1b {
			return
		}
	}
}

// chatKey reads the next chat character. Ctrl-A from the sysop toggles the
// capture log; Esc only counts when the sysop presses it.
func (n *Node) chatKey() (byte, bool, bool) {
	for {
		ch, err := n.T.GetKey(0)
		if err != nil {
			return 0, false, false
		}
		sysop := n.T.FromSysop
		if ch == 1 && sysop {
			if n.logFile != nil {
				n.closeLog()
			} else {
				n.openLog()
			}
		}
		if ch == 0x1b && !sysop {
			continue
		}
		if ch == 0x1b || ch == 8 || ch == '\r' || ch >= 32 && ch <= 254 {
			return ch, sysop, true
		}
	}
}

// chatInput is Pascal HandleChatInput for the non-IEMSI chat.
func (n *Node) chatInput(ch byte, sysop bool, st *chatState) {
	t := n.T
	var out []byte
	switch ch {
	case '\r':
		t.WriteRaw([]byte("\r\n"))
		if n.logFile != nil {
			fmt.Fprintf(n.logFile, "%s\r\n", bytes.TrimRight(n.logStr, "\r"))
		}
		n.logStr = n.logStr[:0]
	case 0x1b:
	case 8, 127:
		out = []byte{8, ' ', 8}
		if len(n.logStr) > 1 {
			n.logStr = n.logStr[:len(n.logStr)-2]
		} else {
			n.logStr = n.logStr[:0]
		}
	default:
		out = []byte{ch}
	}
	color := byte(chatUserColor)
	if sysop {
		color = chatSysopColor
	}
	if st.color != color {
		t.WriteRA(fmt.Sprintf("`A%d:", color))
		st.color = color
		st.word = st.word[:0]
	}
	if len(out) > 0 {
		t.WriteRaw(out)
	}
}

// openLog is Pascal OpenLogFile: ask for the capture file and append to it.
func (n *Node) openLog() {
	if n.logFile != nil {
		return
	}
	name := n.logName
	if name == "" {
		name = "chat.log"
	}
	if n.Win != nil && n.Keys != nil {
		done := n.sysopScreen()
		s, key := n.editBox(2, 10, 76, 14, "Capture filename", 70, name, false)
		done()
		if key != keyEnter {
			return
		}
		name = s
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	n.logName = name
	n.logFile = f
	now := time.Now()
	fmt.Fprintf(f, "\r\n** Capture log file opened at %s on %s\r\n", now.Format("15:04"), now.Format("01-02-06"))
	fmt.Fprintf(f, "** Chatting with %s of %s\r\n\r\n", n.Line.User.Name, n.Line.User.Location)
}

// closeLog is Pascal CloseLogFile.
func (n *Node) closeLog() {
	if n.logFile == nil {
		return
	}
	fmt.Fprintf(n.logFile, "%s\r\n", n.logStr)
	n.logFile.Close()
	n.logFile = nil
	if n.Win == nil {
		return
	}
	cfg := &n.G.RaConfig
	done := n.sysopScreen()
	n.box(30, 12, 50, 16, cfg.BorderFore, cfg.BorderBack, "")
	n.write(32, 14, attr(cfg.WindFore, cfg.WindBack), "Capturelog closed")
	n.wait(1500 * time.Millisecond)
	done()
}
