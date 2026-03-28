package render

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ---------------------------------------------------------------------------
// AST Node Kinds
// ---------------------------------------------------------------------------

// Node kinds for RST-style grid table elements.
var (
	KindGridTable        = ast.NewNodeKind("GridTable")
	KindGridTableSection = ast.NewNodeKind("GridTableSection")
	KindGridTableRow     = ast.NewNodeKind("GridTableRow")
	KindGridTableCell    = ast.NewNodeKind("GridTableCell")
)

// ---------------------------------------------------------------------------
// AST Node Types
// ---------------------------------------------------------------------------

// GridTable is a block node representing an RST-style grid table.
type GridTable struct {
	ast.BaseBlock
}

// Kind returns KindGridTable.
func (n *GridTable) Kind() ast.NodeKind { return KindGridTable }

// Dump dumps the node for debugging.
func (n *GridTable) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// NewGridTable creates a new GridTable node.
func NewGridTable() *GridTable {
	return &GridTable{}
}

// GridTableSection represents either <thead> or <tbody>.
type GridTableSection struct {
	ast.BaseBlock
	IsHead bool
}

// Kind returns KindGridTableSection.
func (n *GridTableSection) Kind() ast.NodeKind { return KindGridTableSection }

// Dump dumps the node for debugging.
func (n *GridTableSection) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"IsHead": fmt.Sprintf("%v", n.IsHead),
	}, nil)
}

// NewGridTableSection creates a new GridTableSection node.
func NewGridTableSection(isHead bool) *GridTableSection {
	return &GridTableSection{IsHead: isHead}
}

// GridTableRow represents <tr>.
type GridTableRow struct {
	ast.BaseBlock
}

// Kind returns KindGridTableRow.
func (n *GridTableRow) Kind() ast.NodeKind { return KindGridTableRow }

// Dump dumps the node for debugging.
func (n *GridTableRow) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// NewGridTableRow creates a new GridTableRow node.
func NewGridTableRow() *GridTableRow {
	return &GridTableRow{}
}

// GridTableCell represents <td> or <th>.
type GridTableCell struct {
	ast.BaseBlock
	ColSpan    int
	RowSpan    int
	IsHead     bool
	rawContent []byte
}

// Kind returns KindGridTableCell.
func (n *GridTableCell) Kind() ast.NodeKind { return KindGridTableCell }

// Dump dumps the node for debugging.
func (n *GridTableCell) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"ColSpan": fmt.Sprintf("%d", n.ColSpan),
		"RowSpan": fmt.Sprintf("%d", n.RowSpan),
		"IsHead":  fmt.Sprintf("%v", n.IsHead),
	}, nil)
}

// NewGridTableCell creates a new GridTableCell node.
func NewGridTableCell(colSpan, rowSpan int, isHead bool) *GridTableCell {
	return &GridTableCell{
		ColSpan: colSpan,
		RowSpan: rowSpan,
		IsHead:  isHead,
	}
}

// ---------------------------------------------------------------------------
// Grid Parsing Logic
// ---------------------------------------------------------------------------

// parsedCell holds the result of parsing a single cell from the grid.
type parsedCell struct {
	Row, Col         int
	RowSpan, ColSpan int
	IsHead           bool
	Content          []byte // stripped, ready for recursive parse
}

// topBorderPattern matches a valid top border line: +---+---+ etc.
var topBorderPattern = regexp.MustCompile(`^\+[-=+]+\+\s*$`)

