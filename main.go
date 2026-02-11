package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-colorable"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yagi-agent/yagi/engine"
)

//go:embed models.txt
var modelsTxt string

var (
	selectedProvider *Provider
	model            string
	quiet            bool
	verbose          bool

	chatMu     sync.Mutex
	chatCancel context.CancelFunc
	stderr     = colorable.NewColorableStderr()

	autonomousMode bool
	planningMode   bool

	eng *engine.Engine
)

const name = "yagi"

const version = "0.0.34"

var revision = "HEAD"

func setupBuiltInTools() {
	eng.RegisterTool("get_yagi_info", "Get information about yagi", json.RawMessage(`{
		"type": "object",
		"properties": {
			"info_type": {
				"type": "string",
				"enum": ["version", "model"],
				"description": "What information to get: 'version' or 'model'"
			}
		},
		"required": ["info_type"]
	}`), func(ctx context.Context, args string) (string, error) {
		var req struct {
			InfoType string `json:"info_type"`
		}
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", err
		}
		switch req.InfoType {
		case "version":
			return fmt.Sprintf("yagi version %s (revision: %s/%s)", version, revision, runtime.Version()), nil
		case "model":
			if selectedProvider != nil {
				return fmt.Sprintf("%s/%s", selectedProvider.Name, eng.Model()), nil
			}
			return eng.Model(), nil
		default:
			return "", fmt.Errorf("unknown info_type: %s", req.InfoType)
		}
	}, true)

	eng.RegisterTool("saveMemoryEntry", "Save information to memory. Use this when user wants to remember something.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "A short identifier for what to remember (e.g., 'user_name', 'favorite_language', 'agent_language')"
			},
			"value": {
				"type": "string",
				"description": "The information to remember"
			}
		},
		"required": ["key", "value"]
	}`), func(ctx context.Context, args string) (string, error) {
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", err
		}
		return saveMemoryEntry(ctx, req.Key, req.Value)
	}, true)

	eng.RegisterTool("getMemoryEntry", "Retrieve information from memory.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "The identifier of the information to recall"
			}
		},
		"required": ["key"]
	}`), func(ctx context.Context, args string) (string, error) {
		var req struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", err
		}
		return getMemoryEntry(ctx, req.Key)
	}, true)

	eng.RegisterTool("deleteMemoryEntry", "Delete information from memory.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "The identifier of the information to forget"
			}
		},
		"required": ["key"]
	}`), func(ctx context.Context, args string) (string, error) {
		var req struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", err
		}
		return deleteMemoryEntry(ctx, req.Key)
	}, true)

	eng.RegisterTool("listMemoryEntries", "List all saved information.", json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`), func(ctx context.Context, args string) (string, error) {
		return listMemoryEntries(ctx)
	}, true)
}

type parsedFlags struct {
	modelFlag   string
	apiKeyFlag  string
	listFlag    bool
	showVersion bool
	stdioMode   bool
	skillFlag   string
	resumeFlag  bool
}

func parseFlags() parsedFlags {
	var f parsedFlags

	defaultModel := os.Getenv("YAGI_MODEL")
	if defaultModel == "" {
		defaultModel = "openai/gpt-4.1-nano"
	}

	flag.StringVar(&f.modelFlag, "model", defaultModel, "Provider/model (e.g. google/gemini-2.5-pro)")
	flag.StringVar(&f.apiKeyFlag, "key", "", "API key (overrides environment variable)")
	flag.BoolVar(&f.listFlag, "list", false, "List available providers")
	flag.BoolVar(&quiet, "quiet", false, "Suppress informational messages")
	flag.BoolVar(&verbose, "verbose", false, "Show verbose output including plugin loading")
	flag.BoolVar(&skipApproval, "yes", false, "Skip plugin approval prompts (use with caution)")
	flag.BoolVar(&f.showVersion, "v", false, "Show version")
	flag.BoolVar(&f.stdioMode, "stdio", false, "Run in STDIO mode for editor integration")
	flag.StringVar(&f.skillFlag, "skill", "", "Use a specific skill (e.g., 'explain', 'refactor', 'debug')")
	flag.BoolVar(&f.resumeFlag, "resume", false, "Resume previous session for the current directory")
	flag.Parse()

	return f
}

