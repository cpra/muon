package tui

import (
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

type markdownDocument struct {
	blocks []markdownBlock
}

type markdownBlockKind int

const (
	markdownParagraph markdownBlockKind = iota
	markdownHeading
	markdownList
	markdownCodeBlock
	markdownTableBlock
)

type markdownBlock struct {
	kind  markdownBlockKind
	level int
	spans []markdownSpan
	items [][]markdownSpan
	lines []string
	table markdownTableData
}

type markdownTableData struct {
	rows []markdownTableRow
}

type markdownTableRow struct {
	header bool
	cells  [][]markdownSpan
}

type markdownSpan struct {
	text string
	bold bool
	code bool
}

type inlineFlags struct {
	bold bool
	code bool
}

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.Table))

func parseMarkdownDocument(content string) markdownDocument {
	if content == "" {
		return markdownDocument{}
	}

	source := []byte(content)
	reader := gmtext.NewReader(source)
	root := markdownParser.Parser().Parse(reader)
	return markdownDocument{blocks: parseMarkdownBlocks(root, source)}
}

func parseMarkdownBlocks(node ast.Node, source []byte) []markdownBlock {
	var blocks []markdownBlock
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			blocks = append(blocks, markdownBlock{
				kind:  markdownHeading,
				level: n.Level,
				spans: parseInlineSpans(n, source, inlineFlags{}),
			})
		case *ast.Paragraph:
			blocks = append(blocks, markdownBlock{
				kind:  markdownParagraph,
				spans: parseInlineSpans(n, source, inlineFlags{}),
			})
		case *ast.List:
			blocks = append(blocks, parseListBlock(n, source))
		case *ast.FencedCodeBlock:
			blocks = append(blocks, markdownBlock{
				kind:  markdownCodeBlock,
				lines: splitMarkdownCodeLines(string(n.Text(source))),
			})
		case *ast.CodeBlock:
			blocks = append(blocks, markdownBlock{
				kind:  markdownCodeBlock,
				lines: splitMarkdownCodeLines(string(n.Text(source))),
			})
		case *extast.Table:
			blocks = append(blocks, parseTableBlock(n, source))
		default:
			blocks = append(blocks, parseMarkdownBlocks(child, source)...)
		}
	}
	return blocks
}

func parseTableBlock(table *extast.Table, source []byte) markdownBlock {
	rows := make([]markdownTableRow, 0)
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch row := child.(type) {
		case *extast.TableHeader:
			rows = append(rows, markdownTableRow{header: true, cells: parseTableCells(row, source)})
		case *extast.TableRow:
			rows = append(rows, markdownTableRow{cells: parseTableCells(row, source)})
		}
	}
	return markdownBlock{kind: markdownTableBlock, table: markdownTableData{rows: rows}}
}

func parseTableCells(row ast.Node, source []byte) [][]markdownSpan {
	cells := make([][]markdownSpan, 0)
	for child := row.FirstChild(); child != nil; child = child.NextSibling() {
		cell, ok := child.(*extast.TableCell)
		if !ok {
			continue
		}
		cells = append(cells, parseInlineSpans(cell, source, inlineFlags{}))
	}
	return cells
}

func parseListBlock(list *ast.List, source []byte) markdownBlock {
	items := make([][]markdownSpan, 0)
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}
		spans := parseListItemSpans(item, source)
		items = append(items, spans)
	}
	return markdownBlock{kind: markdownList, items: items}
}

func parseListItemSpans(item *ast.ListItem, source []byte) []markdownSpan {
	var spans []markdownSpan
	firstBlock := true
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Paragraph:
			if !firstBlock {
				appendMarkdownText(&spans, " ", inlineFlags{})
			}
			spans = append(spans, parseInlineSpans(n, source, inlineFlags{})...)
			firstBlock = false
		case *ast.TextBlock:
			if !firstBlock {
				appendMarkdownText(&spans, " ", inlineFlags{})
			}
			spans = append(spans, parseInlineSpans(n, source, inlineFlags{})...)
			firstBlock = false
		case *ast.Heading:
			if !firstBlock {
				appendMarkdownText(&spans, " ", inlineFlags{})
			}
			spans = append(spans, parseInlineSpans(n, source, inlineFlags{bold: true})...)
			firstBlock = false
		case *ast.FencedCodeBlock:
			text := strings.Join(splitMarkdownCodeLines(string(n.Text(source))), " ")
			if text != "" {
				if !firstBlock {
					appendMarkdownText(&spans, " ", inlineFlags{})
				}
				appendMarkdownText(&spans, text, inlineFlags{code: true})
				firstBlock = false
			}
		}
	}
	return spans
}

