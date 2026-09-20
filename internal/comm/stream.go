package comm

import (
	"io"
	"time"
)

type Stream interface {
	io.ReadWriteCloser
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Local() bool
}

type nopDeadlines struct{ io.ReadWriteCloser }

func (n nopDeadlines) SetReadDeadline(time.Time) error  { return nil }
func (n nopDeadlines) SetWriteDeadline(time.Time) error { return nil }
func (n nopDeadlines) Local() bool                      { return false }

func Wrap(rwc io.ReadWriteCloser) Stream { return nopDeadlines{rwc} }
