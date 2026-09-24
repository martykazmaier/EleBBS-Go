package menu

import (
	"strconv"
	"strings"

	"elebbs/internal/lang"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

func (e *Engine) raEditor(header string, quote []string) []string {
	const width = 70
	maxLines := maxEditLines
	e.T.ClearScreen()
	e.T.WriteRA("`A3:" + header)
	e.T.Println("")
	e.T.WriteRA("`A3:" + e.T.RalGet(lang.MaxOf) + " " + strconv.Itoa(maxLines) + " " + e.T.RalGet(lang.Lines) + ", 70 " + e.T.RalGet(lang.ChrPerLn))
	e.T.Println("")
	e.T.Println("")
	lines := append([]string{}, quote...)
	for i, ln := range lines {
		e.editorLine(e.T, i+1, ln)
	}
	cur := len(lines)
	for {
		if cur >= maxLines {
			e.T.Println("")
			e.T.WriteRA("`A3:" + e.T.RalGet(lang.MaxLen))
			e.T.Println("")
			break
		}
		e.T.WriteRA("`A2:" + strconv.Itoa(cur+1) + "`X4:: ")
		e.T.WriteRA("`A3:")
		s, err := e.T.GetString(width, false, false)
		e.T.Println("")
		if err != nil {
			return nil
		}
		s = strings.TrimRight(s, "\r")
		if s == "" {
			break
		}
		lines = append(lines, s)
		cur++
	}
	for {
		e.T.Println("")
		e.T.WriteRA("`A14:" + e.T.RalGet(lang.FuncAvail) + "  [" + strconv.Itoa(len(lines)) + " " + e.T.RalGet(lang.LinesTxt) + "]")
		e.T.Println("")
		e.T.WriteRA("`A11:" + e.dsp(lang.Continue) + ",     " + e.dsp(lang.List) + ", " + e.dsp(lang.Save) + ",    " + e.dsp(lang.Quit1))
		e.T.Println("")
		e.T.Println("")
		e.T.WriteRA(e.T.RalGet(lang.Select))
		ch, err := e.T.GetKey(0)
		if err != nil {
			return nil
		}
		ch = pascal.UpCase(string(ch))[0]
		e.T.Println("")
		switch ch {
		case e.ralKey(lang.Continue, 'C'):
			more := e.raEditorContinue(lines, maxLines, width)
			lines = append(lines, more...)
		case e.ralKey(lang.List, 'L'):
			for i, ln := range lines {
				e.editorLine(e.T, i+1, ln)
			}
		case e.ralKey(lang.Save, 'S'):
			return lines
		case e.ralKey(lang.Quit1, 'Q'):
			if e.T.AskYesNo(lang.Sure, false) {
				return nil
			}
		}
	}
}

func (e *Engine) raEditorContinue(existing []string, maxLines, width int) []string {
	var extra []string
	cur := len(existing)
	for cur+len(extra) < maxLines {
		e.T.WriteRA("`A2:" + strconv.Itoa(cur+len(extra)+1) + "`X4:: ")
		e.T.WriteRA("`A3:")
		s, err := e.T.GetString(width, false, false)
		e.T.Println("")
		if err != nil || pascal.Trim(s) == "" {
			break
		}
		extra = append(extra, s)
	}
	return extra
}

func (e *Engine) dsp(nr int) string {
	if e.T != nil && e.T.Ral != nil {
		return e.T.Ral.GetDsp(nr, "[", "]")
	}
	return e.T.RalStr(nr)
}

func (e *Engine) editorLine(t *term.IO, n int, ln string) {
	col := "`A3:"
	if strings.Contains(ln[:min(len(ln), 15)], ">") {
		col = "`A7:"
	}
	t.WriteRA("`A2:" + strconv.Itoa(n) + "`X4::`X6:" + col + ln)
	t.Println("")
}
