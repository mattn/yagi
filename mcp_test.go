package main

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMarshalSchema_Nil(t *testing.T) {
	got := marshalSchema(nil)
	want := `{"type":"object"}`
	if string(got) != want {
		t.Errorf("marshalSchema(nil) = %s, want %s", got, want)
	}
}

func TestMarshalSchema_Map(t *testing.T) {
	got := marshalSchema(map[string]any{"type": "string"})
	if !json.Valid(got) {
		t.Fatalf("marshalSchema returned invalid JSON: %s", got)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["type"] != "string" {
		t.Errorf("expected type=string, got %v", m["type"])
	}
}

func TestMarshalSchema_ValidStruct(t *testing.T) {
	type schema struct {
		Type       string   `json:"type"`
		Properties []string `json:"properties"`
	}
	input := schema{Type: "object", Properties: []string{"name", "age"}}
	got := marshalSchema(input)
	if !json.Valid(got) {
		t.Fatalf("marshalSchema returned invalid JSON: %s", got)
	}
	var s schema
	if err := json.Unmarshal(got, &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if s.Type != "object" {
		t.Errorf("expected type=object, got %s", s.Type)
	}
	if len(s.Properties) != 2 || s.Properties[0] != "name" || s.Properties[1] != "age" {
		t.Errorf("unexpected properties: %v", s.Properties)
	}
}

func TestContentToString_Empty(t *testing.T) {
	got := contentToString([]mcp.Content{})
	if got != "" {
		t.Errorf("contentToString(empty) = %q, want %q", got, "")
	}
}

func TestContentToString_SingleText(t *testing.T) {
	content := []mcp.Content{
		&mcp.TextContent{Text: "hello"},
	}
	got := contentToString(content)
	if got != "hello" {
		t.Errorf("contentToString = %q, want %q", got, "hello")
	}
}

func TestContentToString_MultipleText(t *testing.T) {
	content := []mcp.Content{
		&mcp.TextContent{Text: "hello"},
		&mcp.TextContent{Text: " world"},
	}
	got := contentToString(content)
	want := "hello world"
	if got != want {
		t.Errorf("contentToString = %q, want %q", got, want)
	}
}
