//go:build windows

package files

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func dosShortName(path string) string {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return filepath.Base(path)
	}
	n, err := windows.GetShortPathName(p, nil, 0)
	if err != nil || n == 0 {
		return filepath.Base(path)
	}
	buf := make([]uint16, n)
	n, err = windows.GetShortPathName(p, &buf[0], n)
	if err != nil || n == 0 {
		return filepath.Base(path)
	}
	return windows.UTF16ToString(buf[:n])
}
