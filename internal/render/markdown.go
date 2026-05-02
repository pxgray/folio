package render

import (
	"bytes"
	"html/template"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/toc"
)

// untrustedPolicy is the bluemonday policy for sanitizing untrusted Markdown output.
// It extends UGCPolicy to preserve heading IDs, which are required for TOC anchor links.
var untrustedPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Preserve goldmark-generated heading IDs so TOC anchor links work.
	p.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	// Preserve colspan/rowspan on table cells for RST grid table rendering.
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	// Preserve chroma syntax-highlighting classes on code elements.
	p.AllowAttrs("class").OnElements("pre", "code", "span")
	// Preserve mermaid diagram code attributes.
	p.AllowAttrs("data-language").OnElements("pre", "code")
	return p
}()

// rwPool pools LinkRewriter instances to reduce allocations across Render calls.
var rwPool = sync.Pool{
	New: func() any { return &LinkRewriter{} },
}

// goldmarkUntrusted and goldmarkTrusted are pre-configured goldmark instances
// shared across all Render calls. They are immutable after construction and
// safe for concurrent use. The baked-in transformer delegates to the per-call
// LinkRewriter stored in a package-level pointer, set before each Parse call.
var (
	goldmarkUntrusted  goldmark.Markdown
	goldmarkTrusted    goldmark.Markdown
	activeLinkRewriter *LinkRewriter
)

// bakedInTransformer is a goldmark AST transformer baked into the goldmark
// config. It delegates to the per-call LinkRewriter via activeLinkRewriter.
type bakedInTransformer struct{}

func (bakedInTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	if activeLinkRewriter != nil {
		activeLinkRewriter.Transform(node, reader, pc)
	}
}

func init() {
	rendererOptsUntrusted := []renderer.Option{html.WithHardWraps(), html.WithXHTML()}
	rendererOptsTrusted := []renderer.Option{html.WithHardWraps(), html.WithXHTML(), html.WithUnsafe()}

	goldmarkUntrusted = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			&frontmatter.Extender{},
			&GridTableExtension{},
			&mermaid.Extender{},
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(bakedInTransformer{}, 999)),
		),
		goldmark.WithRendererOptions(rendererOptsUntrusted...),
	)

	goldmarkTrusted = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			&frontmatter.Extender{},
			&GridTableExtension{},
			&mermaid.Extender{},
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(bakedInTransformer{}, 999)),
		),
		goldmark.WithRendererOptions(rendererOptsTrusted...),
	)
}

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
//   - trusted:  when false, raw HTML in Markdown is escaped and output is
//     sanitized via bluemonday; when true, raw HTML passes through
func Render(src []byte, repoBase, filePath, ref string, trusted bool) (Result, error) {
	rw := rwPool.Get().(*LinkRewriter)
	rw.RepoBase = repoBase
	rw.FilePath = filePath
	rw.Ref = ref

	// Set the active LinkRewriter for the baked-in transformer to delegate to.
	// This is safe because goldmark Parse is synchronous (not concurrent).
	activeLinkRewriter = rw
	defer func() {
		activeLinkRewriter = nil
		rwPool.Put(rw)
	}()

	md := goldmarkUntrusted
	if trusted {
		md = goldmarkTrusted
	}

	// Parse with a context so the frontmatter extension can store its data.
	pctx := parser.NewContext()
	pctx.Set(gridTableLinkRewriterKey, rw)
	pctx.Set(gridTableTrustedKey, trusted)
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
				// TOC is goldmark-generated from heading IDs; always safe.
				tocHTML = template.HTML(`<nav class="toc-nav">` + tocBuf.String() + `</nav>`) //nolint:gosec
			}
		}
	}

	// Render document HTML.
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return Result{}, err
	}

	var content template.HTML
	if trusted {
		// Trusted repos may embed raw HTML; no sanitization applied.
		content = template.HTML(buf.String()) //nolint:gosec
	} else {
		// Untrusted repos: goldmark already escapes raw HTML (no WithUnsafe).
		// bluemonday is defense-in-depth against any goldmark edge cases.
		content = template.HTML(untrustedPolicy.SanitizeBytes(buf.Bytes())) //nolint:gosec
	}

	return Result{Content: content, TOC: tocHTML}, nil
}
