package menu

import (
	"fmt"
	"strings"

	"elebbs/internal/door"
	"elebbs/internal/files"
	"elebbs/internal/lang"
	"elebbs/internal/pascal"
)

func (e *Engine) tagListDir() string {
	if e == nil {
		return "."
	}
	return door.DropDir(e.G, e.Line)
}

func (e *Engine) loadTagList() []files.TagFile {
	return files.LoadTagList(e.tagListDir())
}

func (e *Engine) saveTagList(tags []files.TagFile) {
	_ = files.SaveTagList(e.tagListDir(), tags)
}

func (e *Engine) taggedFound() []files.Found {
	return files.FoundFromTags(e.G, e.Files, e.loadTagList())
}

func tagFromFound(f files.Found) files.TagFile {
	return files.TagFile{
		Name:      f.Hdr.Name,
		Password:  f.Hdr.Password,
		Attrib:    f.Hdr.Attrib,
		AreaNum:   f.Area.AreaNum,
		RecordNum: f.Hdr.RecordNum + 1,
		Size:      int32(f.Hdr.Size),
		FileDate:  int32(f.Hdr.FileDate),
		Cost:      int32(f.Hdr.Cost),
	}
}

func (e *Engine) addNamesToTag(names []string, area uint16, global, anyFile bool) {
	if len(names) == 0 {
		return
	}
	found := e.lookupDownload(names, area, global, anyFile)
	if len(found) == 0 {
		return
	}
	tags := e.loadTagList()
	for _, f := range found {
		if len(tags) >= files.MaxTagged {
			break
		}
		if files.IsTagged(e.G, e.Files, tags, f.Hdr.Name, f.Area.AreaNum) {
			continue
		}
		tags = append(tags, tagFromFound(f))
	}
	e.saveTagList(tags)
}

// showTaggedFiles is Pascal ShowTaggedFiles: clear screen, then taglist.ra.
func (e *Engine) showTaggedFiles(fromListing bool) bool {
	e.T.ClearScreen()
	tags := e.loadTagList()
	if len(tags) == 0 {
		e.T.WriteRA("`A12:")
		if fromListing {
			e.T.WriteRA(e.T.RalGet(lang.NoTagged1))
		} else {
			e.T.WriteRA(e.T.RalGet(lang.NoTagged2))
		}
		e.T.Println("")
		return false
	}
	e.T.WriteRA("`A3:" + e.T.RalGet(lang.Taglist))
	e.T.Println("")
	var prev uint16
	var total int32
	n := 0
	freeN := 0
	var freeBytes int32
	for i, t := range tags {
		if t.AreaNum != prev {
			a, ok := files.FindArea(e.Files, t.AreaNum)
			e.T.Println("")
			if ok {
				e.T.WriteRA("`A14:" + a.Name)
			} else {
				e.T.WriteRA(fmt.Sprintf("`A14:Area %d", t.AreaNum))
			}
			e.T.Println("")
			prev = t.AreaNum
		}
		name := files.DisplayName(e.G, e.Files, t)
		e.T.WriteRA(fmt.Sprintf(" `A7:%02d `A3:%s`X48:%dk`X24:", i+1, name, t.Size/1024))
		if t.Free() {
			e.T.WriteRA(" " + e.T.RalGet(lang.FreeDl))
			freeBytes += t.Size
			freeN++
		}
		if t.Password != "" {
			e.T.WriteRA(" " + e.T.RalGet(lang.Password2))
		}
		e.T.Println("")
		total += t.Size
		n++
	}
	e.T.Println("")
	e.T.WriteRA(fmt.Sprintf("`A11:%d %s, %dk", n, e.T.RalGet(lang.Files1), total/1024))
	if freeBytes > 0 {
		e.T.WriteRA(fmt.Sprintf(" / %d %s, %dk %s", freeN, e.T.RalGet(lang.Files1), freeBytes/1024, e.T.RalGet(lang.FreeDl)))
	}
	e.T.Println("")
	return true
}

