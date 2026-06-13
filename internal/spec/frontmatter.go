package spec

import (
	"errors"
	"os"
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

// split separates the YAML front matter block from the body. The block is
// returned without its `---` delimiters; the body is everything after the
// closing `---` line. Only YAML front matter is supported in v0.
func split(src []byte) (block, body string, err error) {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	s = strings.TrimPrefix(s, bom)
	if !strings.HasPrefix(s, "---\n") {
		return "", "", ErrNoFrontmatter
	}
	lines := strings.Split(s[len("---\n"):], "\n")
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t") == "---" {
			return strings.Join(lines[:i], "\n"), strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", "", ErrNoFrontmatter
}

// parseFrontmatter parses the leading YAML front matter of src.
func parseFrontmatter(src []byte) (Frontmatter, error) {
	block, _, err := split(src)
	if err != nil {
		return nil, err
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, err
	}
	if fm == nil {
		fm = Frontmatter{}
	}
	return fm, nil
}

// ParseFile reads path and returns its parsed front matter and the body bytes
// (everything after the closing `---`). The body is read locally — callers that
// syndicate must still send only fragment fields, never the body (spec §1).
func ParseFile(path string) (Frontmatter, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	block, body, err := split(data)
	if err != nil {
		return nil, nil, err
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, nil, err
	}
	if fm == nil {
		fm = Frontmatter{}
	}
	return fm, []byte(body), nil
}
