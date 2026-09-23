package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
)

type mimeFile struct {
	Name string
	Type string
	Data []byte
}

func decodeTransfer(enc string, data []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "base64":
		b, err := base64.StdEncoding.DecodeString(stripWS(string(data)))
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(stripWS(string(data)))
			if err != nil {
				return data
			}
		}
		return b
	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(data))
		b, err := io.ReadAll(r)
		if err != nil {
			return data
		}
		return b
	default:
		return data
	}
}

func stripWS(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func splitHeadersBody(raw []byte) (hdr, body []byte) {
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	raw = bytes.ReplaceAll(raw, []byte("\r"), []byte("\n"))
	if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		return raw[:i], raw[i+2:]
	}
	return raw, nil
}

func headersCRLF(h []byte) []byte {
	h = bytes.ReplaceAll(h, []byte("\r\n"), []byte("\n"))
	h = bytes.ReplaceAll(h, []byte("\r"), []byte("\n"))
	return bytes.ReplaceAll(h, []byte("\n"), []byte("\r\n"))
}

func parseMIME(raw []byte) (out []byte, files []mimeFile) {
	msg, err := mail.ReadMessage(bytes.NewReader(normalizeCRLF(raw)))
	if err != nil {
		return raw, nil
	}
	ct := msg.Header.Get("Content-Type")
	media, params, err := mime.ParseMediaType(ct)
	if err != nil || media == "" {
		return raw, nil
	}
	body, _ := io.ReadAll(msg.Body)
	enc := msg.Header.Get("Content-Transfer-Encoding")
	if strings.HasPrefix(strings.ToLower(media), "multipart/") {
		text, files := walkMultipart(body, params["boundary"])
		hdr, _ := splitHeadersBody(raw)
		out = append(headersCRLF(hdr), '\r', '\n', '\r', '\n')
		out = append(out, []byte(strings.ReplaceAll(text, "\n", "\r\n"))...)
		return out, files
	}
	fn := partFileName(msg.Header.Get("Content-Disposition"), params)
	if isBinaryAttachPart(media, fn) {
		decoded := decodeTransfer(enc, body)
		files = append(files, mimeFile{
			Name: sniffAttachName(media, fn, decoded),
			Type: media,
			Data: decoded,
		})
		hdr, _ := splitHeadersBody(raw)
		out = append(headersCRLF(hdr), '\r', '\n', '\r', '\n')
		return out, files
	}
	return raw, nil
}

func walkMultipart(body []byte, boundary string) (text string, files []mimeFile) {
	if boundary == "" {
		return string(body), nil
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var texts []string
	plain := ""
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		data, _ := io.ReadAll(p)
		_ = p.Close()
		media, params, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		decoded := decodeTransfer(p.Header.Get("Content-Transfer-Encoding"), data)
		fn := p.FileName()
		if fn == "" {
			fn = params["name"]
		}
		if fn == "" {
			fn = partFileName(p.Header.Get("Content-Disposition"), params)
		}
		if strings.HasPrefix(strings.ToLower(media), "multipart/") {
			innerText, innerFiles := walkMultipart(decoded, params["boundary"])
			if innerText != "" {
				texts = append(texts, innerText)
			}
			files = append(files, innerFiles...)
			continue
		}
		if isBinaryAttachPart(media, fn) {
			files = append(files, mimeFile{Name: sniffAttachName(media, fn, decoded), Type: media, Data: decoded})
			continue
		}
		low := strings.ToLower(media)
		if strings.HasPrefix(low, "text/plain") || low == "" {
			if plain == "" {
				plain = string(decoded)
			}
			continue
		}
		if strings.HasPrefix(low, "text/") {
			texts = append(texts, string(decoded))
		}
	}
	if plain != "" {
		return plain, files
	}
	return strings.Join(texts, "\r\n"), files
}

func partFileName(disp string, params map[string]string) string {
	if disp != "" {
		_, dparams, err := mime.ParseMediaType(disp)
		if err == nil {
			if n := dparams["filename"]; n != "" {
				return n
			}
		}
	}
	if params != nil {
		if n := params["name"]; n != "" {
			return n
		}
	}
	return ""
}

func isBinaryAttachPart(media, fn string) bool {
	if strings.TrimSpace(fn) != "" {
		return true
	}
	low := strings.ToLower(strings.TrimSpace(media))
	if low == "" || strings.HasPrefix(low, "text/plain") || strings.HasPrefix(low, "text/html") || strings.HasPrefix(low, "multipart/") {
		return false
	}
	return strings.HasPrefix(low, "image/") ||
		strings.HasPrefix(low, "audio/") ||
		strings.HasPrefix(low, "video/") ||
		strings.HasPrefix(low, "application/") ||
		strings.HasPrefix(low, "model/")
}

func sniffAttachName(media, fn string, data []byte) string {
	name := defaultAttachName(media, fn)
	if !genericAttachName(name) {
		return name
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "attach.png"
	}
	if len(data) >= 3 && string(data[:3]) == "GIF" {
		return "attach.gif"
	}
	if len(data) >= 3 && string(data[:3]) == "\xff\xd8\xff" {
		return "attach.jpg"
	}
	return name
}

func genericAttachName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || strings.HasPrefix(n, "attach.") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(n))
	if ext == ".tmp" || ext == ".bin" || ext == ".dat" {
		return true
	}
	base := strings.TrimSuffix(n, ext)
	return strings.HasPrefix(base, "att") && len(base) <= 8
}

