//go:build windows

package comm

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// LocalSocketPair returns a loopback TCP pair. bbs is used by EleBBS; peer is
// relayed to the original session (SSH). handle is an inheritable socket for doors.
func LocalSocketPair() (bbs Stream, peer net.Conn, handle uintptr, file *os.File, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, 0, nil, err
	}
	type acc struct {
		c   net.Conn
		err error
	}
	ch := make(chan acc, 1)
	go func() {
		_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(3 * time.Second))
		c, e := ln.Accept()
		ch <- acc{c, e}
		_ = ln.Close()
	}()
	dial, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
	if err != nil {
		_ = ln.Close()
		return nil, nil, 0, nil, err
	}
	a := <-ch
	if a.err != nil {
		_ = dial.Close()
		return nil, nil, 0, nil, a.err
	}
	tcp, ok := dial.(*net.TCPConn)
	if !ok {
		_ = dial.Close()
		_ = a.c.Close()
		return nil, nil, 0, nil, fmt.Errorf("not TCP")
	}
	f, err := tcp.File()
	if err != nil {
		_ = tcp.Close()
		_ = a.c.Close()
		return nil, nil, 0, nil, err
	}
	// File() is a duplicate; close the net.Conn so the poller does not race
	// with doorSock / an inherited door on the same socket.
	_ = tcp.Close()
	h := DupForDoor(uintptr(f.Fd()))
	if h == 0 {
		h = uintptr(f.Fd())
		_ = windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
	}
	st, err := FromDoorHandle(h)
	if err != nil {
		_ = f.Close()
		_ = a.c.Close()
		return nil, nil, 0, nil, err
	}
	return st, a.c, uintptr(h), f, nil
}
