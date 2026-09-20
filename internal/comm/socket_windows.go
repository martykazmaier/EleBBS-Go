//go:build windows

package comm

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	solSocket      = 0xFFFF
	soRcvTimeo     = 0x1006
	soSndTimeo     = 0x1005
	ipprotoTCP     = 6
	tcpNoDelay     = 1
	fionbio        = 0x8004667E
	invalidSocket  = ^uintptr(0)
	wsaWouldBlock  = 10035
	wsaTimedOut    = 10060
	blockTimeoutMS = 86400000
)

var (
	ws2_32          = windows.NewLazySystemDLL("ws2_32.dll")
	procIoctlsocket = ws2_32.NewProc("ioctlsocket")
	procRecv        = ws2_32.NewProc("recv")
	procSend        = ws2_32.NewProc("send")
	procClosesocket = ws2_32.NewProc("closesocket")
)

func initWinsock() error {
	var d windows.WSAData
	return windows.WSAStartup(uint32(0x0202), &d)
}

func FromDoorHandle(handle uintptr) (Stream, error) {
	if handle == 0 || handle == invalidSocket {
		return nil, fmt.Errorf("invalid socket handle")
	}
	if err := initWinsock(); err != nil {
		return nil, fmt.Errorf("WSAStartup: %w", err)
	}
	h := windows.Handle(handle)
	skipIOCP(h)
	_ = windows.SetsockoptInt(h, ipprotoTCP, tcpNoDelay, 1)
	var nb uint32
	_, _, _ = procIoctlsocket.Call(uintptr(h), fionbio, uintptr(unsafe.Pointer(&nb)))
	_ = windows.SetsockoptInt(h, solSocket, soRcvTimeo, blockTimeoutMS)
	_ = windows.SetsockoptInt(h, solSocket, soSndTimeo, blockTimeoutMS)
	_ = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
	s := &doorSock{h: h, own: false}
	s.local = sockAddr(windows.Getsockname, h)
	s.remote = sockAddr(windows.Getpeername, h)
	return s, nil
}

type doorSock struct {
	h        windows.Handle
	own      bool
	mu       sync.Mutex
	rdl, wdl time.Time
	local    net.Addr
	remote   net.Addr
}

func (s *doorSock) Local() bool { return false }

func (s *doorSock) SocketHandle() uintptr { return uintptr(s.h) }

func (s *doorSock) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	deadline := s.rdl
	s.mu.Unlock()
	if !deadline.IsZero() {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		ms := int(left.Milliseconds())
		if ms < 1 {
			ms = 1
		}
		_ = windows.SetsockoptInt(s.h, solSocket, soRcvTimeo, ms)
		defer windows.SetsockoptInt(s.h, solSocket, soRcvTimeo, blockTimeoutMS)
	}
	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		n, _, err := procRecv.Call(uintptr(s.h), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), 0)
		if n == ^uintptr(0) {
			errno := syscall.Errno(0)
			if err != nil {
				errno = err.(syscall.Errno)
			}
			if errno == syscall.Errno(wsaWouldBlock) {
				wait := 10 * time.Millisecond
				if !deadline.IsZero() {
					left := time.Until(deadline)
					if left <= 0 {
						return 0, os.ErrDeadlineExceeded
					}
					if left < wait {
						wait = left
					}
				}
				time.Sleep(wait)
				continue
			}
			if errno == syscall.Errno(wsaTimedOut) {
				return 0, os.ErrDeadlineExceeded
			}
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return int(n), nil
	}
}

func (s *doorSock) Write(p []byte) (int, error) {
	sent := 0
	for sent < len(p) {
		n, _, err := procSend.Call(uintptr(s.h), uintptr(unsafe.Pointer(&p[sent])), uintptr(len(p)-sent), 0)
		if n == ^uintptr(0) {
			errno := syscall.Errno(0)
			if err != nil {
				errno = err.(syscall.Errno)
			}
			if errno == syscall.Errno(wsaWouldBlock) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return sent, err
		}
		sent += int(n)
	}
	return sent, nil
}

func (s *doorSock) TakeOwnership() { s.own = true }

func (s *doorSock) Close() error {
	if !s.own {
		return nil
	}
	_, _, err := procClosesocket.Call(uintptr(s.h))
	if err != syscall.Errno(0) {
		return err
	}
	return nil
}

func (s *doorSock) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	s.rdl = t
	s.mu.Unlock()
	return nil
}
func (s *doorSock) SetWriteDeadline(t time.Time) error {
	s.mu.Lock()
	s.wdl = t
	s.mu.Unlock()
	return nil
}

func sockAddr(fn func(windows.Handle) (windows.Sockaddr, error), h windows.Handle) net.Addr {
	sa, err := fn(h)
	if err != nil {
		return nil
	}
	switch a := sa.(type) {
	case *windows.SockaddrInet4:
		return &net.TCPAddr{IP: net.IP(a.Addr[:]), Port: a.Port}
	case *windows.SockaddrInet6:
		return &net.TCPAddr{IP: net.IP(a.Addr[:]), Port: a.Port}
	default:
		return nil
	}
}
