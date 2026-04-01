# Plan: Top Navigation Bar with Per-Repo Sections

## Goal
Add a top navigation bar that allows a single repo to define multiple "sections," each with its own left sidebar navigation. Sections are configured in `folio.yml` and do not need to correspond to specific URL patterns — files can be anywhere in the repo.

## Design Overview

### folio.yml Format (New)
```yaml
title: My Project
sections:
  - label: Docs
    nav:
      - Overview: docs/index.md
      - Getting Started: docs/getting-started.md
      - Reference:
        - API: docs/api.md
  - label: API Reference
    nav:
      - Endpoints: api/endpoints.md
      - Models: api/models.md
  - label: Guides
    nav:
      - Tutorial: guides/tutorial.md
      - Migration: guides/migration.md
```

**Backward compatibility**: If `sections` is absent, existing behavior continues unchanged (single nav from `nav:` key).

### Active Section Detection
The active section is determined by checking which section's nav tree contains the current page's file path. If no section matches (or `sections` is empty), no top nav is rendered.

---

## Implementation Steps

### 1. Extend `nav` Package — Data Structures

**File:** `internal/nav/nav.go`

- Add new `Section` struct:
  ```go
  type Section struct {
      Label       string
      Nav         []Item
      DefaultPath string // first leaf path, for tab linking
  }
  ```
- Add new `ParseResult` struct:
  ```go
  type ParseResult struct {
      Title    string
      Sections []Section  // nil when sections not defined
  }
  ```
- Add `ParseWithSections(data []byte) (ParseResult, error)` function that parses both `sections` and legacy `nav` keys
- Keep existing `Parse()` function for backward compatibility (internally calls `ParseWithSections` and returns the first section's nav or the legacy nav)

### 2. Extend `nav` Package — Section Parsing

**File:** `internal/nav/nav.go`

- Add internal `sectionYML` struct for YAML parsing:
  ```go
  type sectionYML struct {
      Label string    `yaml:"label"`
      Nav   yaml.Node `yaml:"nav"`
  }
  ```
- Add `parseSections(node *yaml.Node) []Section` function
- Handle both `sections:` key (new) and `nav:` key (legacy) in the YAML

### 3. Extend `nav` Package — Active Section Lookup

**File:** `internal/nav/nav.go`

- Add `FindActiveSection(sections []Section, filePath string) (Section, int, bool)` function
- Uses existing `navCoversPath` logic to find which section's nav contains the current file
- Returns the section, its index, and whether one was found

### 4. Extend Data Model

**File:** `internal/web/doc.go`

- Extend `docData` struct with new fields:
  ```go
  type docData struct {
      // ... existing fields ...
      Sections      []nav.Section  // all top nav sections
      ActiveSection int            // index of active section (-1 if none)
  }
  ```

### 5. Update Nav Loading in Web Handler

**File:** `internal/web/doc.go`

- Modify `loadNav()` to return `nav.ParseResult` instead of `[]nav.Item`
- Rename to `loadNavResult()` or similar
- Update `serveRepo()` and `serveRepoRoot()` to use the new result
- Update `loadNavAndCheck()` to work with sections:
  - If sections exist, find active section and return its nav
  - If no sections, fall back to existing single-nav behavior
- Update `navCoversPath` calls to check against the active section's nav

### 6. Update Template — base.html

**File:** `internal/assets/templates/base.html`

- Add top navigation bar between the existing `<header class="topbar">` and `<div class="page-body">`:
  ```html
  {{if .Sections}}
  <nav class="topnav" aria-label="sections">
    <ul class="topnav-list">
      {{range $i, $section := .Sections}}
      <li class="topnav-item{{if eq $i $.ActiveSection}} active{{end}}">
        <a href="{{$.RepoBase}}/{{$section.DefaultPath}}{{if $.Ref}}?ref={{$.Ref}}{{end}}">
          {{$section.Label}}
        </a>
      </li>
      {{end}}
    </ul>
  </nav>
  {{end}}
  ```

### 7. Update `firstNavLeaf` Usage

**File:** `internal/web/doc.go`

- The existing `firstNavLeaf()` function finds the first leaf in a `[]nav.Item`
- Use it to compute each section's `DefaultPath` when building the `ParseResult`
- This can be done in the nav package itself during parsing, or in the web handler

### 8. Add CSS Styles

**File:** `internal/assets/static/style.css`

Add styles for the top navigation bar after the existing `.topbar` styles:
```css
/* ── Top Navigation (Sections) ── */
.topnav {
  background: var(--pico-background-color);
  border-bottom: 1px solid var(--pico-muted-border-color);
  padding: 0 1.5rem;
}
.topnav-list {
  display: flex;
  list-style: none;
  margin: 0 !important;
  padding: 0;
  gap: 0;
}
.topnav-item { margin: 0; }
.topnav-item a {
  display: block;
  padding: 0.6rem 1rem;
  font-size: 0.9rem;
  color: var(--pico-muted-color);
  text-decoration: none;
  border-bottom: 2px solid transparent;
}
.topnav-item a:hover {
  color: var(--pico-color);
}
.topnav-item.active a {
  color: var(--pico-primary);
  border-bottom-color: var(--pico-primary);
  font-weight: 600;
}

@media (max-width: 1024px) {
  .topnav {
    padding: 0 1rem;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }
  .topnav-list {
    flex-wrap: nowrap;
  }
}
```

### 9. Update `serveMarkdownPage` to Pass Sections

**File:** `internal/web/doc.go`

- In `serveMarkdownPage`, populate `Sections` and `ActiveSection` fields in `docData`
- Use the `ParseResult.Sections` from nav loading
- Use `FindActiveSection` to determine the active index

---

## Files Modified

| File | Changes |
|------|---------|
| `internal/nav/nav.go` | Add `Section`, `ParseResult`, `ParseWithSections`, `FindActiveSection` |
| `internal/web/doc.go` | Update `docData`, `loadNav`, `loadNavAndCheck`, `serveMarkdownPage` |
| `internal/assets/templates/base.html` | Add top nav bar HTML |
| `internal/assets/static/style.css` | Add `.topnav-*` styles |

## Backward Compatibility

- Existing `folio.yml` files with only `title:` and `nav:` continue to work unchanged
- The `Parse()` function signature is preserved
- New `ParseWithSections()` is additive
- If `sections` is not defined, no top nav is rendered

## Edge Cases

1. **Empty sections list**: Treated as no sections (fallback to single nav)
2. **Section with empty nav**: Section tab still renders but links to repo root
3. **Page not covered by any section**: No top nav rendered, sidebar uses legacy nav if available
4. **Single section**: Top nav still renders (user may add more later)
5. **Mobile**: Top nav scrolls horizontally if sections overflow
