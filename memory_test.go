package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemory(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()
	memoryPath = filepath.Join(tmpDir, "memory.json")

	// Test setting memory
	if err := setMemory("test_key", "test_value"); err != nil {
		t.Fatalf("setMemory failed: %v", err)
	}

	// Test getting memory
	value := getMemory("test_key")
	if value != "test_value" {
		t.Errorf("getMemory returned %q, want %q", value, "test_value")
	}

	// Test getting non-existent key
	value = getMemory("nonexistent")
	if value != "" {
		t.Errorf("getMemory for nonexistent key returned %q, want empty string", value)
	}

	// Test getAllMemory
	allMemory := getAllMemory()
	if len(allMemory) != 1 {
		t.Errorf("getAllMemory returned %d entries, want 1", len(allMemory))
	}
	if allMemory["test_key"] != "test_value" {
		t.Errorf("getAllMemory[test_key] = %q, want %q", allMemory["test_key"], "test_value")
	}

	// Test memory persistence
	if err := loadMemory(tmpDir); err != nil {
		t.Fatalf("loadMemory failed: %v", err)
	}
	value = getMemory("test_key")
	if value != "test_value" {
		t.Errorf("After reload, getMemory returned %q, want %q", value, "test_value")
	}

	// Test deleteMemory
	if err := deleteMemory("test_key"); err != nil {
		t.Fatalf("deleteMemory failed: %v", err)
	}
	value = getMemory("test_key")
	if value != "" {
		t.Errorf("After delete, getMemory returned %q, want empty string", value)
	}

	// Test getMemoryAsMarkdown
	setMemory("user_name", "mattn")
	setMemory("language", "Go")
	md := getMemoryAsMarkdown()
	if md == "" {
		t.Error("getMemoryAsMarkdown returned empty string")
	}
	if len(md) < 10 {
		t.Errorf("getMemoryAsMarkdown returned %q, seems too short", md)
	}

	// Test empty memory markdown
	deleteMemory("user_name")
	deleteMemory("language")
	md = getMemoryAsMarkdown()
	if md != "" {
		t.Errorf("getMemoryAsMarkdown for empty memory returned %q, want empty string", md)
	}
}

func TestLoadMemoryNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	err := loadMemory(tmpDir)
	if err != nil {
		t.Errorf("loadMemory on non-existent file should not error, got: %v", err)
	}
}

func TestMemoryConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	memoryPath = filepath.Join(tmpDir, "memory.json")

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			setMemory("key", "value")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify the file is valid
	if _, err := os.Stat(memoryPath); err != nil {
		t.Errorf("memory.json not created: %v", err)
	}
}
