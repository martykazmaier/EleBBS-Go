package sysop

import (
	"fmt"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
	"elebbs/internal/userbase"
)

type fieldKind int

const (
	fText fieldKind = iota
	fWord
	fInt
	fByte
	fFlags
	fPassword
	fSex
	fDateFmt
	fTime
	fDate
	fUserFlags
	fCombined
	fProto
)

// editField is Pascal EditInformation[]: where a field sits and what it edits.
type editField struct {
	x, y, width int
	kind        fieldKind
	min, max    int64
	str         *string
	word        *uint16
	i32         *int32
	b           *byte
}

func (n *Node) userFields() []editField {
	u := &n.Line.User
	text := func(x, y, w int, p *string) editField { return editField{x: x, y: y, width: w, kind: fText, str: p} }
	word := func(x, y, w int, min, max int64, p *uint16) editField {
		return editField{x: x, y: y, width: w, kind: fWord, min: min, max: max, word: p}
	}
	num := func(x, y, w int, p *int32) editField { return editField{x: x, y: y, width: w, kind: fInt, i32: p} }
	byt := func(x, y, w int, min, max int64, p *byte) editField {
		return editField{x: x, y: y, width: w, kind: fByte, min: min, max: max, b: p}
	}
	flags := func(y, i int) editField { return editField{x: 13, y: y, width: 8, kind: fFlags, b: &u.Flags[i]} }
	date := func(y int, p *string) editField { return editField{x: 41, y: y, width: 8, kind: fDate, str: p} }
	return []editField{
		text(13, 2, 35, &u.Name),
		text(13, 3, 35, &u.Handle),
		text(13, 4, 25, &u.Location),
		{x: 27, y: 5, width: 15, kind: fPassword},
		word(13, 6, 5, 0, 65535, &u.Security),
		text(13, 7, 15, &u.VoicePhone),
		text(13, 8, 15, &u.DataPhone),
		flags(9, 0), flags(10, 1), flags(11, 2), flags(12, 3),
		num(13, 13, 7, &u.Credit),
		num(13, 14, 7, &u.Pending),
		word(13, 15, 5, 0, 65535, &u.Group),
		{x: 13, y: 16, width: 7, kind: fSex},
		text(11, 17, 35, &u.ForwardTo),
		text(11, 18, 50, &u.Address1),
		text(11, 19, 50, &u.Address2),
		text(11, 20, 50, &u.Address3),
		text(11, 21, 50, &u.Organisation),
		text(11, 22, 66, &u.Comment),
		{x: 41, y: 6, width: 5, kind: fTime, str: &u.LastTime},
		date(7, &u.LastDate), date(8, &u.FirstDate), date(9, &u.SubDate), date(10, &u.BirthDate),
		word(44, 11, 5, 0, 32000, &n.Line.TimeLimit),
		word(44, 12, 2, 1, 99, &u.ScreenLength),
		byt(44, 13, 3, 0, 255, &u.LastPwdChange),
		byt(44, 14, 3, 0, 255, &u.LastDOBCheck),
		{x: 44, y: 15, width: 11, kind: fDateFmt},
		{x: 72, y: 2, kind: fUserFlags},
		{x: 72, y: 3, kind: fCombined},
		num(72, 4, 7, &u.Uploads),
		num(72, 5, 7, &u.Downloads),
		num(72, 6, 7, &u.UploadsK),
		num(72, 7, 7, &u.DownloadsK),
		num(72, 8, 7, &u.TodayK),
		word(72, 9, 5, 0, 65535, &u.MsgsPosted),
		num(72, 10, 7, &u.LastRead),
		num(72, 11, 7, &u.NoCalls),
		word(72, 12, 5, 0, 65535, &u.MsgArea),
		word(72, 13, 5, 0, 65535, &u.FileArea),
		word(72, 14, 5, 0, 65535, &u.FileGroup),
		word(72, 15, 5, 0, 65535, &u.MsgGroup),
		{x: 72, y: 16, width: 1, kind: fProto},
		byt(72, 17, 3, 0, 255, &u.Language),
	}
}

