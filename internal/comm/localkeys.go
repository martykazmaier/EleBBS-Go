package comm

// LocalKey is a key pressed on the node's own console window (Pascal
// Crt.ReadKey on the local keyboard). Ch is the CP437 character; when it is
// zero, Scan is the PC extended scan code (Alt-C = 46, Alt-E = 18, Alt-H = 35,
// cursor keys 72/80/75/77, F1 = 59, Alt-F1 = 104).
type LocalKey struct {
	Ch   byte
	Scan byte
}

// LocalKeys polls the node window's keyboard without blocking.
type LocalKeys interface {
	Poll() (LocalKey, bool)
}
