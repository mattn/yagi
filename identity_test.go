package main

import (
	"os"
	"path/filepath"
	"testing"
)

func resetIdentityState() {
	systemPrompt = ""
	skillPrompts = map[string]string{}
}

func TestLoadIdentity_NonExistent(t *testing.T) {
	resetIdentityState()
	err := loadIdentity("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if systemPrompt != "" {
		t.Fatalf("expected empty systemPrompt, got %q", systemPrompt)
	}
}

func TestLoadIdentity_Valid(t *testing.T) {
	resetIdentityState()
	dir := t.TempDir()
	content := "You are a helpful assistant"
	if err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := loadIdentity(dir); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if systemPrompt != content {
		t.Fatalf("expected systemPrompt %q, got %q", content, systemPrompt)
	}
}

func TestLoadSkills_NonExistent(t *testing.T) {
	resetIdentityState()
	err := loadSkills("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestLoadSkills_Valid(t *testing.T) {
	resetIdentityState()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "code skill content"
	if err := os.WriteFile(filepath.Join(skillsDir, "code.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := loadSkills(dir); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got, ok := skillPrompts["code"]; !ok {
		t.Fatal("expected skillPrompts to contain key \"code\"")
	} else if got != content {
		t.Fatalf("expected skillPrompts[\"code\"] = %q, got %q", content, got)
	}
}

func TestLoadSkills_SkipsNonMd(t *testing.T) {
	resetIdentityState()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "notes.txt"), []byte("text content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := loadSkills(dir); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := skillPrompts["notes"]; ok {
		t.Fatal("expected .txt file to be skipped")
	}
}

func TestLoadSkills_SkipsDirectories(t *testing.T) {
	resetIdentityState()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	subDir := filepath.Join(skillsDir, "subdir.md")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := loadSkills(dir); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := skillPrompts["subdir"]; ok {
		t.Fatal("expected directory to be skipped")
	}
}

func TestGetSystemMessage_Empty(t *testing.T) {
	resetIdentityState()
	if got := getSystemMessage(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestGetSystemMessage_OnlySystem(t *testing.T) {
	resetIdentityState()
	systemPrompt = "system"
	if got := getSystemMessage(""); got != "system" {
		t.Fatalf("expected %q, got %q", "system", got)
	}
}

func TestGetSystemMessage_WithSkill(t *testing.T) {
	resetIdentityState()
	systemPrompt = "system"
	skillPrompts["test"] = "skill content"
	want := "system\n---\nskill content"
	if got := getSystemMessage("test"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGetSystemMessage_UnknownSkill(t *testing.T) {
	resetIdentityState()
	systemPrompt = "system"
	if got := getSystemMessage("unknown"); got != "system" {
		t.Fatalf("expected %q, got %q", "system", got)
	}
}
