//go:build !windows

package door

import "elebbs/internal/cfgrec"

func AttachSessionConsole(*cfgrec.LineCfg) {}
