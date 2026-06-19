// Command crofty turns Markdown into a website you build and deploy to your own
// accounts. It wraps Hugo to build and deploys straight to your Cloudflare
// account over its API (no Node or Wrangler), and never talks to a server of
// ours — there isn't one.
package main

import (
	"os"

	"github.com/ShiroDoromoto/crofty/internal/cli"
)

// version is injected at release time via -ldflags "-X main.version=…" — the
// convention wharfy (and GoReleaser underneath it) uses. A plain `go build`
// from source leaves it empty, so cli.Version keeps its "dev" default.
var version string

func main() {
	if version != "" {
		cli.Version = version
	}
	os.Exit(cli.Run(os.Args[1:]))
}
