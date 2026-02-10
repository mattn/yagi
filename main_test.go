package main

import (
	"context"
	"encoding/json"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func resetGlobals() {
	tools = nil
	toolFuncs = map[string]func(context.Context, string) (string, error){}
	toolMeta = map[string]toolMetadata{}
	skipApproval = true
	pluginApprovals = nil
}

func TestRegisterTool(t *testing.T) {
	resetGlobals()

	params := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	registerTool("test_tool", "A test tool", params, func(ctx context.Context, args string) (string, error) {
		return args, nil
	}, false)

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

	registerTool("echo", "echoes input", json.RawMessage(`{}`), func(ctx context.Context, args string) (string, error) {
		return "result:" + args, nil
	}, false)

	got := executeTool(context.Background(), "echo", "hello")
	if got != "result:hello" {
		t.Errorf("expected %q, got %q", "result:hello", got)
	}
}

func TestExecuteTool_Unknown(t *testing.T) {
	resetGlobals()

	got := executeTool(context.Background(), "nonexistent", "")
	expected := "Unknown tool: nonexistent"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestRegisterTool_Multiple(t *testing.T) {
	resetGlobals()

	for i, name := range []string{"tool_a", "tool_b", "tool_c"} {
		n := name
		registerTool(n, "desc "+n, json.RawMessage(`{}`), func(ctx context.Context, args string) (string, error) {
			return n, nil
		}, false)
		if len(tools) != i+1 {
			t.Fatalf("after registering %s: expected %d tools, got %d", n, i+1, len(tools))
		}
	}

	for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
		if _, ok := toolFuncs[name]; !ok {
			t.Errorf("expected %s in toolFuncs map", name)
		}
	}
}
