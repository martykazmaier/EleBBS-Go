//go:build !windows

package comm

import (
	"fmt"
	"os"
	"time"
)

type consoleStream struct {
	rdl time.Time
}

func OpenConsole() (Stream, error) {
	return &consoleStream{}, nil
}

func (c *consoleStream) Local() bool                       { return true }
func (c *consoleStream) Read(b []byte) (int, error)        { return os.Stdin.Read(b) }
func (c *consoleStream) Write(b []byte) (int, error)       { return os.Stdout.Write(b) }
func (c *consoleStream) Close() error                      { return nil }
func (c *consoleStream) SetReadDeadline(t time.Time) error { c.rdl = t; return nil }
func (c *consoleStream) SetWriteDeadline(time.Time) error  { return nil }

func OpenInherited(handle uintptr) (Stream, error) {
	return nil, fmt.Errorf("inherited WinSock handles require Windows")
}