// editorLabels is Pascal ShowEditLayout: label, right edge column, row.
var editorLabels = []struct {
	start, stop, y int
	s              string
}{
	{2, 11, 2, "Name :"}, {2, 11, 3, "Handle :"}, {2, 11, 4, "Location :"}, {2, 11, 5, "Password :"},
	{2, 11, 6, "Security :"}, {2, 11, 7, "Home# :"}, {2, 11, 8, "Data# :"},
	{2, 11, 9, "A flags :"}, {2, 11, 10, "B flags :"}, {2, 11, 11, "C flags :"}, {2, 11, 12, "D flags :"},
	{2, 11, 13, "Credit :"}, {2, 11, 14, "Pending :"}, {2, 11, 15, "Group :"}, {2, 11, 16, "Sex :"},
	{2, 9, 17, "Fwd   :"}, {2, 9, 18, "Addr1 :"}, {2, 9, 19, "Addr2 :"}, {2, 9, 20, "Addr3 :"},
	{2, 9, 21, "Organ.:"}, {2, 9, 22, "Comnt :"},
	{26, 39, 6, "Last time :"}, {26, 39, 7, "Last date :"}, {26, 39, 8, "1st date :"},
	{26, 39, 9, "Sub date :"}, {26, 39, 10, "Birthdate :"},
	{26, 42, 11, "Time remaining :"}, {26, 42, 12, "Screen length :"}, {26, 42, 13, "Last pwd change :"},
	{26, 42, 14, "Last DOB check :"}, {26, 42, 15, "Date format :"},
	{54, 70, 2, "Flags  "}, {54, 70, 3, "Combined  "}, {54, 70, 4, "Uploads :"}, {54, 70, 5, "Dnloads :"},
	{54, 70, 6, "UploadK :"}, {54, 70, 7, "DnLoadK :"}, {54, 70, 8, "TodayK :"},
	{54, 70, 9, "Messages posted :"}, {54, 70, 10, "High msg read :"}, {54, 70, 11, "Number of calls :"},
	{54, 70, 12, "Last msg area :"}, {54, 70, 13, "Last file area :"}, {54, 70, 14, "Last file group :"},
	{54, 70, 15, "Last msg gr. :"}, {54, 70, 16, "Protocol :"}, {54, 70, 17, "Language :"},
}

func rightJust(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	return strings.Repeat(" ", w-len(s)) + s
}

func sexStr(b byte) string {
	switch b {
	case 1:
		return "Male   "
	case 2:
		return "Female "
	}
	return "Unknown"
}

var dateFormats = [9]string{"", "DD-MM-YY   ", "MM-DD-YY   ", "YY-MM-DD   ", "DD-Mmm-YY  ",
	"DD-MM-YYYY ", "MM-DD-YYYY ", "YYYY-MM-DD ", "DD-Mmm-YYYY"}

func dateFmtStr(b byte) string {
	if b < 1 || b > 8 {
		b = 1
	}
	return dateFormats[b]
}

// flagsStr is Pascal Byte2Flags: bit 0 first, X = set.
func flagsStr(b byte) string {
	var s [8]byte
	for i := range s {
		s[i] = '-'
		if b&(1<<i) != 0 {
			s[i] = 'X'
		}
	}
	return string(s[:])
}

func flagsByte(s string) byte {
	var b byte
	for i := 0; i < len(s) && i < 8; i++ {
		if s[i] == 'X' || s[i] == 'x' {
			b |= 1 << i
		}
	}
	return b
}

func (n *Node) fieldValue(f editField) string {
	u := &n.Line.User
	switch f.kind {
	case fText, fTime:
		return *f.str
	case fDate:
		if *f.str == "" {
			return "  -  -  "
		}
		return *f.str
	case fWord:
		return strconv.Itoa(int(*f.word))
	case fInt:
		return strconv.Itoa(int(*f.i32))
	case fByte:
		return strconv.Itoa(int(*f.b))
	case fFlags:
		return flagsStr(*f.b)
	case fSex:
		return sexStr(u.Sex)
	case fDateFmt:
		return dateFmtStr(u.DateFormat)
	case fProto:
		if u.DefaultProto == 0 {
			return ""
		}
		return pascal.FromCP437([]byte{u.DefaultProto})
	}
	return ""
}

