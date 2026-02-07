package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NonExistentDir(t *testing.T) {
	appConfig = Config{Prompt: "🐐 "}

	err := loadConfig("/tmp/yagi-test-nonexistent-dir")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if appConfig.Prompt != "🐐 " {
		t.Fatalf("expected default prompt, got %q", appConfig.Prompt)
	}
}

func TestLoadConfig_ValidConfig(t *testing.T) {
	appConfig = Config{Prompt: "🐐 "}

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"prompt":">>> "}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = loadConfig(dir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if appConfig.Prompt != ">>> " {
		t.Fatalf("expected prompt %q, got %q", ">>> ", appConfig.Prompt)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	appConfig = Config{Prompt: "🐐 "}

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{invalid`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = loadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
