//go:build windows

package telsrv

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"elebbs/internal/cfgrec"
	"elebbs/internal/comm"
	"golang.org/x/sys/windows"
)

func spawnEleBBS(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, exe string, conn net.Conn, node int, ip string) error {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return spawnRelayed(g, tn, exe, conn, node, ip)
	}
	f, err := tcp.File()
	if err != nil {
		return fmt.Errorf("socket file: %w", err)
	}
	defer f.Close()
	// Pascal TelSrv does not recv on the socket while EleBBS runs. Close the
	// net.Conn so Go's poller cannot steal data from NetCom.
	_ = conn.Close()
	src := windows.Handle(f.Fd())
	comm.SkipIOCP(uintptr(src))
	_ = windows.SetHandleInformation(src, windows.HANDLE_FLAG_INHERIT, 0)
	return SpawnEleBBSHandle(g, tn, exe, uintptr(src), node, ip, nil)
}

// spawnRelayed gives EleBBS a loopback socket and relays it to conn, for
// connections EleBBS cannot use directly (TLS, WebSocket).
func spawnRelayed(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, exe string, conn net.Conn, node int, ip string) error {
	bbsSt, peer, h, hf, err := comm.LocalSocketPair()
	if err != nil {
		return fmt.Errorf("socket pair: %w", err)
	}
	defer hf.Close()
	defer bbsSt.Close()
	defer peer.Close()
	go comm.Relay(peer, conn)
	return SpawnEleBBSHandle(g, tn, exe, h, node, ip, nil)
}

// SpawnEleBBSHandle is Pascal TelSrv NewExec: CREATE_NEW_CONSOLE EleBBS, inherit socket, wait.
func SpawnEleBBSHandle(g *cfgrec.GlobalCfg, tn cfgrec.TelnetCfg, exe string, handle uintptr, node int, ip string, extraEnv map[string]string) error {
	if handle == 0 || handle == ^uintptr(0) {
		return fmt.Errorf("invalid socket handle")
	}
	proc := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, windows.Handle(handle), proc, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return fmt.Errorf("DuplicateHandle: %w", err)
	}
	defer func() {
		_ = windows.SetHandleInformation(dup, windows.HANDLE_FLAG_INHERIT, 0)
		_ = windows.Closesocket(dup)
	}()
	comm.SkipIOCP(uintptr(dup))
	_ = windows.SetHandleInformation(dup, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)

	dir := nodeDir(tn, node)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	} else {
		dir = strings.TrimRight(strings.TrimSpace(g.RaConfig.SysPath), `\/`)
	}
	cmdline := windowsCmdLine(exe, fmt.Sprintf("-C1 -XC -XT -B65529 -H%d -N%d -XI%s", uintptr(dup), node, ip))
	exe16, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cmd16, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		return err
	}
	var dir16 *uint16
	if dir != "" {
		dir16, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
	}
	si := windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_SHOWNORMAL
	if tn.Attrib&(1<<1) != 0 {
		si.ShowWindow = windows.SW_MINIMIZE
	}
	if tn.Attrib&(1<<2) != 0 {
		si.ShowWindow = windows.SW_HIDE
	}
	flags := uint32(windows.CREATE_NEW_CONSOLE | windows.CREATE_UNICODE_ENVIRONMENT | windows.NORMAL_PRIORITY_CLASS)
	env := utf16Env(extraEnv)
	var pi windows.ProcessInformation
	err = windows.CreateProcess(exe16, cmd16, nil, nil, true, flags, env, dir16, &si, &pi)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	_, err = windows.WaitForSingleObject(pi.Process, windows.INFINITE)
	return err
}

func utf16Env(extra map[string]string) *uint16 {
	if len(extra) == 0 {
		return nil
	}
	env := os.Environ()
	idx := map[string]int{}
	for i, e := range env {
		if j := strings.IndexByte(e, '='); j > 0 {
			idx[strings.ToUpper(e[:j])] = i
		}
	}
	for k, v := range extra {
		kv := k + "=" + v
		if i, ok := idx[strings.ToUpper(k)]; ok {
			env[i] = kv
		} else {
			env = append(env, kv)
		}
	}
	var buf []uint16
	for _, e := range env {
		u, err := syscall.UTF16FromString(e)
		if err != nil {
			continue
		}
		buf = append(buf, u...)
	}
	buf = append(buf, 0)
	return &buf[0]
}

func windowsCmdLine(exe, rest string) string {
	quoted := exe
	if strings.ContainsAny(exe, " \t") && !strings.HasPrefix(exe, `"`) {
		quoted = `"` + exe + `"`
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return quoted
	}
	return quoted + " " + rest
}
