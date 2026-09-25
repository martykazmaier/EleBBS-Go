//go:build windows

package comm

import (
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestLocalConsoleHandlesSurviveGC(t *testing.T) {
	keys, _ := OpenLocalKeys().(*conKeys)
	win, _ := OpenLocalWindow().(*conWindow)
	if keys == nil || win == nil {
		t.Skip("no console")
	}
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	var mode uint32
	if err := windows.GetConsoleMode(keys.h, &mode); err != nil {
		t.Fatalf("node keyboard handle closed after GC: %v", err)
	}
	if err := windows.GetConsoleMode(win.h, &mode); err != nil {
		t.Fatalf("node window handle closed after GC: %v", err)
	}
}
