package config

import (
	"os"
	"path/filepath"
	"strings"

	"elebbs/internal/cfgrec"
)

func sysInfoPath(g *cfgrec.GlobalCfg) string {
	if g == nil {
		return "sysinfo.bbs"
	}
	root := strings.TrimRight(g.RaConfig.SysPath, `\/`)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, "sysinfo.bbs")
}

func ReadSysInfo(g *cfgrec.GlobalCfg) cfgrec.SysInfo {
	b, err := os.ReadFile(sysInfoPath(g))
	if err != nil || len(b) < 4 {
		return cfgrec.SysInfo{}
	}
	if len(b) < cfgrec.SysInfoSize {
		tmp := make([]byte, cfgrec.SysInfoSize)
		copy(tmp, b)
		b = tmp
	}
	return cfgrec.ParseSysInfo(b)
}

func WriteSysInfo(g *cfgrec.GlobalCfg, s cfgrec.SysInfo) error {
	return os.WriteFile(sysInfoPath(g), cfgrec.EncodeSysInfo(s), 0644)
}

func BumpSysInfoCalls(g *cfgrec.GlobalCfg, line *cfgrec.LineCfg) {
	if line == nil {
		return
	}
	line.SysInfo.TotalCalls++
	if line.LoggedOn {
		line.SysInfo.LastCaller = line.User.Name
		line.SysInfo.LastHandle = line.User.Handle
	}
	_ = WriteSysInfo(g, line.SysInfo)
}