func loadConfigurations() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		u, err := user.Current()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(u.HomeDir, ".config")
	}
	configDir = filepath.Join(configDir, "yagi")
	if err := loadConfig(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
	}
	if err := loadIdentity(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load identity: %v\n", err)
	}
	if err := loadSkills(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load skills: %v\n", err)
	}
	if err := loadMemory(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load memory: %v\n", err)
	}
	if err := loadPlugins(filepath.Join(configDir, "tools"), configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load plugins: %v\n", err)
	}
	if err := loadMCPConfig(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load MCP config: %v\n", err)
	}
	if err := loadExtraProviders(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load extra providers: %v\n", err)
	}
	return configDir
}

func setupProvider(modelFlag, apiKeyFlag string) *openai.Client {
	providerName, modelName, ok := strings.Cut(modelFlag, "/")
	if !ok {
		fmt.Fprintf(os.Stderr, "Invalid model format: %s\nUse provider/model format (e.g. google/gemini-2.5-pro)\nRun with -list to see available providers.\n", modelFlag)
		os.Exit(1)
	}
	selectedProvider = findProvider(providerName)
	if selectedProvider == nil {
		fmt.Fprintf(os.Stderr, "Unknown provider: %s\nRun with -list to see available providers.\n", providerName)
		os.Exit(1)
	}

	model = modelName

	apiKey := apiKeyFlag
	if apiKey == "" && selectedProvider.EnvKey != "" {
		apiKey = os.Getenv(selectedProvider.EnvKey)
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "%s environment variable or -key flag is required\n", selectedProvider.EnvKey)
			os.Exit(1)
		}
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = selectedProvider.APIURL
	client := openai.NewClientWithConfig(config)
	eng.SetClient(client)
	eng.SetModel(model)
	return client
}

func readOneshotInput() string {
	if args := flag.Args(); len(args) > 0 {
		return strings.Join(args, " ")
	}
	if fi, _ := os.Stdin.Stat(); fi.Mode()&os.ModeCharDevice == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		return strings.TrimSpace(string(b))
	}
	return ""
}

func runInteractiveLoop(client *openai.Client, skillFlag, configDir string, resume bool) {
	if !quiet {
		fmt.Fprintf(os.Stderr, "Chat [%s/%s] (type 'exit' to quit)\n", selectedProvider.Name, model)
		fmt.Fprintln(os.Stderr)
	}

	workDir, _ := os.Getwd()

	var messages []openai.ChatCompletionMessage

	if resume && configDir != "" && workDir != "" {
		restored, err := loadSession(configDir, workDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load session: %v\n", err)
		} else if len(restored) > 0 {
			messages = restored
			if !quiet {
				fmt.Fprintf(os.Stderr, "[resumed %d messages from previous session]\n\n", len(restored))
			}
		}
	}

	if err := initReadline(appConfig.Prompt+" ", configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to init readline: %v\n", err)
	}
	defer closeReadline()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	quitCh := make(chan struct{})
	var lastInterrupt time.Time

	go func() {
		for range sigCh {
			now := time.Now()
			chatMu.Lock()
			cancel := chatCancel
			chatMu.Unlock()

			if cancel != nil {
				cancel()
			}
			if now.Sub(lastInterrupt) < 500*time.Millisecond {
				fmt.Fprintln(os.Stderr)
				close(quitCh)
				return
			}
			lastInterrupt = now
		}
	}()

	for {
		inputCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			input, err := readlineInput(appConfig.Prompt + " ")
			if err != nil {
				errCh <- err
			} else {
				inputCh <- input
			}
		}()

		var input string
		select {
		case <-quitCh:
			return
		case err := <-errCh:
			if isInterrupt(err) {
				now := time.Now()
				if now.Sub(lastInterrupt) < 500*time.Millisecond {
					return
				}
				lastInterrupt = now
				continue
			}
			return
		case input = <-inputCh:
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "exit" {
			break
		}

		if strings.HasPrefix(input, "/") {
			handleSlashCommand(input, &client, configDir, &messages, skillFlag)
			continue
		}

		// Planning mode: ask AI to create a plan first
		if planningMode {
			plan, err := generatePlan(input, skillFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating plan: %v\n", err)
				continue
			}
			fmt.Fprintln(stderr, "\n[Plan]")
			fmt.Fprintln(stderr, plan)

			response, err := readFromTTY("\nExecute this plan? [y/yes/ok or n/no]: ")
			if err != nil {
				fmt.Fprintf(stderr, "Error reading response: %v\n", err)
				continue
			}
			response = strings.TrimSpace(strings.ToLower(response))
			confirmed := response == "y" || response == "yes" || response == "ok"

			if !confirmed {
				fmt.Fprintln(stderr, "Plan cancelled.")
				continue
			}
			fmt.Fprintln(stderr, "Executing plan...")
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: input,
		})

		runChat(&messages, skillFlag)
		fmt.Println()

		if configDir != "" && workDir != "" {
			if err := saveSession(configDir, workDir, messages); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
			}
		}

		select {
		case <-quitCh:
			return
		default:
		}
	}
}

