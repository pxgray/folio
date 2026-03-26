package nav

import (
	"testing"
)

func TestParse_Basic(t *testing.T) {
	input := []byte(`
title: My Project
nav:
  - Overview: docs/index.md
  - Getting Started: docs/getting-started.md
  - Reference:
    - API: docs/api/index.md
    - Config: docs/configuration.md
`)
	title, items, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if title != "My Project" {
		t.Errorf("title = %q, want %q", title, "My Project")
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}

	// Leaf item.
	if items[0].Label != "Overview" || items[0].Path != "docs/index.md" {
		t.Errorf("items[0] = %+v", items[0])
	}
	if len(items[0].Children) != 0 {
		t.Errorf("items[0] should have no children")
	}

	// Section with children.
	ref := items[2]
	if ref.Label != "Reference" || ref.Path != "" {
		t.Errorf("items[2] = %+v", ref)
	}
	if len(ref.Children) != 2 {
		t.Fatalf("len(ref.Children) = %d, want 2", len(ref.Children))
	}
	if ref.Children[0].Label != "API" || ref.Children[0].Path != "docs/api/index.md" {
		t.Errorf("ref.Children[0] = %+v", ref.Children[0])
	}
}

func TestParse_Empty(t *testing.T) {
	_, items, err := Parse([]byte("title: Empty\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %v", items)
	}
}

func TestAutoGenerate(t *testing.T) {
	tree := map[string][]WalkEntry{
		"": {
			{Name: "README.md", IsDir: false},
			{Name: "docs", IsDir: true},
			{Name: ".hidden", IsDir: false},
		},
		"docs": {
			{Name: "index.md", IsDir: false},
			{Name: "setup.md", IsDir: false},
			{Name: "api", IsDir: true},
		},
		"docs/api": {
			{Name: "index.md", IsDir: false},
		},
	}
	walker := func(p string) ([]WalkEntry, error) {
		return tree[p], nil
	}

	items := AutoGenerate(walker, "")

	// Should have README.md + docs/ section.
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(items), items)
	}
	// docs/ comes first (dir before file).
	if items[0].Label != "docs" || len(items[0].Children) == 0 {
		t.Errorf("items[0] = %+v", items[0])
	}
	// README.md label.
	if items[1].Label != "README" || items[1].Path != "README.md" {
		t.Errorf("items[1] = %+v", items[1])
	}

	// docs/ children: api/ first, then index.md, setup.md.
	docsChildren := items[0].Children
	if docsChildren[0].Label != "api" {
		t.Errorf("docs children[0] = %+v, want api", docsChildren[0])
	}
	if docsChildren[1].Label != "index" || docsChildren[1].Path != "docs/index.md" {
		t.Errorf("docs children[1] = %+v", docsChildren[1])
	}
}

func TestLabelFromFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"index.md", "index"},
		{"getting-started.md", "getting started"},
		{"api_reference.md", "api reference"},
		{"README.md", "README"},
	}
	for _, c := range cases {
		if got := labelFromFilename(c.in); got != c.want {
			t.Errorf("labelFromFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
