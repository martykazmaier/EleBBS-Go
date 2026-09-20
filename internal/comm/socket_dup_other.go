//go:build !windows

package comm

import (
	"fmt"
	"net"
)

func DupForDoor(h uintptr) uintptr { return 0 }

func DupHandle(h uintptr) uintptr { return 0 }

func CloseSocket(uintptr) {}

func SkipIOCP(uintptr) {}

func StealTCP(conn net.Conn) (uintptr, func(), error) {
	_ = conn
	return 0, func() {}, fmt.Errorf("socket inherit requires Windows")
}
