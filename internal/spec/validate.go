package spec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FileReport is the validation result for a single content file.
type FileReport struct {
	File   string  `json:"file"`
	OK     bool    `json:"ok"`
	Issues []Issue `json:"issues"`
}

// ValidateFile reads path, parses its front matter, and validates it against
// spec v0. contentRoot (the project's content/ directory, or "") is used to
// classify the page kind; classification also self-infers from the path.
func ValidateFile(path, contentRoot string) FileReport {
	rep := FileReport{File: path, Issues: []Issue{}}

	data, err := os.ReadFile(path)
	if err != nil {
		rep.Issues = append(rep.Issues, issue("(file)", "read", SeverityError, err.Error(),
			"a readable file", "Check the path and permissions."))
		return rep
	}

	fm, err := parseFrontmatter(data)
	if err != nil {
		if errors.Is(err, ErrNoFrontmatter) {
			rep.Issues = append(rep.Issues, issue("(frontmatter)", "required", SeverityError, nil,
				"a leading --- YAML block",
				`Add a frontmatter block at the very top, between two '---' lines.`))
		} else {
			rep.Issues = append(rep.Issues, issue("(frontmatter)", "parse", SeverityError, err.Error(),
				"valid YAML", "Fix the YAML syntax in the frontmatter block."))
		}
		return rep
	}

	rep.Issues = append(rep.Issues, Validate(fm, classify(path, contentRoot))...)
	rep.OK = !hasError(rep.Issues)
	return rep
}

func hasError(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// classify decides whether a file is a post (dated), a standalone page, or a
// section index, from its location under content/.
func classify(path, contentRoot string) PageKind {
	switch filepath.Base(path) {
	case "_index.md", "_index.markdown":
		return KindSection
	}
	rel := relUnderContent(path, contentRoot)
	if rel == "" {
		return KindPage // can't locate content/: be lenient
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) <= 1 {
		return KindPage // e.g. content/about.md
	}
	return KindPost // e.g. content/posts/<bundle>/index.md
}

// relUnderContent returns path relative to the content directory, or "" if it
// cannot be located.
func relUnderContent(path, contentRoot string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if contentRoot != "" {
		if r, err := filepath.Rel(contentRoot, abs); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	segs := strings.Split(filepath.ToSlash(abs), "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i] == "content" && i < len(segs)-1 {
			return strings.Join(segs[i+1:], "/")
		}
	}
	return ""
}
