package cli

// FTPS deploy backend: uploads dist/ over FTP-with-TLS (explicit AUTH TLS), the
// secure transfer mode budget shared hosting almost always offers. Plain FTP is
// deliberately not supported — FTPS reaches the same hosts without sending the
// password in the clear. The password comes from the keychain or a hidden TTY
// prompt, never through an assistant.

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/secret"
)

// ftpsSecretStore keeps FTPS passwords in the OS keychain, keyed by host:user.
func ftpsSecretStore() *secret.Store { return secret.New("ftps") }

// ftpsDeployer is a resolved FTPS destination ready to receive dist/.
type ftpsDeployer struct {
	addr      string // host:port
	user      string
	password  string
	tlsConfig *tls.Config
	remoteDir string
}

func (d *ftpsDeployer) Deploy(distDir string, progress func(string)) (string, error) {
	files, hasEdge, err := scanDistTree(distDir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files to deploy in %s", distDir)
	}

	conn, err := ftp.Dial(d.addr,
		ftp.DialWithTimeout(30*time.Second),
		ftp.DialWithExplicitTLS(d.tlsConfig),
	)
	if err != nil {
		return "", fmt.Errorf("connecting to %s over FTPS: %w\n"+
			"  If the server uses a shared/self-signed certificate, set deploy.tlsSkipVerify.", d.addr, err)
	}
	defer conn.Quit()
	if err := conn.Login(d.user, d.password); err != nil {
		return "", fmt.Errorf("FTPS login failed for %s: %w", d.user, err)
	}

	warnInPlace(progress)
	if hasEdge {
		warnEdgeFiles(progress)
	}

	// Create the web root and each subdirectory (FTP has no recursive mkdir).
	made := map[string]bool{}
	ftpsEnsureDir(conn, d.remoteDir, made)
	for _, dir := range remoteDirs(files) {
		ftpsEnsureDir(conn, joinRemote(d.remoteDir, dir), made)
	}

	progress(fmt.Sprintf("Uploading %d files to %s:%s …", len(files), d.addr, d.remoteDir))
	for _, f := range files {
		if err := ftpsStorFile(conn, f.abs, joinRemote(d.remoteDir, f.rel)); err != nil {
			return "", fmt.Errorf("uploading %s: %w", f.rel, err)
		}
	}
	return "", nil // a plain host's public URL depends on the owner's domain
}

// ftpsStorFile streams one local file to a remote path.
func ftpsStorFile(conn *ftp.ServerConn, localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()
	return conn.Stor(remotePath, src)
}

// ftpsEnsureDir creates each segment of a remote directory path, skipping ones
// already made this session. MakeDir errors are ignored: "already exists" is the
// normal case and a genuinely broken path surfaces on the following Stor.
func ftpsEnsureDir(conn *ftp.ServerConn, dir string, made map[string]bool) {
	abs := strings.HasPrefix(dir, "/")
	cur := ""
	for _, p := range strings.Split(strings.Trim(dir, "/"), "/") {
		if p == "" {
			continue
		}
		if cur == "" {
			cur = p
		} else {
			cur += "/" + p
		}
		full := cur
		if abs {
			full = "/" + cur
		}
		if made[full] {
			continue
		}
		made[full] = true
		_ = conn.MakeDir(full)
	}
}

// joinRemote joins a remote base directory and a slash-relative path, preserving
// a leading slash on absolute bases.
func joinRemote(base, rel string) string {
	return strings.TrimRight(base, "/") + "/" + rel
}

// connectFTPS resolves the FTPS destination from config + secrets, prompting on
// a TTY for the password when missing. reauth forces a fresh password prompt.
func connectFTPS(proj *project.Project, cfg *project.Config, reauth bool) (Deployer, error) {
	sc := deployServerConfig{
		host: cfg.Deploy.Host, port: cfg.Deploy.Port,
		user: cfg.Deploy.User, path: cfg.Deploy.Path,
	}
	if err := requireServerConfig(&sc, proj.ConfigPath()); err != nil {
		return nil, err
	}
	port := sc.port
	if port == 0 {
		port = 21
	}

	pw, err := resolveSecret(ftpsSecretStore(), sc.host+":"+sc.user, "password",
		fmt.Sprintf("Password for %s@%s", sc.user, sc.host), reauth)
	if err != nil {
		return nil, err
	}
	return &ftpsDeployer{
		addr:     net.JoinHostPort(sc.host, strconv.Itoa(port)),
		user:     sc.user,
		password: pw,
		tlsConfig: &tls.Config{
			ServerName:         sc.host,
			InsecureSkipVerify: cfg.Deploy.TLSSkipVerify, //nolint:gosec // opt-in for shared/self-signed certs
		},
		remoteDir: sc.path,
	}, nil
}
