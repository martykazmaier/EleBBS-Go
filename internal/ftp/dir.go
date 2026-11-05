package ftp

import (
	"strings"

	"elebbs/internal/files"
	"elebbs/internal/pascal"
)

// ftpListArg drops leading FTP LIST flags (-a, -l) the same way origin/main
// did (HasPrefix "-"). Pascal GetPartsFromDir would treat "-a" as FileName.
func ftpListArg(arg string) string {
	arg = strings.TrimSpace(arg)
	for arg != "" && strings.HasPrefix(arg, "-") {
		i := strings.IndexByte(arg, ' ')
		if i < 0 {
			return ""
		}
		arg = strings.TrimSpace(arg[i+1:])
	}
	return arg
}

// breakUpDirStr is Pascal BreakUpDirStr.
func breakUpDirStr(dirName string, listArea, listGroup int) (groupName, areaName, fileName, wild string, startRoot bool, outArea, outGroup int) {
	outArea, outGroup = listArea, listGroup
	dirName = strings.ReplaceAll(dirName, `\`, "/")
	for strings.Contains(dirName, "../") {
		dirName = strings.ReplaceAll(dirName, "../", "")
	}
	for strings.Contains(dirName, "./") {
		dirName = strings.ReplaceAll(dirName, "./", "")
	}
	if strings.HasPrefix(dirName, "/") {
		startRoot = true
	}
	var word [4]string
	rest := strings.Trim(dirName, "/")
	n := 0
	for rest != "" && n < 3 {
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			word[n+1] = rest
			rest = ""
		} else {
			word[n+1] = rest[:i]
			rest = rest[i+1:]
		}
		n++
	}
	for i := 1; i <= 3; i++ {
		if strings.ContainsAny(word[i], "*?") {
			wild = word[i]
			word[i] = ""
		}
	}
	if startRoot {
		outArea, outGroup = -1, -1
	}
	if word[3] != "" {
		groupName, areaName, fileName = word[1], word[2], word[3]
	} else if word[2] != "" {
		if outGroup > 0 {
			areaName, fileName = word[1], word[2]
		} else {
			groupName, areaName = word[1], word[2]
		}
	} else {
		if outGroup > 0 && outArea > 0 {
			fileName = word[1]
		}
		if outGroup > 0 && outArea < 0 {
			areaName = word[1]
		}
		if outGroup < 0 {
			groupName = word[1]
		}
	}
	return groupName, areaName, fileName, wild, startRoot, outArea, outGroup
}

// partsFromDir is Pascal GetPartsFromDir. LIST ignores invalid; CWD does not.
func (ss *session) partsFromDir(param string, listGroup, listArea int) (g, a int, wild string, invalid bool) {
	g, a = listGroup, listArea
	param = strings.TrimSpace(param)
	if param == "" {
		return g, a, "", false
	}
	groupName, areaName, fileName, wild, startRoot, a, g := breakUpDirStr(param, listArea, listGroup)
	if startRoot {
		g, a = -1, -1
	}
	if groupName != "" {
		if ng, ok := ss.searchGroupName(groupName); ok {
			g = ng
		} else {
			invalid = true
		}
	}
	if areaName != "" {
		if na, ok := ss.searchAreaName(areaName); ok {
			a = na
		} else {
			invalid = true
			a = -1
		}
	}
	if wild == "" {
		wild = fileName
	}
	return g, a, wild, invalid
}

func (ss *session) searchGroupName(name string) (int, bool) {
	want := pascal.UpCase(strings.TrimSpace(files.ConvDirName(name)))
	for _, g := range ss.srv.grps {
		if !files.GroupAccess(g, ss.user) {
			continue
		}
		if pascal.UpCase(files.ConvDirName(g.Name)) == want {
			return int(g.AreaNum), true
		}
	}
	return -1, false
}

func (ss *session) searchAreaName(name string) (int, bool) {
	want := pascal.UpCase(strings.TrimSpace(files.ConvDirName(name)))
	for _, a := range ss.srv.areas {
		if !files.CheckFileAreaAccess(a, ss.user, false, false, false, true, 0) {
			continue
		}
		if pascal.UpCase(files.ConvDirName(a.Name)) == want {
			return int(a.AreaNum), true
		}
	}
	return -1, false
}
