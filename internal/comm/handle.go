package comm

// SocketOf walks stream wrappers and returns a native Winsock handle, or 0.
func SocketOf(s Stream) uintptr {
	for s != nil {
		if h, ok := s.(interface{ SocketHandle() uintptr }); ok {
			if v := h.SocketHandle(); v != 0 && v != ^uintptr(0) {
				return v
			}
		}
		u, ok := s.(interface{ Unwrap() Stream })
		if !ok {
			break
		}
		next := u.Unwrap()
		if next == s {
			break
		}
		s = next
	}
	return 0
}
