package tui

import (
	"strings"
	"testing"
)

func TestParseMarkdownDocumentSubset(t *testing.T) {
	doc := parseMarkdownDocument("# Title\n\n- item **bold** `code`\n\n```go\nfmt.Println(\"hi\")\n```")

	if len(doc.blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(doc.blocks))
	}

	if doc.blocks[0].kind != markdownHeading {
		t.Fatalf("expected first block to be heading, got %v", doc.blocks[0].kind)
	}
	if got := markdownSpanText(doc.blocks[0].spans); got != "Title" {
		t.Fatalf("expected heading text %q, got %q", "Title", got)
	}

	if doc.blocks[1].kind != markdownList {
		t.Fatalf("expected second block to be list, got %v", doc.blocks[1].kind)
	}
	if len(doc.blocks[1].items) != 1 {
		t.Fatalf("expected one list item, got %d", len(doc.blocks[1].items))
	}
	item := doc.blocks[1].items[0]
	if got := markdownSpanText(item); got != "item bold code" {
		t.Fatalf("expected list item text %q, got %q", "item bold code", got)
	}
	if !hasMarkdownSpan(item, func(span markdownSpan) bool { return span.bold && span.text == "bold" }) {
		t.Fatalf("expected bold span in list item")
	}
	if !hasMarkdownSpan(item, func(span markdownSpan) bool { return span.code && span.text == "code" }) {
		t.Fatalf("expected code span in list item")
	}

	if doc.blocks[2].kind != markdownCodeBlock {
		t.Fatalf("expected third block to be code block, got %v", doc.blocks[2].kind)
	}
	if len(doc.blocks[2].lines) != 1 || doc.blocks[2].lines[0] != "fmt.Println(\"hi\")" {
		t.Fatalf("unexpected code block lines: %#v", doc.blocks[2].lines)
	}
}

func TestParseMarkdownDocumentTable(t *testing.T) {
	doc := parseMarkdownDocument("| Benefit | Description |\n|---------|-------------|\n| Better DX | Improved autocomplete |")

	if len(doc.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(doc.blocks))
	}
	if doc.blocks[0].kind != markdownTableBlock {
		t.Fatalf("expected table block, got %v", doc.blocks[0].kind)
	}
	if len(doc.blocks[0].table.rows) != 2 {
		t.Fatalf("expected 2 table rows, got %d", len(doc.blocks[0].table.rows))
	}
	if !doc.blocks[0].table.rows[0].header {
		t.Fatalf("expected first row to be table header")
	}
	if got := markdownSpanText(doc.blocks[0].table.rows[1].cells[1]); got != "Improved autocomplete" {
		t.Fatalf("expected second row description cell, got %q", got)
	}
}

func TestWrapStyledSpansPreservesStylesAcrossWrap(t *testing.T) {
	boldStyle := assistantStyle.Bold(true)
	lines := wrapStyledSpans("", "", []styledSpan{
		{text: "abcd", style: assistantStyle},
		{text: "12", style: markdownInlineCodeStyle},
		{text: "XY", style: boldStyle},
	}, 5)

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if got := styledLineText(lines[0]); got != "abcd1" {
		t.Fatalf("expected first line text %q, got %q", "abcd1", got)
	}
	if got := styledLineText(lines[1]); got != "2XY" {
		t.Fatalf("expected second line text %q, got %q", "2XY", got)
	}

	if len(lines[0].spans) != 2 || !stylesEqual(lines[0].spans[1].style, markdownInlineCodeStyle) {
		t.Fatalf("expected wrapped code span to keep code style on first line: %#v", lines[0].spans)
	}
	if len(lines[1].spans) != 2 || !stylesEqual(lines[1].spans[0].style, markdownInlineCodeStyle) {
		t.Fatalf("expected wrapped code span to keep code style on second line: %#v", lines[1].spans)
	}
	if !stylesEqual(lines[1].spans[1].style, boldStyle) {
		t.Fatalf("expected bold style on trailing span: %#v", lines[1].spans[1])
	}
}

