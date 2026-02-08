package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Prompt string `json:"prompt"`
}

var appConfig = Config{
	Prompt: "> ",
}

func loadConfig(configDir string) error {
	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &appConfig)
}
