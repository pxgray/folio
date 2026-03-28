package render

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// GridTableRenderer is a goldmark NodeRenderer for RST-style grid table nodes.
type GridTableRenderer struct {
	html.Config
}

// RegisterFuncs registers render functions for all four grid table node kinds.
func (r *GridTableRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindGridTable, r.renderGridTable)
	reg.Register(KindGridTableSection, r.renderGridTableSection)
	reg.Register(KindGridTableRow, r.renderGridTableRow)
	reg.Register(KindGridTableCell, r.renderGridTableCell)
}

func (r *GridTableRenderer) renderGridTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<table>\n")
	} else {
		_, _ = w.WriteString("</table>\n")
	}
	return ast.WalkContinue, nil
}

func (r *GridTableRenderer) renderGridTableSection(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	section, ok := node.(*GridTableSection)
	if !ok {
		return ast.WalkContinue, nil
	}
	if entering {
		if section.IsHead {
			_, _ = w.WriteString("<thead>\n")
		} else {
			_, _ = w.WriteString("<tbody>\n")
		}
	} else {
		if section.IsHead {
			_, _ = w.WriteString("</thead>\n")
		} else {
			_, _ = w.WriteString("</tbody>\n")
		}
	}
	return ast.WalkContinue, nil
}

func (r *GridTableRenderer) renderGridTableRow(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<tr>\n")
	} else {
		_, _ = w.WriteString("</tr>\n")
	}
	return ast.WalkContinue, nil
}

func (r *GridTableRenderer) renderGridTableCell(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	cell, ok := node.(*GridTableCell)
	if !ok {
		return ast.WalkContinue, nil
	}

	if entering {
		tag := "td"
		if cell.IsHead {
			tag = "th"
		}
		_, _ = w.WriteString("<" + tag)
		if cell.ColSpan > 1 {
			_, _ = w.WriteString(fmt.Sprintf(" colspan=\"%d\"", cell.ColSpan))
		}
		if cell.RowSpan > 1 {
			_, _ = w.WriteString(fmt.Sprintf(" rowspan=\"%d\"", cell.RowSpan))
		}
		_, _ = w.WriteString(">\n")
	} else {
		tag := "td"
		if cell.IsHead {
			tag = "th"
		}
		_, _ = w.WriteString("</" + tag + ">\n")
	}
	return ast.WalkContinue, nil
}