// parseGrid parses the raw grid table lines into a slice of parsedCells.
func parseGrid(lines [][]byte) ([]parsedCell, error) {
	if len(lines) < 3 {
		return nil, fmt.Errorf("grid table too short: need at least 3 lines")
	}

	// Step 1: Extract column boundary positions from the top border.
	topBorder := lines[0]
	colBounds := extractColBounds(topBorder)
	if len(colBounds) < 2 {
		return nil, fmt.Errorf("grid table top border must have at least 2 column boundaries")
	}
	numCols := len(colBounds) - 1

	// Step 2-5: Process lines, build cells.
	type activeCell struct {
		startRow int
		col      int
		colSpan  int
		isHead   bool
		lines    [][]byte
	}

	// activeCells tracks the currently open cell for each column.
	// nil means no cell is open for that column.
	activeCells := make([]*activeCell, numCols)

	var result []parsedCell
	logicalRow := -1 // incremented on each full separator
	headBodyBoundary := -1

	// classifySeparatorLine determines which columns end and which continue,
	// and whether the line is a head separator (contains '=').
	classifySeparatorLine := func(line []byte) (ends []bool, isHeadSep bool) {
		ends = make([]bool, numCols)
		isHeadSep = false
		for i := 0; i < numCols; i++ {
			left := colBounds[i]
			right := colBounds[i+1]
			if right > len(line) {
				right = len(line)
			}
			if left+1 >= right {
				// Degenerate column
				ends[i] = true
				continue
			}
			region := line[left+1 : right]
			allSep := true
			hasEquals := false
			for _, ch := range region {
				if ch == '=' {
					hasEquals = true
				} else if ch != '-' {
					allSep = false
					break
				}
			}
			if allSep {
				ends[i] = true
				if hasEquals {
					isHeadSep = true
				}
			}
			// else: content continues, cell spans
		}
		return ends, isHeadSep
	}

	// extractCellContentSpanning extracts content for a spanning cell covering
	// columns [startCol, startCol+colSpan).
	extractCellContentSpanning := func(line []byte, startCol, colSpan int) []byte {
		left := colBounds[startCol] + 1
		endCol := startCol + colSpan
		if endCol > numCols {
			endCol = numCols
		}
		right := colBounds[endCol]
		if left >= len(line) {
			return nil
		}
		if right > len(line) {
			right = len(line)
		}
		return line[left:right]
	}

	// finalizeCell finishes an active cell and adds it to results.
	finalizeCell := func(ac *activeCell) {
		content := stripConsistentPadding(ac.lines)
		result = append(result, parsedCell{
			Row:     ac.startRow,
			Col:     ac.col,
			RowSpan: logicalRow - ac.startRow + 1,
			ColSpan: ac.colSpan,
			IsHead:  ac.isHead,
			Content: content,
		})
	}

	// detectColSpanGroups looks at a separator line and determines which
	// columns are merged (no '+' at interior boundary).
	detectColSpanGroups := func(line []byte) [][]int {
		// Each group is a list of column indices that are merged.
		var groups [][]int
		currentGroup := []int{0}
		for i := 1; i < numCols; i++ {
			pos := colBounds[i]
			if pos < len(line) && line[pos] == '+' {
				groups = append(groups, currentGroup)
				currentGroup = []int{i}
			} else {
				currentGroup = append(currentGroup, i)
			}
		}
		groups = append(groups, currentGroup)
		return groups
	}

	// Process line 0 (the top border) to initialise cells for the first row.
	// The top border acts as a full separator that opens the first row of cells.
	{
		topLine := lines[0]
		groups := detectColSpanGroups(topLine)
		logicalRow = 0
		for _, group := range groups {
			startCol := group[0]
			colSpan := len(group)
			ac := &activeCell{
				startRow: 0,
				col:      startCol,
				colSpan:  colSpan,
				isHead:   true, // tentative; corrected in the IsHead fixup pass
			}
			for _, c := range group {
				activeCells[c] = ac
			}
		}
	}

	// Process remaining lines (content and separators).
	for lineIdx := 1; lineIdx < len(lines); lineIdx++ {
		line := lines[lineIdx]
		if len(line) == 0 {
			continue
		}

		if line[0] == '+' {
			// Separator line (full or partial).
			ends, isHeadSep := classifySeparatorLine(line)

			// Count how many columns end vs continue.
			endCount := 0
			for _, e := range ends {
				if e {
					endCount++
				}
			}

			isFullSep := endCount == numCols

			if isFullSep {
				// Full separator: finalize all active cells, start new row.
				// Use a seen set to avoid double-finalizing colspan cells
				// (multiple activeCells entries may point to the same activeCell).
				seen := make(map[*activeCell]bool)
				for i := 0; i < numCols; i++ {
					if activeCells[i] != nil {
						ac := activeCells[i]
						if !seen[ac] {
							seen[ac] = true
							finalizeCell(ac)
						}
						activeCells[i] = nil
					}
				}

				if isHeadSep {
					headBodyBoundary = logicalRow + 1
				}

				logicalRow++

				// Detect colspan groups from this separator line.
				groups := detectColSpanGroups(line)
				for _, group := range groups {
					startCol := group[0]
					colSpan := len(group)
					isHead := headBodyBoundary < 0 // no head/body boundary yet → head

					ac := &activeCell{
						startRow: logicalRow,
						col:      startCol,
						colSpan:  colSpan,
						isHead:   isHead,
					}
					for _, c := range group {
						activeCells[c] = ac
					}
				}
			} else {
				// Partial separator: some columns end, some continue.
				// Finalize cells for columns that end, deduplicating colspan cells.
				seenPartial := make(map[*activeCell]bool)
				for i := 0; i < numCols; i++ {
					if ends[i] && activeCells[i] != nil {
						ac := activeCells[i]
						if !seenPartial[ac] {
							seenPartial[ac] = true
							finalizeCell(ac)
						}
						activeCells[i] = nil
					}
				}

				// For columns that continue on a partial separator, the separator
				// line itself may contain content in the continuing columns.
				// Extract that content.
				for i := 0; i < numCols; i++ {
					if !ends[i] && activeCells[i] != nil {
						ac := activeCells[i]
						// Only extract once per spanning cell.
						if ac.col == i {
							content := extractCellContentSpanning(line, ac.col, ac.colSpan)
							ac.lines = append(ac.lines, content)
						}
					}
				}

				// Start new cells for the columns that just ended, at the current logical row.
				// But first we need a sub-separator detection for colspan among
				// the ending columns. For simplicity, each ending column gets its
				// own cell (colspan=1) starting at the current logicalRow.
				// Detect colspan among ending columns in this partial separator.
				endingGroups := detectEndingColSpanGroups(line, colBounds, ends, numCols)
				for _, group := range endingGroups {
					startCol := group[0]
					colSpan := len(group)
					isHead := headBodyBoundary < 0

					ac := &activeCell{
						startRow: logicalRow,
						col:      startCol,
						colSpan:  colSpan,
						isHead:   isHead,
					}
					for _, c := range group {
						activeCells[c] = ac
					}
				}
			}

		} else if line[0] == '|' {
			// Content line: extract per-column content.
			for i := 0; i < numCols; i++ {
				ac := activeCells[i]
				if ac == nil {
					continue
				}
				// Only extract once per spanning cell (at the owner column).
				if ac.col == i {
					content := extractCellContentSpanning(line, ac.col, ac.colSpan)
					ac.lines = append(ac.lines, content)
				}
			}
		}
	}

	// Finalize any remaining active cells that have content. Cells created by
	// the bottom border (the final full separator) will have no content lines
	// and are intentionally skipped — they represent the structural closing of
	// the table, not actual data cells.
	seenRemaining := make(map[*activeCell]bool)
	for i := 0; i < numCols; i++ {
		if activeCells[i] != nil {
			ac := activeCells[i]
			if len(ac.lines) == 0 || seenRemaining[ac] {
				activeCells[i] = nil
				continue
			}
			seenRemaining[ac] = true
			finalizeCell(ac)
			activeCells[i] = nil
		}
	}

	// Sort cells by (Row, Col) to ensure consistent ordering for AST building.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Row != result[j].Row {
			return result[i].Row < result[j].Row
		}
		return result[i].Col < result[j].Col
	})

	// Fix up IsHead based on headBodyBoundary.
	if headBodyBoundary >= 0 {
		for i := range result {
			result[i].IsHead = result[i].Row < headBodyBoundary
		}
	} else {
		// No head/body separator found — all cells are body cells.
		for i := range result {
			result[i].IsHead = false
		}
	}

	return result, nil
}

