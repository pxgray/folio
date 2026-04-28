package render

import (
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// LinkRewriter is a goldmark ASTTransformer that rewrites Markdown link and
// image destinations to internal Folio URLs.
//
// Rules:
//   - Absolute URLs (http/https//) and fragment-only (#) links are left unchanged.
//   - Paths starting with "/" are left unchanged.
//   - Relative paths are resolved against the directory of filePath.
//   - Resolved .md paths → {repoBase}/{resolved}[?ref=...]
//   - Other resolved paths → {repoBase}/-/raw/{resolved}[?ref=...]
//   - Paths that would escape the repo root are left unchanged.
type LinkRewriter struct {
	RepoBase string // e.g. "/github.com/owner/repo"
	FilePath string // e.g. "docs/setup.md"
	Ref      string // e.g. "main", "abc123", or ""
}

// Priority returns the transformer priority. Lower runs first; 999 runs last.
func (t *LinkRewriter) Priority() int { return 999 }

func (t *LinkRewriter) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			dest := string(node.Destination)
			node.Destination = []byte(t.rewrite(dest))
		case *ast.Image:
			dest := string(node.Destination)
			node.Destination = []byte(t.rewrite(dest))
		}
		return ast.WalkContinue, nil
	})
}

func (t *LinkRewriter) rewrite(dest string) string {
	if isAbsoluteURL(dest) || strings.HasPrefix(dest, "/") {
		return dest
	}

	// Resolve relative to the directory containing the current file.
	dir := path.Dir(t.FilePath)
	if dir == "." {
		dir = ""
	}
	var resolved string
	if dir == "" {
		resolved = path.Clean(dest)
	} else {
		resolved = path.Clean(dir + "/" + dest)
	}

	// Reject paths that escape the repo root.
	if strings.HasPrefix(resolved, "..") {
		return dest
	}

	var refSuffix string
	if t.Ref != "" {
		refSuffix = "?ref=" + t.Ref
	}

	if strings.HasSuffix(resolved, ".md") {
		return t.RepoBase + "/" + resolved + refSuffix
	}
	return t.RepoBase + "/-/raw/" + resolved + refSuffix
}

func isAbsoluteURL(s string) bool {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "//") {
		return true
	}
	// Check for any URI scheme (mailto:, tel:, data:, ftp://, etc.)
	if i := strings.Index(s, ":"); i > 0 {
		for j := 0; j < i; j++ {
			c := s[j]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(s, "#") {
		return true
	}
	return false
}

func isFragmentOnly(s string) bool {
	return strings.HasPrefix(s, "#")
}
