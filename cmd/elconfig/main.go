package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/crc"
	"elebbs/internal/pascal"
	"elebbs/internal/tui"
)

func main() {
	args := strings.ToUpper(strings.Join(os.Args[1:], " "))
	if strings.Contains(args, "?") {
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
		drawChrome(scr)
		pw := scr.Prompt(32, 13, 16, "", "")
		if crc.RA(pw, true) != crc.RA(g.RaConfig.KeyboardPwd, true) && !strings.EqualFold(pw, g.RaConfig.KeyboardPwd) {
			return
		}
	}

	if strings.Contains(args, "-L") || strings.Contains(args, "/L") {
		editLanguage(scr, g)
		return
	}

	startMenu(scr, g)
}

func help() string {
	return `* EleBBS CONFIG - Command line parameters

-L                     - Edit languages
-M                     - Edit menus
-B                     - Force black & white (monochrome) mode
-N                     - No dupe msg/file area number checking

`
}

func drawChrome(scr *tui.Screen) {
	scr.Cls()
	scr.Bar(1, tui.Norm, ' ', 80)
	scr.At(2, 1, tui.Norm, " File ")
	scr.At(10, 1, tui.Norm, " System ")
	scr.At(20, 1, tui.Norm, " Options ")
	scr.At(32, 1, tui.Norm, " Modem ")
	scr.At(42, 1, tui.Norm, " Manager ")
	scr.Bar(2, 0x07, '═', 80)
	scr.Bar(24, 0x07, '─', 80)
	scr.At(2, 25, tui.Norm, "F1=help  Alt-Z=shell  Esc=exit")
	scr.At(22, 12, tui.Hi, " "+cfgrec.FullProgName+" CONFIG "+cfgrec.VersionID+" ")
	scr.At(22, 14, tui.Hi, " Copyright (C) 1996-2003 Maarten Bekers ")
	scr.At(22, 16, tui.Hi, " All rights reserved. ")
}

func startMenu(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	dirty := false
	tn := config.LoadTelnet(g)
	tnDirty := false
	for {
		drawChrome(scr)
		choice := tui.Menu(scr, 5, 3, "", []string{
			"File", "System", "Options", "Modem", "Manager", "Exit",
		}, 0)
		switch choice {
		case 0:
			var quit bool
			dirty, quit = fileMenu(scr, g, dirty, tn, tnDirty)
			if quit {
				return
			}
		case 1:
			systemMenu(scr, g)
			dirty = true
		case 2:
			optionsMenu(scr, g)
			dirty = true
		case 3:
			if modemMenu(scr, g, &tn) {
				tnDirty = true
			}
		case 4:
			managerInfo(scr, g)
		default:
			if (dirty || tnDirty) && !confirm(scr, "Save configuration before exit?") {
				return
			}
			if dirty {
				saveConfig(scr, g)
			}
			if tnDirty {
				if err := config.SaveTelnet(g, tn); err != nil {
					scr.At(2, 22, 0x0C, err.Error())
					scr.ReadKey()
				}
			}
			return
		}
	}
}

func fileMenu(scr *tui.Screen, g *cfgrec.GlobalCfg, dirty bool, tn cfgrec.TelnetCfg, tnDirty bool) (bool, bool) {
	c := tui.Menu(scr, 6, 4, "File", []string{"Save", "Info", "DOS Shell", "Exit"}, 0)
	switch c {
	case 0:
		saveConfig(scr, g)
		if tnDirty {
			_ = config.SaveTelnet(g, tn)
		}
		return false, false
	case 1:
		about(scr)
	case 2:
		return dirty, false
	case 3:
		if dirty {
			saveConfig(scr, g)
		}
		if tnDirty {
			_ = config.SaveTelnet(g, tn)
		}
		return false, true
	}
	return dirty, false
}

func systemMenu(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	c := tui.Menu(scr, 12, 4, "System", []string{"Paths", "Site info", "Security", "Prompts"}, 0)
	switch c {
	case 0:
		editFields(scr, g, "Paths", []field{
			{"Menu path", &g.RaConfig.MenuPath, 60},
			{"Text path", &g.RaConfig.TextPath, 60},
			{"Msg base", &g.RaConfig.MsgBasePath, 60},
			{"System path", &g.RaConfig.SysPath, 60},
			{"File base", &g.RaConfig.FileBase, 60},
		})
	case 1:
		editFields(scr, g, "Site info", []field{
			{"System name", &g.RaConfig.SystemName, 30},
			{"Sysop", &g.RaConfig.Sysop, 35},
			{"Location", &g.RaConfig.Location, 40},
		})
	case 2:
		editNum(scr, g, "New user security", &g.RaConfig.NewSecurity)
		editNum(scr, g, "Password tries", &g.RaConfig.PasswordTries)
		s := g.RaConfig.KeyboardPwd
		s = scr.Prompt(10, 14, 15, "Keyboard password: ", s)
		g.RaConfig.KeyboardPwd = s
	case 3:
		editFields(scr, g, "Prompts", []field{
			{"Logon prompt", &g.RaConfig.LogonPrompt, 40},
			{"Language prompt", &g.RaConfig.LanguagePrompt, 40},
		})
		g.ElConfig.CapitalizeUsername = confirmOnce(scr, "Capitalize usernames (CONFIG.ELE)?", g.ElConfig.CapitalizeUsername)
	}
}

