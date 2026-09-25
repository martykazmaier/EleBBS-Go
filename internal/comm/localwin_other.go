//go:build !windows

package comm

// OpenLocalWindow has no node window outside Windows.
func OpenLocalWindow() LocalWindow { return nil }
