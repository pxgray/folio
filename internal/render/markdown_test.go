package render

import (
	"strings"
	"testing"
)

func TestRender_NoTOC(t *testing.T) {
	src := []byte("# Hello\n\n## Section\n\nContent.")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.TOC != "" {
		t.Errorf("expected empty TOC without frontmatter, got %q", result.TOC)
	}
	if !strings.Contains(string(result.Content), "<h1") {
		t.Errorf("expected h1 in content, got %q", result.Content)
	}
}

func TestRender_WithTOC(t *testing.T) {
	src := []byte("---\ntoc: true\n---\n# Hello\n\n## Section One\n\n## Section Two\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.TOC == "" {
		t.Error("expected non-empty TOC with toc: true in frontmatter")
	}
	if !strings.Contains(string(result.TOC), `class="toc-nav"`) {
		t.Errorf("expected toc-nav class in TOC HTML, got %q", result.TOC)
	}
	if !strings.Contains(string(result.TOC), "Section One") {
		t.Errorf("expected section heading in TOC, got %q", result.TOC)
	}
	// Frontmatter should not appear in content.
	if strings.Contains(string(result.Content), "toc: true") {
		t.Errorf("frontmatter leaked into content: %q", result.Content)
	}
}

func TestRender_TOCFalseExplicit(t *testing.T) {
	src := []byte("---\ntoc: false\n---\n# Hello\n\n## Section\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.TOC != "" {
		t.Errorf("expected empty TOC with toc: false, got %q", result.TOC)
	}
}

func TestRender_XSS_Untrusted(t *testing.T) {
	// Without WithUnsafe(), goldmark escapes raw HTML.
	// <script> must not appear as a live tag in untrusted mode.
	src := []byte("# Title\n\n<script>alert(1)</script>\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(result.Content), "<script>") {
		t.Errorf("script tag not stripped in untrusted mode, got: %q", result.Content)
	}
}

func TestRender_XSS_Trusted(t *testing.T) {
	// WithUnsafe() is on in trusted mode: raw HTML passes through.
	src := []byte("# Title\n\n<script>alert(1)</script>\n")
	result, err := Render(src, "/repo", "doc.md", "", true)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(result.Content), "<script>") {
		t.Errorf("script tag unexpectedly absent in trusted mode, got: %q", result.Content)
	}
}

func TestRender_SyntaxHighlighting_WithLanguage(t *testing.T) {
	src := []byte("```go\nfunc main() {}\n```\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	// Chroma wraps the block in <pre class="chroma">.
	if !strings.Contains(html, `class="chroma"`) {
		t.Errorf("expected chroma class on pre, got: %q", html)
	}
	// Chroma emits token spans for a known language.
	if !strings.Contains(html, `<span class="`) {
		t.Errorf("expected token spans in highlighted output, got: %q", html)
	}
}

func TestRender_SyntaxHighlighting_NoLanguage(t *testing.T) {
	src := []byte("```\nplain text\n```\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	// Should still produce a pre/code block even without a language.
	if !strings.Contains(html, "<pre") {
		t.Errorf("expected pre element, got: %q", html)
	}
	if !strings.Contains(html, "plain text") {
		t.Errorf("expected code content, got: %q", html)
	}
}

func TestRender_SyntaxHighlighting_ClassesPreservedUntrusted(t *testing.T) {
	src := []byte("```go\nvar x = 1\n```\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	// Bluemonday must not strip chroma span classes.
	if !strings.Contains(html, `class="`) {
		t.Errorf("bluemonday stripped all classes from highlighted output: %q", html)
	}
}

func TestRender_MarkdownSyntax_Untrusted(t *testing.T) {
	// Markdown syntax (not raw HTML) must render correctly in untrusted mode.
	src := []byte("# Title\n\n**bold** and *em* and [link](https://example.com)\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(result.Content), "<strong>bold</strong>") {
		t.Errorf("bold not rendered: %q", result.Content)
	}
	if !strings.Contains(string(result.Content), "<em>em</em>") {
		t.Errorf("em not rendered: %q", result.Content)
	}
	if !strings.Contains(string(result.Content), `href="https://example.com"`) {
		t.Errorf("link not rendered: %q", result.Content)
	}
}
