package comm

// LocalWindow is Pascal FastScrn on the node's console: sysop-only windows
// drawn straight into the screen buffer. Coordinates are 1-based within the
// visible window; text is CP437.
type LocalWindow interface {
	WriteAt(x, y int, attr byte, s []byte)
	GotoXY(x, y int)
	// Save is Pascal SaveScreen; calling the result is RestoreScreen.
	Save() (restore func())
}