func parseInlineSpans(node ast.Node, source []byte, flags inlineFlags) []markdownSpan {
	var spans []markdownSpan
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			appendMarkdownText(&spans, string(n.Text(source)), flags)
			if n.SoftLineBreak() || n.HardLineBreak() {
				appendMarkdownText(&spans, "\n", flags)
			}
		case *ast.String:
			appendMarkdownText(&spans, string(n.Value), flags)
		case *ast.Emphasis:
			nextFlags := flags
			if n.Level >= 2 {
				nextFlags.bold = true
			}
			spans = append(spans, parseInlineSpans(n, source, nextFlags)...)
		case *ast.CodeSpan:
			nextFlags := flags
			nextFlags.code = true
			spans = append(spans, parseInlineSpans(n, source, nextFlags)...)
		case *ast.Link:
			spans = append(spans, parseInlineSpans(n, source, flags)...)
		case *ast.AutoLink:
			appendMarkdownText(&spans, string(n.Text(source)), flags)
		default:
			spans = append(spans, parseInlineSpans(n, source, flags)...)
		}
	}
	return spans
}

func appendMarkdownText(spans *[]markdownSpan, text string, flags inlineFlags) {
	if text == "" {
		return
	}
	items := *spans
	if len(items) > 0 {
		last := &items[len(items)-1]
		if last.bold == flags.bold && last.code == flags.code {
			last.text += text
			*spans = items
			return
		}
	}
	*spans = append(items, markdownSpan{text: text, bold: flags.bold, code: flags.code})
}

func splitMarkdownCodeLines(text string) []string {
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return []string{""}
	}
	return strings.Split(trimmed, "\n")
}

func renderMarkdownDocument(doc markdownDocument, width int) []styledLine {
	if len(doc.blocks) == 0 {
		return wrapStyledSpans("", "", []styledSpan{{text: "", style: assistantStyle}}, width)
	}

	lines := make([]styledLine, 0, len(doc.blocks)*2)
	for i, block := range doc.blocks {
		if i > 0 {
			lines = append(lines, styledLine{})
		}

		switch block.kind {
		case markdownHeading:
			lines = append(lines, wrapStyledSpans("", "", styleMarkdownSpans(block.spans, markdownHeadingStyle), width)...)
		case markdownParagraph:
			lines = append(lines, wrapStyledSpans("", "", styleMarkdownSpans(block.spans, assistantStyle), width)...)
		case markdownList:
			for itemIndex, item := range block.items {
				_ = itemIndex
				lines = append(lines, wrapStyledSpans("• ", "  ", styleMarkdownSpans(item, assistantStyle), width)...)
			}
		case markdownCodeBlock:
			for _, line := range block.lines {
				codeLine := []styledSpan{{text: line, style: markdownCodeBlockStyle}}
				wrapped := wrapStyledSpans("    ", "    ", codeLine, width, styledSpan{text: "", style: markdownCodeBlockStyle})
				for i := range wrapped {
					wrapped[i].fill = true
					wrapped[i].fillStyle = markdownCodeBlockStyle
				}
				lines = append(lines, wrapped...)
			}
		case markdownTableBlock:
			lines = append(lines, renderMarkdownTable(block.table, width)...)
		}
	}

	return lines
}

func renderMarkdownTable(table markdownTableData, width int) []styledLine {
	if len(table.rows) == 0 {
		return nil
	}

	colCount := 0
	for _, row := range table.rows {
		if len(row.cells) > colCount {
			colCount = len(row.cells)
		}
	}
	if colCount == 0 {
		return nil
	}

	colWidths := tableColumnWidths(table, width, colCount)
	lines := make([]styledLine, 0, len(table.rows)*2+2)
	border := renderTableBorderLine(colWidths)
	lines = append(lines, border)

	for rowIndex, row := range table.rows {
		lines = append(lines, renderMarkdownTableRow(row, colWidths)...)
		if row.header || rowIndex == len(table.rows)-1 {
			lines = append(lines, border)
		}
	}

	return lines
}

func tableColumnWidths(table markdownTableData, width, colCount int) []int {
	widths := make([]int, colCount)
	for i := range widths {
		widths[i] = 3
	}

	for _, row := range table.rows {
		for col := 0; col < colCount; col++ {
			if col >= len(row.cells) {
				continue
			}
			cellWidth := markdownSpansMaxLineWidth(row.cells[col])
			if cellWidth > widths[col] {
				widths[col] = cellWidth
			}
		}
	}

	available := width - (1 + 3*colCount)
	if available < colCount {
		available = colCount
	}
	for sumInts(widths) > available {
		idx := maxWidthIndex(widths)
		if widths[idx] <= 1 {
			break
		}
		widths[idx]--
	}
	return widths
}

func renderTableBorderLine(colWidths []int) styledLine {
	line := styledLine{}
	appendStyledText(&line, "+", markdownTableBorderStyle)
	for _, width := range colWidths {
		appendStyledText(&line, strings.Repeat("-", width+2), markdownTableBorderStyle)
		appendStyledText(&line, "+", markdownTableBorderStyle)
	}
	return line
}

