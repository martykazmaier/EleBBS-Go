package cfgrec

import (
	"elebbs/internal/pascal"
)

func EncodeExitinfo(line *LineCfg) []byte {
	w := &pascal.Writer{}
	baud := line.Baud
	w.U16(baud)
	si := EncodeSysInfo(line.SysInfo)
	if len(si) < SysInfoSize {
		si = append(si, make([]byte, SysInfoSize-len(si))...)
	}
	w.B = append(w.B, si[:SysInfoSize]...)
	w.Pad(TimeLogSize)
	u := EncodeUser(line.User)
	if len(u) < UsersSize {
		u = append(u, make([]byte, UsersSize-len(u))...)
	}
	w.B = append(w.B, u[:UsersSize]...)
	w.Pad(EventSize)
	w.Bool(false) // NetMailEntered
	w.Bool(false) // EchoMailEntered
	w.PString(5, line.LoginTime)
	w.PString(8, line.LoginDate)
	w.U16(line.TimeLimit)
	w.I32(0) // LoginSec
	rec := int16(line.User.Record)
	if rec < 0 {
		rec = 0
	}
	w.I16(rec)
	w.U16(0) // ReadThru
	w.U16(0) // NumberPages
	w.U16(0) // DownloadLimit
	w.PString(5, line.LoginTime)
	w.I32(line.User.PasswordCRC)
	w.Bool(false) // WantChat
	w.I16(0)      // DeductedTime
	for i := 0; i < 50; i++ {
		w.PString(8, line.MenuStack[i])
	}
	w.U8(byte(line.MenuStackPtr))
	w.Pad(UsersXiSize)
	w.Bool(line.ErrorFreeConnect)
	w.Bool(false) // SysopNext
	w.Bool(line.EmsiSession)
	w.PString(40, line.EmsiUser.CrtDef)
	w.PString(40, line.EmsiUser.Protocols)
	w.PString(40, line.EmsiUser.Capabilities)
	w.PString(40, line.EmsiUser.Requests)
	w.PString(40, line.EmsiUser.Software)
	w.U8(0)
	w.U8(0)
	w.U8(0)
	w.PString(80, "")
	w.U8(0)
	w.PString(8, "")
	w.U16(0)
	w.Bool(line.AvatarOn)
	w.Bool(line.RipOn)
	w.U8(0)
	if len(w.B) < ExitinfoSize {
		w.Pad(ExitinfoSize - len(w.B))
	}
	if len(w.B) > ExitinfoSize {
		return w.B[:ExitinfoSize]
	}
	return w.B
}
