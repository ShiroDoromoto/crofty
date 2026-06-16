package cli

// A Deployer publishes a built dist/ to one destination. Each deploy provider
// (Cloudflare Pages, SFTP, FTPS) implements it, so deploy.go can dispatch on the
// configured provider without knowing the wire details. The build and output-
// contract steps run once, before any Deployer is constructed — a Deployer only
// uploads an already-verified dist/.
type Deployer interface {
	// Deploy uploads everything under distDir to the destination. progress
	// receives short human-readable lines so the caller can print them. It
	// returns the site's live URL when the provider knows it ("" when it can't —
	// e.g. an SFTP/FTPS host whose public URL depends on the owner's domain).
	Deploy(distDir string, progress func(string)) (url string, err error)
}

// supportedProviders is the single source of truth for the deploy backends
// crofty offers, in the order they're presented to the user (best experience
// first). Code branches (deploy.go), the init picker, and the agent docs all key
// off this list so they can't drift apart.
func supportedProviders() []string { return []string{"cloudflare", "sftp", "ftps"} }

// isSupportedProvider reports whether p is a known deploy backend.
func isSupportedProvider(p string) bool {
	for _, v := range supportedProviders() {
		if p == v {
			return true
		}
	}
	return false
}

// cloudflareDeployer publishes to Cloudflare Pages via the native Direct Upload
// sequence (cfdeploy.go). It is a thin wrapper that carries the resolved
// credentials so the upload logic itself is unchanged.
type cloudflareDeployer struct {
	token     string
	accountID string
	project   string
	branch    string
}

func (d *cloudflareDeployer) Deploy(distDir string, progress func(string)) (string, error) {
	return cfDeployDir(d.token, d.accountID, d.project, d.branch, distDir, progress)
}
