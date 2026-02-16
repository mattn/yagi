package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

var rl *readline.Instance
var mux *inputMux

func modelCompleter() []string {
	models := make([]string, len(modelList))
	for i, m := range modelList {
		models[i] = m.Name
	}
	return models
}

func initReadline(prompt, configDir string) error {
	models := modelCompleter()
	var modelItems []readline.PrefixCompleterInterface
	for _, m := range models {
		modelItems = append(modelItems, readline.PcItem(m))
	}

	mux = newInputMux(readline.Stdin)

	cfg := &readline.Config{
		Prompt:                 prompt,
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",
		Stderr:                 os.Stderr,
		Stdin:                  mux,
		DisableAutoSaveHistory: true,
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("/help"),
			readline.PcItem("/model", modelItems...),
			readline.PcItem("/clear"),
			readline.PcItem("/memory"),
			readline.PcItem("/revoke"),
			readline.PcItem("/agent"),
			readline.PcItem("/plan"),
			readline.PcItem("/mode"),
		),
	}
	if configDir != "" {
		cfg.HistoryFile = filepath.Join(configDir, "history")
	}
	var err error
	rl, err = readline.NewEx(cfg)
	if err != nil {
		return err
	}
	enableBracketPaste()
	return nil
}

func closeReadline() {
	if rl != nil {
		disableBracketPaste()
		rl.Close()
	}
}

func readlineInput(prompt string) (string, error) {
	var b strings.Builder
	first := true
	rl.SetPrompt(prompt)

	for {
		line, err := rl.Readline()
		if err != nil {
			return "", err
		}

		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		first = false

		soft := mux.popEnterSoft()
		if !soft {
			full := b.String()
			if strings.TrimSpace(full) != "" {
				rl.SaveHistory(full)
			}
			return full, nil
		}

		rl.SetPrompt("... ")
	}
}

func isInterrupt(err error) bool {
	return err == readline.ErrInterrupt
}

func setReadlineBuffer(text string) {
	if rl != nil {
		rl.WriteStdin([]byte(text))
	}
}
