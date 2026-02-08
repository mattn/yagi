package main

import "testing"

func TestFindProvider_Exists(t *testing.T) {
	names := []string{"openai", "google", "anthropic", "deepseek", "mistral", "groq", "xai", "perplexity", "together", "fireworks", "cerebras", "cohere", "openrouter", "sambanova", "zai"}
	for _, name := range names {
		p := findProvider(name)
		if p == nil {
			t.Errorf("findProvider(%q) returned nil, want non-nil", name)
			continue
		}
		if p.Name != name {
			t.Errorf("findProvider(%q).Name = %q, want %q", name, p.Name, name)
		}
	}
}

func TestFindProvider_NotFound(t *testing.T) {
	p := findProvider("nonexistent")
	if p != nil {
		t.Errorf("findProvider(%q) = %+v, want nil", "nonexistent", p)
	}
}

func TestProviders_UniqueNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range providers {
		if seen[p.Name] {
			t.Errorf("duplicate provider name: %q", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestProviders_NonEmpty(t *testing.T) {
	for _, p := range providers {
		if p.Name == "" {
			t.Error("provider has empty Name")
		}
		if p.APIURL == "" {
			t.Errorf("provider %q has empty APIURL", p.Name)
		}
		if p.EnvKey == "" {
			t.Errorf("provider %q has empty EnvKey", p.Name)
		}
	}
}