// showField is Pascal MakeEditorsField / ShowPaddings.
func (n *Node) showField(f editField, editing bool) {
	cfg := &n.G.RaConfig
	a := attr(cfg.WindFore, cfg.WindBack)
	if editing {
		a = attr(cfg.HiFore, cfg.WindBack)
	}
	switch f.kind {
	case fPassword:
		n.write(13, 5, attr(cfg.WindFore, cfg.WindBack), "Not visible")
	case fSex, fDateFmt:
		n.write(f.x, f.y, a, n.fieldValue(f))
	case fUserFlags, fCombined:
	default:
		n.write(f.x, f.y, a, fieldText(n.fieldValue(f), f.width, false))
	}
}

// EditUser is Pascal UserEdit(True) from Alt-E: edit the online caller's
// record on the node window while the caller waits.
func (n *Node) EditUser() {
	if !n.Line.LoggedOn || n.Win == nil || n.Keys == nil {
		return
	}
	cfg := &n.G.RaConfig
	old := n.Line.User
	done := n.sysopScreen()
	n.box(1, 1, 80, 25, cfg.BorderFore, cfg.BorderBack, "Edit user")
	lbl := attr(cfg.WindFore, cfg.WindBack)
	for _, l := range editorLabels {
		n.write(l.start, l.y, lbl, rightJust(l.s, l.stop-l.start+1))
	}
	help := attr(cfg.HiFore, cfg.WindBack)
	n.write(9, 24, help, "(TAB) Next field")
	n.write(31, 24, help, "(SHIFT-TAB) Previous field")
	n.write(63, 24, help, "(ESC) Exit")
	fields := n.userFields()
	for _, f := range fields {
		n.showField(f, false)
	}
	for cur := 0; ; {
		switch n.editField(fields, cur) {
		case keyEnter, keyDown:
			cur++
		case keyUp:
			cur--
		case keyEsc:
			done()
			n.finishEdit(old)
			return
		}
		if cur < 0 {
			cur = len(fields) - 1
		}
		if cur >= len(fields) {
			cur = 0
		}
	}
}

// finishEdit is the end of Pascal UserEdit: terminal settings follow the
// edited attributes, then "Save changes (Y/n)" when the record changed.
func (n *Node) finishEdit(old cfgrec.User) {
	u := &n.Line.User
	if (u.Attribute^old.Attribute)&cfgrec.UserANSI != 0 {
		n.Line.AnsiOn = u.Attribute&cfgrec.UserANSI != 0
	}
	if (u.Attribute2^old.Attribute2)&(1<<1) != 0 {
		n.Line.AvatarOn = u.Attribute2&(1<<1) != 0
	}
	if (u.Attribute2^old.Attribute2)&cfgrec.User2Guest != 0 {
		n.Line.GuestUser = u.Attribute2&cfgrec.User2Guest != 0
	}
	if *u == old {
		return
	}
	done := n.sysopScreen()
	n.box(23, 10, 50, 14, 0, 7, "")
	n.write(26, 12, 0x70, "Save changes (Y/n) ? ░")
	var ch byte
	for ch != 'Y' && ch != 'N' {
		n.Win.GotoXY(47, 12)
		ch = upper(n.readKey().Ch)
		if ch == '\r' {
			ch = 'Y'
		}
	}
	done()
	if ch == 'N' {
		*u = old
		return
	}
	_ = userbase.Write(n.G, *u)
}

func digits(extra string) func(byte) bool {
	return func(c byte) bool { return c >= '0' && c <= '9' || strings.IndexByte(extra, c) >= 0 }
}

