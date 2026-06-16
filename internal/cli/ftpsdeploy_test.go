package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

// End-to-end: ftpsDeployer.Deploy must connect with explicit TLS (AUTH TLS), log
// in, and recreate the dist/ tree under the remote root. The server is an
// in-process ftpserverlib instance with a runtime self-signed cert on localhost —
// no Docker. It exercises the whole client path (Dial+TLS → Login → MKD → STOR).
func TestFTPSDeploy_E2E(t *testing.T) {
	dst := t.TempDir()
	addr := newFTPSServer(t, "deploy", "s3cret", dst)

	src := writeTree(t, map[string]string{
		"index.html":             "<h1>home</h1>",
		"posts/hello/index.html": "<p>hello</p>",
		"assets/site.css":        "body{}",
		"a/b/c/deep.txt":         "deep",
	})

	d := &ftpsDeployer{
		addr:      addr,
		user:      "deploy",
		password:  "s3cret",
		tlsConfig: &tls.Config{InsecureSkipVerify: true}, // self-signed test cert
		remoteDir: "/site",
	}
	if _, err := d.Deploy(src, func(string) {}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// The server's filesystem is rooted at dst, so /site lands under dst/site.
	assertTreeUploaded(t, src, dst+"/site")
}

// Wrong credentials must fail login rather than silently succeed.
func TestFTPSDeploy_BadPassword(t *testing.T) {
	dst := t.TempDir()
	addr := newFTPSServer(t, "deploy", "s3cret", dst)
	src := writeTree(t, map[string]string{"index.html": "x"})

	d := &ftpsDeployer{
		addr:      addr,
		user:      "deploy",
		password:  "wrong",
		tlsConfig: &tls.Config{InsecureSkipVerify: true},
		remoteDir: "/site",
	}
	if _, err := d.Deploy(src, func(string) {}); err == nil {
		t.Fatal("expected a login failure, got nil")
	}
}

// --- in-process FTPS server (ftpserverlib) -------------------------------

type ftpsTestDriver struct {
	user, pass string
	fs         afero.Fs
	tlsConfig  *tls.Config
	settings   *ftpserver.Settings
}

func (d *ftpsTestDriver) GetSettings() (*ftpserver.Settings, error) { return d.settings, nil }
func (d *ftpsTestDriver) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "crofty test server", nil
}
func (d *ftpsTestDriver) ClientDisconnected(ftpserver.ClientContext) {}
func (d *ftpsTestDriver) GetTLSConfig() (*tls.Config, error)        { return d.tlsConfig, nil }

func (d *ftpsTestDriver) AuthUser(_ ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	if user == d.user && pass == d.pass {
		return d.fs, nil // ClientDriver is just an afero.Fs
	}
	return nil, errors.New("bad credentials")
}

// newFTPSServer starts an in-process FTPS server serving rootDir and returns its
// address. Passive transfers use an ephemeral port (nil range), and EPSV reuses
// the control IP, so localhost works without any port-range plumbing.
func newFTPSServer(t *testing.T, user, pass, rootDir string) string {
	t.Helper()
	driver := &ftpsTestDriver{
		user: user, pass: pass,
		fs:        afero.NewBasePathFs(afero.NewOsFs(), rootDir),
		tlsConfig: selfSignedTLS(t),
		settings: &ftpserver.Settings{
			ListenAddr:  "127.0.0.1:0",
			TLSRequired: ftpserver.ClearOrEncrypted, // allow AUTH TLS upgrade
		},
	}
	server := ftpserver.NewFtpServer(driver)
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Stop() })
	return server.Addr()
}

// selfSignedTLS builds a throwaway TLS cert for the test server. The client uses
// InsecureSkipVerify, so the cert's subject/SAN don't need to match.
func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "crofty-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  priv,
		}},
		MinVersion: tls.VersionTLS12,
	}
}