func renderMarkdownTableRow(row markdownTableRow, colWidths []int) []styledLine {
	cellLines := make([][]styledLine, len(colWidths))
	rowHeight := 1
	for col := range colWidths {
		var spans []markdownSpan
		if col < len(row.cells) {
			spans = row.cells[col]
		}
		baseStyle := assistantStyle
		if row.header {
			baseStyle = markdownTableHeaderStyle
		}
		cellLines[col] = wrapStyledSpans("", "", styleMarkdownSpans(spans, baseStyle), colWidths[col])
		if len(cellLines[col]) > rowHeight {
			rowHeight = len(cellLines[col])
		}
	}

	lines := make([]styledLine, 0, rowHeight)
	for rowLine := 0; rowLine < rowHeight; rowLine++ {
		line := styledLine{}
		appendStyledText(&line, "|", markdownTableBorderStyle)
		for col, colWidth := range colWidths {
			baseStyle := assistantStyle
			if row.header {
				baseStyle = markdownTableHeaderStyle
			}
			appendStyledText(&line, " ", baseStyle)
			if rowLine < len(cellLines[col]) {
				for _, span := range cellLines[col][rowLine].spans {
					appendStyledText(&line, span.text, span.style)
				}
				pad := colWidth - styledLineWidth(cellLines[col][rowLine])
				if pad > 0 {
					appendStyledText(&line, strings.Repeat(" ", pad), baseStyle)
				}
			} else {
				appendStyledText(&line, strings.Repeat(" ", colWidth), baseStyle)
			}
			appendStyledText(&line, " ", baseStyle)
			appendStyledText(&line, "|", markdownTableBorderStyle)
		}
		lines = append(lines, line)
	}
	return lines
}

func markdownSpansMaxLineWidth(spans []markdownSpan) int {
	maxWidth := 0
	current := 0
	for _, span := range spans {
		for _, r := range span.text {
			if r == '\n' {
				if current > maxWidth {
					maxWidth = current
				}
				current = 0
				continue
			}
			current += runeWidth(r)
		}
	}
	if current > maxWidth {
		maxWidth = current
	}
	if maxWidth == 0 {
		return 1
	}
	return maxWidth
}

func styledLineWidth(line styledLine) int {
	width := 0
	for _, span := range line.spans {
		width += runewidthString(span.text)
	}
	return width
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func maxWidthIndex(values []int) int {
	idx := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[idx] {
			idx = i
		}
	}
	return idx
}

func styleMarkdownSpans(spans []markdownSpan, baseStyle tcell.Style) []styledSpan {
	styled := make([]styledSpan, 0, len(spans))
	for _, span := range spans {
		style := baseStyle
		if span.bold {
			style = style.Bold(true)
		}
		if span.code {
			style = markdownInlineCodeStyle
			if span.bold {
				style = style.Bold(true)
			}
		}
		styled = append(styled, styledSpan{text: span.text, style: style})
	}
	return styled
}

func wrapStyledSpans(firstPrefix, restPrefix string, spans []styledSpan, width int, prefixStyles ...styledSpan) []styledLine {
	if width < 1 {
		width = 1
	}

	prefixStyle := assistantStyle
	if len(prefixStyles) > 0 {
		prefixStyle = prefixStyles[0].style
	}

	lines := []styledLine{{}}
	lineWidths := []int{0}
	prefixWidths := []int{0}
	current := 0

	startLine := func(prefix string) {
		lines = append(lines, styledLine{})
		lineWidths = append(lineWidths, 0)
		prefixWidths = append(prefixWidths, 0)
		current++
		if prefix != "" {
			appendStyledText(&lines[current], prefix, prefixStyle)
			width := runewidthString(prefix)
			lineWidths[current] = width
			prefixWidths[current] = width
		}
	}

	if firstPrefix != "" {
		appendStyledText(&lines[current], firstPrefix, prefixStyle)
		width := runewidthString(firstPrefix)
		lineWidths[current] = width
		prefixWidths[current] = width
	}

	if len(spans) == 0 {
		return lines
	}

	for _, span := range spans {
		for _, r := range span.text {
			if r == '\n' {
				startLine(restPrefix)
				continue
			}

			rw := runeWidth(r)
			if lineWidths[current]+rw > width && lineWidths[current] > prefixWidths[current] {
				startLine(restPrefix)
			}

			appendStyledText(&lines[current], string(r), span.style)
			lineWidths[current] += rw
		}
	}

	return lines
}

func appendStyledText(line *styledLine, text string, style tcell.Style) {
	if text == "" {
		return
	}
	if len(line.spans) > 0 {
		last := &line.spans[len(line.spans)-1]
		if stylesEqual(last.style, style) {
			last.text += text
			return
		}
	}
	line.spans = append(line.spans, styledSpan{text: text, style: style})
}

func stylesEqual(a, b tcell.Style) bool {
	return a.GetForeground() == b.GetForeground() &&
		a.GetBackground() == b.GetBackground() &&
		a.GetAttributes() == b.GetAttributes() &&
		a.GetUnderlineColor() == b.GetUnderlineColor() &&
		a.GetUnderlineStyle() == b.GetUnderlineStyle()
}
