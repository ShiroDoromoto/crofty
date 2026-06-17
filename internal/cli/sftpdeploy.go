package cli

// SFTP deploy backend: uploads dist/ to a server over SSH (VPS / cloud / shared
// hosting that offers SFTP). Auth is a password or an SSH private key; secrets
// are read from the keychain or a hidden TTY prompt, never through an assistant.
// Host keys are pinned trust-on-first-use so a later key change is caught.

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/secret"
)

// sftpSecretStore keeps SFTP passwords / key passphrases in the OS keychain,
// keyed by host:user so several sites on one server share nothing by accident.
func sftpSecretStore() *secret.Store { return secret.New("sftp") }

// sftpDeployer is a resolved SFTP destination ready to receive dist/.
type sftpDeployer struct {
	addr      string // host:port
	sshConfig *ssh.ClientConfig
	remoteDir string
}

func (d *sftpDeployer) Deploy(distDir string, progress func(string)) (string, error) {
	files, hasEdge, err := scanDistTree(distDir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files to deploy in %s", distDir)
	}

	conn, err := ssh.Dial("tcp", d.addr, d.sshConfig)
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", d.addr, err)
	}
	defer conn.Close()
	client, err := sftp.NewClient(conn)
	if err != nil {
		return "", fmt.Errorf("opening SFTP session: %w", err)
	}
	defer client.Close()

	warnInPlace(progress)
	if hasEdge {
		warnEdgeFiles(progress)
	}
	progress(fmt.Sprintf("Uploading %d files to %s:%s …", len(files), d.addr, d.remoteDir))
	return "", uploadTreeSFTP(client, d.remoteDir, files, progress)
}

// uploadTreeSFTP creates the web root and every subdirectory, then writes each
// file. Split out from Deploy so it can be tested against an in-memory SFTP
// server without a live SSH connection.
func uploadTreeSFTP(client *sftp.Client, remoteDir string, files []serverFile, progress func(string)) error {
	if err := client.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("creating %s on the server: %w", remoteDir, err)
	}
	for _, dir := range remoteDirs(files) {
		if err := client.MkdirAll(path.Join(remoteDir, dir)); err != nil {
			return fmt.Errorf("creating %s on the server: %w", dir, err)
		}
	}
	for _, f := range files {
		if err := sftpPutFile(client, f.abs, path.Join(remoteDir, f.rel)); err != nil {
			return fmt.Errorf("uploading %s: %w", f.rel, err)
		}
	}
	return nil
}

