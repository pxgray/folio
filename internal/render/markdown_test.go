package render

import (
	"strings"
	"testing"
)

func TestRender_NoTOC(t *testing.T) {
	src := []byte("# Hello\n\n## Section\n\nContent.")
	result, err := Render(src, "/repo", "doc.md", "")
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
	result, err := Render(src, "/repo", "doc.md", "")
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
	result, err := Render(src, "/repo", "doc.md", "")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.TOC != "" {
		t.Errorf("expected empty TOC with toc: false, got %q", result.TOC)
	}
}
