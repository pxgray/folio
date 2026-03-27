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
