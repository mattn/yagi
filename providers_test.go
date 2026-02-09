package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	// Default providers do not allow name duplication, but the extra providers
	// do, so we check the defaultProvider instead of the provider .
	seen := make(map[string]bool)
	for _, p := range defaultProviders {
		if seen[p.Name] {
			t.Errorf("duplicate provider name: %q", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestProviders_NonEmpty(t *testing.T) {
	// Default providers do not allow empty EnvKey, but the extra providers do,
	// so we check the defaultProvider instead of the provider .
	for _, p := range defaultProviders {
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

func resetProviders() {
	providers = defaultProviders
}

func TestLoadExtraProviders_NonExistentDir(t *testing.T) {
	resetProviders()
	err := loadExtraProviders("/tmp/yagi-test-nonexistent-dir")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// Expect the first provider keeps match with default's one.
	if providers[0] != defaultProviders[0] {
		t.Fatalf("expected default providers, got %+v", providers[0])
	}
}

func TestLoadExtraProviders_Valid(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "providers.json"), []byte(`[{"name":"test","apiurl":"http://127.0.0.1:8080/v1"}]`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resetProviders()
	err = loadExtraProviders(dir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	want := Provider{
		Name:   "test",
		APIURL: "http://127.0.0.1:8080/v1",
	}
	if providers[0] != want {
		t.Fatalf("expected first provider %+v, got %+v", want, providers[0])
	}
}

func TestLoadExtraProviders_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "providers.json"), []byte(`[invalid]`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resetProviders()
	err = loadExtraProviders(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	// Expect the first provider keeps match with default's one.
	if providers[0] != defaultProviders[0] {
		t.Fatalf("expected default providers, got %+v", providers[0])
	}
}
