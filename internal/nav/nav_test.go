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

func TestNavCoversPath(t *testing.T) {
	items := []Item{
		{Label: "API", Path: "docs/api/index.md"},
		{Label: "Config", Path: "docs/configuration.md"},
		{Label: "Reference", Children: []Item{
			{Label: "API", Path: "docs/api/index.md"},
		}},
	}

	cases := []struct {
		filePath string
		want     bool
	}{
		{"docs/api/index.md", true},
		{"docs/configuration.md", true},
		{"docs/api/reference.md", true},
		{"docs/api/v2/endpoints.md", true},
		{"docs/getting-started.md", false},
		{"other/file.md", false},
		{"", false},
	}

	for _, c := range cases {
		got := navCoversPath(items, c.filePath)
		if got != c.want {
			t.Errorf("navCoversPath(%q) = %v, want %v", c.filePath, got, c.want)
		}
	}
}

func TestNavCoversPath_SiblingPaths(t *testing.T) {
	// navCoversPath should cover sibling files in the same directory
	// and nested children under that directory.
	items := []Item{
		{Label: "API Index", Path: "docs/api/index.md"},
	}

	// A sibling file at the same level should be covered.
	if !navCoversPath(items, "docs/api/reference.md") {
		t.Error("sibling file should be covered by nav item")
	}

	// A child file nested under the same directory should be covered.
	if !navCoversPath(items, "docs/api/v2/endpoints.md") {
		t.Error("nested child file should be covered by nav item")
	}

	// A file in a different directory should NOT be covered.
	if navCoversPath(items, "docs/getting-started.md") {
		t.Error("file in different directory should not be covered")
	}
}

func TestFindActiveSection(t *testing.T) {
	sections := []Section{
		{
			Label:       "Docs",
			DefaultPath: "docs/index.md",
			Nav: []Item{
				{Label: "Overview", Path: "docs/index.md"},
				{Label: "Getting Started", Path: "docs/getting-started.md"},
			},
		},
		{
			Label:       "API",
			DefaultPath: "docs/api/index.md",
			Nav: []Item{
				{Label: "API", Path: "docs/api/index.md"},
				{Label: "Config", Path: "docs/configuration.md"},
			},
		},
	}

	cases := []struct {
		filePath         string
		wantSectionLabel string
		wantIndex        int
		wantFound        bool
	}{
		{"docs/index.md", "Docs", 0, true},
		{"docs/getting-started.md", "Docs", 0, true},
		{"docs/api/index.md", "API", 1, true},
		{"docs/configuration.md", "API", 1, true},
		{"docs/api/reference.md", "API", 1, true},
		{"other/file.md", "", -1, false},
	}

	for _, c := range cases {
		sec, idx, found := FindActiveSection(sections, c.filePath)
		if found != c.wantFound {
			t.Errorf("FindActiveSection(%q): found = %v, want %v", c.filePath, found, c.wantFound)
		}
		if found && sec.Label != c.wantSectionLabel {
			t.Errorf("FindActiveSection(%q): section = %q, want %q", c.filePath, sec.Label, c.wantSectionLabel)
		}
		if found && idx != c.wantIndex {
			t.Errorf("FindActiveSection(%q): index = %d, want %d", c.filePath, idx, c.wantIndex)
		}
	}
}

func TestFindActiveSection_SiblingFiles(t *testing.T) {
	// Regression: sibling files within the same nav section should
	// all resolve to the same active section.
	sections := []Section{
		{
			Label: "Reference",
			Nav: []Item{
				{Label: "API", Path: "docs/api/index.md"},
			},
		},
	}

	// These are all under docs/api/ but different leaf files.
	filePaths := []string{
		"docs/api/index.md",
		"docs/api/reference.md",
		"docs/api/v2/endpoints.md",
	}

	for _, fp := range filePaths {
		sec, _, found := FindActiveSection(sections, fp)
		if !found {
			t.Errorf("FindActiveSection(%q) not found, expected Reference section", fp)
		}
		if sec.Label != "Reference" {
			t.Errorf("FindActiveSection(%q): section = %q, want %q", fp, sec.Label, "Reference")
		}
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
