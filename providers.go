package main

type Provider struct {
	Name   string
	APIURL string
	Model  string
	EnvKey string
}

var providers = []Provider{
	{
		Name:   "openai",
		APIURL: "https://api.openai.com/v1",
		Model:  "gpt-4.1-nano",
		EnvKey: "OPENAI_API_KEY",
	},
	{
		Name:   "gemini",
		APIURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		Model:  "gemini-2.5-flash",
		EnvKey: "GEMINI_API_KEY",
	},
	{
		Name:   "anthropic",
		APIURL: "https://api.anthropic.com/v1",
		Model:  "claude-sonnet-4-20250514",
		EnvKey: "ANTHROPIC_API_KEY",
	},
	{
		Name:   "deepseek",
		APIURL: "https://api.deepseek.com/v1",
		Model:  "deepseek-chat",
		EnvKey: "DEEPSEEK_API_KEY",
	},
	{
		Name:   "mistral",
		APIURL: "https://api.mistral.ai/v1",
		Model:  "mistral-small-latest",
		EnvKey: "MISTRAL_API_KEY",
	},
	{
		Name:   "groq",
		APIURL: "https://api.groq.com/openai/v1",
		Model:  "llama-3.3-70b-versatile",
		EnvKey: "GROQ_API_KEY",
	},
	{
		Name:   "xai",
		APIURL: "https://api.x.ai/v1",
		Model:  "grok-3-mini-fast",
		EnvKey: "XAI_API_KEY",
	},
	{
		Name:   "perplexity",
		APIURL: "https://api.perplexity.ai",
		Model:  "sonar",
		EnvKey: "PERPLEXITY_API_KEY",
	},
	{
		Name:   "together",
		APIURL: "https://api.together.xyz/v1",
		Model:  "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		EnvKey: "TOGETHER_API_KEY",
	},
	{
		Name:   "fireworks",
		APIURL: "https://api.fireworks.ai/inference/v1",
		Model:  "accounts/fireworks/models/llama4-maverick-instruct-basic",
		EnvKey: "FIREWORKS_API_KEY",
	},
	{
		Name:   "cerebras",
		APIURL: "https://api.cerebras.ai/v1",
		Model:  "llama-3.3-70b",
		EnvKey: "CEREBRAS_API_KEY",
	},
	{
		Name:   "cohere",
		APIURL: "https://api.cohere.com/compatibility/v1",
		Model:  "command-r-plus",
		EnvKey: "COHERE_API_KEY",
	},
	{
		Name:   "openrouter",
		APIURL: "https://openrouter.ai/api/v1",
		Model:  "openai/gpt-4.1-nano",
		EnvKey: "OPENROUTER_API_KEY",
	},
	{
		Name:   "sambanova",
		APIURL: "https://api.sambanova.ai/v1",
		Model:  "Meta-Llama-3.3-70B-Instruct",
		EnvKey: "SAMBANOVA_API_KEY",
	},
	{
		Name:   "glm",
		APIURL: "https://open.bigmodel.cn/api/paas/v4",
		Model:  "glm-4.5-flash",
		EnvKey: "GLM_API_KEY",
	},
}

func findProvider(name string) *Provider {
	for i := range providers {
		if providers[i].Name == name {
			return &providers[i]
		}
	}
	return nil
}
