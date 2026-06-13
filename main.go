// Command crofty turns Markdown into a website you build and deploy to your own
// accounts. It wraps Hugo (build) and Wrangler (deploy) and never talks to a
// server of ours — there isn't one.
package main

import (
	"os"

	"github.com/shirodoromoto/crofty/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
