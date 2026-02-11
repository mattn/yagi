package engine

import (
	openai "github.com/sashabaranov/go-openai"
)

func UserMessage(content string) []openai.ChatCompletionMessage {
	return []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: content,
		},
	}
}