// detectEndingColSpanGroups detects colspan groups among ending columns on a
// partial separator line. Adjacent ending columns without a '+' between them
// are merged.
func detectEndingColSpanGroups(line []byte, colBounds []int, ends []bool, numCols int) [][]int {
	var groups [][]int
	var currentGroup []int

	for i := 0; i < numCols; i++ {
		if !ends[i] {
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
				currentGroup = nil
			}
			continue
		}

		if len(currentGroup) == 0 {
			currentGroup = []int{i}
		} else {
			// Check if there's a '+' at the boundary between the previous column
			// in the group and this one.
			pos := colBounds[i]
			if pos < len(line) && line[pos] == '+' {
				groups = append(groups, currentGroup)
				currentGroup = []int{i}
			} else {
				currentGroup = append(currentGroup, i)
			}
		}
	}
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}
	return groups
}

// extractColBounds finds positions of '+' in the top border line.
func extractColBounds(line []byte) []int {
	var bounds []int
	for i, ch := range line {
		if ch == '+' {
			bounds = append(bounds, i)
		}
	}
	return bounds
}

// stripConsistentPadding removes consistent leading whitespace from cell
// content lines. It finds the minimum number of leading spaces across all
// non-empty lines and strips that many from each.
func stripConsistentPadding(lines [][]byte) []byte {
	if len(lines) == 0 {
		return nil
	}

	// Find minimum leading spaces across non-empty lines.
	minSpaces := math.MaxInt
	for _, line := range lines {
		trimmed := bytes.TrimRight(line, " ")
		if len(trimmed) == 0 {
			continue // skip empty lines
		}
		spaces := 0
		for _, ch := range trimmed {
			if ch == ' ' {
				spaces++
			} else {
				break
			}
		}
		if spaces < minSpaces {
			minSpaces = spaces
		}
	}
	if minSpaces == math.MaxInt {
		minSpaces = 0
	}

	// Strip minSpaces from each line, then right-trim trailing spaces.
	var buf bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		if len(line) <= minSpaces {
			// empty or shorter than min — becomes empty line
			continue
		}
		stripped := line[minSpaces:]
		stripped = bytes.TrimRight(stripped, " ")
		buf.Write(stripped)
	}

	// Trim trailing newlines.
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// ---------------------------------------------------------------------------
// BlockParser
// ---------------------------------------------------------------------------

