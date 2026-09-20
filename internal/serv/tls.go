package serv

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
)

// LoadTLS builds a TLS config for implicit FTPS/NNTPS.
//
// Certificate selection, in order:
//  1. -CERT:file -KEY:file
//  2. -CERT:file                 (combined PEM: certificate + private key)
//  3. sysPath/eleserv.pem        (combined PEM)
//  4. sysPath/eleserv.crt + sysPath/eleserv.key
//  5. ./eleserv.pem or ./eleserv.crt + ./eleserv.key
func LoadTLS(certFile, keyFile, sysPath string) (*tls.Config, string, error) {
	certFile, keyFile, src, err := resolveCert(certFile, keyFile, sysPath)
	if err != nil {
		return nil, "", err
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, src, fmt.Errorf("load TLS certificate %s: %w", src, err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}, src, nil
}

func resolveCert(certFile, keyFile, sysPath string) (cert, key, src string, err error) {
	if certFile != "" {
		if keyFile == "" {
			keyFile = certFile
		}
		return certFile, keyFile, certFile, nil
	}
	type pair struct{ cert, key, src string }
	cands := []pair{
		{join(sysPath, "eleserv.pem"), join(sysPath, "eleserv.pem"), "eleserv.pem"},
		{join(sysPath, "eleserv.crt"), join(sysPath, "eleserv.key"), "eleserv.crt + eleserv.key"},
		{"eleserv.pem", "eleserv.pem", ".\\eleserv.pem"},
		{"eleserv.crt", "eleserv.key", ".\\eleserv.crt + eleserv.key"},
	}
	for _, c := range cands {
		if c.cert == "" {
			continue
		}
		if fileExists(c.cert) && fileExists(c.key) {
			return c.cert, c.key, c.src, nil
		}
	}
	return "", "", "", fmt.Errorf("no TLS certificate found (use -CERT:file.pem or -CERT:cert.pem -KEY:key.pem)")
}

func join(dir, name string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
