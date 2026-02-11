package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yagi-agent/yagi/engine"
)

func loadTestPlugin(t *testing.T, pluginPath string) {
	t.Helper()
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Skipf("plugin not found: %s", pluginPath)
	}
	eng = engine.New(engine.Config{})
	skipApproval = true
	pluginApprovals = nil
	tmpDir := t.TempDir()
	approvals := &approvalRecord{Directories: make(map[string][]string)}
	if err := loadPlugin(pluginPath, tmpDir, tmpDir, approvals); err != nil {
		t.Fatalf("loadPlugin(%s): %v", pluginPath, err)
	}
}

func pluginPath(name string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "yagi", "tools", name)
}

func TestPluginReadFile(t *testing.T) {
	loadTestPlugin(t, pluginPath("read_file.go"))

	tmp := filepath.Join(t.TempDir(), "hello.txt")
	os.WriteFile(tmp, []byte("hello world"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp})
	got := eng.ExecuteTool(context.Background(), "read_file", string(args))
	if got != "hello world" {
		t.Errorf("read_file: got %q, want %q", got, "hello world")
	}
}

func TestPluginReadFile_NotFound(t *testing.T) {
	loadTestPlugin(t, pluginPath("read_file.go"))

	args, _ := json.Marshal(map[string]string{"path": "/nonexistent/file.txt"})
	got := eng.ExecuteTool(context.Background(), "read_file", string(args))
	if !strings.Contains(got, "Error") {
		t.Errorf("read_file nonexistent: expected error, got %q", got)
	}
}

func TestPluginWriteFile(t *testing.T) {
	loadTestPlugin(t, pluginPath("write_file.go"))

	tmp := filepath.Join(t.TempDir(), "out.txt")
	args, _ := json.Marshal(map[string]string{"path": tmp, "content": "written content"})
	got := eng.ExecuteTool(context.Background(), "write_file", string(args))
	if !strings.Contains(got, "Successfully") {
		t.Errorf("write_file: got %q", got)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != "written content" {
		t.Errorf("file content: got %q, want %q", string(data), "written content")
	}
}

func TestPluginListFiles(t *testing.T) {
	loadTestPlugin(t, pluginPath("list_files.go"))

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	args, _ := json.Marshal(map[string]string{"path": dir})
	got := eng.ExecuteTool(context.Background(), "list_files", string(args))
	if !strings.Contains(got, "a.txt") {
		t.Errorf("list_files: missing a.txt in %q", got)
	}
	if !strings.Contains(got, "subdir") {
		t.Errorf("list_files: missing subdir in %q", got)
	}
	if !strings.Contains(got, "D subdir") {
		t.Errorf("list_files: subdir should be prefixed with D, got %q", got)
	}
	if !strings.Contains(got, "F a.txt") {
		t.Errorf("list_files: a.txt should be prefixed with F, got %q", got)
	}
}

func TestPluginListFiles_Empty(t *testing.T) {
	loadTestPlugin(t, pluginPath("list_files.go"))

	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": dir})
	got := eng.ExecuteTool(context.Background(), "list_files", string(args))
	if got != "" {
		t.Errorf("list_files empty dir: got %q, want empty", got)
	}
}

func TestPluginRunCommand(t *testing.T) {
	loadTestPlugin(t, pluginPath("run_command.go"))

	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	got := eng.ExecuteTool(context.Background(), "run_command", string(args))
	if got != "hello" {
		t.Errorf("run_command echo: got %q, want %q", got, "hello")
	}
}

func TestPluginRunCommand_WorkingDir(t *testing.T) {
	loadTestPlugin(t, pluginPath("run_command.go"))

	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"command": "pwd", "working_directory": dir})
	got := eng.ExecuteTool(context.Background(), "run_command", string(args))
	if !strings.Contains(got, filepath.Base(dir)) {
		t.Errorf("run_command pwd: got %q, expected to contain %q", got, dir)
	}
}

func TestPluginRunCommand_Failure(t *testing.T) {
	loadTestPlugin(t, pluginPath("run_command.go"))

	args, _ := json.Marshal(map[string]string{"command": "false"})
	got := eng.ExecuteTool(context.Background(), "run_command", string(args))
	if !strings.Contains(got, "Error:") {
		t.Errorf("run_command false: expected error, got %q", got)
	}
}

func TestPluginEditFile_Replace(t *testing.T) {
	loadTestPlugin(t, pluginPath("edit_file.go"))

	tmp := filepath.Join(t.TempDir(), "edit.txt")
	os.WriteFile(tmp, []byte("foo bar baz"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp, "old_str": "bar", "new_str": "qux"})
	got := eng.ExecuteTool(context.Background(), "edit_file", string(args))
	if !strings.Contains(got, "Successfully") {
		t.Errorf("edit_file replace: got %q", got)
	}

	data, _ := os.ReadFile(tmp)
	if string(data) != "foo qux baz" {
		t.Errorf("edit_file content: got %q, want %q", string(data), "foo qux baz")
	}
}

