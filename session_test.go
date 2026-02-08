package main

import (
	"fmt"
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestSessionSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := "/home/user/project"

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
		{Role: openai.ChatMessageRoleAssistant, Content: "hi there"},
		{Role: openai.ChatMessageRoleUser, Content: "how are you?"},
		{Role: openai.ChatMessageRoleAssistant, Content: "I'm fine"},
	}

	if err := saveSession(tmpDir, workDir, messages); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}

	loaded, err := loadSession(tmpDir, workDir)
	if err != nil {
		t.Fatalf("loadSession failed: %v", err)
	}

	if len(loaded) != len(messages) {
		t.Fatalf("loaded %d messages, want %d", len(loaded), len(messages))
	}

	for i, m := range loaded {
		if m.Role != messages[i].Role || m.Content != messages[i].Content {
			t.Errorf("message[%d]: got {%s, %s}, want {%s, %s}", i, m.Role, m.Content, messages[i].Role, messages[i].Content)
		}
	}
}

func TestSessionFiltersSystem(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := "/home/user/project"

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "system prompt"},
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
		{Role: openai.ChatMessageRoleAssistant, Content: "hi"},
	}

	if err := saveSession(tmpDir, workDir, messages); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}

	loaded, err := loadSession(tmpDir, workDir)
	if err != nil {
		t.Fatalf("loadSession failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("loaded %d messages, want 2 (system filtered)", len(loaded))
	}
	if loaded[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("first message role: got %s, want user", loaded[0].Role)
	}
}

func TestSessionPerDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	msgs1 := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "project A"},
	}
	msgs2 := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "project B"},
	}

	saveSession(tmpDir, "/home/user/projectA", msgs1)
	saveSession(tmpDir, "/home/user/projectB", msgs2)

	loaded1, _ := loadSession(tmpDir, "/home/user/projectA")
	loaded2, _ := loadSession(tmpDir, "/home/user/projectB")

	if len(loaded1) != 1 || loaded1[0].Content != "project A" {
		t.Errorf("projectA session: got %v", loaded1)
	}
	if len(loaded2) != 1 || loaded2[0].Content != "project B" {
		t.Errorf("projectB session: got %v", loaded2)
	}
}

func TestSessionLoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	loaded, err := loadSession(tmpDir, "/nonexistent")
	if err != nil {
		t.Fatalf("loadSession should not error on nonexistent: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil, got %v", loaded)
	}
}

func TestSessionClear(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := "/home/user/project"

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
	}
	saveSession(tmpDir, workDir, messages)

	if err := clearSession(tmpDir, workDir); err != nil {
		t.Fatalf("clearSession failed: %v", err)
	}

	loaded, err := loadSession(tmpDir, workDir)
	if err != nil {
		t.Fatalf("loadSession after clear failed: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after clear, got %v", loaded)
	}
}

func TestSessionClearNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	if err := clearSession(tmpDir, "/nonexistent"); err != nil {
		t.Errorf("clearSession on nonexistent should not error: %v", err)
	}
}

func TestSessionWithToolCalls(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := "/home/user/project"

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "list files"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{
					ID:   "call_1",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "list_files",
						Arguments: `{"path":"."}`,
					},
				},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "file1.go\nfile2.go", ToolCallID: "call_1"},
		{Role: openai.ChatMessageRoleAssistant, Content: "Here are the files."},
	}

	if err := saveSession(tmpDir, workDir, messages); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}

	loaded, err := loadSession(tmpDir, workDir)
	if err != nil {
		t.Fatalf("loadSession failed: %v", err)
	}

	if len(loaded) != 4 {
		t.Fatalf("loaded %d messages, want 4", len(loaded))
	}

	if len(loaded[1].ToolCalls) != 1 {
		t.Fatalf("tool calls: got %d, want 1", len(loaded[1].ToolCalls))
	}
	if loaded[1].ToolCalls[0].Function.Name != "list_files" {
		t.Errorf("tool call name: got %s, want list_files", loaded[1].ToolCalls[0].Function.Name)
	}
	if loaded[2].ToolCallID != "call_1" {
		t.Errorf("tool call id: got %s, want call_1", loaded[2].ToolCallID)
	}
}

func TestTruncateMessages(t *testing.T) {
	msgs := make([]openai.ChatCompletionMessage, 0)
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf("q%d", i)},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: fmt.Sprintf("a%d", i)},
		)
	}

	result := truncateMessages(msgs, 6)
	if len(result) != 6 {
		t.Fatalf("got %d messages, want 6", len(result))
	}
	if result[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("first message should be user, got %s", result[0].Role)
	}
	if result[0].Content != "q7" {
		t.Errorf("first message content: got %s, want q7", result[0].Content)
	}
}

func TestTruncateMessagesSkipsToolOrphan(t *testing.T) {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "q1"},
		{Role: openai.ChatMessageRoleAssistant, Content: "a1"},
		{Role: openai.ChatMessageRoleUser, Content: "q2"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				{ID: "call_1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "tool", Arguments: "{}"}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "result", ToolCallID: "call_1"},
		{Role: openai.ChatMessageRoleAssistant, Content: "a2"},
		{Role: openai.ChatMessageRoleUser, Content: "q3"},
		{Role: openai.ChatMessageRoleAssistant, Content: "a3"},
	}

	result := truncateMessages(msgs, 3)
	if result[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("first message should be user, got %s", result[0].Role)
	}
	if result[0].Content != "q3" {
		t.Errorf("first message content: got %s, want q3", result[0].Content)
	}
}

func TestTruncateMessagesNoOp(t *testing.T) {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
		{Role: openai.ChatMessageRoleAssistant, Content: "hi"},
	}
	result := truncateMessages(msgs, 10)
	if len(result) != 2 {
		t.Fatalf("got %d messages, want 2", len(result))
	}
}

func TestSessionEmptyMessages(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := "/home/user/project"

	if err := saveSession(tmpDir, workDir, nil); err != nil {
		t.Fatalf("saveSession with nil should not error: %v", err)
	}

	fp := sessionFilePath(tmpDir, workDir)
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("session file should not be created for empty messages")
	}
}
