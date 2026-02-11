package main

import (
	"context"
	"encoding/json"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yagi-agent/yagi/engine"
)

func newTestEngine() *engine.Engine {
	return engine.New(engine.Config{})
}

func TestRegisterTool(t *testing.T) {
	e := newTestEngine()

	params := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	e.RegisterTool("test_tool", "A test tool", params, func(ctx context.Context, args string) (string, error) {
		return args, nil
	}, false)

	if len(e.Tools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(e.Tools()))
	}
	if e.Tools()[0].Type != openai.ToolTypeFunction {
		t.Errorf("expected tool type %v, got %v", openai.ToolTypeFunction, e.Tools()[0].Type)
	}
	if e.Tools()[0].Function.Name != "test_tool" {
		t.Errorf("expected tool name %q, got %q", "test_tool", e.Tools()[0].Function.Name)
	}
	if e.Tools()[0].Function.Description != "A test tool" {
		t.Errorf("expected description %q, got %q", "A test tool", e.Tools()[0].Function.Description)
	}
}

func TestRegisterTool_Multiple(t *testing.T) {
	e := newTestEngine()

	for i, name := range []string{"tool_a", "tool_b", "tool_c"} {
		n := name
		e.RegisterTool(n, "desc "+n, json.RawMessage(`{}`), func(ctx context.Context, args string) (string, error) {
			return n, nil
		}, false)
		if len(e.Tools()) != i+1 {
			t.Fatalf("after registering %s: expected %d tools, got %d", n, i+1, len(e.Tools()))
		}
	}
}
