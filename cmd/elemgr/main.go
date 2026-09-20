package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/crc"
	"elebbs/internal/files"
	"elebbs/internal/pascal"
	"elebbs/internal/tui"
	"elebbs/internal/userbase"
)

func main() {
	opt := strings.ToUpper(strings.Join(os.Args[1:], " "))
	if strings.Contains(opt, "?") {
		fmt.Print(help())
		os.Exit(255)
	}

	g, err := config.Load("", bbs.ExeDir())
	if err != nil {
		fmt.Println("* " + err.Error())
		os.Exit(255)
	}

	scr := tui.New()
	defer scr.Close()

	if cfgrec.HasKeyboardPwd(g.RaConfig.KeyboardPwd) {
		scr.Cls()
		header(scr)
		pw := scr.Prompt(20, 12, 16, "Password: ", "")
		if crc.RA(pw, true) != crc.RA(g.RaConfig.KeyboardPwd, true) && !strings.EqualFold(pw, g.RaConfig.KeyboardPwd) {
			return
		}
	}

	directUser := strings.Contains(opt, "-U") || strings.Contains(opt, "/U")
	directFile := strings.Contains(opt, "-F") || strings.Contains(opt, "/F")
	if directUser {
		userEditor(scr, g)
		return
	}
	if directFile {
		fileMgr(scr, g)
		return
	}

	for {
		scr.Cls()
		header(scr)
		choice := tui.Menu(scr, 33, 9, "Manager", []string{
			"Files", "Users", "Info", "DOS Shell", "Exit",
		}, 0)
		switch choice {
		case 0:
			fileMgr(scr, g)
		case 1:
			userEditor(scr, g)
		case 2:
			about(scr)
		case 3:
			scr.Close()
			c := exec.Command("cmd")
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			_ = c.Run()
			scr = tui.New()
		default:
			return
		}
	}
}

func header(scr *tui.Screen) {
	scr.Bar(1, tui.Title, ' ', 80)
	scr.At(1, 1, tui.Title, cfgrec.FullProgName+" MANAGER v"+cfgrec.VersionID)
	scr.At(45, 1, tui.Title, "Copyright 1997-2003 Maarten Bekers")
	scr.Bar(2, 0x07, '═', 80)
	scr.Bar(24, 0x07, '─', 80)
	scr.At(1, 25, tui.Norm, "Enter=select  Esc=back  Ins=add  Del=delete")
}

func about(scr *tui.Screen) {
	scr.Box(22, 9, 62, 18, tui.Blue, "About "+cfgrec.FullProgName+" Manager")
	scr.At(26, 11, tui.Blue, cfgrec.FullProgName+" Manager "+cfgrec.VersionID)
	scr.At(26, 13, tui.Blue, "Copyright 1997-2003")
	scr.At(26, 14, tui.Blue, "Maarten Bekers.")
	scr.At(26, 16, tui.Blue, "Go/Win32 port — All Rights Reserved")
	scr.ReadKey()
}

func help() string {
	return `* EleBBS MANAGER - Command line parameters

-U                     - Go directly to the usermanager
-F                     - Start directly in the filebase manager
-N                     - Don't verify the filepaths exist
-SIMPLE                - Use a very basic character set for the screen

`
}

func userEditor(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	users, err := userbase.List(g)
	if err != nil {
		scr.At(2, 12, 0x0C, err.Error())
		scr.ReadKey()
		return
	}
	top, sel := 0, 0
	const rows = 20
	for {
		scr.Cls()
		header(scr)
		scr.At(1, 3, tui.Norm, " Name                                Location                     #    Sec   Del")
		scr.Bar(4, 0x07, '═', 80)
		if sel < top {
			top = sel
		}
		if sel >= top+rows {
			top = sel - rows + 1
		}
		if top < 0 {
			top = 0
		}
		for i := 0; i < rows; i++ {
			idx := top + i
			if idx >= len(users) {
				scr.At(1, 5+i, tui.Norm, strings.Repeat(" ", 80))
				continue
			}
			u := users[idx]
			del := " "
			if u.Deleted() {
				del = "■"
			}
			line := fmt.Sprintf(" %-34s %-28s %-4d %-5d %s", clip(u.Name, 34), clip(u.Location, 28), u.Record, u.Security, del)
			attr := byte(tui.Norm)
			if idx == sel {
				attr = tui.Hi
			}
			scr.At(1, 5+i, attr, pad80(line))
		}
		k := scr.ReadKey()
		switch k.Name {
		case "esc":
			return
		case "up":
			if sel > 0 {
				sel--
			}
		case "down":
			if sel < len(users)-1 {
				sel++
			}
		case "pgup":
			sel -= rows
			if sel < 0 {
				sel = 0
			}
		case "pgdn":
			sel += rows
			if sel >= len(users) {
				sel = len(users) - 1
			}
		case "ins":
			u := userbase.NewDefaults(g)
			if editUser(scr, g, &u, true) {
				if nu, err := userbase.Append(g, u); err == nil {
					users = append(users, nu)
					sel = len(users) - 1
				}
			}
		case "del":
			if len(users) == 0 {
				break
			}
			users[sel].Attribute ^= cfgrec.UserDeleted
			_ = userbase.WriteAndIndex(g, users[sel])
		case "enter":
			if len(users) == 0 {
				break
			}
			u := users[sel]
			if editUser(scr, g, &u, false) {
				_ = userbase.WriteAndIndex(g, u)
				users[sel] = u
			}
		default:
			if k.Ch >= 32 {
				for i, u := range users {
					if strings.HasPrefix(pascal.UpCase(u.Name), pascal.UpCase(string(k.Ch))) {
						sel = i
						break
					}
				}
			}
		}
		if sel < 0 {
			sel = 0
		}
	}
}

