package main

type Provider struct {
	Name   string
	APIURL string
	EnvKey string
}

var providers = []Provider{
	{
		Name:   "openai",
		APIURL: "https://api.openai.com/v1",
		EnvKey: "OPENAI_API_KEY",
	},
	{
		Name:   "google",
		APIURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		EnvKey: "GEMINI_API_KEY",
	},
	{
		Name:   "anthropic",
		APIURL: "https://api.anthropic.com/v1",
		EnvKey: "ANTHROPIC_API_KEY",
	},
	{
		Name:   "deepseek",
		APIURL: "https://api.deepseek.com/v1",
		EnvKey: "DEEPSEEK_API_KEY",
	},
	{
		Name:   "mistral",
		APIURL: "https://api.mistral.ai/v1",
		EnvKey: "MISTRAL_API_KEY",
	},
	{
		Name:   "groq",
		APIURL: "https://api.groq.com/openai/v1",
		EnvKey: "GROQ_API_KEY",
	},
	{
		Name:   "xai",
		APIURL: "https://api.x.ai/v1",
		EnvKey: "XAI_API_KEY",
	},
	{
		Name:   "perplexity",
		APIURL: "https://api.perplexity.ai",
		EnvKey: "PERPLEXITY_API_KEY",
	},
	{
		Name:   "together",
		APIURL: "https://api.together.xyz/v1",
		EnvKey: "TOGETHER_API_KEY",
	},
	{
		Name:   "fireworks",
		APIURL: "https://api.fireworks.ai/inference/v1",
		EnvKey: "FIREWORKS_API_KEY",
	},
	{
		Name:   "cerebras",
		APIURL: "https://api.cerebras.ai/v1",
		EnvKey: "CEREBRAS_API_KEY",
	},
	{
		Name:   "cohere",
		APIURL: "https://api.cohere.com/compatibility/v1",
		EnvKey: "COHERE_API_KEY",
	},
	{
		Name:   "openrouter",
		APIURL: "https://openrouter.ai/api/v1",
		EnvKey: "OPENROUTER_API_KEY",
	},
	{
		Name:   "sambanova",
		APIURL: "https://api.sambanova.ai/v1",
		EnvKey: "SAMBANOVA_API_KEY",
	},
	{
		Name:   "zai",
		APIURL: "https://open.bigmodel.cn/api/paas/v4",
		EnvKey: "Z_AI_API_KEY",
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
