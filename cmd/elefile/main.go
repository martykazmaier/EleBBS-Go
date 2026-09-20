package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"elebbs/internal/bbs"
	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/files"
	"elebbs/internal/logx"
)

func main() {
	fmt.Println("ELEFILE; EleBBS File Database maintenance utility, Version " + cfgrec.VersionID)
	fmt.Println("         Copyright 1997-2003 Maarten Bekers, All rights reserved.")
	fmt.Println("         Go/Win32 port.")
	fmt.Println()

	args := os.Args[1:]
	if len(args) == 0 || strings.Contains(strings.Join(args, " "), "?") && (len(args) == 1 || strings.Contains(args[0], "?")) {
		help()
		os.Exit(255)
	}

	g, err := config.Load("", bbs.ExeDir())
	if err != nil {
		fmt.Println(" * " + err.Error())
		os.Exit(255)
	}

	cmd := strings.ToUpper(args[0])
	rest := args[1:]
	logx.Write(g, 0, '>', "ELEFILE "+cmd)
	fmt.Println(cmd)
	fmt.Println(strings.Repeat("═", 80))

	areas := files.LoadAreas(g)
	if len(areas) == 0 {
		fatal("FILES.RA not found or empty")
	}

	var ok bool
	switch cmd {
	case "ADD":
		if len(rest) < 2 {
			fatal("ADD <area#> <name> [uploader_name] [description]")
		}
		a := mustArea(areas, rest[0])
		up := ""
		desc := ""
		if len(rest) > 2 {
			up = rest[2]
		}
		if len(rest) > 3 {
			desc = strings.Join(rest[3:], " ")
		}
		showArea(a)
		if err := files.Add(g, a, rest[1], up, desc); err != nil {
			fatal(err.Error())
		}
		ok = true
	case "INDEX":
		spec := specOf(rest)
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			if err := files.RebuildIndex(g, a); err != nil {
				files.Status("Area " + itoa(int(a.AreaNum)) + " " + err.Error())
			} else {
				ok = true
			}
		}
	case "CLEAN":
		spec, km := flags(rest)
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			n, err := files.Clean(g, a, km)
			if err != nil {
				files.Status(err.Error())
				continue
			}
			if n > 0 {
				files.Status(fmt.Sprintf("Removed %d entries", n))
			}
			_, _ = files.Compress(g, a)
			ok = true
		}
	case "COMPRESS":
		spec := specOf(rest)
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			n, err := files.Compress(g, a)
			if err != nil {
				files.Status(err.Error())
				continue
			}
			files.Status(fmt.Sprintf("Compressed, removed %d deleted records", n))
			ok = true
		}
	case "EXPORT":
		spec, out, ra := exportArgs(rest)
		if out == "" {
			out = "files.txt"
		}
		ok = files.Export(g, files.SelectAreas(areas, spec), out, ra) == nil
	case "IMPORT":
		if len(rest) < 2 {
			fatal("IMPORT <area#> <input file> [/ERASE] [/Uuploader] [/MISSING]")
		}
		a := mustArea(areas, rest[0])
		in := rest[1]
		erase, missing, up := false, false, ""
		for _, p := range rest[2:] {
			u := strings.ToUpper(p)
			switch {
			case u == "/ERASE":
				erase = true
			case u == "/MISSING":
				missing = true
			case strings.HasPrefix(u, "/U"):
				up = p[2:]
			}
		}
		showArea(a)
		n, err := files.ImportList(g, a, in, up, erase, missing)
		if err != nil {
			fatal(err.Error())
		}
		files.Status(fmt.Sprintf("Imported %d files", n))
		ok = true
	case "KILL", "LOCK", "UNLOCK":
		if len(rest) < 1 {
			fatal(cmd + " <filespec> [area#|@arealist]")
		}
		spec := ""
		if len(rest) > 1 {
			spec = rest[1]
		}
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			n, err := files.SetAttrib(g, a, rest[0], cmd)
			if err != nil {
				files.Status(err.Error())
				continue
			}
			files.Status(fmt.Sprintf("%s %d file(s)", cmd, n))
			ok = true
		}
	case "FILELIST":
		if len(rest) < 1 {
			fatal("FILELIST <output file> [area#] [/Ddays] [/NOHDR] [/7BIT] [/FORMF]")
		}
		out := rest[0]
		spec, days, nohdr, seven, ff := listFlags(rest[1:])
		err := files.FileList(g, files.SelectAreas(areas, spec), out, days, nohdr, seven, ff)
		if err != nil {
			fatal(err.Error())
		}
		ok = true
	case "SORT":
		spec, byDate, rev := sortFlags(rest)
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			if err := files.Sort(g, a, byDate, rev); err != nil {
				files.Status(err.Error())
				continue
			}
			ok = true
		}
	case "ADOPT":
		if len(rest) < 1 {
			fatal("ADOPT <filespec> [area#|@arealist]")
		}
		spec := ""
		if len(rest) > 1 {
			spec = rest[1]
		}
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			n, err := files.Adopt(g, a, rest[0])
			if err != nil {
				files.Status(err.Error())
				continue
			}
			files.Status(fmt.Sprintf("Adopted %d file(s)", n))
			ok = true
		}
	case "UPDATE":
		if len(rest) < 1 {
			fatal("UPDATE <filespec> [area#] [TOUCH|TOUCHMOD]")
		}
		spec := ""
		touchMod := false
		for _, p := range rest[1:] {
			u := strings.ToUpper(p)
			if u == "TOUCHMOD" {
				touchMod = true
				continue
			}
			if u == "TOUCH" {
				continue
			}
			spec = p
		}
		for _, a := range files.SelectAreas(areas, spec) {
			showArea(a)
			n, err := files.UpdateTimes(g, a, rest[0], touchMod)
			if err != nil {
				files.Status(err.Error())
				continue
			}
			files.Status(fmt.Sprintf("Updated %d file(s)", n))
			ok = true
		}
	case "DESCRIBE":
		files.Status("DESCRIBE (FILE_ID.DIZ extract) is not fully ported; use EleMGR to edit descriptions.")
		ok = true
	case "REARC":
		files.Status("REARC is not ported (needs archive converters).")
	case "HTMLIST":
		spec, days, _, _, _ := listFlags(rest)
		out := "files.htm"
		if err := files.HTMLList(g, files.SelectAreas(areas, spec), out, days); err != nil {
			fatal(err.Error())
		}
		files.Status("Wrote " + out)
		ok = true
	default:
		fatal("Unknown command. Type ELEFILE ? for help")
	}
	if !ok && cmd != "REARC" && cmd != "DESCRIBE" {
		os.Exit(1)
	}
}

