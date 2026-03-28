package render

import (
	"strings"
	"testing"
)

// TestGridTable_BasicNoHeader tests a body-only table (no === separator).
func TestGridTable_BasicNoHeader(t *testing.T) {
	src := []byte(
		"+-------+-------+-------+\n" +
			"| A     | B     | C     |\n" +
			"+-------+-------+-------+\n" +
			"| D     | E     | F     |\n" +
			"+-------+-------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, "<tbody>") {
		t.Errorf("expected <tbody>, got: %q", html)
	}
	if !strings.Contains(html, "<td>") {
		t.Errorf("expected <td>, got: %q", html)
	}
	if strings.Contains(html, "<thead>") {
		t.Errorf("expected no <thead> in body-only table, got: %q", html)
	}
	// Verify actual cell text is rendered (not garbled grid border bytes).
	for _, want := range []string{"A", "B", "C", "D", "E", "F"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected cell text %q in output, got: %q", want, html)
		}
	}
}

// TestGridTable_HeaderTable tests a table with a === header/body separator.
func TestGridTable_HeaderTable(t *testing.T) {
	src := []byte(
		"+-------+-------+\n" +
			"| Head1 | Head2 |\n" +
			"+=======+=======+\n" +
			"| Row1  | Row2  |\n" +
			"+-------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, "<thead>") {
		t.Errorf("expected <thead>, got: %q", html)
	}
	if !strings.Contains(html, "<th>") {
		t.Errorf("expected <th>, got: %q", html)
	}
	if !strings.Contains(html, "<tbody>") {
		t.Errorf("expected <tbody>, got: %q", html)
	}
	if !strings.Contains(html, "<td>") {
		t.Errorf("expected <td>, got: %q", html)
	}
	// Verify actual cell content renders correctly.
	if !strings.Contains(html, "Head1") {
		t.Errorf("expected header text 'Head1', got: %q", html)
	}
	if !strings.Contains(html, "Head2") {
		t.Errorf("expected header text 'Head2', got: %q", html)
	}
	if !strings.Contains(html, "Row1") {
		t.Errorf("expected body text 'Row1', got: %q", html)
	}
	if !strings.Contains(html, "Row2") {
		t.Errorf("expected body text 'Row2', got: %q", html)
	}
}

// TestGridTable_HeaderOnly tests a table with only header rows and no body rows.
func TestGridTable_HeaderOnly(t *testing.T) {
	src := []byte(
		"+-------+-------+\n" +
			"| Head1 | Head2 |\n" +
			"+=======+=======+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, "<table>") {
		t.Errorf("expected <table>, got: %q", html)
	}
	if !strings.Contains(html, "<thead>") {
		t.Errorf("expected <thead>, got: %q", html)
	}
	// No body data rows.
	if strings.Contains(html, "<td>") {
		t.Errorf("expected no <td> in header-only table, got: %q", html)
	}
}

