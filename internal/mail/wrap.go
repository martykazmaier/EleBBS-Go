package mail

import "strings"

// SoftCR is the Fido/JAM soft carriage return (ASCII 141). Pascal GetString
// skips it and word-wraps at MaxLen instead.
const SoftCR = '\x8d'

// WrapLines is Pascal AbsMsgObj.GetString(MaxLen) over a whole buffer:
// hard CR ends a line, soft CR is ignored, long runs wrap on spaces.
func WrapLines(text string, maxLen int) []string {
	if maxLen < 1 {
		maxLen = 79
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, wrapRun(para, maxLen)...)
	}
	return out
}

func wrapRun(s string, maxLen int) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	var cur []byte
	lastSpace := -1
	flush := func(cut int) {
		if cut < 0 {
			cut = len(cur)
		}
		line := string(cur[:cut])
		out = append(out, line)
		rest := cur[cut:]
		if len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		cur = append(cur[:0], rest...)
		lastSpace = -1
		for i, b := range cur {
			if b == ' ' {
				lastSpace = i
			}
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == SoftCR || ch == 0 {
			continue
		}
		if ch == '\n' {
			flush(len(cur))
			continue
		}
		cur = append(cur, ch)
		if ch == ' ' {
			lastSpace = len(cur) - 1
		}
		if len(cur) < maxLen {
			continue
		}
		if lastSpace > 0 {
			flush(lastSpace)
		} else {
			flush(len(cur))
		}
	}
	if len(cur) > 0 || len(out) == 0 {
		out = append(out, string(cur))
	}
	return out
}
