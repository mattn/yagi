package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
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
)

//go:embed models.txt
var modelsTxt string

type toolMetadata struct {
	safe bool
}

var (
	selectedProvider *Provider
	model            string
	tools            []openai.Tool
	toolFuncs        = map[string]func(context.Context, string) (string, error){}
	toolMeta         = map[string]toolMetadata{}
	quiet            bool
	verbose          bool

	chatMu     sync.Mutex
	chatCancel context.CancelFunc
	stderr     = colorable.NewColorableStderr()

	// Run modes for autonomous and planning capabilities
	autonomousMode bool
	planningMode   bool
)

func registerTool(name, description string, parameters json.RawMessage, fn func(context.Context, string) (string, error), safe bool) {
	var params openai.FunctionDefinition
	params.Name = name
	params.Description = description
	params.Parameters = parameters

	tools = append(tools, openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &params,
	})
	toolFuncs[name] = fn
	toolMeta[name] = toolMetadata{safe: safe}
}

var toolAlternatives = map[string][]string{
	"web_search":   {"fetch_url"},
	"fetch_url":    {"web_search"},
	"read_file":    {"list_files", "glob", "search_files"},
	"edit_file":    {"write_file", "read_file"},
	"write_file":   {"edit_file"},
	"delete_file":  {"list_files"},
	"list_files":   {"glob", "search_files"},
	"glob":         {"list_files", "search_files"},
	"search_files": {"glob", "read_file"},
	"run_command":  {"read_file", "write_file"},
}

func suggestAlternatives(name string) string {
	alts, ok := toolAlternatives[name]
	if !ok {
		return ""
	}
	var available []string
	for _, alt := range alts {
		if _, exists := toolFuncs[alt]; exists {
			available = append(available, alt)
		}
	}
	if len(available) == 0 {
		return ""
	}
	return fmt.Sprintf(" (alternatives: %s)", strings.Join(available, ", "))
}

func executeTool(ctx context.Context, name, arguments string) string {
	if fn, ok := toolFuncs[name]; ok {
		meta, isMeta := toolMeta[name]
		if !skipApproval && isMeta && !meta.safe && pluginApprovals != nil {
			if !isPluginApproved(pluginApprovals, pluginWorkDir, name) {
				if !requestApproval(name, pluginWorkDir, arguments) {
					return "Error: Plugin not approved by user"
				}
				addPluginApproval(pluginApprovals, pluginWorkDir, name)
				if err := saveApprovalRecords(pluginConfigDir, pluginApprovals); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save approval: %v\n", err)
				}
			}
		}
		result, err := fn(ctx, arguments)
		if err != nil {
			return fmt.Sprintf("Error: %v%s", err, suggestAlternatives(name))
		}
		return result
	}
	return fmt.Sprintf("Unknown tool: %s", name)
}

type toolResult struct {
	id     string
	output string
}

func executeToolsConcurrently(ctx context.Context, toolCalls []openai.ToolCall) []openai.ChatCompletionMessage {
	results := make([]toolResult, len(toolCalls))
	var wg sync.WaitGroup
	for i, tc := range toolCalls {
		wg.Add(1)
		go func(i int, tc openai.ToolCall) {
			defer wg.Done()
			results[i] = toolResult{
				id:     tc.ID,
				output: executeTool(ctx, tc.Function.Name, tc.Function.Arguments),
			}
		}(i, tc)
	}
	wg.Wait()

	msgs := make([]openai.ChatCompletionMessage, len(results))
	for i, r := range results {
		msgs[i] = openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    r.output,
			ToolCallID: r.id,
		}
	}
	return msgs
}

func processStreamResponse(stream *openai.ChatCompletionStream) (string, []openai.ToolCall, error) {
	var fullContent strings.Builder
	toolCallsMap := make(map[int]*openai.ToolCall)
	var finishReason openai.FinishReason
	inThinking := false

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, err
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		finishReason = choice.FinishReason

		if reasoning := choice.Delta.ReasoningContent; reasoning != "" && !quiet {
			if !inThinking {
				fmt.Fprint(stderr, "\x1b[2K\x1b[36m[thinking]\x1b[0m ")
				inThinking = true
			}
		}

		if content := choice.Delta.Content; content != "" {
			if inThinking {
				fmt.Fprint(stderr, "\x1b[2K\r")
				inThinking = false
			}
			if !quiet {
				fmt.Print(content)
			}
			fullContent.WriteString(content)
		}

		for _, tc := range choice.Delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			existing, ok := toolCallsMap[idx]
			if !ok {
				existing = &openai.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
				}
				existing.Function.Name = tc.Function.Name
				toolCallsMap[idx] = existing
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name += tc.Function.Name
				}
			}
			existing.Function.Arguments += tc.Function.Arguments
		}
	}

	var toolCalls []openai.ToolCall
	if finishReason == openai.FinishReasonToolCalls && len(toolCallsMap) > 0 {
		toolCalls = make([]openai.ToolCall, 0, len(toolCallsMap))
		for i := 0; i < len(toolCallsMap); i++ {
			if tc, ok := toolCallsMap[i]; ok {
				toolCalls = append(toolCalls, *tc)
			}
		}
	}

	return fullContent.String(), toolCalls, nil
}

