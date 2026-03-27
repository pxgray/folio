package render

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/toc"
)

// Result holds the rendered outputs from a Markdown source.
type Result struct {
	Content template.HTML
	TOC     template.HTML // non-empty only when frontmatter contains toc: true
}

// Render converts Markdown src to an HTML fragment (safe for use in
// html/template with template.HTML).
//
//   - repoBase: URL prefix for this repo, e.g. "/github.com/owner/repo"
//   - filePath: path of the current file within the repo, e.g. "docs/setup.md"
//   - ref:      ?ref= value; empty string means default branch
func Render(src []byte, repoBase, filePath, ref string) (Result, error) {
	rw := &LinkRewriter{RepoBase: repoBase, FilePath: filePath, Ref: ref}
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			&frontmatter.Extender{},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(rw, 999)),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(), // allow raw HTML in Markdown (e.g. <img> tags)
		),
	)

	// Parse with a context so the frontmatter extension can store its data.
	pctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(pctx))

	// Decode frontmatter to check for toc: true.
	var fm struct {
		TOC bool `yaml:"toc"`
	}
	if d := frontmatter.Get(pctx); d != nil {
		_ = d.Decode(&fm) // ignore decode error; use zero value
	}

	// Extract TOC if the page requested it.
	var tocHTML template.HTML
	if fm.TOC {
		tree, err := toc.Inspect(doc, src)
		if err == nil && tree != nil {
			list := toc.RenderList(tree)
			if list != nil {
				var tocBuf bytes.Buffer
				_ = md.Renderer().Render(&tocBuf, src, list)
				tocHTML = template.HTML(`<nav class="toc-nav">` + tocBuf.String() + `</nav>`) //nolint:gosec // toc rendered by goldmark
			}
		}
	}

	// Render document HTML.
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return Result{}, err
	}
	return Result{
		Content: template.HTML(buf.String()), //nolint:gosec // goldmark output is sanitized HTML
		TOC:     tocHTML,
	}, nil
}
