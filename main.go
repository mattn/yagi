package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

var (
	selectedProvider *Provider
	tools            []openai.Tool
	toolFuncs        = map[string]func(string) string{}
	quiet            bool
)

func registerTool(name, description string, parameters json.RawMessage, fn func(string) string) {
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
		return fn(arguments)
	}
	return fmt.Sprintf("Unknown tool: %s", name)
}

func chat(client *openai.Client, messages []openai.ChatCompletionMessage) (string, []openai.ToolCall, error) {
	stream, err := client.CreateChatCompletionStream(
		context.Background(),
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
			fmt.Print(content)
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

func main() {
	var (
		modelFlag  string
		apiKeyFlag string
		listFlag   bool
	)
	flag.StringVar(&modelFlag, "model", "openai", "Provider name or provider/model")
	flag.StringVar(&apiKeyFlag, "key", "", "API key (overrides environment variable)")
	flag.BoolVar(&listFlag, "list", false, "List available providers and models")
	flag.BoolVar(&quiet, "quiet", false, "Suppress informational messages")
	flag.BoolVar(&skipApproval, "yes", false, "Skip plugin approval prompts (use with caution)")
	flag.Parse()

	if u, err := user.Current(); err == nil {
		configDir := filepath.Join(u.HomeDir, ".config", "yagi")
		if err := loadPlugins(filepath.Join(configDir, "tools"), configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load plugins: %v\n", err)
		}
		if err := loadMCPConfig(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load MCP config: %v\n", err)
		}
		defer closeMCPConnections()
	}

	if listFlag {
		fmt.Println("Available providers:")
		for _, p := range providers {
			fmt.Printf("  %-12s model=%-30s env=%s\n", p.Name, p.Model, p.EnvKey)
		}
		return
	}

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
	client := openai.NewClientWithConfig(config)

	messages := []openai.ChatCompletionMessage{}

	oneshot := ""
	if args := flag.Args(); len(args) > 0 {
		oneshot = strings.Join(args, " ")
	} else if fi, _ := os.Stdin.Stat(); fi.Mode()&os.ModeCharDevice == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		oneshot = strings.TrimSpace(string(b))
	}

	if oneshot != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: oneshot,
		})
		runChat(client, messages)
		fmt.Println()
		return
	}

	reader := bufio.NewReader(os.Stdin)

	if !quiet {
		fmt.Fprintf(os.Stderr, "%s Chat [%s] (type 'exit' to quit)\n", selectedProvider.Name, selectedProvider.Model)
		fmt.Fprintln(os.Stderr)
	}

	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "exit" {
			break
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: input,
		})

		runChat(client, messages)
		fmt.Println()
	}
}

func runChat(client *openai.Client, messages []openai.ChatCompletionMessage) {
	for {
		content, toolCalls, err := chat(client, messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			break
		}

		if len(toolCalls) > 0 {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})

			for _, tc := range toolCalls {
				if !quiet {
				fmt.Fprintf(os.Stderr, "[tool: %s(%s)]\n", tc.Function.Name, tc.Function.Arguments)
			}
				output := executeTool(tc.Function.Name, tc.Function.Arguments)
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    output,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		})
		break
	}
}
