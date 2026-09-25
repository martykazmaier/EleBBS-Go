// Package sysop is Pascal syskey.pas and friends: what the sysop can do
// from a node's console window while a caller is online.
package sysop

import (
	"os"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
	"elebbs/internal/logx"
	"elebbs/internal/term"
)

// Extended scan codes handled by Pascal LocalCommand.
const (
	ScanAltE = 18
	ScanAltD = 32
	ScanAltH = 35
	ScanAltC = 46
)

// Node is one online session as seen from its console window.
type Node struct {
	T    *term.IO
	G    *cfgrec.GlobalCfg
	Line *cfgrec.LineCfg
	Win  comm.LocalWindow
	Keys comm.LocalKeys
	// Pause stops the snoop mirror from drawing while a sysop window is up.
	Pause func() (resume func())

	pwdAccepted bool
	busy        bool
	chatting    bool
	sleep       func(time.Duration)

	logFile *os.File // chat capture
	logName string
	logStr  []byte
}

// Command is Pascal LocalCommand for the extended keys the sysop presses in
// the node window.
func (n *Node) Command(scan byte) {
	if n.busy {
		return
	}
	n.busy = true
	defer func() {
		n.busy = false
		n.T.SetTimeOut()
	}()
	switch scan {
	case ScanAltH:
		if n.keyAccess(false) {
			n.hangUp()
		}
	case ScanAltE:
		if n.keyAccess(true) {
			n.EditUser()
		}
	case ScanAltC:
		if n.keyAccess(true) {
			n.busy = false
			if !n.chatting {
				n.Chat()
			}
		}
	case ScanAltD:
		if n.keyAccess(true) {
			n.Line.Snooping = !n.Line.Snooping
		}
	}
}

// keyAccess is Pascal KeyAccess: LimitLocal disables the keys, and a
// keyboard password must be entered once (optionally removing the lock).
func (n *Node) keyAccess(askRemove bool) bool {
	cfg := &n.G.RaConfig
	if cfg.LimitLocal {
		return false
	}
	if !cfgrec.HasKeyboardPwd(cfg.KeyboardPwd) || n.pwdAccepted {
		return true
	}
	if n.Win == nil || n.Keys == nil {
		return false
	}
	done := n.sysopScreen()
	defer done()
	pw, key := n.editBox(23, 10, 57, 14, "Password", 15, "", true)
	if key != '\r' {
		pw = ""
	}
	if !strings.EqualFold(strings.TrimSpace(pw), strings.TrimSpace(cfg.KeyboardPwd)) {
		return false
	}
	if askRemove {
		n.box(23, 10, 57, 14, cfg.BorderFore, cfg.BorderBack, "Unlock keyboard")
		n.write(26, 12, attr(cfg.BorderFore, cfg.BorderBack), "Remove password lock (y,N)? ░")
		for {
			n.Win.GotoXY(54, 12)
			k := n.readKey()
			ch := upper(k.Ch)
			if ch == '\r' || ch == 'Y' || ch == 'N' {
				n.pwdAccepted = ch == 'Y'
				break
			}
		}
	}
	return true
}

// hangUp is Pascal HangUp from Alt-H.
func (n *Node) hangUp() {
	logx.Write(n.G, n.Line.RaNodeNr, '!', "User off-line")
	if n.Win != nil {
		resume := n.pause()
		cfg := &n.G.RaConfig
		n.box(30, 12, 50, 16, cfg.BorderFore, cfg.BorderBack, "")
		n.write(33, 14, attr(cfg.WindFore, cfg.WindBack), "Terminating call")
		resume()
	}
	n.T.HangUp()
}

func (n *Node) pause() func() {
	if n.Pause == nil {
		return func() {}
	}
	return n.Pause()
}

// sysopScreen is Pascal SaveScreen + LocalOnly for a sysop window; the
// result restores the screen and lets snooping continue.
func (n *Node) sysopScreen() func() {
	resume := n.pause()
	restore := n.Win.Save()
	return func() {
		restore()
		resume()
	}
}

func (n *Node) wait(d time.Duration) {
	if n.sleep != nil {
		n.sleep(d)
		return
	}
	time.Sleep(d)
}

// readKey is Pascal GetLocalKey: wait for a node-window key.
func (n *Node) readKey() comm.LocalKey {
	for {
		if k, ok := n.Keys.Poll(); ok {
			return k
		}
		n.wait(20 * time.Millisecond)
	}
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}
