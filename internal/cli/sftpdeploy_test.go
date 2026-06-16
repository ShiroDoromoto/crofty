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
)

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
	if _, err := d.Deploy(src, func(string) {}); err != nil {
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
	if _, err := d.Deploy(src, func(string) {}); err == nil {
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
