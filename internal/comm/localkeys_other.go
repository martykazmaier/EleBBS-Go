//go:build !windows

package comm

// OpenLocalKeys has no node window outside Windows.
func OpenLocalKeys() LocalKeys { return nil }