func TestRenderMarkdownDocumentWrapsBulletContinuation(t *testing.T) {
	doc := parseMarkdownDocument("- alpha beta gamma\n- delta")
	lines := renderMarkdownDocument(doc, 8)

	if len(lines) != 4 {
		t.Fatalf("expected 4 wrapped lines, got %d", len(lines))
	}
	if got := styledLineText(lines[0]); got != "• alpha " {
		t.Fatalf("expected first bullet line %q, got %q", "• alpha ", got)
	}
	if got := styledLineText(lines[1]); got != "  beta g" {
		t.Fatalf("expected continuation line %q, got %q", "  beta g", got)
	}
	if got := styledLineText(lines[2]); got != "  amma" {
		t.Fatalf("expected final continuation line %q, got %q", "  amma", got)
	}
	if got := styledLineText(lines[3]); got != "• delta" {
		t.Fatalf("expected second bullet line %q, got %q", "• delta", got)
	}
}

func TestRenderMarkdownDocumentCodeBlockFillsWholeLine(t *testing.T) {
	doc := parseMarkdownDocument("```\nalpha\nbeta\n```")
	lines := renderMarkdownDocument(doc, 20)

	if len(lines) != 2 {
		t.Fatalf("expected 2 code lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !line.fill {
			t.Fatalf("expected code line %d to fill background", i)
		}
		if !stylesEqual(line.fillStyle, markdownCodeBlockStyle) {
			t.Fatalf("expected code line %d to use code block fill style", i)
		}
	}
}

func TestRenderMarkdownDocumentTableGrid(t *testing.T) {
	doc := parseMarkdownDocument("| Benefit | Description |\n|---------|-------------|\n| Better DX | Improved autocomplete and docs |")
	lines := renderMarkdownDocument(doc, 30)

	if len(lines) < 4 {
		t.Fatalf("expected rendered table lines, got %d", len(lines))
	}
	if got := styledLineText(lines[0]); !strings.HasPrefix(got, "+") {
		t.Fatalf("expected top border line, got %q", got)
	}
	if got := styledLineText(lines[1]); !strings.Contains(got, "Benefit") || !strings.Contains(got, "Description") {
		t.Fatalf("expected header row content, got %q", got)
	}
	if got := styledLineText(lines[2]); !strings.HasPrefix(got, "+") {
		t.Fatalf("expected header separator line, got %q", got)
	}
	joined := joinStyledLines(lines)
	if !strings.Contains(joined, "Improved") {
		t.Fatalf("expected table body content in rendered output: %q", joined)
	}
	if !strings.Contains(joined, "autoc") || !strings.Contains(joined, "omplete") {
		t.Fatalf("expected wrapped table content in rendered output: %q", joined)
	}
}

func TestHandleAppEventCachesAssistantMarkdown(t *testing.T) {
	app := &App{width: 40, height: 20}
	app.handleAppEvent(appEvent{kind: appEventDone, content: "# Cached"})

	if len(app.entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(app.entries))
	}
	if app.entries[0].kind != entryAssistant {
		t.Fatalf("expected assistant entry, got %v", app.entries[0].kind)
	}
	if len(app.entries[0].markdown.blocks) != 1 {
		t.Fatalf("expected cached markdown blocks, got %d", len(app.entries[0].markdown.blocks))
	}
	if app.entries[0].markdown.blocks[0].kind != markdownHeading {
		t.Fatalf("expected cached heading block, got %v", app.entries[0].markdown.blocks[0].kind)
	}
	if got := markdownSpanText(app.entries[0].markdown.blocks[0].spans); got != "Cached" {
		t.Fatalf("expected cached heading text %q, got %q", "Cached", got)
	}
}

func markdownSpanText(spans []markdownSpan) string {
	text := ""
	for _, span := range spans {
		text += span.text
	}
	return text
}

func hasMarkdownSpan(spans []markdownSpan, match func(markdownSpan) bool) bool {
	for _, span := range spans {
		if match(span) {
			return true
		}
	}
	return false
}

func styledLineText(line styledLine) string {
	text := ""
	for _, span := range line.spans {
		text += span.text
	}
	return text
}

func joinStyledLines(lines []styledLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, styledLineText(line))
	}
	return strings.Join(parts, "\n")
}
