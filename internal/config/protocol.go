package config

import (
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func protocolDirs(g *cfgrec.GlobalCfg) []string {
	var dirs []string
	if g != nil {
		dirs = append(dirs, g.RaConfig.SysPath, g.RaConfig.MsgBasePath)
	}
	dirs = append(dirs, ".")
	return dirs
}

func loadProtocolRecords(g *cfgrec.GlobalCfg) []cfgrec.Protocol {
	var raw []byte
	for _, dir := range protocolDirs(g) {
		root := strings.TrimRight(dir, `\/`)
		if root == "" {
			continue
		}
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "protocol.ra"))
		if err != nil {
			raw, err = os.ReadFile(filepath.Join(root, "PROTOCOL.RA"))
		}
		if err == nil && len(raw) > 0 {
			break
		}
		raw = nil
	}
	if len(raw) == 0 {
		return nil
	}
	sz := cfgrec.ProtocolRecSize(len(raw))
	n := len(raw) / sz
	out := make([]cfgrec.Protocol, 0, n)
	for i := 0; i < n; i++ {
		off := i * sz
		if off+20 > len(raw) {
			break
		}
		p := cfgrec.ParseProtocol(raw[off:])
		if p.Name == "" && p.ActiveKey == 0 {
			continue
		}
		out = append(out, p)
	}
	return out
}

// LoadProtocols returns PROTOCOL.RA entries that SelectProtocol would list.
func LoadProtocols(g *cfgrec.GlobalCfg) []cfgrec.Protocol {
	var out []cfgrec.Protocol
	for _, p := range loadProtocolRecords(g) {
		if p.Name == "" || p.ActiveKey == 0 {
			continue
		}
		if p.Attribute != 1 && p.Attribute != 2 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func internProtocolName(key byte) string {
	if key >= 'a' && key <= 'z' {
		key -= 32
	}
	switch key {
	case 'X':
		return "Xmodem"
	case '1':
		return "Xmodem/1K"
	case 'Q':
		return "Xmodem/1K-G"
	case 'Y':
		return "Ymodem"
	case 'G':
		return "Ymodem-G"
	case 'Z':
		return "Zmodem"
	}
	return ""
}

// FindProtocol returns the PROTOCOL.RA record for key (external protocols).
// Internal X/Y/Z keys are not synthesized here; download uses DnCmdString.
func FindProtocol(g *cfgrec.GlobalCfg, key byte) cfgrec.Protocol {
	if key == 0 || key == ' ' {
		return cfgrec.Protocol{}
	}
	want := pascal.UpCase(string([]byte{key}))
	for _, p := range loadProtocolRecords(g) {
		if p.ActiveKey == 0 {
			continue
		}
		if pascal.UpCase(string([]byte{p.ActiveKey})) == want {
			return p
		}
	}
	return cfgrec.Protocol{}
}

// ProtocolName is RA user code ! : Name of the user's default protocol
// (Pascal GetProtocolRecord / usrcodes '!').
func ProtocolName(g *cfgrec.GlobalCfg, key byte) string {
	if key == 0 || key == ' ' {
		return ""
	}
	if n := internProtocolName(key); n != "" {
		return n
	}
	want := pascal.UpCase(string([]byte{key}))
	for _, p := range loadProtocolRecords(g) {
		if p.ActiveKey == 0 {
			continue
		}
		if pascal.UpCase(string([]byte{p.ActiveKey})) == want {
			return p.Name
		}
	}
	return ""
}
