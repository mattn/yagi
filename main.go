package main

import (
	"context"
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

	openai "github.com/sashabaranov/go-openai"
)

var (
	selectedProvider *Provider
	tools            []openai.Tool
	toolFuncs        = map[string]func(string) (string, error){}
	quiet            bool
	verbose          bool

	chatMu     sync.Mutex
	chatCancel context.CancelFunc
)

func registerTool(name, description string, parameters json.RawMessage, fn func(string) (string, error)) {
	var params openai.FunctionDefinition
	params.Name = name
	params.Description = description
	params.Parameters = parameters

	tools = append(tools, openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &params,
	})
	toolFuncs[name] = fn
}

func executeTool(name, arguments string) string {
	if fn, ok := toolFuncs[name]; ok {
		if !skipApproval && pluginApprovals != nil {
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
		result, err := fn(arguments)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return result
	}
	return fmt.Sprintf("Unknown tool: %s", name)
}

func processStreamResponse(stream *openai.ChatCompletionStream) (string, []openai.ToolCall, error) {
	var fullContent strings.Builder
	toolCallsMap := make(map[int]*openai.ToolCall)
	var finishReason openai.FinishReason

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

		if content := choice.Delta.Content; content != "" {
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

func chat(ctx context.Context, client *openai.Client, messages []openai.ChatCompletionMessage, skill string) (string, []openai.ToolCall, error) {
	systemMsg := getSystemMessage(skill)
	if systemMsg != "" && (len(messages) == 0 || messages[0].Role != openai.ChatMessageRoleSystem) {
		systemMsgObj := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMsg,
		}
		messages = append([]openai.ChatCompletionMessage{systemMsgObj}, messages...)
	}

	stream, err := client.CreateChatCompletionStream(
		ctx,
		openai.ChatCompletionRequest{
			Model:    selectedProvider.Model,
			Messages: messages,
			Tools:    tools,
		},
	)
	if err != nil {
		return "", nil, err
	}
	defer stream.Close()

	return processStreamResponse(stream)
}

const name = "yagi"

const version = "0.0.18"

var revision = "HEAD"

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
		defaultModel = "openai"
	}

	flag.StringVar(&f.modelFlag, "model", defaultModel, "Provider name or provider/model")
	flag.StringVar(&f.apiKeyFlag, "key", "", "API key (overrides environment variable)")
	flag.BoolVar(&f.listFlag, "list", false, "List available providers and models")
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
	return configDir
}

func setupProvider(modelFlag, apiKeyFlag string) *openai.Client {
	providerName, modelName, _ := strings.Cut(modelFlag, "/")
	selectedProvider = findProvider(providerName)
	if selectedProvider == nil {
		fmt.Fprintf(os.Stderr, "Unknown provider: %s\nAvailable providers:", providerName)
		for _, p := range providers {
			fmt.Fprintf(os.Stderr, " %s", p.Name)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	if modelName != "" {
		selectedProvider.Model = modelName
	}

	apiKey := apiKeyFlag
	if apiKey == "" {
		apiKey = os.Getenv(selectedProvider.EnvKey)
	}
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "%s environment variable or -key flag is required\n", selectedProvider.EnvKey)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "%s Chat [%s] (type 'exit' to quit)\n", selectedProvider.Name, selectedProvider.Model)
		fmt.Fprintln(os.Stderr)
	}

	workDir, _ := os.Getwd()

	var history []string
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

	restoreTerminal, err := enableRawMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to enable raw mode: %v\n", err)
	} else {
		defer restoreTerminal()
	}

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
			input, err := readline(appConfig.Prompt+" ", history)
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
			_ = err
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

		history = append(history, input)

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

	if f.listFlag {
		fmt.Println("Available providers:")
		for _, p := range providers {
			fmt.Printf("  %-12s model=%-30s env=%s\n", p.Name, p.Model, p.EnvKey)
		}
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

	for {
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

			for _, tc := range toolCalls {
				if !quiet {
					fmt.Fprintf(os.Stderr, "[tool: %s(%s)]\n", tc.Function.Name, tc.Function.Arguments)
				}
				output := executeTool(tc.Function.Name, tc.Function.Arguments)
				*messages = append(*messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    output,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		*messages = append(*messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		})
		break
	}
}
