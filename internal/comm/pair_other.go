//go:build !windows

package comm

import (
	"fmt"
	"net"
	"os"
)

func LocalSocketPair() (Stream, net.Conn, uintptr, *os.File, error) {
	return nil, nil, 0, nil, fmt.Errorf("socket pairs require Windows")
}