func editUser(scr *tui.Screen, g *cfgrec.GlobalCfg, u *cfgrec.User, isNew bool) bool {
	type field struct {
		label string
		get   func() string
		set   func(string)
	}
	fields := []field{
		{"Name", func() string { return u.Name }, func(s string) { u.Name = s }},
		{"Handle", func() string { return u.Handle }, func(s string) { u.Handle = s }},
		{"Location", func() string { return u.Location }, func(s string) { u.Location = s }},
		{"Security", func() string { return fmt.Sprintf("%d", u.Security) }, func(s string) { u.Security = u16(s) }},
		{"Password", func() string { return "(hidden)" }, func(s string) {
			if s != "" && s != "(hidden)" {
				u.Password = s
				u.PasswordCRC = crc.RA(s, true)
			}
		}},
		{"Voice #", func() string { return u.VoicePhone }, func(s string) { u.VoicePhone = s }},
		{"Data #", func() string { return u.DataPhone }, func(s string) { u.DataPhone = s }},
		{"Credit", func() string { return fmt.Sprintf("%d", u.Credit) }, func(s string) { u.Credit = i32(s) }},
		{"Calls", func() string { return fmt.Sprintf("%d", u.NoCalls) }, func(s string) { u.NoCalls = i32(s) }},
		{"Msg area", func() string { return fmt.Sprintf("%d", u.MsgArea) }, func(s string) { u.MsgArea = u16(s) }},
		{"File area", func() string { return fmt.Sprintf("%d", u.FileArea) }, func(s string) { u.FileArea = u16(s) }},
		{"Protocol", func() string {
			if u.DefaultProto == 0 {
				return ""
			}
			return string([]byte{u.DefaultProto})
		}, func(s string) {
			s = strings.TrimSpace(s)
			if s == "" {
				u.DefaultProto = cfgrec.DefaultTransferProto
				return
			}
			u.DefaultProto = pascal.UpCase(s)[0]
		}},
		{"Group", func() string { return fmt.Sprintf("%d", u.Group) }, func(s string) { u.Group = u16(s) }},
		{"Comment", func() string { return u.Comment }, func(s string) { u.Comment = s }},
	}
	cur := 0
	for {
		scr.Cls()
		header(scr)
		title := "Edit user"
		if isNew {
			title = "New user"
		}
		scr.Box(8, 5, 72, 22, tui.Blue, title)
		for i, f := range fields {
			attr := byte(tui.Blue)
			if i == cur {
				attr = tui.Hi
			}
			scr.At(12, 6+i, attr, fmt.Sprintf("%-12s %s", f.label, clip(f.get(), 40)))
		}
		scr.At(12, 21, tui.Blue, "Enter=edit  Esc=save  A=ANSI  D=deleted")
		k := scr.ReadKey()
		switch k.Name {
		case "esc":
			return u.Name != ""
		case "up":
			if cur > 0 {
				cur--
			}
		case "down":
			if cur < len(fields)-1 {
				cur++
			}
		case "enter":
			v := scr.Prompt(12, 21, 40, fields[cur].label+": ", fields[cur].get())
			if fields[cur].label != "Password" || v != "(hidden)" {
				fields[cur].set(v)
			}
		default:
			switch k.Ch {
			case 'A', 'a':
				u.Attribute ^= cfgrec.UserANSI
			case 'D', 'd':
				u.Attribute ^= cfgrec.UserDeleted
			}
		}
	}
}

