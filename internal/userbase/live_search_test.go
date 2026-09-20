package userbase

import (
	"os"
	"path/filepath"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
)

func TestSearchShuratoLive(t *testing.T) {
	msg := `C:\Users\Owner\OneDrive\ele\MSGBASE`
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.MsgBasePath = msg
	g.CfgPath = filepath.Join(filepath.Dir(msg), "CONFIG.RA")
	st, err := os.Stat(filepath.Join(msg, cfgrec.UserBaseName))
	if err != nil {
		t.Skip(err)
	}
	t.Logf("path=%s size=%d stride=%d records=%d", Path(g, cfgrec.UserBaseName), st.Size(), cfgrec.UsersSize, st.Size()/int64(cfgrec.UsersSize))
	for _, name := range []string{"Shurato", "shurato", "SHURATO", "Martin Kazmaier", "martin kazmaier"} {
		u, ok := Search(g, name)
		if !ok {
			t.Errorf("Search(%q) missed", name)
			continue
		}
		t.Logf("Search(%q) -> %q / %q rec=%d", name, u.Name, u.Handle, u.Record)
	}

	raw, err := os.ReadFile(g.CfgPath)
	if err != nil {
		t.Log("no CONFIG.RA", err)
		return
	}
	c := cfgrec.ParseConfig(raw)
	t.Logf("CONFIG Sysop=%q OneWord=%v MsgBase=%q SysPath=%q", c.Sysop, c.OneWord, c.MsgBasePath, c.SysPath)
	cfg, err := config.Load(filepath.Dir(msg), filepath.Dir(msg))
	if err != nil {
		t.Log("Load", err)
		return
	}
	t.Logf("Load MsgBase=%q Path=%s", cfg.RaConfig.MsgBasePath, Path(cfg, cfgrec.UserBaseName))
	if _, ok := Search(cfg, "Shurato"); !ok {
		t.Error("Search via Load() missed Shurato")
	}
}
