package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

func withCFServer(t *testing.T, h http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(h)
	prev := cfAPIBase
	cfAPIBase = srv.URL
	return func() { cfAPIBase = prev; srv.Close() }
}

// withTerminal fixes whether crofty believes it can ask the user something, so a
// test can drive the interactive and the CI path from the same place.
func withTerminal(t *testing.T, yes bool) {
	t.Helper()
	prev := canAsk
	canAsk = func() bool { return yes }
	t.Cleanup(func() { canAsk = prev })
}

func TestCFListAccounts(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"message":"Invalid token"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"acct1","name":"My Account"}]}`))
	})()

	accts, err := cfListAccounts("good")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(accts) != 1 || accts[0].id != "acct1" || accts[0].name != "My Account" {
		t.Fatalf("got %+v", accts)
	}

	if _, err := cfListAccounts("bad"); err == nil {
		t.Fatal("expected an error for a rejected token")
	}
}

func TestPickAccountKeepsReachablePin(t *testing.T) {
	// A pinned account the token can still reach is used as-is, without listing.
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			t.Errorf("should not list accounts when the pin is still reachable")
		}
		w.Write([]byte(`{"success":true,"result":[]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "pinned"
	got, ok, err := pickAccount("tok", cfg, "")
	if err != nil || !ok || got.id != "pinned" {
		t.Fatalf("got (%+v, %v, %v), want pinned", got, ok, err)
	}
}

func TestPickAccountStalePinFallsThrough(t *testing.T) {
	// The token can't reach the pinned account but can list exactly one other —
	// at a terminal crofty must switch to it instead of dead-ending on --account.
	withTerminal(t, true)
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/old/pages/projects":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
		case "/accounts":
			w.Write([]byte(`{"success":true,"result":[{"id":"new","name":"New Account"}]}`))
		default:
			w.Write([]byte(`{"success":true,"result":[]}`))
		}
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	got, ok, err := pickAccount("tok", cfg, "")
	if err != nil || !ok || got.id != "new" {
		t.Fatalf("got (%+v, %v, %v), want new", got, ok, err)
	}
}

func TestPickAccountExplicitFlagWins(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/chosen/pages/projects" {
			w.Write([]byte(`{"success":true,"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	got, ok, err := pickAccount("tok", cfg, "chosen")
	if err != nil || !ok || got.id != "chosen" {
		t.Fatalf("got (%+v, %v, %v), want chosen", got, ok, err)
	}
}

// pinnedTo returns a project rooted in a fresh temp dir plus a config pinned to
// accountID — the shape connectCloudflare sees on a CI checkout.
func pinnedTo(t *testing.T, accountID string) (*project.Project, *project.Config) {
	t.Helper()
	cfg := &project.Config{}
	cfg.Deploy.AccountID = accountID
	return &project.Project{Root: t.TempDir()}, cfg
}

// cfPagesFor serves Pages access on one account for exactly one bearer token,
// and fails every other request — so a test can prove *which* token was used.
func cfPagesFor(t *testing.T, wantToken, accountID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+wantToken || r.URL.Path != "/accounts/"+accountID+"/pages/projects" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":[]}`))
	}
}