// GridTableParser is a goldmark BlockParser that recognises RST-style grid
// tables and collects their raw lines.
type GridTableParser struct {
	lines [][]byte
}

// Trigger returns the byte that triggers this parser ('+').
func (p *GridTableParser) Trigger() []byte { return []byte{'+'} }

// Open is called when goldmark sees a line starting with '+'.
// The parser framework advances past the line automatically after Open returns.
func (p *GridTableParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	line = bytes.TrimRight(line, "\r\n")

	if !topBorderPattern.Match(line) {
		return nil, parser.NoChildren
	}

	// Validate we have at least two '+' (at least one column).
	bounds := extractColBounds(line)
	if len(bounds) < 2 {
		return nil, parser.NoChildren
	}

	p.lines = nil
	p.lines = append(p.lines, copyBytes(line))
	return NewGridTable(), parser.Continue | parser.NoChildren
}

// Continue is called for each subsequent line after Open.
// The parser framework advances past the line automatically after Continue returns.
func (p *GridTableParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if len(line) == 0 {
		return parser.Close
	}

	trimmed := bytes.TrimRight(line, "\r\n")

	// Accept content lines (start with |) and separator lines (start with +).
	if len(trimmed) > 0 && (trimmed[0] == '|' || trimmed[0] == '+') {
		p.lines = append(p.lines, copyBytes(trimmed))
		return parser.Continue | parser.NoChildren
	}

	return parser.Close
}

// Close is called when the block ends. It parses the collected lines into
// the grid table AST structure.
func (p *GridTableParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	table, ok := node.(*GridTable)
	if !ok {
		return
	}

	cells, err := parseGrid(p.lines)
	if err != nil {
		// Graceful fallback: leave the table empty.
		return
	}

	buildGridTableAST(table, cells)
	p.lines = nil
}

// CanInterruptParagraph returns false.
func (p *GridTableParser) CanInterruptParagraph() bool { return false }

// CanAcceptIndentedCode returns false.
func (p *GridTableParser) CanAcceptIndentedCode() bool { return false }