func optionsMenu(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	c := tui.Menu(scr, 22, 4, "Options", []string{"New users", "Display", "One-word names", "Ask handle"}, 0)
	switch c {
	case 0:
		editNum(scr, g, "New user security", &g.RaConfig.NewSecurity)
		editByte(scr, g, "Min password length", &g.RaConfig.MinPwdLen)
		editByte(scr, g, "New user language", &g.RaConfig.NewUserLang)
	case 1:
		editByte(scr, g, "ANSI (0=Yes 1=No 2=Ask 3=Only)", &g.RaConfig.ANSI)
		editNum(scr, g, "Page length", &g.RaConfig.PageLength)
	case 2:
		g.RaConfig.OneWord = confirmOnce(scr, "Allow one-word names?", g.RaConfig.OneWord)
	case 3:
		g.RaConfig.AskHandle = confirmOnce(scr, "Ask new users for a handle?", g.RaConfig.AskHandle)
	}
}

func modemMenu(scr *tui.Screen, g *cfgrec.GlobalCfg, tn *cfgrec.TelnetCfg) bool {
	c := tui.Menu(scr, 36, 4, "Modem", []string{"Telnet program path", "Node directories", "Telnet port"}, 0)
	if c < 0 {
		return false
	}
	switch c {
	case 0:
		tn.ProgramPath = scr.Prompt(8, 14, 50, "Program path: ", tn.ProgramPath)
	case 1:
		tn.NodeDirectories = scr.Prompt(8, 14, 50, "Node dirs: ", tn.NodeDirectories)
	case 2:
		s := scr.Prompt(8, 14, 6, "Port: ", strconv.Itoa(int(tn.ServerPort)))
		if n, err := strconv.Atoi(s); err == nil {
			tn.ServerPort = int32(n)
		}
	}
	return true
}

func managerInfo(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	scr.Box(18, 8, 62, 18, tui.Blue, "Manager")
	scr.At(22, 10, tui.Blue, "Message/file areas, menus, languages")
	scr.At(22, 12, tui.Blue, "and events are edited with EleMGR /")
	scr.At(22, 13, tui.Blue, "the original ELCONFIG area editors.")
	scr.At(22, 15, tui.Blue, "This port edits CONFIG.RA / CONFIG.ELE.")
	scr.ReadKey()
}

func about(scr *tui.Screen) {
	scr.Box(18, 8, 62, 18, tui.Blue, "Info")
	scr.At(22, 11, tui.Blue, cfgrec.FullProgName+" CONFIG "+cfgrec.VersionID)
	scr.At(22, 13, tui.Blue, "Copyright 1996-2003 Maarten Bekers")
	scr.ReadKey()
}

func editLanguage(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	lang := config.LoadLanguage(g, 0)
	editFields(scr, g, "Language", []field{
		{"Name", &lang.Name, 20},
		{"RAL file", &lang.DefName, 60},
		{"Menu path", &lang.MenuPath, 60},
		{"Text path", &lang.TextPath, 60},
	})
}

type field struct {
	label string
	val   *string
	max   int
}

func editFields(scr *tui.Screen, g *cfgrec.GlobalCfg, title string, fields []field) {
	_ = g
	scr.Cls()
	drawChrome(scr)
	scr.Box(4, 6, 76, 8+len(fields), tui.Blue, title)
	for i, f := range fields {
		*f.val = scr.Prompt(6, 8+i, f.max, padLab(f.label)+": ", *f.val)
	}
}

func editNum(scr *tui.Screen, g *cfgrec.GlobalCfg, title string, v *uint16) {
	_ = g
	s := scr.Prompt(10, 14, 6, title+": ", strconv.Itoa(int(*v)))
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n < 65536 {
		*v = uint16(n)
	}
}

func editByte(scr *tui.Screen, g *cfgrec.GlobalCfg, title string, v *byte) {
	_ = g
	s := scr.Prompt(10, 14, 4, title+": ", strconv.Itoa(int(*v)))
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n < 256 {
		*v = byte(n)
	}
}

func padLab(s string) string {
	if len(s) >= 16 {
		return s
	}
	return s + strings.Repeat(" ", 16-len(s))
}

func confirm(scr *tui.Screen, q string) bool {
	return confirmOnce(scr, q, true)
}

func confirmOnce(scr *tui.Screen, q string, def bool) bool {
	hint := " [Y/n] "
	if !def {
		hint = " [y/N] "
	}
	scr.At(8, 22, tui.Cyan, q+hint)
	k := scr.ReadKey()
	switch k.Ch {
	case 'Y', 'y':
		return true
	case 'N', 'n':
		return false
	case 13:
		return def
	}
	return def
}

func saveConfig(scr *tui.Screen, g *cfgrec.GlobalCfg) {
	g.RaConfig.SyncRaw()
	path := g.CfgPath
	if path == "" {
		path = "CONFIG.RA"
	}
	if err := os.WriteFile(path, g.RaConfig.Raw, 0644); err != nil {
		scr.At(2, 22, 0x0C, err.Error())
		scr.ReadKey()
		return
	}
	ele := g.ElePath
	if ele == "" {
		ele = filepath.Join(filepath.Dir(path), "CONFIG.ELE")
		g.ElePath = ele
	}
	if g.ElConfig.VersionID == 0 {
		g.ElConfig.VersionID = cfgrec.EleVersID
	}
	if err := os.WriteFile(ele, cfgrec.EncodeEleConfig(g.ElConfig), 0644); err != nil {
		scr.At(2, 22, 0x0C, err.Error())
		scr.ReadKey()
		return
	}
	scr.At(2, 22, 0x0A, "Saved "+path+" and "+ele+"  "+pascal.Trim(g.RaConfig.SystemName))
	scr.ReadKey()
}
