package lang

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

type File struct {
	data []byte
	off  []uint16
	n    int
}

func Load(g *cfgrec.GlobalCfg, rec cfgrec.Language) *File {
	names := ralFileNames(rec.DefName)
	dirs := []string{
		rec.TextPath, rec.MenuPath, rec.QuesPath,
	}
	if g != nil {
		dirs = append(dirs, g.RaConfig.SysPath, g.RaConfig.TextPath, g.RaConfig.MenuPath, g.RaConfig.MsgBasePath)
	}
	dirs = append(dirs, ".")
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		root := strings.TrimRight(d, `\/`)
		for _, n := range names {
			if n == "" {
				continue
			}
			for _, p := range ralCandidates(root, n) {
				b, err := os.ReadFile(p)
				if err != nil || len(b) < 4 {
					continue
				}
				if f := parse(b); f != nil {
					return f
				}
			}
		}
	}
	return &File{}
}

func ralFileNames(def string) []string {
	def = strings.TrimSpace(def)
	if def == "" {
		return []string{"english.ral", "ENGLISH.RAL", "English.ral"}
	}
	var names []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, e := range names {
			if strings.EqualFold(e, s) {
				return
			}
		}
		names = append(names, s)
	}
	add(def)
	base := filepath.Base(def)
	add(base)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext == "" {
		add(stem + ".ral")
		add(stem + ".RAL")
		add(strings.ToUpper(stem) + ".RAL")
		add(strings.ToLower(stem) + ".ral")
	} else {
		add(strings.ToUpper(base))
		add(strings.ToLower(base))
		add(stem + ".ral")
		add(stem + ".RAL")
	}
	add("english.ral")
	add("ENGLISH.RAL")
	return names
}

// ralCandidates expands RA-style paths (\ELE\ENGLISH.ral) and joins relative names.
func ralCandidates(root, name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, e := range out {
			if strings.EqualFold(e, p) {
				return
			}
		}
		out = append(out, p)
	}
	if filepath.IsAbs(name) {
		add(name)
		return out
	}
	if strings.HasPrefix(name, `\`) || strings.HasPrefix(name, `/`) {
		if cwd, err := os.Getwd(); err == nil {
			if vol := filepath.VolumeName(cwd); vol != "" {
				add(vol + name)
			}
		}
		add(name)
	}
	if root != "" {
		add(filepath.Join(root, filepath.Base(name)))
		if !strings.HasPrefix(name, `\`) && !strings.HasPrefix(name, `/`) {
			add(filepath.Join(root, name))
		}
	}
	add(name)
	return out
}

func parse(b []byte) *File {
	count := int(binary.LittleEndian.Uint16(b[:2]))
	if count <= 0 || count > 2000 {
		return nil
	}
	need := 2 * (count + 1)
	if len(b) < need {
		return nil
	}
	off := make([]uint16, count+1)
	for i := 0; i <= count && (i*2+2) <= len(b); i++ {
		off[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
	}
	return &File{data: b, off: off, n: count}
}

func decodeStart(o uint16) int {
	hi := o >> 8
	if hi == 0 {
		return int(o) - 255
	}
	return int(o) - int(hi-1)
}

func decodeEnd(o uint16) int {
	return int(o) - int(o>>8)
}

func toHex(b byte) string {
	const hexd = "0123456789ABCDEF"
	return string([]byte{hexd[b>>4], hexd[b&0x0F]})
}

func (f *File) Entries() int {
	if f == nil {
		return 0
	}
	return f.n
}

func (f *File) raw(nr int, withColor bool) (s string, color byte, hasKey bool) {
	if f == nil || f.n == 0 || nr <= 0 || nr > f.n || nr >= len(f.off) {
		return "", 0, false
	}
	start := decodeStart(f.off[nr])
	end := len(f.data)
	if nr < f.n && nr+1 < len(f.off) {
		end = decodeEnd(f.off[nr+1])
	}
	if start < 0 {
		start = 0
	}
	if start >= len(f.data) {
		return "", 0, false
	}
	if end <= start || end > len(f.data) {
		end = len(f.data)
	}
	if start > 0 {
		c := f.data[start-1]
		if c != 0 && c != 255 {
			color = c
		}
	}
	var b []byte
	if withColor && color != 0 {
		b = append(b, 0x0B, '[')
		b = append(b, toHex(color)...)
	}
	for i := start; i < end; i++ {
		c := f.data[i]
		if c == 255 {
			hasKey = true
			break
		}
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return pascal.FromCP437(b), color, hasKey
}

// Get is Pascal RalGet: optional ^K[color plus the raw prompt (keys included).
// A loaded .RAL file is the source of truth — empty entries stay empty and are
// never replaced with the built-in English defaults.
func (f *File) Get(nr int) string {
	if f == nil || f.n == 0 {
		return defaults[nr]
	}
	s, _, _ := f.raw(nr, true)
	return s
}

// GetStr is Pascal RalGetStr: color prefix plus prompt text with keys stripped.
func (f *File) GetStr(nr int) string {
	if f == nil || f.n == 0 {
		return defaults[nr]
	}
	s, _, hasKey := f.raw(nr, true)
	if !hasKey {
		return s
	}
	col := ""
	body := s
	if len(body) >= 4 && body[0] == 0x0B && body[1] == '[' {
		col = body[:4]
		body = body[4:]
	}
	if i := strings.IndexByte(body, ' '); i >= 0 {
		body = body[i+1:]
	}
	return col + body
}

// GetKeys is Pascal RalGetKeys (no color): letters before the first space.
func (f *File) GetKeys(nr int) string {
	if f == nil || f.n == 0 {
		return keysFromDefault(nr)
	}
	s, _, hasKey := f.raw(nr, false)
	if !hasKey || s == "" {
		return keysFromDefault(nr)
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func (f *File) GetKey(nr int) byte {
	k := f.GetKeys(nr)
	if k == "" {
		return 0
	}
	return pascal.UpCase(k)[0]
}

func (f *File) DefaultKey(nr int) byte {
	k := f.GetKeys(nr)
	if k == "" {
		return 0
	}
	return k[len(k)-1]
}

func (f *File) GetDsp(nr int, left, right string) string {
	if left == "" {
		left = "["
	}
	if right == "" {
		right = "]"
	}
	k := f.GetKey(nr)
	if k == 0 {
		return f.GetStr(nr)
	}
	return left + string(k) + right + " " + f.GetStr(nr)
}

func (f *File) YesNo(yes bool) string {
	if yes {
		return f.GetStr(Yes)
	}
	return f.GetStr(No)
}

func (f *File) OnOff(on bool) string {
	if on {
		return f.GetStr(ON1)
	}
	return f.GetStr(Off)
}

func keysFromDefault(nr int) string {
	s := defaults[nr]
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, ' '); i > 0 && i <= 8 {
		k := s[:i]
		ok := true
		for j := 0; j < len(k); j++ {
			c := k[j]
			if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '=' {
				ok = false
				break
			}
		}
		if ok {
			return k
		}
	}
	return ""
}

func Default(nr int) string { return defaults[nr] }
