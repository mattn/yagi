package main

import (
	"os"
	"path/filepath"

	"github.com/chzyer/readline"
)

var rl *readline.Instance

func initReadline(prompt, configDir string) error {
	cfg := &readline.Config{
		Prompt:          prompt,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stderr:          os.Stderr,
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
