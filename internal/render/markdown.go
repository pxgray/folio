package render

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Render converts Markdown src to an HTML fragment (safe for use in
// html/template with template.HTML).
//
//   - repoBase: URL prefix for this repo, e.g. "/github.com/owner/repo"
//   - filePath: path of the current file within the repo, e.g. "docs/setup.md"
//   - ref:      ?ref= value; empty string means default branch
func Render(src []byte, repoBase, filePath, ref string) (template.HTML, error) {
	rw := &LinkRewriter{RepoBase: repoBase, FilePath: filePath, Ref: ref}
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
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

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil //nolint:gosec // goldmark output is sanitized HTML
}
