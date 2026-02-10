package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeHash(t *testing.T) {
	input := []byte("hello world")
	expected := sha256.Sum256(input)
	want := hex.EncodeToString(expected[:])
	got := computeHash(input)
	if got != want {
		t.Errorf("computeHash(%q) = %q, want %q", input, got, want)
	}
}

func TestComputeHash_Empty(t *testing.T) {
	got := computeHash([]byte{})
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("computeHash(empty) = %q, want %q", got, want)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	input := []byte("deterministic test input")
	h1 := computeHash(input)
	h2 := computeHash(input)
	if h1 != h2 {
		t.Errorf("computeHash not deterministic: %q != %q", h1, h2)
	}
}

func TestIsPluginApproved_Approved(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA", "pluginB"},
		},
	}
	if !isPluginApproved(approvals, "/work/dir", "pluginA") {
		t.Error("expected pluginA to be approved")
	}
}

func TestIsPluginApproved_NotApproved(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA"},
		},
	}
	if isPluginApproved(approvals, "/work/dir", "pluginX") {
		t.Error("expected pluginX to not be approved")
	}
}

func TestIsPluginApproved_DifferentDir(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir1": {"pluginA"},
		},
	}
	if isPluginApproved(approvals, "/work/dir2", "pluginA") {
		t.Error("expected pluginA to not be approved in different directory")
	}
}

func TestAddPluginApproval_New(t *testing.T) {
	approvals := &approvalRecord{
		Directories: make(map[string][]string),
	}
	addPluginApproval(approvals, "/work/dir", "pluginA")
	plugins := approvals.Directories["/work/dir"]
	if len(plugins) != 1 || plugins[0] != "pluginA" {
		t.Errorf("expected [pluginA], got %v", plugins)
	}
}

func TestAddPluginApproval_Duplicate(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA"},
		},
	}
	addPluginApproval(approvals, "/work/dir", "pluginA")
	plugins := approvals.Directories["/work/dir"]
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin after duplicate add, got %d: %v", len(plugins), plugins)
	}
}

func TestLoadSaveApprovalRecords(t *testing.T) {
	tmpDir := t.TempDir()
	record := &approvalRecord{
		Directories: map[string][]string{
			"/proj/a": {"plugin1", "plugin2"},
			"/proj/b": {"plugin3"},
		},
	}
	if err := saveApprovalRecords(tmpDir, record); err != nil {
		t.Fatalf("saveApprovalRecords: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "approved_plugins.json")); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	loaded, err := loadApprovalRecords(tmpDir)
	if err != nil {
		t.Fatalf("loadApprovalRecords: %v", err)
	}
	for dir, plugins := range record.Directories {
		got := loaded.Directories[dir]
		if len(got) != len(plugins) {
			t.Errorf("dir %q: expected %v, got %v", dir, plugins, got)
			continue
		}
		for i := range plugins {
			if got[i] != plugins[i] {
				t.Errorf("dir %q[%d]: expected %q, got %q", dir, i, plugins[i], got[i])
			}
		}
	}
}

func TestRemovePluginApproval_Exists(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA", "pluginB", "pluginC"},
		},
	}
	if !removePluginApproval(approvals, "/work/dir", "pluginB") {
		t.Error("expected removePluginApproval to return true")
	}
	plugins := approvals.Directories["/work/dir"]
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d: %v", len(plugins), plugins)
	}
	if isPluginApproved(approvals, "/work/dir", "pluginB") {
		t.Error("pluginB should no longer be approved")
	}
}

func TestRemovePluginApproval_NotExists(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA"},
		},
	}
	if removePluginApproval(approvals, "/work/dir", "pluginX") {
		t.Error("expected removePluginApproval to return false")
	}
}

func TestRemovePluginApproval_LastPlugin(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA"},
		},
	}
	if !removePluginApproval(approvals, "/work/dir", "pluginA") {
		t.Error("expected removePluginApproval to return true")
	}
	if _, exists := approvals.Directories["/work/dir"]; exists {
		t.Error("expected directory entry to be removed when last plugin is revoked")
	}
}

func TestRemoveAllPluginApprovals(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA", "pluginB"},
		},
	}
	count := removeAllPluginApprovals(approvals, "/work/dir")
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
	if _, exists := approvals.Directories["/work/dir"]; exists {
		t.Error("expected directory entry to be removed")
	}
}

func TestRemoveAllPluginApprovals_Empty(t *testing.T) {
	approvals := &approvalRecord{
		Directories: make(map[string][]string),
	}
	count := removeAllPluginApprovals(approvals, "/work/dir")
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestListApprovedPlugins(t *testing.T) {
	approvals := &approvalRecord{
		Directories: map[string][]string{
			"/work/dir": {"pluginA", "pluginB"},
		},
	}
	plugins := listApprovedPlugins(approvals, "/work/dir")
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestListApprovedPlugins_Empty(t *testing.T) {
	approvals := &approvalRecord{
		Directories: make(map[string][]string),
	}
	plugins := listApprovedPlugins(approvals, "/work/dir")
	if plugins != nil {
		t.Errorf("expected nil, got %v", plugins)
	}
}

func TestLoadApprovalRecords_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	record, err := loadApprovalRecords(tmpDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if len(record.Directories) != 0 {
		t.Errorf("expected empty directories, got %v", record.Directories)
	}
}
