//go:build windows

package comm

import (
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

const (
	fileSkipCompletionPortOnSuccess = 1
	fileSkipSetEventOnHandle        = 2
)

func skipIOCP(h windows.Handle) {
	if h == 0 || h == windows.InvalidHandle {
		return
	}
	_ = windows.SetFileCompletionNotificationModes(h, fileSkipCompletionPortOnSuccess|fileSkipSetEventOnHandle)
}

// SkipIOCP detaches a SOCKET from Go's completion port so NetCom can use it.
func SkipIOCP(h uintptr) {
	if h == 0 || h == ^uintptr(0) {
		return
	}
	skipIOCP(windows.Handle(h))
}

func dupHandleInherit(h uintptr) uintptr {
	if h == 0 || h == ^uintptr(0) {
		return 0
	}
	proc := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, windows.Handle(h), proc, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0
	}
	skipIOCP(dup)
	_ = windows.SetHandleInformation(dup, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
	return uintptr(dup)
}

// DupHandle is Pascal GetDuplHandle: inheritable DuplicateHandle of a SOCKET
// for *W/*Y (-H on the command line). The door uses Winsock, not stdin/stdout.
func DupHandle(h uintptr) uintptr {
	return dupHandleInherit(h)
}

// DupForDoor returns an inheritable SOCKET sharing h's connection. The new
// handle is created without WSA_FLAG_OVERLAPPED so NetCom/ntvdm can use it.
func DupForDoor(h uintptr) uintptr {
	if h == 0 || h == ^uintptr(0) {
		return 0
	}
	var info windows.WSAProtocolInfo
	if err := windows.WSADuplicateSocket(windows.Handle(h), uint32(windows.GetCurrentProcessId()), &info); err != nil {
		return dupHandleInherit(h)
	}
	nt, err := windows.WSASocket(info.AddressFamily, info.SocketType, info.Protocol, &info, 0, 0)
	if err != nil {
		nt, err = windows.WSASocket(info.AddressFamily, info.SocketType, info.Protocol, &info, 0, windows.WSA_FLAG_OVERLAPPED)
		if err != nil {
			return dupHandleInherit(h)
		}
	}
	skipIOCP(nt)
	_ = windows.SetHandleInformation(nt, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
	return uintptr(nt)
}

// CloseSocket releases a duplicated SOCKET with closesocket. CloseHandle
// leaves Winsock (and NetFoss) thinking the node is still in use.
func CloseSocket(h uintptr) {
	if h == 0 || h == ^uintptr(0) {
		return
	}
	_ = windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, 0)
	if err := windows.Closesocket(windows.Handle(h)); err != nil {
		_ = windows.CloseHandle(windows.Handle(h))
	}
}

// StealTCP detaches conn from Go's IOCP poller and returns an inheritable
// non-overlapped SOCKET for EleBBS / doors. Call cleanup after the child exits.
func StealTCP(conn net.Conn) (handle uintptr, cleanup func(), err error) {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return 0, nil, fmt.Errorf("not a TCP connection")
	}
	f, err := tcp.File()
	if err != nil {
		return 0, nil, fmt.Errorf("socket file: %w", err)
	}
	_ = conn.Close()
	src := uintptr(f.Fd())
	skipIOCP(windows.Handle(src))
	dup := DupForDoor(src)
	_ = f.Close()
	if dup == 0 {
		return 0, nil, fmt.Errorf("duplicate socket for inherit")
	}
	return dup, func() { CloseSocket(dup) }, nil
}
