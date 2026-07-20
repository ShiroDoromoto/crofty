package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// isolateHome points HOME — and, on Linux, XDG_CONFIG_HOME — at a fresh temp dir
// so a test that touches crofty's global config dir (e.g. its known_hosts store)
// stays hermetic. os.UserConfigDir prefers XDG_CONFIG_HOME on Linux, so setting
// HOME alone leaks to the real ~/.config on CI runners that export it, letting
// one test's pin pollute another. Returns the temp home for tests that also need
// to seed ~/.ssh.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// End-to-end: sftpDeployer.Deploy must connect over a real SSH transport,
// authenticate with a password, and recreate the dist/ tree under the remote
// root. The server is an in-process SSH daemon serving the SFTP subsystem
// (x/crypto/ssh + pkg/sftp) on localhost — no Docker, no new dependency, and it
// exercises the whole client path (ssh.Dial → auth → subsystem → upload).
func TestSFTPDeploy_E2E(t *testing.T) {
	addr := newSSHSFTPServer(t, "deploy", "s3cret")

	src := writeTree(t, map[string]string{
		"index.html":             "<h1>home</h1>",
		"posts/hello/index.html": "<p>hello</p>",
		"assets/site.css":        "body{}",
		"a/b/c/deep.txt":         "deep",
	})
	dst := t.TempDir()

	d := &sftpDeployer{
		addr: addr,
		sshConfig: &ssh.ClientConfig{
			User:            "deploy",
			Auth:            []ssh.AuthMethod{ssh.Password("s3cret")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), // host-key trust is unit-tested separately
		},
		remoteDir: dst,
	}
	if _, err := d.Deploy(assembleBundle(src), func(string) {}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	assertTreeUploaded(t, src, dst)
}

// Wrong credentials must fail the connection rather than silently succeed.
func TestSFTPDeploy_BadPassword(t *testing.T) {
	addr := newSSHSFTPServer(t, "deploy", "s3cret")
	src := writeTree(t, map[string]string{"index.html": "x"})
	d := &sftpDeployer{
		addr: addr,
		sshConfig: &ssh.ClientConfig{
			User:            "deploy",
			Auth:            []ssh.AuthMethod{ssh.Password("wrong")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
		remoteDir: t.TempDir(),
	}
	if _, err := d.Deploy(assembleBundle(src), func(string) {}); err == nil {
		t.Fatal("expected an auth failure, got nil")
	}
}

// newSSHSFTPServer starts an in-process SSH server that accepts one user/password
// and serves the SFTP subsystem over the real filesystem. Returns its address.
func newSSHSFTPServer(t *testing.T, user, pass string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, given []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(given) == pass {
				return nil, nil
			}
			return nil, fmt.Errorf("authentication failed")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSSHConn(conn, cfg)
		}
	}()
	return ln.Addr().String()
}

// serveSSHConn handles one SSH connection: it accepts session channels and, on a
// "sftp" subsystem request, serves SFTP over that channel.
func serveSSHConn(conn net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				// A subsystem request payload is a length-prefixed string.
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
				_ = req.Reply(ok, nil)
			}
		}(requests)
		server, err := sftp.NewServer(ch)
		if err != nil {
			continue
		}
		// Close the channel once Serve returns (client closed its end) so the
		// client's recv goroutine gets EOF and client.Close() doesn't block.
		go func(ch interface{ Close() error }) {
			_ = server.Serve()
			_ = ch.Close()
		}(ch)
	}
}

// The host-key store is trust-on-first-use: an unseen host is accepted, the same
// key later passes, and a DIFFERENT key for a known host is flagged as changed.
func TestMatchKnownHost(t *testing.T) {
	const host = "example.com:22"
	line := host + " ssh-ed25519 AAAAKEYONE"
	known := []byte("other.com:22 ssh-rsa ZZZ\n" + line + "\n")

	if found, _ := matchKnownHost(known, "newhost.com:22", "newhost.com:22 ssh-ed25519 X"); found {
		t.Error("an unseen host must report found=false (trust-on-first-use)")
	}
	if found, match := matchKnownHost(known, host, line); !found || !match {
		t.Errorf("same host+key: found=%v match=%v; want true,true", found, match)
	}
	changed := host + " ssh-ed25519 AAAAKEYTWO"
	if found, match := matchKnownHost(known, host, changed); !found || match {
		t.Errorf("changed key: found=%v match=%v; want true,false", found, match)
	}
}

