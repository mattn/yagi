package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	openai "github.com/sashabaranov/go-openai"
)

type ToolFunc func(ctx context.Context, args string) (string, error)

type ToolApprover interface {
	Approve(ctx context.Context, toolName, args string) (bool, error)
}

type toolMetadata struct {
	safe bool
}

type Config struct {
	Client *openai.Client
	Model  string

	SystemMessage func(skill string) string
	Approver      ToolApprover

	MaxRetries        int
	MaxAutonomousIter int
	CompressThreshold int
	MaxContextChars   int
}

type ChatOptions struct {
	Skill        string
	Autonomous   bool
	OnContent    func(text string)
	OnReasoning  func(text string)
	OnToolCall   func(name, arguments string)
	OnToolResult func(name, result string)
	OnToolError  func(name, errMsg string)
	OnCompressed func(oldChars int)
}

type Engine struct {
	client *openai.Client
	model  string

	tools     []openai.Tool
	toolFuncs map[string]ToolFunc
	toolMeta  map[string]toolMetadata
	toolAlts  map[string][]string

	systemMessage func(skill string) string
	approver      ToolApprover

	maxRetries        int
	maxAutonomousIter int
	compressThreshold int
	maxContextChars   int

	mu sync.Mutex
}

func New(cfg Config) *Engine {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	maxAuto := cfg.MaxAutonomousIter
	if maxAuto <= 0 {
		maxAuto = 20
	}
	compressThreshold := cfg.CompressThreshold
	if compressThreshold <= 0 {
		compressThreshold = 80000
	}
	maxContextChars := cfg.MaxContextChars
	if maxContextChars <= 0 {
		maxContextChars = 100000
	}

	return &Engine{
		client:    cfg.Client,
		model:     cfg.Model,
		tools:     nil,
		toolFuncs: make(map[string]ToolFunc),
		toolMeta:  make(map[string]toolMetadata),
		toolAlts: map[string][]string{
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
		},
		systemMessage:     cfg.SystemMessage,
		approver:          cfg.Approver,
		maxRetries:        maxRetries,
		maxAutonomousIter: maxAuto,
		compressThreshold: compressThreshold,
		maxContextChars:   maxContextChars,
	}
}

func (e *Engine) RegisterTool(name, description string, parameters json.RawMessage, fn ToolFunc, safe bool) {
	var params openai.FunctionDefinition
	params.Name = name
	params.Description = description
	params.Parameters = parameters

	e.tools = append(e.tools, openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &params,
	})
	e.toolFuncs[name] = fn
	e.toolMeta[name] = toolMetadata{safe: safe}
}

func (e *Engine) Client() *openai.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.client
}

func (e *Engine) SetClient(client *openai.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *Engine) SetModel(model string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.model = model
}

func (e *Engine) Model() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.model
}

func (e *Engine) Tools() []openai.Tool {
	return e.tools
}

func (e *Engine) HasTool(name string) bool {
	_, ok := e.toolFuncs[name]
	return ok
}

func (e *Engine) ExecuteTool(ctx context.Context, name, arguments string) string {
	result, _ := e.executeTool(ctx, name, arguments)
	return result
}

func (e *Engine) suggestAlternatives(name string) string {
	alts, ok := e.toolAlts[name]
	if !ok {
		return ""
	}
	var available []string
	for _, alt := range alts {
		if _, exists := e.toolFuncs[alt]; exists {
			available = append(available, alt)
		}
	}
	if len(available) == 0 {
		return ""
	}
	return fmt.Sprintf(" (alternatives: %s)", strings.Join(available, ", "))
}

func (e *Engine) executeTool(ctx context.Context, name, arguments string) (string, bool) {
	fn, ok := e.toolFuncs[name]
	if !ok {
		return fmt.Sprintf("Unknown tool: %s", name), true
	}

	meta := e.toolMeta[name]
	if !meta.safe && e.approver != nil {
		approved, err := e.approver.Approve(ctx, name, arguments)
		if err != nil {
			return fmt.Sprintf("Error: approval failed: %v", err), true
		}
		if !approved {
			return "Error: Tool not approved by user", true
		}
	}

	result, err := fn(ctx, arguments)
	if err != nil {
		return fmt.Sprintf("Error: %v%s", err, e.suggestAlternatives(name)), true
	}
	return result, false
}

type toolResult struct {
	id      string
	output  string
	isError bool
}

func (e *Engine) executeToolsConcurrently(ctx context.Context, toolCalls []openai.ToolCall) ([]openai.ChatCompletionMessage, []toolResult) {
	results := make([]toolResult, len(toolCalls))
	var wg sync.WaitGroup
	for i, tc := range toolCalls {
		wg.Add(1)
		go func(i int, tc openai.ToolCall) {
			defer wg.Done()
			output, isErr := e.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			results[i] = toolResult{
				id:      tc.ID,
				output:  output,
				isError: isErr,
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
	return msgs, results
}

func (e *Engine) processStreamResponse(stream *openai.ChatCompletionStream, opts ChatOptions) (string, []openai.ToolCall, error) {
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

		if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
			if opts.OnReasoning != nil {
				opts.OnReasoning(reasoning)
			}
		}

		if content := choice.Delta.Content; content != "" {
			if opts.OnContent != nil {
				opts.OnContent(content)
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

func (e *Engine) chat(ctx context.Context, messages []openai.ChatCompletionMessage, opts ChatOptions) (string, []openai.ToolCall, error) {
	systemMsg := ""
	if e.systemMessage != nil {
		systemMsg = e.systemMessage(opts.Skill)
	}
	if systemMsg != "" && (len(messages) == 0 || messages[0].Role != openai.ChatMessageRoleSystem) {
		systemMsgObj := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMsg,
		}
		messages = append([]openai.ChatCompletionMessage{systemMsgObj}, messages...)
	}

	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return "", nil, lastErr
			}
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return "", nil, lastErr
			}
		}

		e.mu.Lock()
		currentModel := e.model
		e.mu.Unlock()

		stream, err := e.client.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:    currentModel,
				Messages: messages,
				Tools:    e.tools,
			},
		)
		if err != nil {
			lastErr = err
			continue
		}

		content, toolCalls, err := e.processStreamResponse(stream, opts)
		stream.Close()
		if err != nil {
			lastErr = err
			continue
		}

		return content, toolCalls, nil
	}

	return "", nil, fmt.Errorf("failed after %d retries: %w", e.maxRetries, lastErr)
}