func (e *Engine) userAddToTag(anyFile, global, group bool, area uint16) {
	_ = group
	e.T.Println("")
	e.T.WriteRA("`A3:")
	e.T.WriteRA(e.T.RalGet(lang.File1))
	s, _ := e.T.GetString(60, false, false)
	s = pascal.Trim(s)
	var names []string
	for s != "" {
		w, rest := firstWord(s)
		s = rest
		if w != "" {
			names = append(names, w)
		}
	}
	e.addNamesToTag(names, area, global, anyFile)
}

func (e *Engine) deleteFromTag() {
	e.T.Println("")
	e.T.Println("")
	e.T.WriteRA("`A12:" + e.T.RalGet(lang.Delete))
	s, _ := e.T.GetString(80, false, false)
	s = strings.ReplaceAll(s, ",", " ")
	tags := e.loadTagList()
	var names []string
	for s != "" {
		w, rest := firstWord(s)
		s = rest
		w = strings.TrimSpace(w)
		if w == "" {
			break
		}
		if n := fvalWord(w); n > 0 && !strings.Contains(w, "-") {
			if n <= len(tags) {
				names = append(names, files.DisplayName(e.G, e.Files, tags[n-1]))
			}
			continue
		}
		if i := strings.IndexByte(w, '-'); i > 0 && !strings.Contains(w, ".") {
			start := fvalWord(w[:i])
			end := fvalWord(w[i+1:])
			if start > 0 && end >= start {
				for n := start; n <= end && n <= len(tags); n++ {
					names = append(names, files.DisplayName(e.G, e.Files, tags[n-1]))
				}
				continue
			}
		}
		names = append(names, w)
	}
	for _, name := range names {
		tags = files.DeleteFromTagged(e.G, e.Files, tags, name)
	}
	e.saveTagList(tags)
}

func fvalWord(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 && s != "" && s[0] == '0' {
		return 0
	}
	return n
}

func adcKeys(s string) (add, del, clear byte) {
	s = pascal.UpCase(strings.TrimSpace(s))
	if s == "" {
		return 'A', 'D', 'C'
	}
	if len(s) >= 3 {
		return s[0], s[1], s[2]
	}
	if len(s) == 2 {
		return 0, s[0], s[1]
	}
	return 0, s[0], 0
}

// editTagList is Pascal EditTagList.
func (e *Engine) editTagList(fromListing, anyFile, global, group bool, area uint16) {
	for {
		if !e.showTaggedFiles(fromListing) {
			if fromListing {
				return
			}
			e.userAddToTag(anyFile, global, group, area)
			if !e.showTaggedFiles(fromListing) {
				return
			}
		}
		if len(e.loadTagList()) == 0 {
			return
		}
		e.T.Println("")
		prompt := lang.ADC
		keys := e.T.RalKeys(lang.ADC)
		if fromListing {
			prompt = lang.DC
			keys = e.T.RalKeys(lang.DC)
		}
		e.T.WriteRA("`A11:" + e.T.RalStr(prompt))
		add, del, clr := adcKeys(keys)
		if fromListing {
			add = 0
		}
		ch, err := e.T.GetKey(0)
		if err != nil {
			return
		}
		up := byte(0)
		if ch != 0 {
			s := pascal.UpCase(string([]byte{ch}))
			if s != "" {
				up = s[0]
			}
		}
		e.T.Println("")
		if ch == '\r' || ch == '\n' {
			return
		}
		switch up {
		case add:
			if !fromListing && add != 0 {
				e.userAddToTag(anyFile, global, group, area)
			}
		case del:
			e.deleteFromTag()
		case clr:
			e.T.WriteRA("`A12:")
			e.T.Println("")
			e.T.Println("")
			if e.T.AskYesNo(lang.CTag, false) {
				e.saveTagList(nil)
			}
		default:
			continue
		}
	}
}

func (e *Engine) viewTaggedFiles() {
	area := uint16(0)
	if e.Line != nil {
		area = e.Line.User.FileArea
	}
	e.editTagList(false, false, true, false, area)
}
