package serv

import "testing"

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
