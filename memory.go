package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var (
	memoryData = map[string]string{}
	memoryMu   sync.RWMutex
	memoryPath string
)

func loadMemory(configDir string) error {
	memoryPath = filepath.Join(configDir, "memory.json")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	memoryMu.Lock()
	defer memoryMu.Unlock()
	return json.Unmarshal(data, &memoryData)
}

func saveMemory() error {
	memoryMu.RLock()
	data, err := json.MarshalIndent(memoryData, "", "  ")
	memoryMu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(memoryPath, data, 0600)
}

func getMemory(key string) string {
	memoryMu.RLock()
	defer memoryMu.RUnlock()
	return memoryData[key]
}

func setMemory(key, value string) error {
	memoryMu.Lock()
	memoryData[key] = value
	memoryMu.Unlock()
	return saveMemory()
}

func deleteMemory(key string) error {
	memoryMu.Lock()
	delete(memoryData, key)
	memoryMu.Unlock()
	return saveMemory()
}

func getAllMemory() map[string]string {
	memoryMu.RLock()
	defer memoryMu.RUnlock()
	result := make(map[string]string, len(memoryData))
	for k, v := range memoryData {
		result[k] = v
	}
	return result
}

func getMemoryAsMarkdown() string {
	memoryMu.RLock()
	defer memoryMu.RUnlock()

	if len(memoryData) == 0 {
		return ""
	}

	var md string
	md = "\n---\n## Learned Information\n"
	for k, v := range memoryData {
		md += "- " + k + ": " + v + "\n"
	}
	return md
}
