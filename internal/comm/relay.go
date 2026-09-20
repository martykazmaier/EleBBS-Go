package comm

import (
	"io"
	"sync"
)

// Relay copies both directions until either side errors, then closes both.
func Relay(a, b io.ReadWriteCloser) {
	if a == nil || b == nil {
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	copy := func(dst io.WriteCloser, src io.Reader) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		_ = dst.Close()
	}
	go copy(a, b)
	go copy(b, a)
	wg.Wait()
}
