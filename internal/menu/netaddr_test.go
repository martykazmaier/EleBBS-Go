package menu

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/mail"
	"elebbs/internal/term"
)

// writeNodelist writes zone 1 with net 2 holding node 3 (cost 7).
func writeNodelist(t *testing.T, dir string) {
	t.Helper()
	zone := "Zone,1,Zone_One,Earth,Ann,1-1,300\r\n"
	text := zone + "Host,2,Net_Two,Town,Bob,1-2,300\r\n,3,Joe_BBS,Village,Joe,1-3,300\r\n"
	if err := os.WriteFile(filepath.Join(dir, "NODELIST.001"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	inc := make([]byte, 17)
	inc[0] = byte(copy(inc[1:], "NODELIST.001"))
	if err := os.WriteFile(filepath.Join(dir, "NODEINC.RA"), inc, 0644); err != nil {
		t.Fatal(err)
	}
	idx := make([]byte, 20)
	idx[0], idx[1], idx[5] = 0, 1, 1
	idx[10], idx[11], idx[13], idx[15] = 2, 2, 7, 1
	binary.LittleEndian.PutUint32(idx[16:], uint32(len(zone)))
	if err := os.WriteFile(filepath.Join(dir, "NODEIDX.RA"), idx, 0644); err != nil {
		t.Fatal(err)
	}
}

func netmailEngine(t *testing.T, in string) (*Engine, *seqStream, cfgrec.MessageArea) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeNodelist(t, dir)
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.Nodelist = dir
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	g.RaConfig.Address[0] = cfgrec.Addr{Zone: 1, Net: 2, Node: 1}
	g.RaConfig.KillSent = 2   // Ask
	g.RaConfig.CrashSec = 200 // above the user: ask
	a := cfgrec.MessageArea{
		AreaNum:   1,
		Name:      "Netmail",
		Typ:       cfgrec.MsgNetMail,
		MsgKinds:  cfgrec.MsgKindPrivate,
		Attribute: 1 << 7,
		JAMBase:   filepath.Join(dir, "net"),
	}
	st := &seqStream{in: []byte(in)}
	line := &cfgrec.LineCfg{User: cfgrec.User{Name: "Bob", Security: 100, Record: -1}}
	return &Engine{T: term.New(st, g, line), G: g, Line: line}, st, a
}

func TestNetmailPostAsksAddress(t *testing.T) {
	// To, address (confirmed), no header change, kill/sent yes, crash yes,
	// one message line, save.
	eng, st, a := netmailEngine(t, "Joe\r3\rynyyHi\r\rS")
	if !eng.writeMessage(a, "Bob", "", "", "hello", nil, false) {
		t.Fatalf("post failed: %q", st.out.Bytes())
	}
	if !strings.Contains(st.out.String(), "1:2/3, Joe BBS, Village, 7 credits") {
		t.Fatalf("no nodelist match shown: %q", st.out.Bytes())
	}
	art, ok := mail.ReadMsg(a.JAMBase, 1)
	if !ok {
		t.Fatal("no posted msg")
	}
	if art.To != "Joe" || art.Dest != "1:2/3" || art.Orig != "1:2/1" {
		t.Fatalf("to %q dest %q orig %q", art.To, art.Dest, art.Orig)
	}
	if !art.KillSent || !art.Crash || !art.Private {
		t.Fatalf("attrs killsent=%v crash=%v priv=%v", art.KillSent, art.Crash, art.Private)
	}
	if !eng.Line.NetMailEntered {
		t.Fatal("NetMailEntered not set")
	}
}

func TestNetmailUnlistedSendAnyway(t *testing.T) {
	eng, st, a := netmailEngine(t, "Joe\r1:2/9\rynnnHi\r\rS")
	if !eng.writeMessage(a, "Bob", "", "", "hello", nil, false) {
		t.Fatalf("post failed: %q", st.out.Bytes())
	}
	art, ok := mail.ReadMsg(a.JAMBase, 1)
	if !ok {
		t.Fatal("no posted msg")
	}
	if art.Dest != "1:2/9" || art.KillSent || art.Crash {
		t.Fatalf("dest %q killsent=%v crash=%v", art.Dest, art.KillSent, art.Crash)
	}
}

func TestNetmailEmptyAddressAborts(t *testing.T) {
	eng, st, a := netmailEngine(t, "Joe\r\r")
	if eng.writeMessage(a, "Bob", "", "", "hello", nil, false) {
		t.Fatalf("post without address succeeded: %q", st.out.Bytes())
	}
	if _, ok := mail.ReadMsg(a.JAMBase, 1); ok {
		t.Fatal("message saved without an address")
	}
}

func TestNetmailReplyUsesOrigAddress(t *testing.T) {
	eng, st, a := netmailEngine(t, "nnnHi\r\rS")
	if !eng.writeMessage(a, "Bob", "Joe", "1:2/3", "Re: hello", nil, true) {
		t.Fatalf("reply failed: %q", st.out.Bytes())
	}
	art, ok := mail.ReadMsg(a.JAMBase, 1)
	if !ok || art.Dest != "1:2/3" {
		t.Fatalf("dest %q ok %v", art.Dest, ok)
	}
}

func TestNodeListQueryListsNodes(t *testing.T) {
	eng, st, a := netmailEngine(t, "")
	if s := eng.handleNodeListInput("1:2/?", a); s != "" {
		t.Fatalf("query left %q", s)
	}
	if !strings.Contains(st.out.String(), "Joe BBS") {
		t.Fatalf("node listing missing: %q", st.out.Bytes())
	}
}
