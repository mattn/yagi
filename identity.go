package main

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	systemPrompt string
	skillPrompts = map[string]string{}
)

func loadIdentity(configDir string) error {
	var path string

	// Priority: Environment variable > config.json > default
	if envPath := os.Getenv("YAGI_IDENTITY_FILE"); envPath != "" {
		path = envPath
	} else if appConfig.IdentityFile != "" {
		path = appConfig.IdentityFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
	} else {
		path = filepath.Join(configDir, "IDENTITY.md")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	systemPrompt = string(data)
	return nil
}

func loadSkills(configDir string) error {
	skillsDir := filepath.Join(configDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		skillName := strings.TrimSuffix(entry.Name(), ".md")
		path := filepath.Join(skillsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		skillPrompts[skillName] = string(data)
	}

	return nil
}

func getSystemMessage(skill string) string {
	var parts []string

	if systemPrompt != "" {
		parts = append(parts, systemPrompt)
	}

	memoryMd := getMemoryAsMarkdown()
	if memoryMd != "" {
		parts = append(parts, memoryMd)
	}

	if skill != "" {
		if skillContent, ok := skillPrompts[skill]; ok {
			parts = append(parts, "\n---\n", skillContent)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "")
}
