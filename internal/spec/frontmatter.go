package spec

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNoFrontmatter indicates the file has no leading YAML front matter block.
var ErrNoFrontmatter = errors.New("no YAML frontmatter found")

// Frontmatter is the parsed YAML front matter of a content file. Nested maps
// (e.g. the crofty: block) decode to map[string]any.
type Frontmatter map[string]any

// bom is the UTF-8 byte-order mark, stripped before parsing if present.
const bom = "\ufeff"

// parseFrontmatter splits a leading `---` YAML block from src and parses it.
// Only YAML front matter (delimited by lines of exactly `---`) is supported in
// v0; that is what spec v0 mandates.
func parseFrontmatter(src []byte) (Frontmatter, error) {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	s = strings.TrimPrefix(s, bom)

	if !strings.HasPrefix(s, "---\n") && s != "---" {
		return nil, ErrNoFrontmatter
	}
	body := strings.TrimPrefix(s, "---\n")

	// Collect lines until a closing line of exactly `---`.
	var block []string
	closed := false
	for _, ln := range strings.Split(body, "\n") {
		if strings.TrimRight(ln, " \t") == "---" {
			closed = true
			break
		}
		block = append(block, ln)
	}
	if !closed {
		return nil, ErrNoFrontmatter
	}

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &fm); err != nil {
		return nil, err
	}
	if fm == nil {
		fm = Frontmatter{}
	}
	return fm, nil
}
