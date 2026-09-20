package userbase

import (
	"testing"

	"elebbs/internal/cfgrec"
)

func TestNewDefaultsProtocol(t *testing.T) {
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.NewSecurity = 10
	g.RaConfig.NewCredit = 0
	u := NewDefaults(g)
	if u.DefaultProto != '@' {
		t.Fatalf("DefaultProto=%q want @", u.DefaultProto)
	}
	enc := cfgrec.EncodeUser(u)
	got := cfgrec.ParseUser(enc, 0)
	if got.DefaultProto != '@' {
		t.Fatalf("roundtrip DefaultProto=%q", got.DefaultProto)
	}
}