func help() {
	fmt.Println("  ADD      <area#> <name> [uploader_name] [description]")
	fmt.Println("  INDEX    [area#|@arealist]")
	fmt.Println("  CLEAN    [area#|@arealist] [/KM]")
	fmt.Println("  COMPRESS [area#|@arealist]")
	fmt.Println("  EXPORT   [area#|@arealist] [output file] [/RA]")
	fmt.Println("  IMPORT   [area#|@arealist] [input file] [/ERASE] [/Uuploader_name] [/MISSING] [/Cxx]")
	fmt.Println("  KILL     <filespec> [area#|@arealist]")
	fmt.Println("  LOCK     <filespec> [area#|@arealist]")
	fmt.Println("  UNLOCK   <filespec> [area#|@arealist]")
	fmt.Println("  FILELIST <output file> [area#|@arealist] [/Ssecurity] [/Ddays old]")
	fmt.Println("                         [/Bbanner] [/Ffooter] [/NOHDR] [/7BIT] [/FORMF]")
	fmt.Println("  SORT     [area#|@areallist] [DATE] [REVERSE]  (Default=NAME,FORWARD)")
	fmt.Println("  ADOPT    <filespec> [area#|@arealist]")
	fmt.Println("  UPDATE   <filespec> [area#|@arealist] [TOUCH|TOUCHMOD]")
	fmt.Println("  REARC    [area#|@arealist]")
	fmt.Println("  DESCRIBE [area#|@arealist] [filespec]")
	fmt.Println("  HTMLIST  [area#|@arealist] [/Ssecurity] [/Ddays old]")
	fmt.Println()
	fmt.Println("  [] Parameters are optional, <> Parameters are mandatory.")
	fmt.Println("  (Area#=0) means all areas, wildcards are valid.")
}

func fatal(s string) {
	fmt.Println(" * " + s)
	os.Exit(255)
}

func showArea(a cfgrec.FilesArea) {
	fmt.Printf("Area %5d - %s\n", a.AreaNum, a.Name)
}

func mustArea(areas []cfgrec.FilesArea, spec string) cfgrec.FilesArea {
	sel := files.SelectAreas(areas, spec)
	if len(sel) == 0 {
		fatal("Must specify a valid target area number!")
	}
	return sel[0]
}

func specOf(rest []string) string {
	if len(rest) == 0 {
		return "0"
	}
	return rest[0]
}

func flags(rest []string) (spec string, km bool) {
	spec = "0"
	for _, p := range rest {
		if strings.EqualFold(p, "/KM") {
			km = true
			continue
		}
		spec = p
	}
	return spec, km
}

func exportArgs(rest []string) (spec, out string, ra bool) {
	spec = "0"
	for _, p := range rest {
		u := strings.ToUpper(p)
		if u == "/RA" {
			ra = true
			continue
		}
		if spec == "0" && looksSpec(p) {
			spec = p
			continue
		}
		out = p
	}
	return spec, out, ra
}

func looksSpec(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '@' || p[0] == 'G' || p[0] == 'g' {
		return true
	}
	_, err := strconv.Atoi(strings.Split(p, "-")[0])
	return err == nil
}

func listFlags(rest []string) (spec string, days int, nohdr, seven, ff bool) {
	spec = "0"
	for _, p := range rest {
		u := strings.ToUpper(p)
		switch {
		case u == "/NOHDR":
			nohdr = true
		case u == "/7BIT":
			seven = true
		case u == "/FORMF":
			ff = true
		case strings.HasPrefix(u, "/D"):
			days, _ = strconv.Atoi(p[2:])
		case strings.HasPrefix(u, "/S"):
			// security filter not applied yet
		default:
			if looksSpec(p) {
				spec = p
			}
		}
	}
	return
}

func sortFlags(rest []string) (spec string, byDate, rev bool) {
	spec = "0"
	for _, p := range rest {
		u := strings.ToUpper(p)
		switch u {
		case "DATE":
			byDate = true
		case "REVERSE":
			rev = true
		default:
			if looksSpec(p) {
				spec = p
			}
		}
	}
	return
}

func itoa(n int) string { return strconv.Itoa(n) }
