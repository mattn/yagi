package main

import (
	"encoding/json"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func resetGlobals() {
	tools = nil
	toolFuncs = map[string]func(string) string{}
	skipApproval = true
	pluginApprovals = nil
}

func TestRegisterTool(t *testing.T) {
	resetGlobals()

	params := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	registerTool("test_tool", "A test tool", params, func(args string) string {
		return args
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Type != openai.ToolTypeFunction {
		t.Errorf("expected tool type %v, got %v", openai.ToolTypeFunction, tools[0].Type)
	}
	if tools[0].Function.Name != "test_tool" {
		t.Errorf("expected tool name %q, got %q", "test_tool", tools[0].Function.Name)
	}
	if tools[0].Function.Description != "A test tool" {
		t.Errorf("expected description %q, got %q", "A test tool", tools[0].Function.Description)
	}
	if _, ok := toolFuncs["test_tool"]; !ok {
		t.Error("expected test_tool in toolFuncs map")
	}
}

func TestExecuteTool_Known(t *testing.T) {
	resetGlobals()

	registerTool("echo", "echoes input", json.RawMessage(`{}`), func(args string) string {
		return "result:" + args
	})

	got := executeTool("echo", "hello")
	if got != "result:hello" {
		t.Errorf("expected %q, got %q", "result:hello", got)
	}
}

func TestExecuteTool_Unknown(t *testing.T) {
	resetGlobals()

	got := executeTool("nonexistent", "")
	expected := "Unknown tool: nonexistent"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestRegisterTool_Multiple(t *testing.T) {
	resetGlobals()

	for i, name := range []string{"tool_a", "tool_b", "tool_c"} {
		n := name
		registerTool(n, "desc "+n, json.RawMessage(`{}`), func(args string) string {
			return n
		})
		if len(tools) != i+1 {
			t.Fatalf("after registering %s: expected %d tools, got %d", n, i+1, len(tools))
		}
	}

	for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
		if _, ok := toolFuncs[name]; !ok {
			t.Errorf("expected %s in toolFuncs map", name)
		}
	}

	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	for i, expected := range []string{"tool_a", "tool_b", "tool_c"} {
		if tools[i].Function.Name != expected {
			t.Errorf("tools[%d]: expected name %q, got %q", i, expected, tools[i].Function.Name)
		}
	}
}
