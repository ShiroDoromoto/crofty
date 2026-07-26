package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/secret"
)

// noDeployEnv clears every variable crofty reads a credential from, so a test
// says what the environment holds instead of inheriting the developer's shell.
func noDeployEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{cfTokenEnv, sftpPasswordEnv, sftpPassphraseEnv, ftpsPasswordEnv} {
		t.Setenv(name, "")
	}
}

// missingText joins a report's blocks so a test can ask whether it says a thing,
// without caring which block said it.
func missingText(r deployReadiness) string {
	var b strings.Builder
	for _, m := range r.Missing {
		b.WriteString(m.What)
		b.WriteString(" ")
		b.WriteString(m.Fix)
		b.WriteString("\n")
	}
	return b.String()
}

// writeSSHKey writes a fresh ed25519 private key, encrypted with passphrase when
// one is given, and returns its path.
func writeSSHKey(t *testing.T, passphrase string) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		der, merr := x509.MarshalPKCS8PrivateKey(key)
		if merr != nil {
			t.Fatal(merr)
		}
		block = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
		if err != nil {
			t.Fatal(err)
		}
	}
	p := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(p, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func cloudflareConfig(pagesProject, accountID string) *project.Config {
	c := &project.Config{}
	c.Deploy.Provider = "cloudflare"
	c.Deploy.Project = pagesProject
	c.Deploy.AccountID = accountID
	return c
}

func serverConfig(provider, host, user, path, keyPath string) *project.Config {
	c := &project.Config{}
	c.Deploy.Provider = provider
	c.Deploy.Host, c.Deploy.User, c.Deploy.Path, c.Deploy.KeyPath = host, user, path, keyPath
	return c
}

func TestDeployReadyCloudflareTakesTheTokenFromTheEnvironment(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(cfTokenEnv, "env-token")

	r := checkDeployReady(cloudflareConfig("blog", "abc123"), nil)
	if !r.Ready {
		t.Fatalf("a pinned account with a token in the environment should be ready, missing: %s", missingText(r))
	}
	if r.Credential != credentialFromEnv || r.EnvVar != cfTokenEnv {
		t.Errorf("credential = %q from %q; want the environment variable", r.Credential, r.EnvVar)
	}
}

func TestDeployReadyCloudflareWantsThePinnedAccount(t *testing.T) {
	// The one people trip over: the token is in the environment, but nothing
	// says which account, and with no terminal there is nobody to pick.
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(cfTokenEnv, "env-token")

	r := checkDeployReady(cloudflareConfig("blog", ""), nil)
	if r.Ready {
		t.Fatal("an unpinned account should not read as ready")
	}
	if r.Credential != credentialFromEnv {
		t.Errorf("credential = %q; the token is still there, only the account is not", r.Credential)
	}
	if msg := missingText(r); !strings.Contains(msg, "deploy.accountId") || !strings.Contains(msg, "--account") {
		t.Errorf("report must name both ways to pin an account: %s", msg)
	}
}

func TestDeployReadyCloudflareFindsASavedToken(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)
	if err := saveCFToken("abc123", "saved-token"); err != nil {
		t.Fatal(err)
	}

	r := checkDeployReady(cloudflareConfig("blog", "abc123"), nil)
	if !r.Ready || r.Credential != credentialFromKeychain {
		t.Fatalf("ready=%v credential=%q; want the keychain to answer, missing: %s", r.Ready, r.Credential, missingText(r))
	}
	if r.EnvVar != "" {
		t.Errorf("envVar = %q; nothing came from the environment", r.EnvVar)
	}
}

func TestDeployReadyCloudflareNamesTheVariableWhenThereIsNoToken(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	r := checkDeployReady(cloudflareConfig("blog", "abc123"), nil)
	if r.Ready || r.Credential != credentialMissing {
		t.Fatalf("ready=%v credential=%q; want a stop", r.Ready, r.Credential)
	}
	if msg := missingText(r); !strings.Contains(msg, cfTokenEnv) {
		t.Errorf("report must name the variable a run with no terminal sets: %s", msg)
	}
}

func TestDeployReadyCloudflareWantsAProjectToPublishTo(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(cfTokenEnv, "env-token")

	r := checkDeployReady(cloudflareConfig("", "abc123"), nil)
	if msg := missingText(r); !strings.Contains(msg, "deploy.project") {
		t.Errorf("an empty deploy.project stops the deploy, so the report must say so: %s", msg)
	}
}

func TestDeployReadyNeverCarriesTheSecretItself(t *testing.T) {
	// The report travels: it is printed, and `--json` hands it to an agent. A
	// credential must not ride along in any field.
	freshKeychain(t)
	noDeployEnv(t)
	const secretValue = "s3cr3t-token-value"
	if err := saveCFToken("abc123", secretValue); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sftpPasswordEnv, secretValue)

	for _, cfg := range []*project.Config{
		cloudflareConfig("blog", "abc123"),
		serverConfig("sftp", "example.com", "alice", "/var/www", ""),
	} {
		out, err := json.Marshal(checkDeployReady(cfg, nil))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), secretValue) {
			t.Fatalf("the report leaked a credential: %s", out)
		}
	}
}