// buildGridTableAST constructs the child AST nodes for a GridTable from
// parsed cells.
func buildGridTableAST(table *GridTable, cells []parsedCell) {
	if len(cells) == 0 {
		return
	}

	// Determine if we have a head section.
	hasHead := false
	for _, c := range cells {
		if c.IsHead {
			hasHead = true
			break
		}
	}

	// Group cells by row.
	maxRow := 0
	for _, c := range cells {
		if c.Row > maxRow {
			maxRow = c.Row
		}
	}

	// Determine head/body boundary.
	headEnd := -1
	if hasHead {
		for _, c := range cells {
			if c.IsHead && c.Row > headEnd {
				headEnd = c.Row
			}
		}
	}

	// Create sections and rows.
	var headSection, bodySection *GridTableSection
	if hasHead {
		headSection = NewGridTableSection(true)
		table.AppendChild(table, headSection)
	}
	bodySection = NewGridTableSection(false)
	table.AppendChild(table, bodySection)

	// Build rows: for each logical row, create a GridTableRow and add
	// cells that start at that row.
	rowNodes := make(map[int]*GridTableRow)
	for row := 0; row <= maxRow; row++ {
		// Check if any cell starts at this row.
		hasCells := false
		for _, c := range cells {
			if c.Row == row {
				hasCells = true
				break
			}
		}
		if !hasCells {
			continue
		}

		rowNode := NewGridTableRow()
		rowNodes[row] = rowNode

		var section *GridTableSection
		if hasHead && row <= headEnd {
			section = headSection
		} else {
			section = bodySection
		}
		section.AppendChild(section, rowNode)
	}

	// Add cells to their respective rows, sorted by column.
	// cells from parseGrid are already ordered by (row, col) due to how
	// they're finalized, but let's be explicit.
	for _, c := range cells {
		rowNode := rowNodes[c.Row]
		if rowNode == nil {
			continue
		}
		cell := NewGridTableCell(c.ColSpan, c.RowSpan, c.IsHead)
		cell.rawContent = c.Content
		rowNode.AppendChild(rowNode, cell)
	}
}

// copyBytes makes a copy of a byte slice.
func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// ---------------------------------------------------------------------------
// AST Transformer
// ---------------------------------------------------------------------------

// GridTableTransformer is a goldmark ASTTransformer that recursively parses
// cell content in GridTableCell nodes using a helper goldmark instance.
type GridTableTransformer struct {
	helper goldmark.Markdown
}

// Transform walks the AST looking for GridTableCell nodes and parses their
// rawContent into child AST nodes.
func (t *GridTableTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cell, ok := n.(*GridTableCell)
		if !ok || cell.rawContent == nil {
			return ast.WalkContinue, nil
		}

		// Parse cell content with the helper goldmark instance.
		subDoc := t.helper.Parser().Parse(text.NewReader(cell.rawContent))

		// Re-parent children from the parsed sub-document into the cell.
		for child := subDoc.FirstChild(); child != nil; {
			next := child.NextSibling()
			subDoc.RemoveChild(subDoc, child)
			cell.AppendChild(cell, child)
			child = next
		}
		cell.rawContent = nil
		return ast.WalkContinue, nil
	})
}

// Priority returns the transformer priority. Must run before LinkRewriter (999).
func (t *GridTableTransformer) Priority() int { return 50 }

// ---------------------------------------------------------------------------
// Extender
// ---------------------------------------------------------------------------

// GridTableExtension is a goldmark Extender that adds RST-style grid table
// support.
type GridTableExtension struct{}

// Extend registers the grid table parser, transformer, and renderer with
// the given goldmark instance.
func (e *GridTableExtension) Extend(m goldmark.Markdown) {
	// The helper goldmark instance used by the transformer includes GFM
	// extensions but NOT GridTableExtension, to prevent infinite recursion.
	helper := goldmark.New(goldmark.WithExtensions(extension.GFM))

	m.Parser().AddOptions(
		parser.WithBlockParsers(util.Prioritized(&GridTableParser{}, 500)),
		parser.WithASTTransformers(util.Prioritized(&GridTableTransformer{helper: helper}, 50)),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&GridTableRenderer{}, 500)),
	)
}
