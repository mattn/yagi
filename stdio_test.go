package main

import (
	"encoding/json"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestJSONRPCRequest_Marshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "chat",
		Params:  json.RawMessage(`{"stream":true}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", m["jsonrpc"])
	}
	if m["id"].(float64) != 1 {
		t.Errorf("id = %v, want 1", m["id"])
	}
	if m["method"] != "chat" {
		t.Errorf("method = %v, want chat", m["method"])
	}
	params, ok := m["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params is not an object: %T", m["params"])
	}
	if params["stream"] != true {
		t.Errorf("params.stream = %v, want true", params["stream"])
	}
}

func TestJSONRPCRequest_Unmarshal(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":42,"method":"chat","params":{"messages":[]}}`
	var req JSONRPCRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want \"2.0\"", req.JSONRPC)
	}
	if req.ID.(float64) != 42 {
		t.Errorf("ID = %v, want 42", req.ID)
	}
	if req.Method != "chat" {
		t.Errorf("Method = %q, want \"chat\"", req.Method)
	}
	if req.Params == nil {
		t.Fatal("Params is nil")
	}
}

func TestChatResponse_Marshal(t *testing.T) {
	resp := ChatResponse{Content: "hello", Done: true}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["content"] != "hello" {
		t.Errorf("content = %v, want hello", m["content"])
	}
	if m["done"] != true {
		t.Errorf("done = %v, want true", m["done"])
	}
}

func TestChatResponse_MarshalOmitsEmpty(t *testing.T) {
	resp := ChatResponse{Done: true}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["content"]; ok {
		t.Error("content should be omitted when empty")
	}
	if _, ok := m["error"]; ok {
		t.Error("error should be omitted when empty")
	}
	if m["done"] != true {
		t.Errorf("done = %v, want true", m["done"])
	}
}

func TestChatRequest_Unmarshal(t *testing.T) {
	input := `{"messages":[{"role":"user","content":"hi"}],"stream":true,"model":"gpt-4"}`
	var req ChatRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("Messages[0].Role = %q, want %q", req.Messages[0].Role, openai.ChatMessageRoleUser)
	}
	if req.Messages[0].Content != "hi" {
		t.Errorf("Messages[0].Content = %q, want \"hi\"", req.Messages[0].Content)
	}
	if !req.Stream {
		t.Error("Stream = false, want true")
	}
	if req.Model != "gpt-4" {
		t.Errorf("Model = %q, want \"gpt-4\"", req.Model)
	}
}
