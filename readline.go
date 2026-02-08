package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

var rl *readline.Instance

func modelCompleter() []string {
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(modelsTxt))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			models = append(models, line)
		}
	}
	return models
}

func initReadline(prompt, configDir string) error {
	models := modelCompleter()
	var modelItems []readline.PrefixCompleterInterface
	for _, m := range models {
		modelItems = append(modelItems, readline.PcItem(m))
	}

	cfg := &readline.Config{
		Prompt:          prompt,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stderr:          os.Stderr,
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("/help"),
			readline.PcItem("/model", modelItems...),
			readline.PcItem("/clear"),
			readline.PcItem("/save"),
			readline.PcItem("/memory"),
		),
	}
	if configDir != "" {
		cfg.HistoryFile = filepath.Join(configDir, "history")
	}
	var err error
	rl, err = readline.NewEx(cfg)
	return err
}

func closeReadline() {
	if rl != nil {
		rl.Close()
	}
}

func readlineInput(prompt string) (string, error) {
	rl.SetPrompt(prompt)
	return rl.Readline()
}

func isInterrupt(err error) bool {
	return err == readline.ErrInterrupt
}
