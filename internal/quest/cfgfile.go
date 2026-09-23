package quest

import (
	"os"
	"strconv"
	"strings"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/pascal"
	"elebbs/internal/userbase"
)

type cfgSlot struct {
	f    *os.File
	sort string
	size int
	buf  []byte
	err  int
}

func cfgRecSize(sort string) int {
	switch strings.ToUpper(sort) {
	case "LIMITS":
		return cfgrec.LimitsSize
	case "LANGUAGE":
		return cfgrec.LanguageSize
	case "LASTCALL":
		return cfgrec.LastCallSize
	case "FDBHDR":
		return cfgrec.FilesHdrSize
	case "FDBIDX":
		return cfgrec.FilesIdxSize
	case "USERSBBS":
		return cfgrec.UsersSize
	case "USERSIDX":
		return cfgrec.UsersIdxSize
	case "SYSINFO":
		return cfgrec.SysInfoSize
	case "MENU":
		return cfgrec.MenuSize
	case "MESSAGES":
		return cfgrec.MessageSize
	case "GROUP":
		return cfgrec.GroupSize
	case "FILES":
		return cfgrec.FilesRecSize
	case "MODEM":
		return cfgrec.ModemSize
	case "PROTOCOL":
		return cfgrec.ProtocolSize
	case "CONFIG":
		return 8192
	default:
		return 1024
	}
}

func (q *vm) usersWritable() bool {
	return q.line != nil && q.line.LoggedOn && q.line.User.Record >= 0
}

