package cli

// Whether a deploy would get through here with nobody to answer a prompt.
//
// `crofty doctor` grades the built site; this grades the run around it. The
// question is the one a CI job asks — "will `crofty deploy` publish from this
// environment, or stop at something only a terminal could answer?" — and until
// it was asked here, the only way to find out was to deploy and be stopped, one
// missing piece at a time (D-400).
//
// What it reads is presence, never a value: which variable a credential would
// come from, or whether the keychain holds one at all. No secret is printed, and
// nothing here goes to the network — so a "ready" says the pieces are in place,
// not that the credential still works. That last answer belongs to deploy.

import (
	"os"
	"strings"

	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/secret"
)

// Where the credential for a run with no terminal would come from.
const (
	credentialFromEnv      = "env"      // a CROFTY_-prefixed environment variable
	credentialFromKeychain = "keychain" // saved by an earlier interactive run
	credentialFromSSHKey   = "ssh-key"  // an SFTP key that needs no passphrase
	credentialMissing      = "none"     // nowhere — the run would stop
)

// deployReadiness answers, for the provider this project deploys to, whether a
// run with no terminal has everything it needs. Missing is what such a run would
// stop on, each with the way to set it, worded as `crofty deploy` words it — so
// reading it here and hitting it there don't teach two different fixes.
type deployReadiness struct {
	Provider   string        `json:"provider"`         // cloudflare | sftp | ftps
	Ready      bool          `json:"ready"`            // nothing left that only a terminal could settle
	Credential string        `json:"credential"`       // env | keychain | ssh-key | none
	EnvVar     string        `json:"envVar,omitempty"` // the variable it comes from, when credential is env
	Missing    []deployBlock `json:"missing"`          // never nil
	Note       string        `json:"note,omitempty"`   // a condition crofty cannot see from here
}

// deployBlock is one thing a non-interactive deploy would stop on, and what to
// set so that it doesn't.
type deployBlock struct {
	What string `json:"what"`
	Fix  string `json:"fix"`
}

// checkDeployReady reports whether a deploy could publish this project with no
// terminal. A config it cannot read is itself the answer, so this never fails:
// doctor grades a built site, and a broken deploy config must not take that
// report down with it.
func checkDeployReady(cfg *project.Config, configErr error) deployReadiness {
	if cfg == nil {
		return deployReadiness{
			Credential: credentialMissing,
			Missing: []deployBlock{{
				What: "crofty could not read the deploy config: " + errText(configErr),
				Fix:  "fix " + deployConfigRef + ", or run 'crofty init' in a new directory and copy its deploy block over",
			}},
		}
	}
	provider := deployProvider(cfg)
	var r deployReadiness
	switch provider {
	case "cloudflare":
		r = cloudflareReadiness(cfg)
	case "sftp":
		r = sftpReadiness(cfg)
	case "ftps":
		r = ftpsReadiness(cfg)
	default:
		r = deployReadiness{Credential: credentialMissing, Missing: []deployBlock{{
			What: "deploy provider " + provider + " is not supported",
			Fix:  "set deploy.provider in " + deployConfigRef + " to one of: " + strings.Join(supportedProviders(), ", "),
		}}}
	}
	r.Provider = provider
	r.Ready = len(r.Missing) == 0
	return r
}

// cloudflareReadiness checks the two things a Pages deploy needs settled before
// it runs: a token it may use, and an account it may publish to. The account is
// the one people trip over — a token in the environment says nothing about where
// it should deploy, and with no terminal there is nobody to pick.
func cloudflareReadiness(cfg *project.Config) deployReadiness {
	r := deployReadiness{Missing: []deployBlock{}}
	switch {
	case strings.TrimSpace(os.Getenv(cfTokenEnv)) != "":
		r.Credential, r.EnvVar = credentialFromEnv, cfTokenEnv
	case cfg.Deploy.AccountID != "" && cfTokenStore().Has(cfg.Deploy.AccountID, "api_token"):
		// The saved token is keyed by account, so there is one to find only once
		// an account is pinned — the same door connectCloudflare goes through.
		r.Credential = credentialFromKeychain
	default:
		r.Credential = credentialMissing
		r.Missing = append(r.Missing, deployBlock{
			What: "no Cloudflare API token this run could use",
			Fix:  "set " + cfTokenEnv + " from the runner's secret store, or deploy once at your own terminal to save one in the keychain",
		})
	}
	if cfg.Deploy.Project == "" {
		r.Missing = append(r.Missing, deployBlock{
			What: "deploy.project is empty, so crofty has no Pages project to publish to",
			Fix:  "set deploy.project in " + deployConfigRef + " (the name that becomes <project>.pages.dev)",
		})
	}
	if cfg.Deploy.AccountID == "" {
		r.Missing = append(r.Missing, deployBlock{
			What: "no Cloudflare account pinned — with no terminal, crofty stops rather than guess, unless the token reaches exactly one account",
			Fix:  "set deploy.accountId in " + deployConfigRef + ", or pass --account <id>",
		})
	}
	return r
}

