package mail

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

func msgLines(body []byte) []string {
	s := strings.ReplaceAll(string(nulTrim(body)), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func nulTrim(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return b
}

func firstField(field, line string) (string, bool) {
	if len(line) < len(field) {
		return "", false
	}
	if !strings.EqualFold(line[:len(field)], field) {
		return "", false
	}
	return strings.TrimSpace(line[len(field):]), true
}

// GetFieldLine is Pascal GetFieldLine: first header whose name matches.
func GetFieldLine(field string, body []byte) string {
	for _, line := range msgLines(body) {
		if line == "" {
			return ""
		}
		if _, ok := firstField(field, line); ok {
			return line
		}
	}
	return ""
}

func headerValue(field string, body []byte) string {
	line := GetFieldLine(field, body)
	if line == "" {
		return ""
	}
	if v, ok := firstField(field, line); ok {
		return v
	}
	return ""
}

// GetRawEmail is Pascal GetRawEmail: address inside <>, else the whole string.
func GetRawEmail(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '<')
	j := strings.LastIndexByte(s, '>')
	if i >= 0 && j > i {
		return strings.TrimSpace(s[i+1 : j])
	}
	return s
}

// ExtractToName is Pascal ExtractToName.
// mailAddress true: local-part of the address, dots become spaces.
// mailAddress false: display name (text before <), quotes stripped.
func ExtractToName(body []byte, mailAddress bool, field string) string {
	toWho := headerValue(field, body)
	if mailAddress {
		addr := GetRawEmail(toWho)
		if i := strings.IndexByte(addr, '@'); i >= 0 {
			addr = addr[:i]
		}
		return strings.ReplaceAll(addr, ".", " ")
	}
	if i := strings.IndexByte(toWho, '<'); i >= 0 {
		toWho = toWho[:i]
	}
	toWho = strings.TrimSpace(toWho)
	toWho = strings.Trim(toWho, `"`)
	return strings.TrimSpace(toWho)
}

func MakeHostEmail(host, from string, addr cfgrec.Addr) string {
	from = pascal.Trim(from)
	if strings.Contains(from, "@") {
		return from
	}
	local := strings.ToLower(strings.ReplaceAll(from, " ", "."))
	if host != "" {
		return local + "@" + host
	}
	return fmt.Sprintf("%s@p%d.f%d.n%d.z%d.fidonet.org", local, addr.Point, addr.Node, addr.Net, addr.Zone)
}

func MakeNntpDate(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format(time.RFC1123Z)
}

func parseNntpDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now()
	}
	for _, layout := range []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
		"02 Jan 2006 15:04:05 MST",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func jamDateStr(t time.Time) (date, clock string) {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format("01-02-06"), t.Format("15:04:05")
}

func prependLines(body []byte, lines []string) []byte {
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteString("\r\n")
	}
	b.Write(nulTrim(body))
	return []byte(b.String())
}

func appendDisclaimer(body []byte, path string) []byte {
	if path == "" {
		return body
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return body
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	out := append([]byte{}, nulTrim(body)...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\r', '\n')
	}
	out = append(out, '\r', '\n')
	for _, ln := range strings.Split(text, "\n") {
		out = append(out, []byte(ln)...)
		out = append(out, '\r', '\n')
	}
	return out
}

func splitUserPass(s string) (user, pass string) {
	i := strings.LastIndexByte(s, '@')
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func atoiMail(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
