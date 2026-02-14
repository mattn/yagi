package main

import (
	"strings"
	"testing"
)

func TestTableBuffer_SimpleTable(t *testing.T) {
	var tb tableBuffer
	var out strings.Builder

	chunks := []string{
		"Here is a table:\n",
		"| Name | Age |\n",
		"| --- | --- |\n",
		"| Alice | 30 |\n",
		"| Bob | 25 |\n",
		"Done.\n",
	}

	for _, c := range chunks {
		v := tb.processChunk(c)
		out.WriteString(v)
	}
	out.WriteString(tb.flush())

	result := out.String()

	// Should contain "Here is a table:" verbatim
	if !strings.Contains(result, "Here is a table:") {
		t.Error("expected verbatim text before table")
	}

	// Should contain "Done." verbatim
	if !strings.Contains(result, "Done.") {
		t.Error("expected verbatim text after table")
	}

	// Should NOT contain the raw markdown separator
	if strings.Contains(result, "| --- |") {
		t.Error("expected markdown table to be replaced by rich table")
	}

	// Should contain tablewriter output (box-drawing characters or "+" borders)
	if !strings.Contains(result, "Alice") || !strings.Contains(result, "Bob") {
		t.Error("expected table data in output")
	}

	// Should contain header "NAME" (tablewriter auto-capitalizes)
	if !strings.Contains(result, "NAME") && !strings.Contains(result, "Name") {
		t.Error("expected header in rich table output")
	}

	t.Log("Output:\n" + result)
}

func TestTableBuffer_NoTable(t *testing.T) {
	var tb tableBuffer
	var out strings.Builder

	chunks := []string{
		"Hello world\n",
		"No tables here.\n",
	}

	for _, c := range chunks {
		v := tb.processChunk(c)
		out.WriteString(v)
	}
	out.WriteString(tb.flush())

	result := out.String()
	if result != "Hello world\nNo tables here.\n" {
		t.Errorf("unexpected output: %q", result)
	}
}

func TestTableBuffer_StreamedInSmallChunks(t *testing.T) {
	var tb tableBuffer
	var out strings.Builder

	// Simulate small character-by-character streaming
	full := "| A | B |\n| - | - |\n| 1 | 2 |\n"
	for _, ch := range full {
		v := tb.processChunk(string(ch))
		out.WriteString(v)
	}
	out.WriteString(tb.flush())

	result := out.String()
	if strings.Contains(result, "| - |") {
		t.Error("expected markdown table to be replaced")
	}
	if !strings.Contains(result, "1") || !strings.Contains(result, "2") {
		t.Error("expected table data")
	}
	t.Log("Output:\n" + result)
}

func TestTableBuffer_InvalidTable(t *testing.T) {
	var tb tableBuffer
	var out strings.Builder

	chunks := []string{
		"| col1 | col2 |\n",
		"| not-separator |\n", // invalid — not enough columns and not a separator
		"text\n",
	}

	for _, c := range chunks {
		v := tb.processChunk(c)
		out.WriteString(v)
	}
	out.WriteString(tb.flush())

	result := out.String()
	// Should fall back to plain text
	if !strings.Contains(result, "| col1 | col2 |") {
		t.Error("expected raw lines for invalid table")
	}
}

func TestIsTableRow(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"| a | b |", true},
		{"| --- | --- |", true},
		{"  | x | y |  ", true},
		{"no pipes here", false},
		{"|single|", false},  // only 2 pipes
		{"| a | b | c |", true},
	}
	for _, tt := range tests {
		got := isTableRow(tt.line)
		if got != tt.want {
			t.Errorf("isTableRow(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestIsSeparatorRow(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"| --- | --- |", true},
		{"| :---: | ---: |", true},
		{"| a | b |", false},
		{"not a row", false},
	}
	for _, tt := range tests {
		got := isSeparatorRow(tt.line)
		if got != tt.want {
			t.Errorf("isSeparatorRow(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
