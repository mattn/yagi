package main

import (
	"strings"

	"github.com/olekukonko/tablewriter"
)

// tableBuffer accumulates streamed Markdown table lines and renders them
// as rich tables once the table block ends. Table lines are NOT printed
// to stdout during streaming — only the final rich table is emitted.
type tableBuffer struct {
	lines   []string // accumulated raw table lines
	partial string   // incomplete line being built from streaming chunks
}

func (tb *tableBuffer) active() bool {
	return len(tb.lines) > 0
}

// processChunk handles a streaming text chunk. It returns the portion of the
// chunk that should be printed verbatim (non-table text). Table lines are
// buffered internally and not included in the returned string.
func (tb *tableBuffer) processChunk(text string) string {
	var verbatim strings.Builder

	tb.partial += text

	for {
		nl := strings.IndexByte(tb.partial, '\n')
		if nl < 0 {
			// No complete line yet.
			if !tb.active() && tb.partial != "" && !strings.HasPrefix(strings.TrimSpace(tb.partial), "|") {
				// Not in a table context and doesn't look like the start of one.
				verbatim.WriteString(tb.partial)
				tb.partial = ""
			}
			break
		}

		line := tb.partial[:nl]
		tb.partial = tb.partial[nl+1:]

		if isTableRow(line) {
			tb.lines = append(tb.lines, line)
		} else {
			// Not a table row — flush any buffered table, then output this line.
			verbatim.WriteString(tb.flushTable())
			verbatim.WriteString(line + "\n")
		}
	}

	return verbatim.String()
}

// flush should be called when the stream ends to render any remaining table.
func (tb *tableBuffer) flush() string {
	if tb.partial != "" {
		if isTableRow(tb.partial) {
			tb.lines = append(tb.lines, tb.partial)
		} else {
			out := tb.flushTable()
			return out + tb.partial
		}
		tb.partial = ""
	}
	return tb.flushTable()
}

func (tb *tableBuffer) flushTable() string {
	if len(tb.lines) == 0 {
		return ""
	}

	if len(tb.lines) < 3 || !isSeparatorRow(tb.lines[1]) {
		// Not a valid Markdown table — return lines as plain text.
		var out strings.Builder
		for _, l := range tb.lines {
			out.WriteString(l + "\n")
		}
		tb.lines = nil
		return out.String()
	}

	// Render rich table.
	headers := parseTableRow(tb.lines[0])
	var data [][]string
	for _, line := range tb.lines[2:] {
		data = append(data, parseTableRow(line))
	}

	var buf strings.Builder
	table := tablewriter.NewWriter(&buf)
	table.Header(headers)
	for _, row := range data {
		table.Append(row)
	}
	table.Render()

	tb.lines = nil
	return buf.String()
}

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result
}

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 3
}

func isSeparatorRow(line string) bool {
	if !isTableRow(line) {
		return false
	}
	cells := parseTableRow(line)
	for _, cell := range cells {
		cleaned := strings.TrimLeft(cell, ":")
		cleaned = strings.TrimRight(cleaned, ":")
		cleaned = strings.ReplaceAll(cleaned, "-", "")
		if cleaned != "" {
			return false
		}
	}
	return true
}