func defaultAttachName(media, fn string) string {
	if strings.TrimSpace(fn) != "" {
		return safeAttachName(fn)
	}
	ext := "bin"
	low := strings.ToLower(media)
	if i := strings.Index(low, "/"); i >= 0 {
		sub := low[i+1:]
		if j := strings.IndexAny(sub, ";+"); j >= 0 {
			sub = sub[:j]
		}
		switch sub {
		case "jpeg":
			sub = "jpg"
		case "svg+xml":
			sub = "svg"
		case "octet-stream":
			sub = "bin"
		}
		if sub != "" {
			ok := true
			for _, c := range sub {
				if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
					continue
				}
				ok = false
				break
			}
			if ok {
				ext = sub
			}
		}
	}
	return "attach." + ext
}

func safeAttachName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "attach.bin"
	}
	return name
}

func dos83Name(name string) string {
	name = safeAttachName(name)
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = name[i+1:]
		name = name[:i]
	}
	name = dos83Part(name, 8)
	ext = dos83Part(ext, 3)
	if name == "" {
		name = "ATTACH"
	}
	if ext == "" {
		return name
	}
	return name + "." + ext
}

func dos83Part(s string, n int) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(s) {
		if b.Len() >= n {
			break
		}
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '!' || c == '#' || c == '$' || c == '%' || c == '&' || c == '-' || c == '_' || c == '@' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func normalizeCRLF(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte("\r"), []byte("\n"))
	return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
}

func mimeAttachFiles(rfc822 []byte, paths []string) []byte {
	if len(paths) == 0 {
		return rfc822
	}
	hdr, body := splitHeadersBody(rfc822)
	var keep []string
	subj := ""
	for _, ln := range strings.Split(string(hdr), "\n") {
		switch {
		case hasPrefixFold(ln, "Content-Type:"):
			continue
		case hasPrefixFold(ln, "MIME-Version:"):
			continue
		case hasPrefixFold(ln, "Content-Transfer-Encoding:"):
			continue
		case hasPrefixFold(ln, "Subject:"):
			subj = strings.TrimSpace(ln[8:])
			continue
		default:
			if ln != "" {
				keep = append(keep, ln)
			}
		}
	}
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	if isAttachDirSubject(subj) || subj == "" {
		subj = "File attach"
		if len(names) == 1 {
			subj = names[0]
		}
	}
	bound := fmt.Sprintf("EleBBS-%d", time.Now().UnixNano())
	var b strings.Builder
	for _, ln := range keep {
		b.WriteString(ln)
		b.WriteString("\r\n")
	}
	b.WriteString("Subject: " + subj + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + bound + "\"\r\n\r\n")
	b.WriteString("--" + bound + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	text := strings.ReplaceAll(string(body), "\n", "\r\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\r\n") {
		b.WriteString("\r\n")
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		name := safeAttachName(filepath.Base(p))
		b.WriteString("--" + bound + "\r\n")
		b.WriteString("Content-Type: application/octet-stream; name=\"" + name + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString(data)
		for i := 0; i < len(enc); i += 76 {
			end := i + 76
			if end > len(enc) {
				end = len(enc)
			}
			b.WriteString(enc[i:end])
			b.WriteString("\r\n")
		}
	}
	b.WriteString("--" + bound + "--\r\n")
	return []byte(b.String())
}

func isAttachDirSubject(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, `\/`)
	if s == "" {
		return false
	}
	st, err := os.Stat(s)
	return err == nil && st.IsDir()
}

func canonDir(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	return strings.TrimRight(abs, `\/`)
}

func sameDir(a, b string) bool {
	a, b = canonDir(a), canonDir(b)
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}

func isDriveRoot(p string) bool {
	p = canonDir(p)
	if p == "" {
		return false
	}
	vol := filepath.VolumeName(p)
	rest := strings.Trim(strings.TrimPrefix(p, vol), `\/`)
	return rest == ""
}

func isDirectChild(parent, child string) bool {
	parent, child = canonDir(parent), canonDir(child)
	if parent == "" || child == "" || sameDir(parent, child) {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") && !strings.Contains(rel, "/")
}

func looksLikeAttachTempDir(name string) bool {
	n := strings.ToUpper(strings.TrimSpace(name))
	if len(n) != 8 || n[0] != 'A' || n[1] != 'T' {
		return false
	}
	for i := 2; i < 8; i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	return true
}

func isSystemDir(g *cfgrec.GlobalCfg, dir string) bool {
	if g == nil {
		return false
	}
	dir = canonDir(dir)
	if dir == "" {
		return false
	}
	if isDriveRoot(dir) {
		return true
	}
	for _, p := range []string{
		g.RaConfig.SysPath,
		g.RaConfig.SemPath,
		g.RaConfig.MsgBasePath,
		g.RaConfig.TextPath,
		g.RaConfig.MenuPath,
		g.RaConfig.FileBase,
	} {
		if sameDir(dir, p) {
			return true
		}
	}
	return false
}

// isSafeAttachDir is Pascal readmsg's AttachPath sanity check: the folder
// must be a unique CreateTempDir child, never SysPath / attach root / drive
// root (those hold autoexec.bat, config.sys, emailout.ele.tmp leftovers).
func isSafeAttachDir(g *cfgrec.GlobalCfg, subj string) bool {
	dir := canonDir(subj)
	if dir == "" {
		return false
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	if !looksLikeAttachTempDir(filepath.Base(dir)) {
		return false
	}
	if isSystemDir(g, dir) || sameDir(dir, attachRoot(g)) {
		return false
	}
	if isDirectChild(attachRoot(g), dir) {
		return true
	}
	if g != nil && isDirectChild(g.RaConfig.SysPath, dir) {
		return true
	}
	return false
}
