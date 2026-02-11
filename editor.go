package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func openEditor(initial string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}

	tmpfile, err := os.CreateTemp("", "yagi-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpfile.Name()
	defer os.Remove(tmpPath)

	if initial != "" {
		if _, err := tmpfile.WriteString(initial); err != nil {
			tmpfile.Close()
			return "", fmt.Errorf("failed to write initial content: %w", err)
		}
	}
	tmpfile.Close()

	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read edited content: %w", err)
	}

	return string(content), nil
}