const maxRetries = 3

func chat(ctx context.Context, client *openai.Client, messages []openai.ChatCompletionMessage, skill string) (string, []openai.ToolCall, error) {
	systemMsg := getSystemMessage(skill)
	if systemMsg != "" && (len(messages) == 0 || messages[0].Role != openai.ChatMessageRoleSystem) {
		systemMsgObj := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMsg,
		}
		messages = append([]openai.ChatCompletionMessage{systemMsgObj}, messages...)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return "", nil, lastErr
			}
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			if !quiet {
				fmt.Fprintf(stderr, "\x1b[33m[retry %d/%d in %v]\x1b[0m\n", attempt, maxRetries, wait)
			}
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return "", nil, lastErr
			}
		}

		stream, err := client.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:    model,
				Messages: messages,
				Tools:    tools,
			},
		)
		if err != nil {
			lastErr = err
			continue
		}

		content, toolCalls, err := processStreamResponse(stream)
		stream.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return content, toolCalls, nil
	}
	return "", nil, lastErr
}

const name = "yagi"

const version = "0.0.30"

var revision = "HEAD"

func setupBuiltInTools() {
	registerTool("get_yagi_info", "Get information about yagi", json.RawMessage(`{
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
				return fmt.Sprintf("%s/%s", selectedProvider.Name, model), nil
			}
			return model, nil
		default:
			return "", fmt.Errorf("unknown info_type: %s", req.InfoType)
		}
	}, true)

	registerTool("saveMemoryEntry", "Save information to memory. Use this when user wants to remember something.", json.RawMessage(`{
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

	registerTool("getMemoryEntry", "Retrieve information from memory.", json.RawMessage(`{
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

	registerTool("deleteMemoryEntry", "Delete information from memory.", json.RawMessage(`{
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

	registerTool("listMemoryEntries", "List all saved information.", json.RawMessage(`{
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
	return openai.NewClientWithConfig(config)
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
			handleSlashCommand(input, &client, configDir, &messages)
			continue
		}

		// Planning mode: ask AI to create a plan first
		if planningMode {
			plan, err := generatePlan(client, input, skillFlag)
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

		runChat(client, &messages, skillFlag)
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

func generatePlan(client *openai.Client, userInput, skill string) (string, error) {
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

	stream, err := client.CreateChatCompletionStream(
		ctx,
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
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

func handleSlashCommand(input string, client **openai.Client, configDir string, messages *[]openai.ChatCompletionMessage) {
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
		*client = openai.NewClientWithConfig(config)
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

	setupBuiltInTools()

	if f.listFlag {
		listModels(flag.Args())
		return
	}

	client := setupProvider(f.modelFlag, f.apiKeyFlag)

	if f.stdioMode {
		if err := runSTDIOMode(client); err != nil {
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
		runChat(client, &messages, f.skillFlag)
		fmt.Println()
		return
	}

	runInteractiveLoop(client, f.skillFlag, configDir, f.resumeFlag)
}

func runChat(client *openai.Client, messages *[]openai.ChatCompletionMessage, skill string) {
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

	const maxAutonomousIterations = 20
	iteration := 0

	for {
		iteration++
		if autonomousMode && iteration > maxAutonomousIterations {
			if !quiet {
				fmt.Fprintf(stderr, "\n\x1b[33m[Reached maximum autonomous iterations (%d)]\x1b[0m\n", maxAutonomousIterations)
			}
			break
		}

		*messages = compressContext(ctx, client, *messages)
		content, toolCalls, err := chat(ctx, client, *messages, skill)
		if err != nil {
			if ctx.Err() != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "\n[interrupted]\n")
				}
				break
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			break
		}

		if len(toolCalls) > 0 {
			*messages = append(*messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})

			if !quiet {
				for _, tc := range toolCalls {
					if autonomousMode {
						fmt.Fprintf(stderr, "\n\x1b[36m[autonomous step %d] tool: %s(%s)\x1b[0m\n", iteration, tc.Function.Name, tc.Function.Arguments)
					} else {
						fmt.Fprintf(stderr, "\n\x1b[36m[tool: %s(%s)]\x1b[0m\n", tc.Function.Name, tc.Function.Arguments)
					}
				}
			}
			toolMsgs := executeToolsConcurrently(ctx, toolCalls)
			for _, msg := range toolMsgs {
				if !quiet && (strings.HasPrefix(msg.Content, "Error: ") || strings.HasPrefix(msg.Content, "Unknown tool: ")) {
					fmt.Fprintf(stderr, "\x1b[31m[tool error: %s]\x1b[0m\n", msg.Content)
				}
			}
			*messages = append(*messages, toolMsgs...)
			continue
		}

		*messages = append(*messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		})
		break
	}
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