func TestDeployReadySFTPTakesThePasswordFromTheEnvironment(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(sftpPasswordEnv, "env-pw")

	r := checkDeployReady(serverConfig("sftp", "example.com", "alice", "/var/www", ""), nil)
	if !r.Ready || r.Credential != credentialFromEnv || r.EnvVar != sftpPasswordEnv {
		t.Fatalf("ready=%v credential=%q envVar=%q; missing: %s", r.Ready, r.Credential, r.EnvVar, missingText(r))
	}
	if r.Note == "" {
		t.Error("an SFTP report must still mention the host key a first connection has to trust")
	}
}

func TestDeployReadySFTPNamesTheConnectionFieldsItHasNot(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(sftpPasswordEnv, "env-pw")

	r := checkDeployReady(serverConfig("sftp", "example.com", "", "", ""), nil)
	if r.Ready {
		t.Fatal("a config with no user and no remote path is not ready")
	}
	if msg := missingText(r); !strings.Contains(msg, "user") || !strings.Contains(msg, "path") {
		t.Errorf("report must name the missing fields: %s", msg)
	}
}

func TestDeployReadySFTPKeyWithNoPassphraseNeedsNothingElse(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	r := checkDeployReady(serverConfig("sftp", "example.com", "alice", "/var/www", writeSSHKey(t, "")), nil)
	if !r.Ready || r.Credential != credentialFromSSHKey {
		t.Fatalf("ready=%v credential=%q; an unencrypted key authenticates on its own, missing: %s", r.Ready, r.Credential, missingText(r))
	}
}

func TestDeployReadySFTPEncryptedKeyNamesThePassphraseVariable(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	r := checkDeployReady(serverConfig("sftp", "example.com", "alice", "/var/www", writeSSHKey(t, "hunter2")), nil)
	if r.Ready {
		t.Fatal("an encrypted key with no passphrase anywhere is not ready")
	}
	msg := missingText(r)
	if !strings.Contains(msg, sftpPassphraseEnv) {
		t.Errorf("report must name the passphrase variable: %s", msg)
	}
	if strings.Contains(msg, sftpPasswordEnv) {
		t.Errorf("the password variable does not unlock a key, so it must not be offered: %s", msg)
	}
}

func TestDeployReadySFTPSaysWhenTheKeyCannotBeRead(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	r := checkDeployReady(serverConfig("sftp", "example.com", "alice", "/var/www", filepath.Join(t.TempDir(), "absent")), nil)
	if r.Ready || r.Credential != credentialMissing {
		t.Fatalf("ready=%v credential=%q; a key that isn't there stops the deploy", r.Ready, r.Credential)
	}
	if msg := missingText(r); !strings.Contains(msg, "deploy.keyPath") {
		t.Errorf("report must point at the field naming the key: %s", msg)
	}
}

func TestDeployReadyFTPSReadsItsOwnPasswordVariable(t *testing.T) {
	// One variable per credential: SFTP's password must not answer for FTPS.
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(sftpPasswordEnv, "env-pw")

	r := checkDeployReady(serverConfig("ftps", "example.com", "alice", "/var/www", ""), nil)
	if r.Ready {
		t.Fatal("the SFTP password stood in for the FTPS one")
	}
	if msg := missingText(r); !strings.Contains(msg, ftpsPasswordEnv) {
		t.Errorf("report must name the FTPS variable: %s", msg)
	}

	if err := secret.New("ftps").Set("example.com:alice", "password", "saved"); err != nil {
		t.Fatal(err)
	}
	if r := checkDeployReady(serverConfig("ftps", "example.com", "alice", "/var/www", ""), nil); !r.Ready || r.Credential != credentialFromKeychain {
		t.Fatalf("ready=%v credential=%q; want the keychain to answer", r.Ready, r.Credential)
	}
}

func TestDeployReadyReportsAConfigItCannotRead(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	r := checkDeployReady(nil, errors.New("parsing .crofty/config.json: unexpected end of JSON input"))
	if r.Ready || len(r.Missing) == 0 {
		t.Fatal("an unreadable deploy config is itself a stop")
	}
	if msg := missingText(r); !strings.Contains(msg, "unexpected end of JSON input") {
		t.Errorf("report must carry the reason: %s", msg)
	}
}

func TestDeployReadyMissingIsAlwaysAList(t *testing.T) {
	// `--json` is read by agents: a null where a list belongs makes every reader
	// handle two shapes.
	freshKeychain(t)
	noDeployEnv(t)
	t.Setenv(cfTokenEnv, "env-token")

	out, err := json.Marshal(checkDeployReady(cloudflareConfig("blog", "abc123"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"missing":[]`) {
		t.Errorf("missing must marshal as an empty list: %s", out)
	}
}

func TestDeployProviderDefaultsToTheOneCroftyStartedWith(t *testing.T) {
	freshKeychain(t)
	noDeployEnv(t)

	if got := deployProvider(&project.Config{}); got != "cloudflare" {
		t.Errorf("deployProvider = %q; want cloudflare for a config written before there were others", got)
	}
	if r := checkDeployReady(&project.Config{}, nil); r.Provider != "cloudflare" {
		t.Errorf("report provider = %q; want cloudflare", r.Provider)
	}
}
