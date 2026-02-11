package engine

import (
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

type ProviderInfo struct {
	Name   string
	APIURL string
	EnvKey string
}

var DefaultProviders = []ProviderInfo{
	{Name: "openai", APIURL: "https://api.openai.com/v1", EnvKey: "OPENAI_API_KEY"},
	{Name: "google", APIURL: "https://generativelanguage.googleapis.com/v1beta/openai", EnvKey: "GEMINI_API_KEY"},
	{Name: "anthropic", APIURL: "https://api.anthropic.com/v1", EnvKey: "ANTHROPIC_API_KEY"},
	{Name: "deepseek", APIURL: "https://api.deepseek.com/v1", EnvKey: "DEEPSEEK_API_KEY"},
	{Name: "groq", APIURL: "https://api.groq.com/openai/v1", EnvKey: "GROQ_API_KEY"},
	{Name: "xai", APIURL: "https://api.x.ai/v1", EnvKey: "XAI_API_KEY"},
	{Name: "openrouter", APIURL: "https://openrouter.ai/api/v1", EnvKey: "OPENROUTER_API_KEY"},
}

func NewClientFromProvider(providerName, model, apiKey string) (*openai.Client, error) {
	var provider *ProviderInfo
	for i := range DefaultProviders {
		if DefaultProviders[i].Name == providerName {
			provider = &DefaultProviders[i]
			break
		}
	}
	if provider == nil {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}

	if apiKey == "" && provider.EnvKey != "" {
		apiKey = os.Getenv(provider.EnvKey)
		if apiKey == "" {
			return nil, fmt.Errorf("%s environment variable is required for provider %s", provider.EnvKey, providerName)
		}
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = provider.APIURL
	return openai.NewClientWithConfig(config), nil
}

func UserMessage(content string) []openai.ChatCompletionMessage {
	return []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: content,
		},
	}
}
