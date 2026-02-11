package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	openai "github.com/sashabaranov/go-openai"
)

const maxSessionMessages = 100

const maxContextChars = 100000

const compressThreshold = 80000

type sessionData struct {
	Dir       string                         `json:"dir"`
	UpdatedAt string                         `json:"updated_at"`
	Messages  []openai.ChatCompletionMessage `json:"messages"`
}

func sessionsDir(configDir string) string {
	return filepath.Join(configDir, "sessions")
}

func sessionFilePath(configDir, workDir string) string {
	h := sha256.Sum256([]byte(workDir))
	name := fmt.Sprintf("%x.json", h[:16])
	return filepath.Join(sessionsDir(configDir), name)
}

func saveSession(configDir, workDir string, messages []openai.ChatCompletionMessage) error {
	dir := sessionsDir(configDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	filtered := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		if m.Role == openai.ChatMessageRoleSystem {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return nil
	}

	filtered = truncateMessages(filtered, maxSessionMessages)

	sd := sessionData{
		Dir:       workDir,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Messages:  filtered,
	}

	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFilePath(configDir, workDir), data, 0600)
}

func loadSession(configDir, workDir string) ([]openai.ChatCompletionMessage, error) {
	data, err := os.ReadFile(sessionFilePath(configDir, workDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sd sessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return sd.Messages, nil
}

func truncateMessages(msgs []openai.ChatCompletionMessage, max int) []openai.ChatCompletionMessage {
	if len(msgs) <= max {
		return msgs
	}
	msgs = msgs[len(msgs)-max:]
	for len(msgs) > 0 && msgs[0].Role != openai.ChatMessageRoleUser {
		msgs = msgs[1:]
	}
	return msgs
}

func clearSession(configDir, workDir string) error {
	err := os.Remove(sessionFilePath(configDir, workDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func estimateChars(msgs []openai.ChatCompletionMessage) int {
	total := 0
	for _, m := range msgs {
		total += utf8.RuneCountInString(m.Content)
		for _, tc := range m.ToolCalls {
			total += utf8.RuneCountInString(tc.Function.Arguments)
		}
	}
	return total
}

func compressContext(ctx context.Context, client *openai.Client, messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	chars := estimateChars(messages)
	if chars < compressThreshold {
		return messages
	}

	start := 0
	for i, m := range messages {
		if m.Role == openai.ChatMessageRoleSystem {
			start = i + 1
			break
		}
	}
	if start >= len(messages) {
		return messages
	}

	end := start
	kept := estimateChars(messages[start:])
	for end < len(messages)-2 && kept > maxContextChars/2 {
		kept -= utf8.RuneCountInString(messages[end].Content)
		for _, tc := range messages[end].ToolCalls {
			kept -= utf8.RuneCountInString(tc.Function.Arguments)
		}
		end++
	}

	if end <= start {
		return messages
	}

	for end < len(messages) && messages[end].Role != openai.ChatMessageRoleUser {
		end++
	}
	if end >= len(messages) {
		return messages
	}

	oldMsgs := messages[start:end]
	summary := summarizeMessages(ctx, client, oldMsgs)
	if summary == "" {
		return messages
	}

	if !quiet {
		fmt.Fprintf(stderr, "\x1b[33m[context compressed: %d chars → summarized]\x1b[0m\n", chars)
	}

	var result []openai.ChatCompletionMessage
	result = append(result, messages[:start]...)
	result = append(result, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: "[Previous conversation summary]\n" + summary,
	})
	result = append(result, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: "Understood. I have the context from our previous conversation.",
	})
	result = append(result, messages[end:]...)
	return result
}

func summarizeMessages(ctx context.Context, client *openai.Client, msgs []openai.ChatCompletionMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case openai.ChatMessageRoleUser:
			sb.WriteString("User: ")
			sb.WriteString(m.Content)
			sb.WriteString("\n")
		case openai.ChatMessageRoleAssistant:
			sb.WriteString("Assistant: ")
			if m.Content != "" {
				sb.WriteString(m.Content)
			}
			for _, tc := range m.ToolCalls {
				sb.WriteString("[tool: ")
				sb.WriteString(tc.Function.Name)
				sb.WriteString("]")
			}
			sb.WriteString("\n")
		case openai.ChatMessageRoleTool:
			content := m.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			sb.WriteString("Tool result: ")
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}

	summaryMsgs := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "Summarize the following conversation concisely. Preserve key decisions, file paths, code changes, and important context. Write in the same language as the conversation. Keep it under 500 characters.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: sb.String(),
		},
	}

	stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    model,
		Messages: summaryMsgs,
	})
	if err != nil {
		return ""
	}
	defer stream.Close()

	var result strings.Builder
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(resp.Choices) > 0 {
			result.WriteString(resp.Choices[0].Delta.Content)
		}
	}
	return result.String()
}