func TestConnectCloudflareTakesTheTokenFromTheEnvironment(t *testing.T) {
	// CI has no TTY and no keychain: the token comes from the environment, wins
	// over anything saved, and leaves no trace behind.
	keyring.MockInit()
	if err := saveCFToken("acct", "keychain-token"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(cfTokenEnv, "  env-token\n")
	defer withCFServer(t, cfPagesFor(t, "env-token", "acct"))()

	proj, cfg := pinnedTo(t, "acct")
	tok, acct, proceed, err := connectCloudflare(proj, cfg, "", false)
	if err != nil || !proceed {
		t.Fatalf("connectCloudflare: (%v, %v)", proceed, err)
	}
	if tok != "env-token" || acct.id != "acct" {
		t.Fatalf("got token %q account %q, want the environment's token", tok, acct.id)
	}
	if saved, err := savedCFToken("acct"); err != nil || saved != "keychain-token" {
		t.Fatalf("keychain holds %q (%v) — an env token must not be stored", saved, err)
	}
	if _, err := os.Stat(proj.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("%s was written — an env token must not rewrite the config", proj.ConfigPath())
	}
}

func TestConnectCloudflareLetsReauthOutrankTheEnvironment(t *testing.T) {
	// `crofty connect` passes reauth to save a token. A variable that is merely
	// set must not answer for it, or the run reports a keychain entry it never
	// wrote.
	keyring.MockInit()
	withTerminal(t, false)
	t.Setenv(cfTokenEnv, "env-token")
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("used the environment's token for a --reauth run (%q)", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusForbidden)
	})()

	proj, cfg := pinnedTo(t, "acct")
	_, _, _, err := connectCloudflare(proj, cfg, "", true)
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("got %v, want the prompt to be required", err)
	}
}

func TestConnectCloudflareIgnoresTheGenericTokenName(t *testing.T) {
	// CLOUDFLARE_API_TOKEN is usually set for some other tool; reading it would
	// deploy to whichever account that tool belongs to.
	keyring.MockInit()
	withTerminal(t, false)
	t.Setenv(cfTokenEnv, "")
	t.Setenv("CLOUDFLARE_API_TOKEN", "generic-token")
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("called the Cloudflare API with %q — the generic name must not be read", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusForbidden)
	})()

	proj, cfg := pinnedTo(t, "acct")
	_, _, _, err := connectCloudflare(proj, cfg, "", false)
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("got %v, want the no-token-in-a-non-terminal error", err)
	}
}

func TestConnectCloudflareEnvTokenSurvivesAnUnusableKeychain(t *testing.T) {
	// A runner has no keychain at all. That is not a failure — the token it does
	// have is enough.
	keyring.MockInitWithError(errors.New("no keychain here"))
	defer keyring.MockInit()
	t.Setenv(cfTokenEnv, "env-token")
	defer withCFServer(t, cfPagesFor(t, "env-token", "acct"))()

	proj, cfg := pinnedTo(t, "acct")
	tok, _, proceed, err := connectCloudflare(proj, cfg, "", false)
	if err != nil || !proceed || tok != "env-token" {
		t.Fatalf("got (%q, %v, %v), want the env token", tok, proceed, err)
	}
}

func TestConnectCloudflareKeepsTheSavedTokenWhenNoEnvIsSet(t *testing.T) {
	// Without the variable, the keychain path is exactly what it was.
	keyring.MockInit()
	if err := saveCFToken("acct", "keychain-token"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(cfTokenEnv, "")
	defer withCFServer(t, cfPagesFor(t, "keychain-token", "acct"))()

	proj, cfg := pinnedTo(t, "acct")
	tok, acct, proceed, err := connectCloudflare(proj, cfg, "", false)
	if err != nil || !proceed || tok != "keychain-token" || acct.id != "acct" {
		t.Fatalf("got (%q, %q, %v, %v), want the saved token", tok, acct.id, proceed, err)
	}
}

// cfAccountsList answers /accounts with the given body and refuses Pages access
// on every account — the shape of a token that reaches nothing it is pointed at.
func cfAccountsList(t *testing.T, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
	}
}

func TestPickAccountStopsOnAStalePinWithoutATerminal(t *testing.T) {
	// On CI nobody can confirm a move, so a pin the token can't reach must stop
	// the deploy — never silently publish the site to some other account.
	withTerminal(t, false)
	defer withCFServer(t, cfAccountsList(t, `{"success":true,"result":[{"id":"new","name":"New Account"}]}`))()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	_, ok, err := pickAccount("tok", cfg, "")
	if ok || err == nil {
		t.Fatalf("got (%v, %v), want a stop", ok, err)
	}
	if !strings.Contains(err.Error(), "old") || !strings.Contains(err.Error(), "--account") {
		t.Fatalf("error %q must name the pinned account and the way out", err)
	}
}