func TestPluginEditFile_Append(t *testing.T) {
	loadTestPlugin(t, pluginPath("edit_file.go"))

	tmp := filepath.Join(t.TempDir(), "append.txt")
	os.WriteFile(tmp, []byte("line1\n"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp, "old_str": "", "new_str": "line2\n"})
	got := eng.ExecuteTool(context.Background(), "edit_file", string(args))
	if !strings.Contains(got, "Successfully") {
		t.Errorf("edit_file append: got %q", got)
	}

	data, _ := os.ReadFile(tmp)
	if string(data) != "line1\nline2\n" {
		t.Errorf("edit_file append content: got %q", string(data))
	}
}

func TestPluginEditFile_NotFound(t *testing.T) {
	loadTestPlugin(t, pluginPath("edit_file.go"))

	tmp := filepath.Join(t.TempDir(), "edit.txt")
	os.WriteFile(tmp, []byte("hello"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp, "old_str": "missing", "new_str": "x"})
	got := eng.ExecuteTool(context.Background(), "edit_file", string(args))
	if !strings.Contains(got, "not found") {
		t.Errorf("edit_file not found: got %q", got)
	}
}

func TestPluginEditFile_Duplicate(t *testing.T) {
	loadTestPlugin(t, pluginPath("edit_file.go"))

	tmp := filepath.Join(t.TempDir(), "dup.txt")
	os.WriteFile(tmp, []byte("aaa bbb aaa"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp, "old_str": "aaa", "new_str": "x"})
	got := eng.ExecuteTool(context.Background(), "edit_file", string(args))
	if !strings.Contains(got, "2 times") {
		t.Errorf("edit_file duplicate: got %q", got)
	}
}

func TestPluginMakeDirectory(t *testing.T) {
	loadTestPlugin(t, pluginPath("make_directory.go"))

	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	args, _ := json.Marshal(map[string]string{"path": dir})
	got := eng.ExecuteTool(context.Background(), "make_directory", string(args))
	if !strings.Contains(got, "Created") {
		t.Errorf("make_directory: got %q", got)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestPluginSearchFiles_Glob(t *testing.T) {
	loadTestPlugin(t, pluginPath("search_files.go"))

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(dir, "world.go"), []byte("package main"), 0644)

	args, _ := json.Marshal(map[string]string{"directory": dir, "glob": "*.txt"})
	got := eng.ExecuteTool(context.Background(), "search_files", string(args))
	if !strings.Contains(got, "hello.txt") {
		t.Errorf("search_files glob: missing hello.txt in %q", got)
	}
	if strings.Contains(got, "world.go") {
		t.Errorf("search_files glob: should not contain world.go in %q", got)
	}
}

func TestPluginSearchFiles_Pattern(t *testing.T) {
	loadTestPlugin(t, pluginPath("search_files.go"))

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line one\nfind me here\nline three"), 0644)

	args, _ := json.Marshal(map[string]string{"directory": dir, "pattern": "find me"})
	got := eng.ExecuteTool(context.Background(), "search_files", string(args))
	if !strings.Contains(got, "find me here") {
		t.Errorf("search_files pattern: expected match, got %q", got)
	}
	if !strings.Contains(got, ":2:") {
		t.Errorf("search_files pattern: expected line number 2, got %q", got)
	}
}

func TestPluginSearchFiles_NoMatch(t *testing.T) {
	loadTestPlugin(t, pluginPath("search_files.go"))

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("nothing special"), 0644)

	args, _ := json.Marshal(map[string]string{"directory": dir, "pattern": "NOTFOUND"})
	got := eng.ExecuteTool(context.Background(), "search_files", string(args))
	if !strings.Contains(got, "No matches") {
		t.Errorf("search_files no match: got %q", got)
	}
}

func TestPluginSearchFiles_NoParams(t *testing.T) {
	loadTestPlugin(t, pluginPath("search_files.go"))

	args, _ := json.Marshal(map[string]string{"directory": "."})
	got := eng.ExecuteTool(context.Background(), "search_files", string(args))
	if !strings.Contains(got, "Error") {
		t.Errorf("search_files no params: expected error, got %q", got)
	}
}