// sftpPutFile streams one local file to a remote path.
func sftpPutFile(client *sftp.Client, localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// connectSFTP resolves the SFTP destination from config + secrets, prompting on
// a TTY for anything missing. reauth forces a fresh password/passphrase prompt.
// autoAccept pins an unknown server host key without the interactive TOFU prompt
// (the --yes path, for agent-driven deploys with no human to answer y/N).
func connectSFTP(proj *project.Project, cfg *project.Config, reauth, autoAccept bool) (Deployer, error) {
	sc := deployServerConfig{
		host: cfg.Deploy.Host, port: cfg.Deploy.Port,
		user: cfg.Deploy.User, path: cfg.Deploy.Path, keyPath: cfg.Deploy.KeyPath,
	}
	if err := requireServerConfig(&sc, proj.ConfigPath()); err != nil {
		return nil, err
	}
	port := sc.port
	if port == 0 {
		port = 22
	}

	auth, err := sftpAuthMethod(sc, reauth)
	if err != nil {
		return nil, err
	}
	cb, err := sftpHostKeyCallback(autoAccept)
	if err != nil {
		return nil, err
	}
	return &sftpDeployer{
		addr: net.JoinHostPort(sc.host, strconv.Itoa(port)),
		sshConfig: &ssh.ClientConfig{
			User:            sc.user,
			Auth:            []ssh.AuthMethod{auth},
			HostKeyCallback: cb,
		},
		remoteDir: sc.path,
	}, nil
}

// sftpAuthMethod builds the SSH auth method: a private key (with passphrase if
// the key is encrypted) when keyPath is set, otherwise a password. Secrets come
// from the keychain, or a hidden prompt (then saved) on first use or --reauth.
func sftpAuthMethod(sc deployServerConfig, reauth bool) (ssh.AuthMethod, error) {
	target := sc.host + ":" + sc.user
	store := sftpSecretStore()

	if sc.keyPath != "" {
		keyBytes, err := os.ReadFile(sc.keyPath)
		if err != nil {
			return nil, fmt.Errorf("reading SSH key %s: %w", sc.keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err == nil {
			return ssh.PublicKeys(signer), nil
		}
		if _, needPass := err.(*ssh.PassphraseMissingError); !needPass {
			return nil, fmt.Errorf("parsing SSH key %s: %w", sc.keyPath, err)
		}
		// Encrypted key: resolve its passphrase from keychain or prompt.
		pass, perr := resolveSecret(store, target, "key_passphrase", "Passphrase for "+sc.keyPath, reauth)
		if perr != nil {
			return nil, perr
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(pass))
		if err != nil {
			return nil, fmt.Errorf("decrypting SSH key %s (wrong passphrase?): %w", sc.keyPath, err)
		}
		return ssh.PublicKeys(signer), nil
	}

	pw, err := resolveSecret(store, target, "password", fmt.Sprintf("Password for %s@%s", sc.user, sc.host), reauth)
	if err != nil {
		return nil, err
	}
	return ssh.Password(pw), nil
}

// resolveSecret returns a stored secret, or prompts for it (and saves it) when
// missing or when reauth is set.
func resolveSecret(store *secret.Store, target, field, label string, reauth bool) (string, error) {
	if !reauth {
		if v, err := store.Get(target, field); err == nil && v != "" {
			return v, nil
		}
	}
	v, err := promptSecretTTY(label)
	if err != nil {
		return "", err
	}
	if err := store.Set(target, field, v); err != nil {
		return "", err
	}
	return v, nil
}

// sftpKnownHostsPath is crofty's own trust-on-first-use store of server host
// keys, separate from ~/.ssh/known_hosts so crofty never edits the user's file.
func sftpKnownHostsPath() (string, error) {
	dir, err := project.GlobalDir()
	if err != nil {
		return "", err
	}
	return path.Join(dir, "known_hosts"), nil
}

// sftpHostKeyCallback verifies a server's host key. A key crofty already pinned
// passes; a host the user already trusts in their own ~/.ssh/known_hosts passes
// (read-only — crofty never edits that file); a changed key is a hard error
// (possible MITM); and a genuinely unknown key is pinned non-interactively when
// autoAccept (--yes) is set, else shown (with fingerprint) for the user to accept
// once on a TTY.
func sftpHostKeyCallback(autoAccept bool) (ssh.HostKeyCallback, error) {
	khPath, err := sftpKnownHostsPath()
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		line := hostname + " " + key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal())
		fp := sftpFingerprint(key)

		known, _ := os.ReadFile(khPath)
		if found, match := matchKnownHost(known, hostname, line); found {
			if match {
				return nil // same host, same key
			}
			return fmt.Errorf("host key for %s has CHANGED (fingerprint %s).\n"+
				"  This can mean a server reinstall — or a man-in-the-middle.\n"+
				"  If you trust it, remove the %s line from %s and retry.", hostname, fp, hostname, khPath)
		}

		// Honour a host the user already trusts in their own ~/.ssh/known_hosts —
		// the common case where they've reached this server over ssh/sftp before,
		// so there's nothing new to confirm. Read-only: crofty never writes there.
		if trustedInUserKnownHosts(hostname, remote, key) {
			return nil
		}

		// Genuinely unknown host. Pin it without prompting when --yes was passed
		// (an agent-driven deploy has no human to answer y/N), …
		if autoAccept {
			if err := appendLine(khPath, line); err != nil {
				return fmt.Errorf("recording the host key: %w", err)
			}
			return nil
		}
		// … otherwise ask to trust it (TTY only).
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("unknown host key for %s (fingerprint %s) and no terminal to confirm it.\n"+
				"  Run 'crofty deploy' yourself once to accept it, or pass --yes to trust it on first use.", hostname, fp)
		}
		fmt.Printf("\nThe server %s is presenting host key fingerprint:\n  %s\n", hostname, fp)
		fmt.Print("Trust this host and continue? [y/N]: ")
		var ans string
		fmt.Scanln(&ans)
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			return errors.New("host not trusted — deploy cancelled")
		}
		if err := appendLine(khPath, line); err != nil {
			return fmt.Errorf("recording the host key: %w", err)
		}
		return nil
	}, nil
}

// trustedInUserKnownHosts reports whether the server's host key is already
// trusted in the user's own ~/.ssh/known_hosts. It's consulted read-only, so a
// host the user has reached over ssh/sftp before doesn't trigger crofty's own
// trust-on-first-use prompt. Any parse/lookup failure (missing file, unknown
// host, or a different key) returns false, falling back to crofty's TOFU store.
func trustedInUserKnownHosts(hostname string, remote net.Addr, key ssh.PublicKey) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(khPath); err != nil {
		return false
	}
	cb, err := knownhosts.New(khPath)
	if err != nil {
		return false
	}
	return cb(hostname, remote, key) == nil
}

// matchKnownHost scans known_hosts content for hostname. found=false means the
// host is unseen (trust-on-first-use applies); found=true with match=true is the
// same key; found=true with match=false is a CHANGED key (the recorded line for
// this host differs from the one presented) — treated as a hard error.
func matchKnownHost(known []byte, hostname, line string) (found, match bool) {
	for _, l := range strings.Split(string(known), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) >= 1 && fields[0] == hostname {
			return true, l == line
		}
	}
	return false, false
}

// sftpFingerprint renders a key's SHA256 fingerprint the way OpenSSH shows it.
func sftpFingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// appendLine appends a line to a file, creating it (and parent dir) if needed.
func appendLine(filePath, line string) error {
	if err := os.MkdirAll(path.Dir(filePath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}
