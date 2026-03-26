// Package nav handles sidebar navigation for a repo.
// It reads folio.yml from the repo root (if present) or auto-generates
// navigation from the repository's directory tree.
package nav

import (
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Item is a single entry in the sidebar navigation tree.
type Item struct {
	Label    string
	Path     string // repo-relative path to the target file; empty for section headers
	Children []Item
}

// folioYML is the parsed shape of the folio.yml nav config.
type folioYML struct {
	Title string    `yaml:"title"`
	Nav   yaml.Node `yaml:"nav"`
}

// Parse parses a folio.yml file and returns the nav items.
// The expected YAML shape is:
//
//	title: My Project
//	nav:
//	  - Overview: docs/index.md
//	  - Getting Started: docs/getting-started.md
//	  - Reference:
//	    - API: docs/api/index.md
//	    - Config: docs/configuration.md
func Parse(data []byte) (title string, items []Item, err error) {
	var f folioYML
	if err = yaml.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	title = f.Title
	items = parseNavNode(&f.Nav)
	return title, items, nil
}

// parseNavNode converts a yaml.Node (expected to be a sequence) into []Item.
func parseNavNode(node *yaml.Node) []Item {
	if node == nil || node.Kind == 0 {
		return nil
	}
	// Dereference aliases/documents.
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return parseNavNode(node.Content[0])
	}
	if node.Kind != yaml.SequenceNode {
		return nil
	}

	var items []Item
	for _, entry := range node.Content {
		if entry.Kind != yaml.MappingNode || len(entry.Content) < 2 {
			continue
		}
		// Each map entry has key at [0] and value at [1].
		key := entry.Content[0].Value
		val := entry.Content[1]

		switch val.Kind {
		case yaml.ScalarNode:
			// Leaf: "Label: path/to/file.md"
			items = append(items, Item{Label: key, Path: val.Value})
		case yaml.SequenceNode:
			// Section: "Label:" with a list of children.
			items = append(items, Item{Label: key, Children: parseNavNode(val)})
		}
	}
	return items
}

// AutoGenerate builds a navigation tree by walking the provided tree entries.
// entries is a flat list of (name, isDir) pairs at dirPath within the repo.
// The walker function is called recursively for subdirectories.
func AutoGenerate(walker func(dirPath string) ([]WalkEntry, error), rootPath string) []Item {
	return autoGenDir(walker, rootPath)
}

// WalkEntry is a single entry returned by the walker function.
type WalkEntry struct {
	Name  string
	IsDir bool
}

func autoGenDir(walker func(string) ([]WalkEntry, error), dirPath string) []Item {
	entries, err := walker(dirPath)
	if err != nil {
		return nil
	}

	// Sort: directories first, then alphabetical.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	var items []Item
	for _, e := range entries {
		if strings.HasPrefix(e.Name, ".") || strings.HasPrefix(e.Name, "_") {
			continue // skip hidden / private entries
		}
		if e.IsDir {
			children := autoGenDir(walker, path.Join(dirPath, e.Name))
			if len(children) > 0 {
				items = append(items, Item{Label: e.Name, Children: children})
			}
		} else if strings.HasSuffix(e.Name, ".md") {
			label := labelFromFilename(e.Name)
			filePath := path.Join(dirPath, e.Name)
			if dirPath == "" {
				filePath = e.Name
			}
			items = append(items, Item{Label: label, Path: filePath})
		}
	}
	return items
}

// labelFromFilename converts a filename like "getting-started.md" → "getting started".
func labelFromFilename(name string) string {
	name = strings.TrimSuffix(name, ".md")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}