func TestPickAccountStopsOnSeveralAccountsWithoutATerminal(t *testing.T) {
	withTerminal(t, false)
	defer withCFServer(t, cfAccountsList(t, `{"success":true,"result":[{"id":"a1"},{"id":"a2"}]}`))()

	_, ok, err := pickAccount("tok", &project.Config{}, "")
	if ok || err == nil {
		t.Fatalf("got (%v, %v), want a stop", ok, err)
	}
	if !strings.Contains(err.Error(), "--account") || !strings.Contains(err.Error(), "deploy.accountId") {
		t.Fatalf("error %q must name both ways to say which account", err)
	}
}

func TestPickAccountStopsOnAnUnknownAccountWithoutATerminal(t *testing.T) {
	// A Pages-only token can't list accounts. At a terminal crofty asks for the
	// id; on CI it says which setting supplies it.
	withTerminal(t, false)
	defer withCFServer(t, cfAccountsList(t, `{"success":false,"errors":[{"message":"not allowed"}]}`))()

	_, ok, err := pickAccount("tok", &project.Config{}, "")
	if ok || err == nil {
		t.Fatalf("got (%v, %v), want a stop", ok, err)
	}
	if !strings.Contains(err.Error(), "deploy.accountId") {
		t.Fatalf("error %q must name the setting that answers it", err)
	}
}

func TestPickAccountResolvesWithoutATerminalWhenItIsUnambiguous(t *testing.T) {
	// The two cases CI can settle on its own: a pin the token reaches, and a
	// token that reaches exactly one account.
	withTerminal(t, false)
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			w.Write([]byte(`{"success":true,"result":[{"id":"sole","name":"Sole"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":[]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "pinned"
	got, ok, err := pickAccount("tok", cfg, "")
	if err != nil || !ok || got.id != "pinned" {
		t.Fatalf("pinned: got (%+v, %v, %v)", got, ok, err)
	}

	got, ok, err = pickAccount("tok", &project.Config{}, "")
	if err != nil || !ok || got.id != "sole" {
		t.Fatalf("sole account: got (%+v, %v, %v)", got, ok, err)
	}
}

func TestPickAccountExplicitFlagWinsWithoutATerminal(t *testing.T) {
	withTerminal(t, false)
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			t.Error("listed accounts even though --account named one")
		}
		w.Write([]byte(`{"success":true,"result":[]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	got, ok, err := pickAccount("tok", cfg, "chosen")
	if err != nil || !ok || got.id != "chosen" {
		t.Fatalf("got (%+v, %v, %v), want chosen", got, ok, err)
	}
}

func TestParseMenuChoice(t *testing.T) {
	cases := []struct {
		line string
		max  int
		n    int
		ok   bool
	}{
		{"1\n", 3, 1, true},
		{"  2 \n", 3, 2, true},
		{"3", 3, 3, true},
		{"0\n", 3, 0, false},
		{"4\n", 3, 0, false},
		{"x\n", 3, 0, false},
		{"\n", 3, 0, false},
	}
	for _, c := range cases {
		n, ok := parseMenuChoice(c.line, c.max)
		if n != c.n || ok != c.ok {
			t.Errorf("parseMenuChoice(%q,%d) = (%d,%v), want (%d,%v)", c.line, c.max, n, ok, c.n, c.ok)
		}
	}
}

func TestCFVerifyPagesAccess(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/ok/pages/projects" {
			w.Write([]byte(`{"success":true,"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
	})()

	if err := cfVerifyPagesAccess("tok", "ok"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := cfVerifyPagesAccess("tok", "nope"); err == nil {
		t.Fatal("expected an error for an account the token can't reach")
	}
}
