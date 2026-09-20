//go:build !windows

package comm

import "fmt"

func FromDoorHandle(handle uintptr) (Stream, error) {
	return nil, fmt.Errorf("inherited WinSock handles require Windows")
}
