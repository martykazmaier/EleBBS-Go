package door

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/lang"
	"elebbs/internal/logx"
	"elebbs/internal/term"
)

func Run(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, areas []cfgrec.FilesArea, raw string, showMsg bool) {
	if line == nil {
		return
	}
	sock := socketHandle(t, line)
	dup := uintptr(0)
	inheritNum := ""
	if sock != 0 && sock != ^uintptr(0) {
		dup = dupInheritable(sock)
		if dup != 0 && dup != ^uintptr(0) {
			inheritNum = strconv.FormatUint(uint64(dup), 10)
		} else {
			inheritNum = strconv.FormatUint(uint64(sock), 10)
		}
	}
	flags := expandStars(t, g, line, areas, raw, inheritNum)
	if flags.cmd == "" {
		if dup != 0 && dup != sock {
			closeSocket(dup)
		}
		return
	}
	dropSock := sock
	if showMsg && flags.clearScreen && t != nil {
		t.ClearScreen()
		t.WriteRA("`A14:" + t.RalGet(lang.Loading) + "\r\n")
	}
	dir := DropDir(g, line)
	if err := WriteDropFiles(g, line, t, dir, flags, dropSock); err != nil && g != nil {
		logx.Write(g, line.RaNodeNr, '!', "drop files: "+err.Error())
	}
	if g != nil {
		logx.Write(g, line.RaNodeNr, '>', "OS shell : "+flags.cmd)
	}
	exe, rest := splitPath(flags.cmd)
	if exe == "" {
		if dup != 0 && dup != sock {
			closeSocket(dup)
		}
		return
	}
	if p := config.ExistingFile(g, exe, dir); p != "" {
		exe = p
	}
	handles := []uintptr{}
	if flags.inherit {
		if sock != 0 && sock != ^uintptr(0) {
			setInherit(sock, false)
		}
		if dup != 0 && dup != sock && dup != ^uintptr(0) {
			setInherit(dup, true)
			handles = append(handles, dup)
		}
	} else if sock != 0 && sock != ^uintptr(0) {
		if dup != 0 && dup != sock {
			setInherit(dup, false)
			closeSocket(dup)
			dup = 0
		}
		setInherit(sock, true)
		handles = append(handles, sock)
	}
	spawnExe, spawnRest := exe, rest
	if !flags.inherit {
		if bat, err := writeTmpBatch(g, line, flags, exe, rest); err == nil {
			spawnExe, spawnRest = viaComspec(bat, "")
		} else if isBatch(exe) {
			spawnExe, spawnRest = viaComspec(exe, rest)
		}
	} else if isBatch(exe) {
		spawnExe, spawnRest = viaComspec(exe, rest)
	}
	// Child cwd is the node directory (drop files, DSZ.CTL, DSZLOG filename).
	err := spawnDoor(spawnExe, spawnRest, dir, handles, flags.inherit && !isBatch(exe), 0)
	if dup != 0 && dup != sock {
		closeSocket(dup)
	}
	if !flags.leaveHot {
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil && g != nil {
		logx.Write(g, line.RaNodeNr, '!', "OS shell failed: "+err.Error())
	}
}

func quoteArg(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, `"`) {
		return `"` + s + `"`
	}
	return s
}

func viaComspec(script, args string) (exe, rest string) {
	rest = "/C " + quoteArg(script)
	if strings.TrimSpace(args) != "" {
		rest += " " + args
	}
	return comspec(), rest
}

func tmpBatchPath(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) string {
	dir := DropDir(g, line)
	if dir == "" {
		dir = "."
	}
	n := 1
	if line != nil && line.RaNodeNr > 0 {
		n = line.RaNodeNr
	}
	return filepath.Join(dir, fmt.Sprintf("TMP%d.BAT", n))
}

func writeTmpBatch(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg, flags starFlags, exe, rest string) (string, error) {
	name := tmpBatchPath(g, line)
	cmdLine := exe
	if strings.TrimSpace(rest) != "" {
		cmdLine += " " + rest
	}
	if len(cmdLine) > 255 {
		cmdLine = cmdLine[:255]
	}
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("rem ** Batch file automaticly created by EleBBS/W32\r\n")
	b.WriteString("rem ** You need to call an FOSSIL driver from this batchfile\r\n")
	b.WriteString("rem ** SFOS.BAT is the start batch, UFOS.BAT is to unload it\r\n")
	b.WriteString("rem ** Both files must reside in the same dir where EXITINFO.BBS is created\r\n")
	b.WriteString("\r\n")
	local := line != nil && line.LocalLogon
	if !flags.noBatches && flags.closeHandle && !local {
		sys := "."
		if g != nil {
			sys = strings.TrimRight(strings.TrimSpace(g.RaConfig.SysPath), `\/`)
			if sys == "" {
				sys = "."
			}
		}
		if p := config.ExistingFile(g, "SFOS.BAT", sys); p != "" {
			appendTextFile(&b, p)
		}
		b.WriteString("call ")
		b.WriteString(cmdLine)
		b.WriteString("\r\n")
		if p := config.ExistingFile(g, "UFOS.BAT", sys); p != "" {
			appendTextFile(&b, p)
		}
	} else {
		b.WriteString("call ")
		b.WriteString(cmdLine)
		b.WriteString("\r\n")
	}
	b.WriteString("CLS\r\n")
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(name, []byte(b.String()), 0644); err != nil {
		return "", err
	}
	return name, nil
}

func appendTextFile(b *strings.Builder, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(line)
		b.WriteString("\r\n")
	}
}