// newTestHostKey returns a fresh ed25519 SSH public key for host-key tests.
func newTestHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sshPub
}

// A host the user already trusts in their own ~/.ssh/known_hosts must pass
// without crofty's trust-on-first-use prompt — the case behind the original
// report, where the user had reached the server over sftp before but crofty
// only consulted its own (empty) store and stalled at the y/N prompt.
func TestTrustedInUserKnownHosts(t *testing.T) {
	home := isolateHome(t)

	const addr = "127.0.0.1:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	key := newTestHostKey(t)

	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key)
	if err := os.WriteFile(khPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !trustedInUserKnownHosts(addr, remote, key) {
		t.Error("a host+key present in ~/.ssh/known_hosts must be trusted")
	}
	// A different key for the same host must NOT be trusted.
	if trustedInUserKnownHosts(addr, remote, newTestHostKey(t)) {
		t.Error("a different key for a known host must not be trusted")
	}
}

// With no ~/.ssh/known_hosts at all, the consult is a clean miss (false), not an
// error — crofty falls back to its own TOFU store.
func TestTrustedInUserKnownHosts_NoFile(t *testing.T) {
	isolateHome(t)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	if trustedInUserKnownHosts("127.0.0.1:2222", remote, newTestHostKey(t)) {
		t.Error("with no known_hosts file the consult must return false")
	}
}

// --yes (autoAccept) pins an unknown host key without a prompt, so an
// agent-driven deploy with no human to answer y/N doesn't stall. The pinned key
// is recorded to crofty's own store and recognised on the next connection.
func TestSFTPHostKeyCallback_AutoAccept(t *testing.T) {
	isolateHome(t)
	const addr = "example.com:22"
	remote := &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 22}
	key := newTestHostKey(t)

	cb, err := sftpHostKeyCallback(true)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb(addr, remote, key); err != nil {
		t.Fatalf("auto-accept must trust an unknown host: %v", err)
	}
	// The same key now passes from crofty's store on a second connection.
	cb2, err := sftpHostKeyCallback(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb2(addr, remote, key); err != nil {
		t.Errorf("a previously pinned key must pass without prompting: %v", err)
	}
	// A changed key for that host is a hard error (possible MITM).
	if err := cb2(addr, remote, newTestHostKey(t)); err == nil {
		t.Error("a changed host key must be rejected")
	}
}

// Honouring ~/.ssh/known_hosts must not write to crofty's own store: the user's
// file stays the single source of truth and crofty records nothing.
func TestSFTPHostKeyCallback_UserTrustedNotRecorded(t *testing.T) {
	home := isolateHome(t)
	const addr = "127.0.0.1:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	key := newTestHostKey(t)

	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// autoAccept=false: without the user's file this would error on a non-TTY,
	// but the existing trust lets it pass.
	cb, err := sftpHostKeyCallback(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb(addr, remote, key); err != nil {
		t.Fatalf("a host trusted in ~/.ssh/known_hosts must pass: %v", err)
	}
	if p, err := sftpKnownHostsPath(); err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("crofty must not write its own store when honouring the user's known_hosts (found %s)", p)
		}
	}
}

// appendLine creates the file (and parent dir) and appends, so the TOFU store can
// be written under GlobalDir on first use.
func TestAppendLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "known_hosts")
	if err := appendLine(p, "a"); err != nil {
		t.Fatal(err)
	}
	if err := appendLine(p, "b"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nb\n" {
		t.Errorf("appendLine result = %q; want %q", got, "a\nb\n")
	}
}
