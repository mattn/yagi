# yagi

A CLI chat client for multiple LLM providers with a plugin system powered by [Yaegi](https://github.com/traefik/yaegi).

Tools are written as plain Go source files and loaded at runtime — no recompilation needed.

## Features

- **Multiple LLM Providers**: Support for OpenAI, Gemini, Groq, GLM, and more
- **Dynamic Plugin System**: Write tools in Go without recompilation
- **Persistent Memory**: AI can learn and remember information across conversations
- **Skills System**: Load specialized prompts for different tasks
- **Identity/Persona**: Customize AI behavior with IDENTITY.md
- **Session Resumption**: Resume previous conversations per directory with `-resume`
- **Interactive & One-shot Modes**: Use interactively or pipe commands

## Install

```
go install github.com/yagi-agent/yagi@latest
```

## Usage

```
yagi [options] [prompt]
```

| Flag | Description | Default |
|------|-------------|---------|
| `-model` | Provider name or `provider/model` | `glm` |
| `-key` | API key (overrides environment variable) | |
| `-quiet` | Suppress informational messages | |
| `-list` | List available providers and models | |
| `-resume` | Resume previous session for the current directory | |
| `-skill` | Use a specific skill (e.g., `explain`, `refactor`, `debug`) | |
| `-stdio` | Run in STDIO mode for editor integration | |

When run without arguments, yagi starts in interactive mode. Pass a prompt as arguments or via pipe for one-shot mode.

### Interactive Mode

```bash
# Start interactive chat with the default provider
yagi

# Start interactive chat with Gemini
yagi -model gemini

# Resume the previous session for the current directory
yagi -resume
```

### One-shot Mode

```bash
# Pass a prompt as arguments
yagi "こんにちは"

# Pipe input as a prompt
echo "Write FizzBuzz in Go" | yagi

# Specify a model for one-shot
yagi -model gemini "Explain this error: segmentation fault"

# Pass file contents
cat main.go | yagi "Review this code"

# Pass command output
git diff | yagi "Summarize this diff"
```

### Other

```bash
# List all available providers and models
yagi -list

# Use a specific model
yagi -model gemini/gemini-2.5-pro "Hello"
```

## Providers

| Name | Default Model | Env Variable |
|------|--------------|--------------|
| `glm` | `glm-4.5-flash` | `GLM_API_KEY` |
| `gemini` | `gemini-2.5-flash` | `GEMINI_API_KEY` |
| `groq` | `llama-3.3-70b-versatile` | `GROQ_API_KEY` |
| `sambanova` | `Meta-Llama-3.3-70B-Instruct` | `SAMBANOVA_API_KEY` |

Set the corresponding environment variable before running:

```bash
export GEMINI_API_KEY="your-api-key"
yagi -model gemini
```

## Session Resumption

Yagi automatically saves conversation history per working directory. Use `-resume` to continue where you left off.

```bash
# Work on a project
cd ~/myproject
yagi "Add error handling to main.go"

# Later, resume the conversation
cd ~/myproject
yagi -resume
# [resumed 4 messages from previous session]
```

Sessions are stored in `~/.config/yagi/sessions/` and are keyed by directory path. The last 100 messages (excluding system prompts) are retained. Tool call history is preserved so the AI retains full context.

## Memory System

Yagi can learn and remember information across conversations using the built-in memory system. Learned information is stored in `~/.config/yagi/memory.json` and automatically included in the AI's context.

### Built-in Memory Tools

Three memory management tools are included by default:

- **remember**: Save information for future recall
- **recall**: Retrieve previously saved information
- **list_memories**: View all stored memories

### Example Usage

```bash
$ yagi "My name is Taro"
# AI uses the 'remember' tool to save: user_name = Taro

$ yagi "What's my name?"
# AI retrieves from memory: "Your name is Taro"

$ yagi "I prefer Go over Python"
# AI remembers: favorite_language = Go
```

The AI automatically uses these tools when appropriate. Memory is persistent across sessions and tied to the current config directory.

## Writing Tools

Tools are Go source files placed in `~/.config/yagi/tools/`. Each file is interpreted by Yaegi at startup — no compilation required.

### Recommended Format: Tool Struct

Define a `Tool` struct with the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Tool name (used in function calling) |
| `Description` | `string` | Description shown to the LLM |
| `Parameters` | `string` | JSON Schema for the tool's parameters |
| `Run` | `func(string) (string, error)` | Function that receives a JSON arguments string and returns the result and error |

The package name must be `tool`.

### Minimal Example

```go
package tool

import "encoding/json"

var Tool = struct {
	Name        string
	Description string
	Parameters  string
	Run         func(string) (string, error)
}{
	Name:        "reverse",
	Description: "Reverse the input string",
	Parameters: `{
		"type": "object",
		"properties": {
			"text": {
				"type": "string",
				"description": "The text to reverse"
			}
		},
		"required": ["text"]
	}`,
	Run: func(args string) (string, error) {
		var params struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", err
		}
		runes := []rune(params.Text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes), nil
	},
}
```

### Using the Host API

Tools can import `"hostapi"` to access host-provided functions that require dependencies not available in the Yaegi sandbox.

| Function | Signature | Description |
|----------|-----------|-------------|
| `FetchURL` | `func(url string) string` | Fetch URL content as text (HTML is converted to plain text with links) |
| `WebSocketSend` | `func(url, message string, maxMessages, timeoutSec int) string` | Send a WebSocket message and collect responses as a JSON array |
| `SaveMemory` | `func(key, value string) string` | Save a key-value pair to memory.json (returns "Saved" or error message) |
| `GetMemory` | `func(key string) string` | Retrieve a value from memory by key (returns empty string if not found) |
| `DeleteMemory` | `func(key string) string` | Delete a key from memory (returns "Deleted" or error message) |
| `ListMemory` | `func() string` | List all memory entries as JSON |

#### Example: URL Fetcher

```go
package tool

import (
	"encoding/json"
	"hostapi"
)

var Tool = struct {
	Name        string
	Description string
	Parameters  string
	Run         func(string) (string, error)
}{
	Name:        "fetch_url",
	Description: "Fetch the content of a URL and return it as text",
	Parameters: `{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch"
			}
		},
		"required": ["url"]
	}`,
	Run: func(args string) (string, error) {
		var params struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", err
		}
		return hostapi.FetchURL(params.URL), nil
	},
}
```

#### Example: Memory Tool

```go
package tool

import (
	"encoding/json"
	"hostapi"
)

var Tool = struct {
	Name        string
	Description string
	Parameters  string
	Run         func(string) (string, error)
}{
	Name:        "remember",
	Description: "Remember information for future conversations",
	Parameters: `{
		"type": "object",
		"properties": {
			"key": {"type": "string", "description": "Identifier (e.g., 'user_name')"},
			"value": {"type": "string", "description": "Information to remember"}
		},
		"required": ["key", "value"]
	}`,
	Run: func(args string) (string, error) {
		var params struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", err
		}
		return hostapi.SaveMemory(params.Key, params.Value), nil
	},
}
```

### Available Imports

Tools can use any Go standard library package. For third-party functionality, use the host API described above.

## License

MIT

## Author

Yasuhiro Matsumoto (a.k.a. mattn)
