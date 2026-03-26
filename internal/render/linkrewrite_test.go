package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

func renderWithRewriter(t *testing.T, src, repoBase, filePath, ref string) string {
	t.Helper()
	rw := &LinkRewriter{RepoBase: repoBase, FilePath: filePath, Ref: ref}
	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(rw, 999)),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestLinkRewrite_AbsoluteURLUnchanged(t *testing.T) {
	out := renderWithRewriter(t,
		"[link](https://example.com/foo)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="https://example.com/foo"`) {
		t.Errorf("absolute URL was rewritten: %s", out)
	}
}

func TestLinkRewrite_FragmentUnchanged(t *testing.T) {
	out := renderWithRewriter(t,
		"[link](#section)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="#section"`) {
		t.Errorf("fragment link was rewritten: %s", out)
	}
}

func TestLinkRewrite_AbsolutePathUnchanged(t *testing.T) {
	out := renderWithRewriter(t,
		"[link](/some/path)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="/some/path"`) {
		t.Errorf("absolute path was rewritten: %s", out)
	}
}

func TestLinkRewrite_RelativeMdSameDir(t *testing.T) {
	out := renderWithRewriter(t,
		"[setup](setup.md)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="/github.com/owner/repo/docs/setup.md"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_RelativeMdParentDir(t *testing.T) {
	out := renderWithRewriter(t,
		"[root](../README.md)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="/github.com/owner/repo/README.md"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_RelativeMdWithRef(t *testing.T) {
	out := renderWithRewriter(t,
		"[setup](setup.md)",
		"/github.com/owner/repo", "docs/index.md", "abc1234")
	if !strings.Contains(out, `href="/github.com/owner/repo/docs/setup.md?ref=abc1234"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_ImageRaw(t *testing.T) {
	out := renderWithRewriter(t,
		"![logo](../static/logo.png)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `src="/github.com/owner/repo/-/raw/static/logo.png"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_ImageRawWithRef(t *testing.T) {
	out := renderWithRewriter(t,
		"![logo](../static/logo.png)",
		"/github.com/owner/repo", "docs/index.md", "main")
	if !strings.Contains(out, `src="/github.com/owner/repo/-/raw/static/logo.png?ref=main"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_EscapingRepoRoot(t *testing.T) {
	// A link that would escape the repo root should be left unchanged.
	out := renderWithRewriter(t,
		"[bad](../../outside.md)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="../../outside.md"`) {
		t.Errorf("escaping link was rewritten: %s", out)
	}
}

func TestLinkRewrite_RootFileMdLink(t *testing.T) {
	// File at repo root linking to another root-level file.
	out := renderWithRewriter(t,
		"[guide](guide.md)",
		"/github.com/owner/repo", "README.md", "")
	if !strings.Contains(out, `href="/github.com/owner/repo/guide.md"`) {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestLinkRewrite_NonMdFileRaw(t *testing.T) {
	out := renderWithRewriter(t,
		"[download](../dist/app.tar.gz)",
		"/github.com/owner/repo", "docs/index.md", "")
	if !strings.Contains(out, `href="/github.com/owner/repo/-/raw/dist/app.tar.gz"`) {
		t.Errorf("unexpected output: %s", out)
	}
}
