package logon

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/logx"
	"elebbs/internal/pascal"
	"elebbs/internal/term"
)

const (
	iemsiWait       = 10 * time.Second
	sysEMSINotice   = "Copyright 1996-2003 Maarten Bekers, All Rights reserved"
	sysCapabilities = "ZMO"
)

func iemsiEnabled(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) bool {
	if g == nil || line == nil {
		return false
	}
	if line.Baud == 0 {
		return false
	}
	return g.RaConfig.EMSIEnable != cfgrec.AskNo
}

func startIEMSISession(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) {
	if !iemsiEnabled(g, line) {
		return
	}
	t.WriteRaw([]byte(emsi16Frame("EMSI_IRQ")))
}

func emsi16Frame(kind string) string {
	return "**" + kind + fmt.Sprintf("%04X", crc.EMSI16(kind)) + "\r"
}

func sendEMSINAK(t *term.IO) {
	t.WriteRaw([]byte(emsi16Frame("EMSI_NAK")))
}

func isIEMSIProbe(s string) bool {
	return strings.Contains(pascal.UpCase(s), "**EMSI_")
}

// getIEMSIUser is Pascal IEMSI_GetUser. **EMSI_ has already been consumed
// from the name prompt; the stream still holds ICI<len><data><crc32>.
func getIEMSIUser(t *term.IO, g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) bool {
	hdr, ok := t.ReadBytes(7, iemsiWait) // ICI + 4 hex length
	if !ok || len(hdr) < 7 {
		sendEMSINAK(t)
		sendEMSINAK(t)
		return false
	}
	if pascal.UpCase(string(hdr[:3])) != "ICI" {
		sendEMSINAK(t)
		sendEMSINAK(t)
		return false
	}
	pkgLen, err := strconv.ParseUint(string(hdr[3:7]), 16, 16)
	if err != nil || pkgLen == 0 {
		sendEMSINAK(t)
		sendEMSINAK(t)
		return false
	}
	rest, ok := t.ReadBytes(int(pkgLen)+8, iemsiWait)
	if !ok || len(rest) < int(pkgLen)+8 {
		sendEMSINAK(t)
		return false
	}
	data := rest[:pkgLen]
	gotCRC := pascal.UpCase(strings.TrimSpace(string(rest[pkgLen : pkgLen+8])))
	body := append([]byte("EMSI_ICI"), hdr[3:7]...)
	body = append(body, data...)
	want := fmt.Sprintf("%08X", crc.CRC32(body, 0xFFFFFFFF))
	if gotCRC != want {
		sendEMSINAK(t)
		return false
	}
	skipCR(t)

	u := decodeIEMSI(data)
	line.EmsiUser = u
	line.EmsiSession = true
	if !sendIEMSIISI(t, g) {
		return false
	}
	// Spec: client ACKs twice. Ignore the bytes; drain leftovers.
	_, _ = t.ReadBytes(len("**EMSI_ACK0000"), iemsiWait)
	_, _ = t.ReadBytes(len("**EMSI_ACK0000"), iemsiWait)
	drainIEMSI(t)

	logx.Write(g, line.RaNodeNr, '>', "Established interactive EMSI session")
	if u.Software != "" {
		logx.Write(g, line.RaNodeNr, '>', "Remote is using "+u.Software)
	}
	if u.Requests != "" {
		logx.Write(g, line.RaNodeNr, '>', "Request flags: "+u.Requests)
	}
	return true
}

func skipCR(t *term.IO) {
	for i := 0; i < 4; i++ {
		ch, ok := t.PeekKey()
		if !ok || (ch != '\r' && ch != '\n' && ch != 0) {
			return
		}
		_, _ = t.GetKey(0)
	}
}

func drainIEMSI(t *term.IO) {
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		if _, err := t.GetKey(200 * time.Millisecond); err != nil {
			return
		}
	}
}

func sendIEMSIISI(t *term.IO, g *cfgrec.GlobalCfg) bool {
	data := encodeServerRecord(g)
	hexLen := fmt.Sprintf("%04X", len(data))
	body := append([]byte("EMSI_ISI"+hexLen), data...)
	sum := crc.CRC32(body, 0xFFFFFFFF)
	frame := append([]byte("**EMSI_ISI"+hexLen), data...)
	frame = append(frame, []byte(fmt.Sprintf("%08X", sum))...)
	frame = append(frame, '\r')
	return t.WriteRaw(frame) == nil
}

func encodeServerRecord(g *cfgrec.GlobalCfg) []byte {
	sys := ""
	loc := ""
	sysop := ""
	if g != nil {
		sys = g.RaConfig.SystemName
		loc = g.RaConfig.Location
		sysop = g.RaConfig.Sysop
	}
	fields := []string{
		cfgrec.FullProgName + " " + cfgrec.VersionID,
		sys,
		loc,
		sysop,
		fmt.Sprintf("%08X", uint32(time.Now().Unix())),
		sysEMSINotice,
		"",
		sysCapabilities,
	}
	var b []byte
	for _, f := range fields {
		b = append(b, '{')
		b = append(b, []byte(slashIEMSI(f))...)
		b = append(b, '}')
	}
	return b
}

func slashIEMSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == ']' || c == '}' {
			b.WriteByte(c)
			b.WriteByte(c)
			continue
		}
		if c < 32 || c > 127 {
			fmt.Fprintf(&b, "\\%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func decodeIEMSI(data []byte) cfgrec.IEMSIUser {
	pos := 0
	next := func() string {
		s, n := getBracket(data, pos)
		pos = n
		return asciiIEMSI(s)
	}
	return cfgrec.IEMSIUser{
		Name:         next(),
		Alias:        next(),
		Location:     next(),
		DataNr:       next(),
		VoiceNr:      next(),
		Password:     next(),
		BirthDate:    next(),
		CrtDef:       next(),
		Protocols:    next(),
		Capabilities: next(),
		Requests:     next(),
		Software:     next(),
		XlatTable:    next(),
	}
}

func getBracket(emsi []byte, start int) (string, int) {
	if start < 0 {
		start = 0
	}
	if start >= len(emsi) {
		return "", start
	}
	if emsi[start] != '{' {
		for n := 0; n < 6 && start < len(emsi) && emsi[start] != '{'; n++ {
			start++
		}
		if start >= len(emsi) || emsi[start] != '{' {
			return "", start
		}
	}
	start++
	var dest []byte
	for start < len(emsi) && emsi[start] != '}' {
		dest = append(dest, emsi[start])
		start++
		if start < len(emsi) && emsi[start] == '}' && start+1 < len(emsi) && emsi[start+1] == '}' {
			dest = append(dest, '}')
			start += 2
		}
	}
	if start < len(emsi) && emsi[start] == '}' {
		start++
	}
	return string(dest), start
}

func asciiIEMSI(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '\\' {
			b.WriteByte('\\')
			i++
			continue
		}
		if i+2 < len(s) {
			v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte('\\')
	}
	return b.String()
}

func iemsiNewUser(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) bool {
	return line != nil && line.EmsiSession && g != nil && g.RaConfig.EMSINewUser
}

func applyIEMSINewUser(u *cfgrec.User, em cfgrec.IEMSIUser) {
	crt := pascal.UpCase(em.CrtDef)
	req := pascal.UpCase(em.Requests)
	setBit := func(attr *byte, bit byte, on bool) {
		if on {
			*attr |= bit
		} else {
			*attr &^= bit
		}
	}
	setBit(&u.Attribute, cfgrec.UserANSI, strings.Contains(crt, "ANSI"))
	setBit(&u.Attribute2, cfgrec.User2Avatar, strings.Contains(crt, "AVT0"))
	if u.Attribute2&cfgrec.User2Avatar != 0 {
		u.Attribute |= cfgrec.UserANSI
	}
	setBit(&u.Attribute, cfgrec.UserMore, strings.Contains(req, "MORE"))
	setBit(&u.Attribute, cfgrec.UserClrScr, strings.Contains(req, "CLR"))
	if loc := pascal.Trim(em.Location); loc != "" {
		u.Location = loc
	}
	if h := pascal.Trim(em.Alias); h != "" {
		u.Handle = h
	}
	if v := pascal.Trim(em.VoiceNr); v != "" {
		u.VoicePhone = v
	}
	if d := pascal.Trim(em.DataNr); d != "" {
		u.DataPhone = d
	}
	if bd, ok := iemsiBirthDate(em.BirthDate); ok {
		u.BirthDate = bd
	}
}

func iemsiBirthDate(hex string) (string, bool) {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return "", false
	}
	v, err := strconv.ParseInt(hex, 16, 64)
	if err != nil || v <= 0 {
		return "", false
	}
	tm := time.Unix(v, 0).Local()
	if tm.Year() < 1900 {
		return "", false
	}
	return tm.Format("01-02-06"), true
}

// buildICI is used by tests to assemble a client ICI packet.
func buildICI(u cfgrec.IEMSIUser) []byte {
	fields := []string{
		u.Name, u.Alias, u.Location, u.DataNr, u.VoiceNr, u.Password,
		u.BirthDate, u.CrtDef, u.Protocols, u.Capabilities, u.Requests,
		u.Software, u.XlatTable,
	}
	var data []byte
	for _, f := range fields {
		data = append(data, '{')
		data = append(data, []byte(slashIEMSI(f))...)
		data = append(data, '}')
	}
	hexLen := fmt.Sprintf("%04X", len(data))
	body := append([]byte("EMSI_ICI"+hexLen), data...)
	sum := crc.CRC32(body, 0xFFFFFFFF)
	frame := append([]byte("**"+string(body)), []byte(fmt.Sprintf("%08X", sum))...)
	return append(frame, '\r')
}
