// Command crofty turns Markdown into a website you build and deploy to your own
// accounts. It wraps Hugo to build and deploys straight to your Cloudflare
// account over its API (no Node or Wrangler), and never talks to a server of
// ours — there isn't one.
package main

import (
	"os"

	"github.com/ShiroDoromoto/crofty/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