// editField is Pascal ProcessCurField(EditField=True); it returns how to
// move on.
func (n *Node) editField(fields []editField, i int) byte {
	f := fields[i]
	cfg := &n.G.RaConfig
	hi := attr(cfg.HiFore, cfg.WindBack)
	defer n.showField(f, false)
	switch f.kind {
	case fText:
		for {
			s, key := n.lineEdit(f.x, f.y, hi, *f.str, f.width, nil, false)
			if i == 0 && strings.TrimSpace(s) == "" && key != keyEsc {
				continue
			}
			if strings.TrimSpace(s) != "" || i != 0 {
				*f.str = s
			}
			return key
		}
	case fWord, fInt, fByte:
		return n.editNumber(f, hi)
	case fFlags:
		s, key := n.lineEdit(f.x, f.y, hi, flagsStr(*f.b), 8, func(c byte) bool { return c == '-' || c == 'X' || c == 'x' }, false)
		*f.b = flagsByte(s)
		return key
	case fPassword:
		return n.editPassword()
	case fSex:
		return n.cycle(f, hi, func() {
			if n.Line.User.Sex == 1 {
				n.Line.User.Sex = 2
			} else {
				n.Line.User.Sex = 1
			}
		})
	case fDateFmt:
		return n.cycle(f, hi, func() {
			n.Line.User.DateFormat++
			if n.Line.User.DateFormat > 8 || n.Line.User.DateFormat < 1 {
				n.Line.User.DateFormat = 1
			}
		})
	case fTime:
		s, key := n.lineEdit(f.x, f.y, hi, *f.str, 5, digits(":"), false)
		*f.str = timeField(s)
		return key
	case fDate:
		s, key := n.lineEdit(f.x, f.y, hi, *f.str, 8, digits("-"), false)
		*f.str = dateField(s)
		return key
	case fUserFlags:
		n.Win.GotoXY(f.x, f.y)
		k := n.readKey()
		if k.Ch != '\r' {
			return navKey(k)
		}
		n.editUserFlags()
		return keyDown
	case fCombined:
		n.Win.GotoXY(f.x, f.y)
		return navKey(n.readKey())
	case fProto:
		s, key := n.lineEdit(f.x, f.y, hi, n.fieldValue(f), 1, nil, false)
		if b := pascal.ToCP437(s); len(b) > 0 {
			n.Line.User.DefaultProto = b[0]
		}
		return key
	}
	return keyDown
}

// editNumber is Pascal EditWordField / EditIntegerField / EditByteField:
// out-of-range values are re-edited.
func (n *Node) editNumber(f editField, hi byte) byte {
	extra := ""
	if f.kind == fInt {
		extra = "-"
	}
	val := n.fieldValue(f)
	for {
		s, key := n.lineEdit(f.x, f.y, hi, val, f.width, digits(extra), false)
		v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		switch f.kind {
		case fWord:
			v = min(max(v, 0), 65535)
		case fByte:
			v = min(max(v, 0), 255)
		}
		if (f.min > 0 && v < f.min) || (f.max > 0 && v > f.max) {
			val = s
			continue
		}
		switch f.kind {
		case fWord:
			*f.word = uint16(v)
		case fInt:
			*f.i32 = int32(v)
		case fByte:
			*f.b = byte(v)
		}
		return key
	}
}

// cycle is Pascal EditSex / EditDateFormat: SPACE steps the value.
func (n *Node) cycle(f editField, hi byte, step func()) byte {
	for {
		n.write(f.x, f.y, hi, n.fieldValue(f))
		n.Win.GotoXY(f.x, f.y)
		k := n.readKey()
		if k.Ch != ' ' {
			return navKey(k)
		}
		step()
	}
}

func timeField(s string) string {
	hh, mm, _ := strings.Cut(s, ":")
	h, _ := strconv.Atoi(strings.TrimSpace(hh))
	m, _ := strconv.Atoi(strings.TrimSpace(mm))
	return fmt.Sprintf("%02d:%02d", h, m)
}

