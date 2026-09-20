package config

import (
	"os"
	"path/filepath"

	"elebbs/internal/cfgrec"
)

func sysFile(g *cfgrec.GlobalCfg, names ...string) string {
	if g == nil {
		return ""
	}
	dir := g.RaConfig.SysPath
	for _, n := range names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if len(names) > 0 {
		return filepath.Join(dir, names[0])
	}
	return ""
}

func readRecs(path string, size int) [][]byte {
	b, err := os.ReadFile(path)
	if err != nil || size <= 0 {
		return nil
	}
	n := len(b) / size
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = b[i*size : (i+1)*size]
	}
	return out
}

func MessageAreaName(g *cfgrec.GlobalCfg, areaNum uint16) string {
	if areaNum == 0 {
		return ""
	}
	for _, rec := range readRecs(sysFile(g, "messages.ra", "MESSAGES.RA"), cfgrec.MessageSize) {
		a := cfgrec.ParseMessageArea(rec)
		if a.AreaNum == areaNum {
			return a.Name
		}
	}
	return ""
}

func FileAreaName(g *cfgrec.GlobalCfg, areaNum uint16) string {
	if areaNum == 0 {
		return ""
	}
	for _, rec := range readRecs(sysFile(g, "files.ra", "FILES.RA"), cfgrec.FilesRecSize) {
		a := cfgrec.ParseFilesArea(rec)
		if a.AreaNum == areaNum {
			return a.Name
		}
	}
	return ""
}

func MessageAreaIndex(g *cfgrec.GlobalCfg, areaNum uint16) int {
	if areaNum == 0 {
		return 0
	}
	recs := readRecs(sysFile(g, "messages.ra", "MESSAGES.RA"), cfgrec.MessageSize)
	for i, rec := range recs {
		if cfgrec.ParseMessageArea(rec).AreaNum == areaNum {
			return i + 1
		}
	}
	return 0
}

func FileAreaIndex(g *cfgrec.GlobalCfg, areaNum uint16) int {
	if areaNum == 0 {
		return 0
	}
	recs := readRecs(sysFile(g, "files.ra", "FILES.RA"), cfgrec.FilesRecSize)
	for i, rec := range recs {
		if cfgrec.ParseFilesArea(rec).AreaNum == areaNum {
			return i + 1
		}
	}
	return 0
}

func groupName(path string, num uint16) string {
	if num == 0 {
		return ""
	}
	recs := readRecs(path, cfgrec.GroupSize)
	for _, rec := range recs {
		gr := cfgrec.ParseGroup(rec)
		if gr.AreaNum == num {
			return gr.Name
		}
	}
	if int(num) >= 1 && int(num) <= len(recs) {
		return cfgrec.ParseGroup(recs[num-1]).Name
	}
	return ""
}

func MessageGroupName(g *cfgrec.GlobalCfg, groupNum uint16) string {
	return groupName(sysFile(g, "mgroups.ra", "MGROUPS.RA"), groupNum)
}

func FileGroupName(g *cfgrec.GlobalCfg, groupNum uint16) string {
	return groupName(sysFile(g, "fgroups.ra", "FGROUPS.RA"), groupNum)
}
