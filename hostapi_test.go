package main

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseHTML(t *testing.T, s string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}
	return doc
}

func runExtractText(t *testing.T, input string) string {
	t.Helper()
	doc := parseHTML(t, input)
	var sb strings.Builder
	extractText(doc, &sb)
	return sb.String()
}

func TestExtractText_Simple(t *testing.T) {
	got := runExtractText(t, "<p>Hello World</p>")
	if !strings.Contains(got, "Hello World") {
		t.Errorf("expected output to contain %q, got %q", "Hello World", got)
	}
}

func TestExtractText_SkipsScript(t *testing.T) {
	got := runExtractText(t, "<div><script>var x=1;</script>Hello</div>")
	if strings.Contains(got, "var x=1") {
		t.Errorf("expected output to not contain script content, got %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("expected output to contain %q, got %q", "Hello", got)
	}
}

func TestExtractText_SkipsStyle(t *testing.T) {
	got := runExtractText(t, "<div><style>.foo{}</style>Hello</div>")
	if strings.Contains(got, ".foo") {
		t.Errorf("expected output to not contain style content, got %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("expected output to contain %q, got %q", "Hello", got)
	}
}

func TestExtractText_LinkWithHref(t *testing.T) {
	got := runExtractText(t, `<a href="https://example.com">Click</a>`)
	if !strings.Contains(got, "Click") {
		t.Errorf("expected output to contain %q, got %q", "Click", got)
	}
	if !strings.Contains(got, "(https://example.com)") {
		t.Errorf("expected output to contain %q, got %q", "(https://example.com)", got)
	}
}

func TestExtractText_NestedElements(t *testing.T) {
	got := runExtractText(t, "<div><p>First</p><p>Second</p></div>")
	if !strings.Contains(got, "First") {
		t.Errorf("expected output to contain %q, got %q", "First", got)
	}
	if !strings.Contains(got, "Second") {
		t.Errorf("expected output to contain %q, got %q", "Second", got)
	}
}

func TestExtractText_BrTag(t *testing.T) {
	got := runExtractText(t, "<p>Line1<br>Line2</p>")
	if !strings.Contains(got, "Line1") {
		t.Errorf("expected output to contain %q, got %q", "Line1", got)
	}
	if !strings.Contains(got, "Line2") {
		t.Errorf("expected output to contain %q, got %q", "Line2", got)
	}
	idx1 := strings.Index(got, "Line1")
	idx2 := strings.Index(got, "Line2")
	between := got[idx1+len("Line1") : idx2]
	if !strings.Contains(between, "\n") {
		t.Errorf("expected newline between Line1 and Line2, got %q", between)
	}
}