// dateField is Pascal EditDateField's result: MM-DD-YY, or empty when the
// month or day is zero.
func dateField(s string) string {
	parts := strings.SplitN(s, "-", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	var v [3]int
	for i, p := range parts {
		v[i], _ = strconv.Atoi(strings.TrimSpace(p))
	}
	if v[0] == 0 || v[1] == 0 {
		return ""
	}
	return fmt.Sprintf("%02d-%02d-%02d", v[0], v[1], v[2]%100)
}

// editPassword is Pascal GetNewPassword.
func (n *Node) editPassword() byte {
	cfg := &n.G.RaConfig
	n.Win.GotoXY(13, 5)
	var k = n.readKey()
	for k.Ch != 0 && k.Ch != '\r' && k.Ch != 0x1b && k.Ch != '\t' {
		k = n.readKey()
	}
	if k.Ch != '\r' {
		return navKey(k)
	}
	restore := n.Win.Save()
	defer restore()
	prompt := attr(cfg.HiFore, cfg.BorderBack)
	if !cfg.SavePasswords {
		n.box(11, 3, 43, 7, cfg.BorderFore, cfg.BorderBack, "Password")
		n.write(13, 5, prompt, " Enter new password? (y,N) ░")
		var ch byte
		for ch != 'Y' && ch != 'N' {
			n.Win.GotoXY(40, 5)
			ch = upper(n.readKey().Ch)
			if ch == '\r' {
				ch = 'N'
			}
		}
		if ch == 'N' {
			return keyEnter
		}
	}
	n.box(11, 3, 43, 7, cfg.BorderFore, cfg.BorderBack, "Password")
	n.write(13, 5, prompt, "New password: ")
	pw := ""
	if cfg.SavePasswords {
		pw = n.Line.User.Password
	}
	s, key := n.lineEdit(27, 5, attr(cfg.HiFore, cfg.WindBack), pw, 15, nil, false)
	if key == keyEsc || strings.TrimSpace(s) == "" {
		return keyEnter
	}
	u := &n.Line.User
	userbase.SetPassword(u, s, cfg.StrictPwdChecking)
	if !cfg.SavePasswords {
		u.Password = ""
	}
	return keyEnter
}

var userFlagLabels = []string{"Deleted :", "Clear screen :", "Page pausing :", "ANSI graphics :",
	"AVATAR graphics :", "No-kill :", "Xfer priority :", "Full screen editor :", "Quiet mode :",
	"Hot-keys :", "Full screen viewer:", "Hidden :", "Page priority :", "No new echomail :",
	"Guest :", "Post bill :", "Selected mail only :"}

// editUserFlags is Pascal EditUserFlags: the Y/N attribute bits.
func (n *Node) editUserFlags() {
	cfg := &n.G.RaConfig
	u := &n.Line.User
	bits := []struct {
		p   *byte
		bit uint
	}{
		{&u.Attribute, 0}, {&u.Attribute, 1}, {&u.Attribute, 2}, {&u.Attribute, 3}, {&u.Attribute2, 1},
		{&u.Attribute, 4}, {&u.Attribute, 5}, {&u.Attribute, 6}, {&u.Attribute, 7}, {&u.Attribute2, 0},
		{&u.Attribute2, 2}, {&u.Attribute2, 3}, {&u.Attribute2, 4}, {&u.Attribute2, 5}, {&u.Attribute2, 6},
		{&u.Attribute2, 7}, {&u.Attribute3, 0},
	}
	restore := n.Win.Save()
	defer restore()
	n.box(51, 2, 76, 20, cfg.BorderFore, cfg.BorderBack, "Edit flags")
	lbl := attr(cfg.WindFore, cfg.WindBack)
	hi := attr(cfg.HiFore, cfg.WindBack)
	show := func(i int, a byte) {
		v := "N"
		if *bits[i].p&(1<<bits[i].bit) != 0 {
			v = "Y"
		}
		n.write(74, 3+i, a, v)
	}
	for i, l := range userFlagLabels {
		n.write(53, 3+i, lbl, rightJust(l, 20))
		show(i, lbl)
	}
	hl := 0
	for {
		show(hl, hi)
		n.Win.GotoXY(74, 3+hl)
		k := n.readKey()
		show(hl, lbl)
		if k.Ch == 0x1b {
			return
		}
		if k.Ch != 0 {
			switch upper(k.Ch) {
			case 'Y':
				*bits[hl].p |= 1 << bits[hl].bit
				hl++
			case 'N':
				*bits[hl].p &^= 1 << bits[hl].bit
				hl++
			case '\r':
				hl++
			}
		} else {
			switch k.Scan {
			case 72:
				hl--
			case 80:
				hl++
			case 71:
				hl = 0
			case 79:
				hl = len(bits) - 1
			}
		}
		if hl < 0 {
			hl = len(bits) - 1
		}
		if hl >= len(bits) {
			hl = 0
		}
	}
}