func TestPluginDeleteFile(t *testing.T) {
	loadTestPlugin(t, pluginPath("delete_file.go"))

	tmp := filepath.Join(t.TempDir(), "del.txt")
	os.WriteFile(tmp, []byte("bye"), 0644)

	args, _ := json.Marshal(map[string]string{"path": tmp})
	got := eng.ExecuteTool(context.Background(), "delete_file", string(args))
	if !strings.Contains(got, "Deleted") {
		t.Errorf("delete_file: got %q", got)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestPluginDeleteFile_NotFound(t *testing.T) {
	loadTestPlugin(t, pluginPath("delete_file.go"))

	args, _ := json.Marshal(map[string]string{"path": "/nonexistent/file.txt"})
	got := eng.ExecuteTool(context.Background(), "delete_file", string(args))
	if !strings.Contains(got, "Error") {
		t.Errorf("delete_file nonexistent: expected error, got %q", got)
	}
}

func TestPluginDeleteFile_NonEmptyDir(t *testing.T) {
	loadTestPlugin(t, pluginPath("delete_file.go"))

	dir := t.TempDir()
	sub := filepath.Join(dir, "mydir")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "f.txt"), []byte("x"), 0644)

	args, _ := json.Marshal(map[string]any{"path": sub, "recursive": false})
	got := eng.ExecuteTool(context.Background(), "delete_file", string(args))
	if !strings.Contains(got, "not empty") {
		t.Errorf("delete_file non-empty: expected not empty error, got %q", got)
	}
}

func TestPluginDeleteFile_Recursive(t *testing.T) {
	loadTestPlugin(t, pluginPath("delete_file.go"))

	dir := t.TempDir()
	sub := filepath.Join(dir, "mydir")
	os.MkdirAll(filepath.Join(sub, "nested"), 0755)
	os.WriteFile(filepath.Join(sub, "nested", "f.txt"), []byte("x"), 0644)

	args, _ := json.Marshal(map[string]any{"path": sub, "recursive": true})
	got := eng.ExecuteTool(context.Background(), "delete_file", string(args))
	if !strings.Contains(got, "Deleted") {
		t.Errorf("delete_file recursive: got %q", got)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("directory should be deleted")
	}
}

func TestPluginDeleteFile_EmptyDir(t *testing.T) {
	loadTestPlugin(t, pluginPath("delete_file.go"))

	dir := filepath.Join(t.TempDir(), "empty")
	os.Mkdir(dir, 0755)

	args, _ := json.Marshal(map[string]string{"path": dir})
	got := eng.ExecuteTool(context.Background(), "delete_file", string(args))
	if !strings.Contains(got, "Deleted") {
		t.Errorf("delete_file empty dir: got %q", got)
	}
}

func TestPluginGlob_Recursive(t *testing.T) {
	loadTestPlugin(t, pluginPath("glob.go"))

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte("package pkg"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# README"), 0644)

	args, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "directory": dir})
	got := eng.ExecuteTool(context.Background(), "glob", string(args))
	if !strings.Contains(got, "main.go") {
		t.Errorf("glob **.go: missing main.go in %q", got)
	}
	if !strings.Contains(got, filepath.Join("src", "app.go")) {
		t.Errorf("glob **.go: missing src/app.go in %q", got)
	}
	if !strings.Contains(got, filepath.Join("src", "pkg", "util.go")) {
		t.Errorf("glob **.go: missing src/pkg/util.go in %q", got)
	}
	if strings.Contains(got, "README.md") {
		t.Errorf("glob **.go: should not contain README.md in %q", got)
	}
}

func TestPluginGlob_SubdirPattern(t *testing.T) {
	loadTestPlugin(t, pluginPath("glob.go"))

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]any{"pattern": "src/**/*.go", "directory": dir})
	got := eng.ExecuteTool(context.Background(), "glob", string(args))
	if !strings.Contains(got, "main.go") {
		t.Errorf("glob src/**/*.go: missing main.go in %q", got)
	}
	if strings.Contains(got, "guide.md") {
		t.Errorf("glob src/**/*.go: should not contain guide.md in %q", got)
	}
}

func TestPluginGlob_NoMatch(t *testing.T) {
	loadTestPlugin(t, pluginPath("glob.go"))

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "directory": dir})
	got := eng.ExecuteTool(context.Background(), "glob", string(args))
	if !strings.Contains(got, "No files found") {
		t.Errorf("glob no match: expected 'No files found', got %q", got)
	}
}

func TestPluginGlob_Limit(t *testing.T) {
	loadTestPlugin(t, pluginPath("glob.go"))

	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)), []byte(""), 0644)
	}

	args, _ := json.Marshal(map[string]any{"pattern": "**/*.txt", "directory": dir, "limit": 3})
	got := eng.ExecuteTool(context.Background(), "glob", string(args))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 {
		t.Errorf("glob limit: expected 3 results, got %d: %q", len(lines), got)
	}
}

func TestPluginGlob_SkipsDotDirs(t *testing.T) {
	loadTestPlugin(t, pluginPath("glob.go"))

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]any{"pattern": "**/*", "directory": dir})
	got := eng.ExecuteTool(context.Background(), "glob", string(args))
	if strings.Contains(got, ".git") {
		t.Errorf("glob: should skip .git directory, got %q", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("glob: should contain main.go, got %q", got)
	}
}

func TestPluginFetchURL(t *testing.T) {
	loadTestPlugin(t, pluginPath("fetch_url.go"))

	if !eng.HasTool("fetch_url") {
		t.Fatal("fetch_url not registered")
	}
}