func fileMgr(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	areas := files.LoadAreas(g)
	if len(areas) == 0 {
		scr.At(2, 12, 0x0C, "FILES.RA not found")
		scr.ReadKey()
		return
	}
	top, sel := 0, 0
	const rows = 18
	for {
		scr.Cls()
		header(scr)
		scr.At(1, 3, tui.Norm, "  #    Name                                     Group  Path")
		scr.Bar(4, 0x07, '═', 80)
		if sel < top {
			top = sel
		}
		if sel >= top+rows {
			top = sel - rows + 1
		}
		for i := 0; i < rows; i++ {
			idx := top + i
			if idx >= len(areas) {
				continue
			}
			a := areas[idx]
			line := fmt.Sprintf(" %4d  %-40s %5d  %s", a.AreaNum, clip(a.Name, 40), a.Group, clip(a.FilePath, 22))
			attr := byte(tui.Norm)
			if idx == sel {
				attr = tui.Hi
			}
			scr.At(1, 5+i, attr, pad80(line))
		}
		scr.At(1, 24, tui.Norm, "Enter=files  I=index  C=compress  Esc=back")
		k := scr.ReadKey()
		switch k.Name {
		case "esc":
			return
		case "up":
			if sel > 0 {
				sel--
			}
		case "down":
			if sel < len(areas)-1 {
				sel++
			}
		case "pgup":
			sel -= rows
			if sel < 0 {
				sel = 0
			}
		case "pgdn":
			sel += rows
			if sel >= len(areas) {
				sel = len(areas) - 1
			}
		case "enter":
			fileList(scr, g, areas[sel])
		default:
			switch k.Ch {
			case 'I', 'i':
				_ = files.RebuildIndex(g, areas[sel])
			case 'C', 'c':
				_, _ = files.Compress(g, areas[sel])
			}
		}
	}
}

func fileList(scr *tui.Screen, g *cfgrec.GlobalCfg, area cfgrec.FilesArea) {
	ents, _ := files.ReadFDB(g, area)
	top, sel := 0, 0
	const rows = 16
	for {
		scr.Cls()
		header(scr)
		scr.At(1, 3, tui.Title, fmt.Sprintf(" Area %d - %s", area.AreaNum, area.Name))
		scr.At(1, 4, tui.Norm, " Name         Size      Uploader             DL   Attr")
		scr.Bar(5, 0x07, '═', 80)
		if len(ents) == 0 {
			scr.At(2, 8, tui.Norm, "(empty — Ins to add)")
		}
		if sel < top {
			top = sel
		}
		if sel >= top+rows && len(ents) > 0 {
			top = sel - rows + 1
		}
		for i := 0; i < rows; i++ {
			idx := top + i
			if idx >= len(ents) {
				continue
			}
			e := ents[idx]
			attrch := " "
			if e.Hdr.Deleted() {
				attrch = "D"
			} else if e.Hdr.Locked() {
				attrch = "L"
			} else if e.Hdr.Missing() {
				attrch = "M"
			}
			line := fmt.Sprintf(" %-12s %8d  %-20s %4d  %s", clip(e.Hdr.Name, 12), e.Hdr.Size, clip(e.Hdr.Uploader, 20), e.Hdr.TimesDL, attrch)
			a := byte(tui.Norm)
			if idx == sel {
				a = tui.Hi
			}
			scr.At(1, 6+i, a, pad80(line))
		}
		scr.At(1, 23, tui.Norm, "Ins=add  Del=kill  L=lock  E=edit desc  Esc=back")
		k := scr.ReadKey()
		switch k.Name {
		case "esc":
			return
		case "up":
			if sel > 0 {
				sel--
			}
		case "down":
			if sel < len(ents)-1 {
				sel++
			}
		case "ins":
			name := scr.Prompt(2, 24, 40, "File spec: ", "")
			if name != "" {
				_ = files.Add(g, area, name, g.RaConfig.Sysop, "")
				ents, _ = files.ReadFDB(g, area)
			}
		case "del":
			if len(ents) == 0 {
				break
			}
			ents[sel].Hdr.Attrib |= cfgrec.AttrDeleted
			_ = files.WriteFDB(g, area, ents)
		case "enter":
			if len(ents) == 0 {
				break
			}
			d := scr.Prompt(2, 24, 50, "Desc: ", ents[sel].Desc)
			ents[sel].Desc = d
			_ = files.WriteFDB(g, area, ents)
		default:
			if len(ents) == 0 {
				break
			}
			switch k.Ch {
			case 'L', 'l':
				ents[sel].Hdr.Attrib ^= cfgrec.AttrLocked
				_ = files.WriteFDB(g, area, ents)
			case 'E', 'e':
				d := scr.Prompt(2, 24, 50, "Desc: ", ents[sel].Desc)
				ents[sel].Desc = d
				_ = files.WriteFDB(g, area, ents)
			}
		}
	}
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func pad80(s string) string {
	if len(s) >= 80 {
		return s[:80]
	}
	return s + strings.Repeat(" ", 80-len(s))
}

func u16(s string) uint16 {
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n < 0 {
		n = 0
	}
	return uint16(n)
}

func i32(s string) int32 {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return int32(n)
}