// TestGridTable_MultiLineCell tests a cell spanning two content lines.
// Both source lines are joined into a single <td>; the renderer emits a
// <br/> between them (WithHardWraps is enabled). The adjacent single-line
// cell must NOT contain a <br/>.
func TestGridTable_MultiLineCell(t *testing.T) {
	src := []byte(
		"+------------+-------+\n" +
			"| line one   | Other |\n" +
			"| line two   |       |\n" +
			"+------------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	// There must be a <td> element.
	tdIdx := strings.Index(html, "<td>")
	if tdIdx < 0 {
		t.Fatalf("no <td> found in: %q", html)
	}
	closeTdIdx := strings.Index(html[tdIdx:], "</td>")
	if closeTdIdx < 0 {
		t.Fatalf("no </td> found in: %q", html)
	}
	firstCell := html[tdIdx : tdIdx+closeTdIdx]
	// The first cell spans two content lines; with HardWraps enabled the
	// two lines are rendered as a single paragraph containing a <br/>.
	if !strings.Contains(firstCell, "<br/>") {
		t.Errorf("expected <br/> in multi-line cell (HardWraps), got: %q", firstCell)
	}
	// Verify both lines of text appear in the rendered output.
	if !strings.Contains(firstCell, "line one") {
		t.Errorf("expected 'line one' in multi-line cell, got: %q", firstCell)
	}
	if !strings.Contains(firstCell, "line two") {
		t.Errorf("expected 'line two' in multi-line cell, got: %q", firstCell)
	}
}

// TestGridTable_ParagraphBreakInCell tests that a blank line inside a cell
// produces two <p> tags inside a single <td>.
func TestGridTable_ParagraphBreakInCell(t *testing.T) {
	src := []byte(
		"+------------------+-------+\n" +
			"| Para 1           | Other |\n" +
			"|                  |       |\n" +
			"| Para 2           |       |\n" +
			"+------------------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	// Find the <td> that contains "Para 1".
	tdIdx := strings.Index(html, "<td>")
	closeTdIdx := strings.Index(html, "</td>")
	if tdIdx < 0 || closeTdIdx < 0 {
		t.Fatalf("could not find <td>...</td> in: %q", html)
	}
	cell := html[tdIdx:closeTdIdx]
	pCount := strings.Count(cell, "<p>")
	if pCount < 2 {
		t.Errorf("expected 2 <p> tags inside <td>, got %d; cell: %q", pCount, cell)
	}
}

// TestGridTable_BulletListInCell tests that bullet list items in a cell
// produce <ul> and <li> elements inside <td>.
func TestGridTable_BulletListInCell(t *testing.T) {
	src := []byte(
		"+----------------+-------+\n" +
			"| - item1        | Other |\n" +
			"| - item2        |       |\n" +
			"+----------------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, "<ul>") {
		t.Errorf("expected <ul>, got: %q", html)
	}
	if !strings.Contains(html, "<li>") {
		t.Errorf("expected <li>, got: %q", html)
	}
	// Verify <ul> appears inside the first <td>.
	tdIdx := strings.Index(html, "<td>")
	if tdIdx < 0 {
		t.Fatalf("no <td> found in: %q", html)
	}
	closeTdIdx := strings.Index(html[tdIdx:], "</td>")
	if closeTdIdx < 0 {
		t.Fatalf("no </td> found after <td> in: %q", html)
	}
	cell := html[tdIdx : tdIdx+closeTdIdx]
	if !strings.Contains(cell, "<ul>") {
		t.Errorf("expected <ul> inside <td>, cell: %q", cell)
	}
	if !strings.Contains(cell, "<li>") {
		t.Errorf("expected <li> inside <td>, cell: %q", cell)
	}
	// Verify actual list item text appears.
	if !strings.Contains(cell, "item1") {
		t.Errorf("expected 'item1' inside <td>, cell: %q", cell)
	}
	if !strings.Contains(cell, "item2") {
		t.Errorf("expected 'item2' inside <td>, cell: %q", cell)
	}
}

// TestGridTable_CodeFenceInCell tests that a fenced code block inside a cell
// produces a <code> element inside <td>.
func TestGridTable_CodeFenceInCell(t *testing.T) {
	src := []byte(
		"+--------------------+-------+\n" +
			"| ```go              | Other |\n" +
			"| fmt.Println(\"hi\")  |       |\n" +
			"| ```                |       |\n" +
			"+--------------------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, "<code>") && !strings.Contains(html, "<code ") {
		t.Errorf("expected <code> element, got: %q", html)
	}
	// Verify <code> appears inside the first <td>.
	tdIdx := strings.Index(html, "<td>")
	if tdIdx < 0 {
		t.Fatalf("no <td> found in: %q", html)
	}
	closeTdIdx := strings.Index(html[tdIdx:], "</td>")
	if closeTdIdx < 0 {
		t.Fatalf("no </td> found after <td> in: %q", html)
	}
	cell := html[tdIdx : tdIdx+closeTdIdx]
	if !strings.Contains(cell, "<code>") && !strings.Contains(cell, "<code ") {
		t.Errorf("expected <code> inside <td>, cell: %q", cell)
	}
	// Verify the actual code content appears.
	if !strings.Contains(cell, "fmt.Println") {
		t.Errorf("expected code content 'fmt.Println' inside <td>, cell: %q", cell)
	}
}

// TestGridTable_ColSpan tests that a cell spanning two columns emits colspan="2".
// The merge is indicated by a full-width separator (no interior '+') before the
// spanning row.
func TestGridTable_ColSpan(t *testing.T) {
	src := []byte(
		"+-------+-------+\n" +
			"| A     | B     |\n" +
			"+---------------+\n" +
			"| Span2         |\n" +
			"+-------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, `colspan="2"`) {
		t.Errorf("expected colspan=\"2\", got: %q", html)
	}
}

// TestGridTable_RowSpan tests that a cell spanning two rows emits rowspan="2".
// The span is indicated by a partial separator ('+       +-------+') where the
// spanning column region contains spaces instead of dashes.
func TestGridTable_RowSpan(t *testing.T) {
	src := []byte(
		"+-------+-------+\n" +
			"| SpanR | A     |\n" +
			"+       +-------+\n" +
			"|       | B     |\n" +
			"+-------+-------+\n",
	)
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, `rowspan="2"`) {
		t.Errorf("expected rowspan=\"2\", got: %q", html)
	}
}

// TestGridTable_LinkRewriteInCell tests that a relative .md link inside a cell
// gets rewritten to the repo-internal path in untrusted mode.
func TestGridTable_LinkRewriteInCell(t *testing.T) {
	src := []byte(
		"+--------------------------------+\n" +
			"| [page](other.md)               |\n" +
			"+--------------------------------+\n",
	)
	result, err := Render(src, "/github.com/owner/repo", "docs/index.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, `href="/github.com/owner/repo/docs/other.md"`) {
		t.Errorf("expected rewritten href, got: %q", html)
	}
}

// TestGridTable_InvalidTable tests that malformed grid table syntax does not
// panic and produces non-empty output (graceful fallback).
func TestGridTable_InvalidTable(t *testing.T) {
	// Truncated: only a top border, no content or closing border.
	src := []byte("+-------+-------+\n")
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if html == "" {
		t.Errorf("expected non-empty output for invalid table input, got empty string")
	}
}

// TestGridTable_ColspanRowspanThroughSanitizer tests that colspan and rowspan
// attributes survive the bluemonday sanitizer in untrusted mode.
// Two separate tables are used: one producing colspan="2" and one producing rowspan="2".
func TestGridTable_ColspanRowspanThroughSanitizer(t *testing.T) {
	src := []byte(
		// Table 1: two normal rows, then a merged separator creates a colspan=2 cell.
		"+-------+-------+\n" +
			"| A     | B     |\n" +
			"+---------------+\n" +
			"| Span2         |\n" +
			"+-------+-------+\n" +
			"\n" +
			// Table 2: partial separator creates a rowspan=2 cell.
			"+-------+-------+\n" +
			"| SpanR | A     |\n" +
			"+       +-------+\n" +
			"|       | B     |\n" +
			"+-------+-------+\n",
	)
	// Use trusted=false to trigger the bluemonday sanitizer path.
	result, err := Render(src, "/repo", "doc.md", "", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(result.Content)
	if !strings.Contains(html, `colspan="2"`) {
		t.Errorf("expected colspan=\"2\" to survive sanitizer, got: %q", html)
	}
	if !strings.Contains(html, `rowspan="2"`) {
		t.Errorf("expected rowspan=\"2\" to survive sanitizer, got: %q", html)
	}
}