// sftpReadiness checks the SFTP destination and the credential it authenticates
// with — a key that needs no passphrase, a passphrase for one that does, or a
// password.
func sftpReadiness(cfg *project.Config) deployReadiness {
	sc := serverConfigFrom(cfg)
	r := deployReadiness{Missing: serverConfigBlocks(&sc)}
	// crofty pins a host key on first use, and with no terminal there is nobody
	// to confirm it. Whether this host is already pinned isn't answerable from
	// here (the key it presents is only known once it presents it), so say the
	// condition rather than guess at a verdict.
	r.Note = "a first connection to a host crofty hasn't pinned also needs --yes, to trust its key"

	if sc.keyPath != "" {
		_, _, encrypted, err := sshKeyAt(sc.keyPath)
		switch {
		case err != nil:
			r.Credential = credentialMissing
			r.Missing = append(r.Missing, deployBlock{
				What: "the SSH key this deploy authenticates with is unusable: " + errText(err),
				Fix:  "point deploy.keyPath in " + deployConfigRef + " at the private key file",
			})
		case !encrypted:
			r.Credential = credentialFromSSHKey // the key alone authenticates
		default:
			r.credentialFor(sftpSecretStore(), serverSecretTarget(&sc), "key_passphrase", sftpPassphraseEnv,
				"the passphrase for the SSH key at "+sc.keyPath)
		}
		return r
	}
	r.credentialFor(sftpSecretStore(), serverSecretTarget(&sc), "password", sftpPasswordEnv,
		"a password for "+sc.user+"@"+sc.host)
	return r
}

// ftpsReadiness checks the FTPS destination and its password.
func ftpsReadiness(cfg *project.Config) deployReadiness {
	sc := serverConfigFrom(cfg)
	r := deployReadiness{Missing: serverConfigBlocks(&sc)}
	r.credentialFor(ftpsSecretStore(), serverSecretTarget(&sc), "password", ftpsPasswordEnv,
		"a password for "+sc.user+"@"+sc.host)
	return r
}

// credentialFor records where a run with no terminal would get this secret: the
// variable named by envName, else the keychain entry, else nowhere — in which
// case what to set is added to Missing. what names the credential in the
// author's terms ("a password for you@host"), never its value.
func (r *deployReadiness) credentialFor(store *secret.Store, target, field, envName, what string) {
	switch {
	case strings.TrimSpace(os.Getenv(envName)) != "":
		r.Credential, r.EnvVar = credentialFromEnv, envName
	case store.Has(target, field):
		r.Credential = credentialFromKeychain
	default:
		r.Credential = credentialMissing
		r.Missing = append(r.Missing, deployBlock{
			What: "crofty has nowhere to get " + what,
			Fix:  "set " + envName + " from the runner's secret store, or run 'crofty connect' at a terminal to save it in the keychain",
		})
	}
}

// serverConfigBlocks reports the non-secret connection fields SFTP/FTPS are
// missing as one block, the way requireServerConfig reports them as one error.
func serverConfigBlocks(sc *deployServerConfig) []deployBlock {
	missing := missingServerFields(sc)
	if len(missing) == 0 {
		return []deployBlock{}
	}
	return []deployBlock{{
		What: "the deploy config is missing " + strings.Join(missing, ", "),
		Fix:  serverConfigSetup,
	}}
}

// deployProvider is the backend this project publishes through. An empty
// provider means a config written before crofty had more than one, so it reads
// as the one it had.
func deployProvider(cfg *project.Config) string {
	if cfg == nil || cfg.Deploy.Provider == "" {
		return "cloudflare"
	}
	return cfg.Deploy.Provider
}

// errText renders an error for a report field, where a nil error would otherwise
// print as "%!s(<nil>)".
func errText(err error) string {
	if err == nil {
		return "unknown reason"
	}
	return err.Error()
}