func (q *vm) cfgOpen(rest string) {
	slotW, rest := firstWord(rest)
	nameW, sortW := firstWord(rest)
	slot := atoi(slotW)
	if slot < 1 || slot > maxCfgFiles {
		return
	}
	name := q.value(nameW)
	sort := strings.ToUpper(q.value(sortW))
	q.cfgCloseSlot(slot)
	if sort == "USERSBBS" && q.g != nil {
		name = userbase.Path(q.g, cfgrec.UserBaseName)
	}
	if sort == "USERSIDX" && q.g != nil {
		name = userbase.Path(q.g, cfgrec.UserBaseIdxName)
	}
	if sort == "CONFIG" && q.g != nil && q.g.CfgPath != "" {
		name = q.g.CfgPath
	} else if found := config.ExistingFile(q.g, name); found != "" {
		name = found
	}
	if name == "" {
		return
	}
	flag := os.O_RDWR
	if (sort == "USERSBBS" || sort == "USERSIDX") && !q.usersWritable() {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(name, flag, 0644)
	s := &cfgSlot{sort: sort, size: cfgRecSize(sort), buf: make([]byte, cfgRecSize(sort))}
	if err != nil {
		s.err = 2
		q.cfg[slot] = s
		return
	}
	s.f = f
	if sort == "CONFIG" {
		if st, err := f.Stat(); err == nil && st.Size() > 0 {
			s.size = int(st.Size())
			s.buf = make([]byte, s.size)
		}
	}
	q.cfg[slot] = s
}

func (q *vm) cfgCloseSlot(slot int) {
	if slot < 1 || slot > maxCfgFiles || q.cfg[slot] == nil {
		return
	}
	if q.cfg[slot].f != nil {
		_ = q.cfg[slot].f.Close()
	}
	q.cfg[slot] = nil
}

func (q *vm) cfgSeek(slot, pos int) {
	s := q.cfgSlot(slot)
	if s == nil || s.f == nil {
		return
	}
	if pos < 1 {
		pos = 1
	}
	_, err := s.f.Seek(int64(pos-1)*int64(s.size), 0)
	if err != nil {
		s.err = 1
	}
}

func (q *vm) cfgRead(slot int) {
	s := q.cfgSlot(slot)
	if s == nil || s.f == nil {
		return
	}
	n, err := s.f.Read(s.buf)
	if err != nil || n < s.size {
		s.err = 1
		return
	}
}

func (q *vm) cfgWrite(slot int) {
	s := q.cfgSlot(slot)
	if s == nil || s.f == nil {
		return
	}
	if (s.sort == "USERSBBS" || s.sort == "USERSIDX") && !q.usersWritable() {
		return
	}
	_, err := s.f.Write(s.buf)
	if err != nil {
		s.err = 1
	}
}

func (q *vm) cfgSlot(slot int) *cfgSlot {
	if slot < 1 || slot > maxCfgFiles {
		return nil
	}
	return q.cfg[slot]
}

func (q *vm) cfgGet(slot, field int) string {
	s := q.cfgSlot(slot)
	if s == nil {
		return ""
	}
	if s.sort == "USERSBBS" {
		return mapUsersGet(s.buf, field)
	}
	if s.sort == "CONFIG" {
		return q.cfgGetConfig(s, field)
	}
	if field == 1 {
		return strings.TrimRight(pascal.FromCP437(s.buf), "\x00")
	}
	return ""
}

func (q *vm) cfgGetConfig(s *cfgSlot, field int) string {
	c := cfgrec.ParseConfig(s.buf)
	if q != nil && q.g != nil {
		live := q.g.RaConfig
		if strings.TrimSpace(c.SemPath) == "" {
			c.SemPath = live.SemPath
		}
		if strings.TrimSpace(c.SysPath) == "" {
			c.SysPath = live.SysPath
		}
		if strings.TrimSpace(c.MenuPath) == "" {
			c.MenuPath = live.MenuPath
		}
		if strings.TrimSpace(c.TextPath) == "" {
			c.TextPath = live.TextPath
		}
		if strings.TrimSpace(c.MsgBasePath) == "" {
			c.MsgBasePath = live.MsgBasePath
		}
		if strings.TrimSpace(c.FileBase) == "" {
			c.FileBase = live.FileBase
		}
		if strings.TrimSpace(c.SystemName) == "" {
			c.SystemName = live.SystemName
		}
		if strings.TrimSpace(c.Sysop) == "" {
			c.Sysop = live.Sysop
		}
	}
	switch field {
	case 26:
		return pascal.ForceBack(c.MenuPath)
	case 27:
		return pascal.ForceBack(c.TextPath)
	case 30:
		return pascal.ForceBack(c.MsgBasePath)
	case 31:
		return pascal.ForceBack(c.SysPath)
	case 43:
		return c.SystemName
	case 52:
		return c.Sysop
	case 189:
		return pascal.ForceBack(c.SemPath)
	case 191:
		return pascal.ForceBack(c.FileBase)
	default:
		return ""
	}
}

func (q *vm) cfgSet(slot, field int, val string) {
	s := q.cfgSlot(slot)
	if s == nil {
		return
	}
	if s.sort == "USERSBBS" {
		mapUsersSet(s.buf, field, val)
	}
}

func mapUsersGet(buf []byte, field int) string {
	if len(buf) < cfgrec.UsersSize {
		return ""
	}
	u := cfgrec.ParseUser(buf, 0)
	switch field {
	case 1:
		return u.Name
	case 2:
		return u.Location
	case 3:
		return u.Organisation
	case 4:
		return u.Address1
	case 5:
		return u.Address2
	case 6:
		return u.Address3
	case 7:
		return u.Handle
	case 8:
		return u.Comment
	case 9:
		return strconv.FormatInt(int64(u.PasswordCRC), 10)
	case 10:
		return u.DataPhone
	case 11:
		return u.VoicePhone
	case 12:
		return u.LastTime
	case 13:
		return u.LastDate
	case 14:
		return bitYes(u.Attribute, cfgrec.UserDeleted)
	case 15:
		return bitYes(u.Attribute, cfgrec.UserClrScr)
	case 16:
		return bitYes(u.Attribute, cfgrec.UserMore)
	case 17:
		return bitYes(u.Attribute, cfgrec.UserANSI)
	case 22:
		return bitYes(u.Attribute2, cfgrec.User2HotKeys)
	case 34:
		return strconv.FormatInt(int64(u.Credit), 10)
	case 37:
		return strconv.Itoa(int(u.Security))
	case 39:
		return strconv.FormatInt(int64(u.NoCalls), 10)
	case 46:
		return strconv.Itoa(int(u.ScreenLength))
	case 54:
		return strconv.Itoa(int(u.Language))
	case 56:
		return u.ForwardTo
	case 57:
		return strconv.Itoa(int(u.MsgArea))
	case 58:
		return strconv.Itoa(int(u.FileArea))
	case 59:
		return string(u.DefaultProto)
	case 66:
		return u.Password
	default:
		return ""
	}
}

func mapUsersSet(buf []byte, field int, val string) {
	if len(buf) < cfgrec.UsersSize {
		return
	}
	u := cfgrec.ParseUser(buf, 0)
	switch field {
	case 1:
		u.Name = val
	case 2:
		u.Location = val
	case 7:
		u.Handle = val
	case 8:
		u.Comment = val
	case 37:
		u.Security = uint16(atoi(val))
	case 54:
		u.Language = byte(atoi(val))
	case 56:
		u.ForwardTo = val
	case 57:
		u.MsgArea = uint16(atoi(val))
	case 58:
		u.FileArea = uint16(atoi(val))
	case 66:
		u.Password = pascal.UpCase(val)
	}
	copy(buf, cfgrec.EncodeUser(u))
}

func bitYes(attr, bit byte) string {
	if attr&bit != 0 {
		return "YES"
	}
	return "NO"
}
