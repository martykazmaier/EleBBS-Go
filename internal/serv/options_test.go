package serv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCertFlags(t *testing.T) {
	o := Parse([]string{"-FTPS", `-CERT:C:\bbs\fullchain.pem`, `-KEY:C:\bbs\privkey.pem`})
	if !o.FTP {
		t.Fatal("FTPS")
	}
	if o.CertFile != `C:\bbs\fullchain.pem` {
		t.Fatalf("cert %q", o.CertFile)
	}
	if o.KeyFile != `C:\bbs\privkey.pem` {
		t.Fatalf("key %q", o.KeyFile)
	}
	if o.FTPPort != 990 {
		t.Fatalf("ftps port %d", o.FTPPort)
	}
}

func TestParseCombinedCert(t *testing.T) {
	o := Parse([]string{"-NNTPS", "-CERT:eleserv.pem"})
	if !o.News || o.CertFile != "eleserv.pem" || o.KeyFile != "" {
		t.Fatalf("%+v", o)
	}
	if o.NewsPort != 563 {
		t.Fatalf("nntps port %d", o.NewsPort)
	}
}

func TestParseTelnetXT(t *testing.T) {
	o := Parse([]string{"-XT"})
	if !o.Telnet {
		t.Fatal("-XT should enable telnet spawn server")
	}
	o = Parse([]string{"-TELNET"})
	if !o.Telnet {
		t.Fatal("-TELNET")
	}
}

func TestParseTelnetSAndWSS(t *testing.T) {
	o := Parse([]string{"-TELNETS", "-WSS"})
	if !o.TelnetS || !o.WSS || o.Telnet || o.TelnetSPort != 992 || o.WSSPort != 11235 {
		t.Fatalf("%+v", o)
	}
	o = Parse([]string{"-WSSPORT:8443", "-TELNETSPORT:2323"})
	if !o.WSS || o.WSSPort != 8443 || !o.TelnetS || o.TelnetSPort != 2323 {
		t.Fatalf("%+v", o)
	}
}

func TestParsePasvDynIPWindowsPath(t *testing.T) {
	o := Parse([]string{`-PASVDYNIP:C:\ele\wanip.txt`})
	if o.PasvDyn != `C:\ele\wanip.txt` {
		t.Fatalf("PASVDYNIP %q", o.PasvDyn)
	}
}

func TestReadPasvDynIP(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wanip.txt")
	if err := os.WriteFile(p, []byte(" 174.27.157.148 \r\ncomment\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := ReadPasvDynIP(p); got != "174.27.157.148" {
		t.Fatalf("got %q", got)
	}
	if got := ReadPasvDynIP(filepath.Join(t.TempDir(), "missing.txt")); got != "" {
		t.Fatalf("missing %q", got)
	}
	if got := ReadPasvDynIP("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("literal %q", got)
	}
}

func TestPasvAdvertisedIP(t *testing.T) {
	lan := "192.168.0.3"
	wan := "174.27.157.148"
	if got := PasvAdvertisedIP(lan, "10.2.0.2", ""); got != lan {
		t.Fatalf("no PASVSRVIP: %s", got)
	}
	if got := PasvAdvertisedIP(lan, "8.8.8.8", wan); got != wan {
		t.Fatalf("internet client: %s", got)
	}
	if got := PasvAdvertisedIP(lan, "192.168.0.10", wan); got != lan {
		t.Fatalf("LAN client: %s", got)
	}
	if got := PasvAdvertisedIP("127.0.0.1", "8.8.8.8", wan); got != "127.0.0.1" {
		t.Fatalf("loopback: %s", got)
	}
}

func TestParseSSH(t *testing.T) {
	o := Parse([]string{"-SSH", "-SSHPORT:2222"})
	if !o.SSH {
		t.Fatal("-SSH")
	}
	if o.SSHPort != 2222 {
		t.Fatalf("port %d", o.SSHPort)
	}
	o = Parse([]string{"-SSHPORT:2200"})
	if !o.SSH || o.SSHPort != 2200 {
		t.Fatalf("%+v", o)
	}
}
