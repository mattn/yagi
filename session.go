package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const maxSessionMessages = 100

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