// Chat runs the full chat loop: sends messages, executes tool calls, and returns when the assistant
// produces a final text response or the iteration limit is reached.
func (e *Engine) Chat(ctx context.Context, messages []openai.ChatCompletionMessage, opts ChatOptions) (string, []openai.ChatCompletionMessage, error) {
	iteration := 0

	for {
		iteration++
		if opts.Autonomous && iteration > e.maxAutonomousIter {
			break
		}

		messages = e.compressContext(ctx, messages, opts)
		content, toolCalls, err := e.chat(ctx, messages, opts)
		if err != nil {
			return "", messages, err
		}

		if len(toolCalls) > 0 {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})

			if opts.OnToolCall != nil {
				for _, tc := range toolCalls {
					opts.OnToolCall(tc.Function.Name, tc.Function.Arguments)
				}
			}

			toolMsgs, toolResults := e.executeToolsConcurrently(ctx, toolCalls)
			for i, msg := range toolMsgs {
				if opts.OnToolError != nil && toolResults[i].isError {
					opts.OnToolError(msg.ToolCallID, msg.Content)
				}
				if opts.OnToolResult != nil && !toolResults[i].isError {
					opts.OnToolResult(toolCalls[i].Function.Name, msg.Content)
				}
			}
			messages = append(messages, toolMsgs...)
			continue
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		})
		return content, messages, nil
	}

	return "", messages, nil
}

func (e *Engine) estimateChars(msgs []openai.ChatCompletionMessage) int {
	total := 0
	for _, m := range msgs {
		total += utf8.RuneCountInString(m.Content)
		for _, tc := range m.ToolCalls {
			total += utf8.RuneCountInString(tc.Function.Arguments)
		}
	}
	return total
}

func (e *Engine) compressContext(ctx context.Context, messages []openai.ChatCompletionMessage, opts ChatOptions) []openai.ChatCompletionMessage {
	chars := e.estimateChars(messages)
	if chars < e.compressThreshold {
		return messages
	}

	start := 0
	for i, m := range messages {
		if m.Role == openai.ChatMessageRoleSystem {
			start = i + 1
			break
		}
	}
	if start >= len(messages) {
		return messages
	}

	end := start
	kept := e.estimateChars(messages[start:])
	for end < len(messages)-2 && kept > e.maxContextChars/2 {
		kept -= utf8.RuneCountInString(messages[end].Content)
		for _, tc := range messages[end].ToolCalls {
			kept -= utf8.RuneCountInString(tc.Function.Arguments)
		}
		end++
	}

	if end <= start {
		return messages
	}

	for end < len(messages) && messages[end].Role != openai.ChatMessageRoleUser {
		end++
	}
	if end >= len(messages) {
		return messages
	}

	oldMsgs := messages[start:end]
	summary := e.summarizeMessages(ctx, oldMsgs)
	if summary == "" {
		return messages
	}

	if opts.OnCompressed != nil {
		opts.OnCompressed(chars)
	}

	var result []openai.ChatCompletionMessage
	result = append(result, messages[:start]...)
	result = append(result, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: "[Previous conversation summary]\n" + summary,
	})
	result = append(result, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: "Understood. I have the context from our previous conversation.",
	})
	result = append(result, messages[end:]...)
	return result
}

func (e *Engine) summarizeMessages(ctx context.Context, msgs []openai.ChatCompletionMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case openai.ChatMessageRoleUser:
			sb.WriteString("User: ")
			sb.WriteString(m.Content)
			sb.WriteString("\n")
		case openai.ChatMessageRoleAssistant:
			sb.WriteString("Assistant: ")
			if m.Content != "" {
				sb.WriteString(m.Content)
			}
			for _, tc := range m.ToolCalls {
				sb.WriteString("[tool: ")
				sb.WriteString(tc.Function.Name)
				sb.WriteString("]")
			}
			sb.WriteString("\n")
		case openai.ChatMessageRoleTool:
			content := m.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			sb.WriteString("Tool result: ")
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}

	summaryMsgs := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "Summarize the following conversation concisely. Preserve key decisions, file paths, code changes, and important context. Write in the same language as the conversation. Keep it under 500 characters.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: sb.String(),
		},
	}

	e.mu.Lock()
	currentModel := e.model
	e.mu.Unlock()

	stream, err := e.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    currentModel,
		Messages: summaryMsgs,
	})
	if err != nil {
		return ""
	}
	defer stream.Close()

	var result strings.Builder
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(resp.Choices) > 0 {
			result.WriteString(resp.Choices[0].Delta.Content)
		}
	}
	return result.String()
}
