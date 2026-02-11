package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yagi-agent/yagi/engine"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type ChatRequest struct {
	Messages []openai.ChatCompletionMessage `json:"messages"`
	Stream   bool                           `json:"stream"`
	Model    string                         `json:"model,omitempty"`
}

type ChatResponse struct {
	Content string `json:"content,omitempty"`
	Done    bool   `json:"done,omitempty"`
	Error   string `json:"error,omitempty"`
}

func runSTDIOMode() error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			writeError("Invalid JSON: " + err.Error())
			continue
		}

		// Detect format
		if _, hasJSONRPC := raw["jsonrpc"]; hasJSONRPC {
			handleJSONRPC(line)
		} else {
			handleLineDelimited(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func handleJSONRPC(line string) {
	var req JSONRPCRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		writeJSONRPCError(nil, "Parse error", err.Error())
		return
	}

	if req.Method != "chat" {
		writeJSONRPCError(req.ID, "Method not found", fmt.Sprintf("Unknown method: %s", req.Method))
		return
	}

	var chatReq ChatRequest
	if err := json.Unmarshal(req.Params, &chatReq); err != nil {
		writeJSONRPCError(req.ID, "Invalid params", err.Error())
		return
	}

	if chatReq.Stream {
		if err := streamChat(chatReq.Messages, func(content string) {
			writeJSONRPCResult(req.ID, ChatResponse{Content: content})
		}); err != nil {
			writeJSONRPCError(req.ID, "Chat error", err.Error())
			return
		}
		writeJSONRPCResult(req.ID, ChatResponse{Done: true})
	} else {
		result, err := completeChat(chatReq.Messages)
		if err != nil {
			writeJSONRPCError(req.ID, "Chat error", err.Error())
			return
		}
		writeJSONRPCResult(req.ID, ChatResponse{Content: result, Done: true})
	}
}

func handleLineDelimited(line string) {
	var chatReq ChatRequest
	if err := json.Unmarshal([]byte(line), &chatReq); err != nil {
		writeLine(ChatResponse{Error: "Invalid request: " + err.Error()})
		return
	}

	if chatReq.Stream {
		if err := streamChat(chatReq.Messages, func(content string) {
			writeLine(ChatResponse{Content: content})
		}); err != nil {
			writeLine(ChatResponse{Error: err.Error()})
			return
		}
		writeLine(ChatResponse{Done: true})
	} else {
		result, err := completeChat(chatReq.Messages)
		if err != nil {
			writeLine(ChatResponse{Error: err.Error()})
			return
		}
		writeLine(ChatResponse{Content: result, Done: true})
	}
}

func streamChat(messages []openai.ChatCompletionMessage, onChunk func(string)) error {
	ctx := context.Background()
	opts := engine.ChatOptions{
		OnContent: func(text string) {
			onChunk(text)
		},
	}
	_, _, err := eng.Chat(ctx, messages, opts)
	return err
}

func completeChat(messages []openai.ChatCompletionMessage) (string, error) {
	ctx := context.Background()
	var fullContent strings.Builder
	opts := engine.ChatOptions{
		OnContent: func(text string) {
			fullContent.WriteString(text)
		},
	}
	_, _, err := eng.Chat(ctx, messages, opts)
	if err != nil {
		return "", err
	}
	return fullContent.String(), nil
}

func writeJSONRPCResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func writeJSONRPCError(id interface{}, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"message": message,
			"data":    data,
		},
	}
	respData, _ := json.Marshal(resp)
	fmt.Println(string(respData))
}

func writeLine(data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Println(string(jsonData))
}

func writeError(message string) {
	writeLine(ChatResponse{Error: message})
}
