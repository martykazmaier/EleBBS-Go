//go:build !windows

package files

import "path/filepath"

func dosShortName(path string) string {
	return filepath.Base(path)
}
