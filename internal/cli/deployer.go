package cli

// A Deployer publishes a built site to one destination. Each deploy provider
// (Cloudflare Pages, SFTP, FTPS) implements it, so deploy.go can dispatch on the
// configured provider without knowing the wire details. The build and output-
// contract steps run once, before any Deployer is constructed — a Deployer only
// uploads an already-verified build.
type Deployer interface {
	// Carries reports the non-asset parts this provider can deliver (every
	// provider delivers the assets themselves). The common layer assembles a
	// bundle without knowing the destination, so this is how it learns what a
	// destination can and cannot take.
	Carries() []deployPart

	// Deploy sends the bundle to the destination. progress receives short
	// human-readable lines so the caller can print them. It returns the site's
	// live URL when the provider knows it ("" when it can't — e.g. an SFTP/FTPS
	// host whose public URL depends on the owner's domain).
	Deploy(b deployBundle, progress func(string)) (url string, err error)
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

// Pages takes _headers and _redirects as fields of the deployment itself, so
// they travel as parts rather than as files under the site.
func (d *cloudflareDeployer) Carries() []deployPart { return cfParts() }

func (d *cloudflareDeployer) Deploy(b deployBundle, progress func(string)) (string, error) {
	return cfDeployBundle(d.token, d.accountID, d.project, d.branch, b, progress)
}