func generatePlan(userInput, skill string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	planPrompt := fmt.Sprintf(`The user wants to accomplish the following task:
"%s"

Please create a step-by-step execution plan for this task. List the specific tools you will use and in what order. Be concise but specific.

Format your response as:
1. [Step 1 description] - using [tool name]
2. [Step 2 description] - using [tool name]
...`, userInput)

	systemMsg := getSystemMessage(skill)
	messages := []openai.ChatCompletionMessage{}

	if systemMsg != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMsg,
		})
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: planPrompt,
	})

	stream, err := eng.Client().CreateChatCompletionStream(
		ctx,
		openai.ChatCompletionRequest{
			Model:    eng.Model(),
			Messages: messages,
			Tools:    eng.Tools(),
		},
	)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var plan strings.Builder
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if len(resp.Choices) > 0 && resp.Choices[0].Delta.Content != "" {
			plan.WriteString(resp.Choices[0].Delta.Content)
		}
	}

	return plan.String(), nil
}

func handleSlashCommand(input string, client **openai.Client, configDir string, messages *[]openai.ChatCompletionMessage, skill string) {
	var prevProvider *Provider
	var prevModel string
	if selectedProvider != nil {
		prevProvider = &Provider{
			Name:   selectedProvider.Name,
			APIURL: selectedProvider.APIURL,
			EnvKey: selectedProvider.EnvKey,
		}
		prevModel = model
	}

	parts := strings.Fields(input)
	cmd := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	switch cmd {
	case "/help":
		fmt.Println("Available commands:")
		fmt.Println("  /model [name]   - Show/change model (e.g., /model openai/gpt-4o)")
		fmt.Println("  /agent [on|off] - Toggle autonomous mode (auto-execute tools without approval)")
		fmt.Println("  /plan [on|off]  - Toggle planning mode (show execution plan before acting)")
		fmt.Println("  /mode           - Show current mode settings")
		fmt.Println("  /edit           - Open $EDITOR to compose a message")
		fmt.Println("  /clear          - Clear conversation history")
		fmt.Println("  /revoke [name]  - Revoke plugin approval (use 'all' to revoke all)")
		fmt.Println("  /exit           - Exit yagi")
		fmt.Println("  /help           - Show this help")
		fmt.Println()
		fmt.Println("Tips:")
		fmt.Println("  - Use Tab for auto-completion")
		fmt.Println("  - Start with / to see slash commands")
		fmt.Println("  - Use -model flag to set model on startup")
		fmt.Println("  - Use -list to see available models")
	case "/model":
		if args == "" {
			if selectedProvider != nil {
				fmt.Printf("Current model: %s/%s\n", selectedProvider.Name, model)
			} else {
				fmt.Printf("Current model: %s\n", model)
			}
			return
		}
		providerName, modelName, ok := strings.Cut(args, "/")
		if !ok {
			fmt.Fprintf(os.Stderr, "Invalid model format. Use: provider/model\n")
			return
		}
		newProvider := findProvider(providerName)
		if newProvider == nil {
			fmt.Fprintf(os.Stderr, "Unknown provider: %s\n", providerName)
			return
		}
		selectedProvider = newProvider
		model = modelName
		var apiKey string
		if selectedProvider.EnvKey != "" {
			apiKey = os.Getenv(selectedProvider.EnvKey)
			if apiKey == "" {
				fmt.Fprintf(os.Stderr, "Error: %s is not set. Keeping previous model.\n", selectedProvider.EnvKey)
				selectedProvider = prevProvider
				model = prevModel
				return
			}
		}
		config := openai.DefaultConfig(apiKey)
		config.BaseURL = selectedProvider.APIURL
		newClient := openai.NewClientWithConfig(config)
		*client = newClient
		eng.SetClient(newClient)
		eng.SetModel(model)
		fmt.Printf("Model changed to: %s/%s\n", selectedProvider.Name, model)
	case "/clear":
		*messages = nil
		workDir, _ := os.Getwd()
		if configDir != "" && workDir != "" {
			clearSession(configDir, workDir)
		}
		fmt.Println("Conversation cleared.")
	case "/memory":
		result, err := listMemoryEntries(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Println("Saved memories:")
		fmt.Println(result)
	case "/revoke":
		if pluginApprovals == nil {
			fmt.Fprintf(os.Stderr, "No approval records loaded.\n")
			return
		}
		workDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		if args == "" {
			approved := listApprovedPlugins(pluginApprovals, workDir)
			if len(approved) == 0 {
				fmt.Println("No approved plugins for this directory.")
				return
			}
			fmt.Println("Approved plugins for this directory:")
			for _, name := range approved {
				fmt.Printf("  - %s\n", name)
			}
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  /revoke <name>  - Revoke a specific plugin")
			fmt.Println("  /revoke all     - Revoke all plugins")
			return
		}
		if args == "all" {
			count := removeAllPluginApprovals(pluginApprovals, workDir)
			if count == 0 {
				fmt.Println("No approved plugins for this directory.")
				return
			}
			if err := saveApprovalRecords(pluginConfigDir, pluginApprovals); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save approval: %v\n", err)
				return
			}
			fmt.Printf("Revoked %d plugin(s) for this directory.\n", count)
		} else {
			if !removePluginApproval(pluginApprovals, workDir, args) {
				fmt.Fprintf(os.Stderr, "Plugin %q is not approved for this directory.\n", args)
				return
			}
			if err := saveApprovalRecords(pluginConfigDir, pluginApprovals); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save approval: %v\n", err)
				return
			}
			fmt.Printf("Revoked approval for plugin %q.\n", args)
		}
	case "/agent":
		if args == "" {
			if autonomousMode {
				fmt.Println("Autonomous mode: ON (tools will be executed automatically)")
			} else {
				fmt.Println("Autonomous mode: OFF (approval required for tools)")
			}
			return
		}
		switch strings.ToLower(args) {
		case "on", "true", "1", "yes":
			autonomousMode = true
			skipApproval = true
			fmt.Println("Autonomous mode enabled. Tools will be executed automatically.")
		case "off", "false", "0", "no":
			autonomousMode = false
			skipApproval = false
			fmt.Println("Autonomous mode disabled. Tools require approval.")
		default:
			fmt.Fprintf(os.Stderr, "Usage: /agent [on|off]\n")
		}
	case "/plan":
		if args == "" {
			if planningMode {
				fmt.Println("Planning mode: ON (execution plan will be shown before acting)")
			} else {
				fmt.Println("Planning mode: OFF (immediate execution)")
			}
			return
		}
		switch strings.ToLower(args) {
		case "on", "true", "1", "yes":
			planningMode = true
			fmt.Println("Planning mode enabled. Execution plan will be shown before acting.")
		case "off", "false", "0", "no":
			planningMode = false
			fmt.Println("Planning mode disabled. Immediate execution.")
		default:
			fmt.Fprintf(os.Stderr, "Usage: /plan [on|off]\n")
		}
	case "/mode":
		fmt.Println("Current mode settings:")
		if autonomousMode {
			fmt.Println("  Autonomous mode: ON")
		} else {
			fmt.Println("  Autonomous mode: OFF")
		}
		if planningMode {
			fmt.Println("  Planning mode:   ON")
		} else {
			fmt.Println("  Planning mode:   OFF")
		}
	case "/edit":
		text, err := openEditor(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		text = strings.TrimSpace(text)
		if text == "" {
			fmt.Println("No input provided.")
			return
		}
		if text == strings.TrimSpace(args) {
			fmt.Println("No changes made.")
			return
		}
		setReadlineBuffer(text)
	}
}

