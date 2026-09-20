package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
)

func Path(g *cfgrec.GlobalCfg, node int) string {
	p := g.RaConfig.LogFileName
	p = strings.ReplaceAll(p, "*N", fmt.Sprintf("%d", node))
	p = strings.ReplaceAll(p, "*n", fmt.Sprintf("%d", node))
	if p == "" {
		p = filepath.Join(g.RaConfig.SysPath, fmt.Sprintf("elebbs%d.log", node))
	}
	return p
}

func Write(g *cfgrec.GlobalCfg, node int, kind byte, msg string) {
	f, err := os.OpenFile(Path(g, node), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("%s  %c  %s\r\n", time.Now().Format("01-02-2006 15:04:05"), kind, msg)
	_, _ = f.WriteString(line)
}