func main() {
	f := parseFlags()

	if f.showVersion {
		fmt.Printf("%s %s (rev: %s/%s)\n", name, version, revision, runtime.Version())
		return
	}

	if f.stdioMode {
		quiet = true
		skipApproval = true
	}

	configDir := loadConfigurations()
	defer closeMCPConnections()

	eng = engine.New(engine.Config{
		SystemMessage: func(skill string) string {
			return getSystemMessage(skill)
		},
	})

	setupBuiltInTools()

	if f.listFlag {
		listModels(flag.Args())
		return
	}

	client := setupProvider(f.modelFlag, f.apiKeyFlag)

	if f.stdioMode {
		if err := runSTDIOMode(); err != nil {
			fmt.Fprintf(os.Stderr, "STDIO error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	oneshot := readOneshotInput()
	if oneshot != "" {
		messages := []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: oneshot,
			},
		}
		runChat(&messages, f.skillFlag)
		fmt.Println()
		return
	}

	runInteractiveLoop(client, f.skillFlag, configDir, f.resumeFlag)
}

func runChat(messages *[]openai.ChatCompletionMessage, skill string) {
	ctx, cancel := context.WithCancel(context.Background())
	chatMu.Lock()
	chatCancel = cancel
	chatMu.Unlock()
	defer func() {
		chatMu.Lock()
		chatCancel = nil
		chatMu.Unlock()
		cancel()
	}()

	inThinking := false
	opts := engine.ChatOptions{
		Skill:      skill,
		Autonomous: autonomousMode,
		OnContent: func(text string) {
			if !quiet {
				if inThinking {
					fmt.Fprint(stderr, "\x1b[2K\r")
					inThinking = false
				}
				fmt.Print(text)
			}
		},
		OnReasoning: func(text string) {
			if !quiet && !inThinking {
				fmt.Fprint(stderr, "\x1b[2K\x1b[36m[thinking]\x1b[0m ")
				inThinking = true
			}
		},
		OnToolCall: func(name, arguments string) {
			if !quiet {
				if autonomousMode {
					fmt.Fprintf(stderr, "\n\x1b[36m[autonomous] tool: %s(%s)\x1b[0m\n", name, arguments)
				} else {
					fmt.Fprintf(stderr, "\n\x1b[36m[tool: %s(%s)]\x1b[0m\n", name, arguments)
				}
			}
		},
		OnToolError: func(name, errMsg string) {
			if !quiet {
				fmt.Fprintf(stderr, "\x1b[31m[tool error: %s]\x1b[0m\n", errMsg)
			}
		},
		OnCompressed: func(oldChars int) {
			if !quiet {
				fmt.Fprintf(stderr, "\x1b[33m[context compressed: %d chars → summarized]\x1b[0m\n", oldChars)
			}
		},
	}

	_, updatedMsgs, err := eng.Chat(ctx, *messages, opts)
	if err != nil {
		if ctx.Err() != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "\n[interrupted]\n")
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	*messages = updatedMsgs
}

func listModels(args []string) {
	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}

	scanner := bufio.NewScanner(strings.NewReader(modelsTxt))
	for scanner.Scan() {
		line := scanner.Text()
		if filter == "" || strings.Contains(line, filter) {
			fmt.Println(line)
		}
	}
}
