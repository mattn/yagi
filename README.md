# yagi

A CLI chat client for multiple LLM providers with a plugin system powered by [Yaegi](https://github.com/traefik/yaegi).

Tools are written as plain Go source files and loaded at runtime — no recompilation needed.

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

When run without arguments, yagi starts in interactive mode. Pass a prompt as arguments or via pipe for one-shot mode.

### Interactive Mode

```bash
# Start interactive chat with the default provider
yagi

# Start interactive chat with Gemini
yagi -model gemini
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

## Writing Tools

Tools are Go source files placed in `~/.config/yagi/tools/`. Each file is interpreted by Yaegi at startup — no compilation required.

A tool file must define four package-level symbols:

| Symbol | Type | Description |
|--------|------|-------------|
| `Name` | `string` | Tool name (used in function calling) |
| `Description` | `string` | Description shown to the LLM |
| `Parameters` | `string` | JSON Schema for the tool's parameters |
| `Run` | `func(string) (string, error)` | Function that receives a JSON arguments string and returns the result and error |

The package name must be `tool`.

### Minimal Example

```go
package tool

import "encoding/json"

var Name = "reverse"
var Description = "Reverse the input string"
var Parameters = `{
	"type": "object",
	"properties": {
		"text": {
			"type": "string",
			"description": "The text to reverse"
		}
	},
	"required": ["text"]
}`

func Run(args string) (string, error) {
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
}
```

### Using the Host API

Tools can import `"hostapi"` to access host-provided functions that require dependencies not available in the Yaegi sandbox.

| Function | Signature | Description |
|----------|-----------|-------------|
| `FetchURL` | `func(url string) string` | Fetch URL content as text (HTML is converted to plain text with links) |
| `WebSocketSend` | `func(url, message string, maxMessages, timeoutSec int) string` | Send a WebSocket message and collect responses as a JSON array |

```go
package tool

import (
	"encoding/json"
	"hostapi"
)

var Name = "fetch_url"
var Description = "Fetch the content of a URL and return it as text"
var Parameters = `{
	"type": "object",
	"properties": {
		"url": {
			"type": "string",
			"description": "The URL to fetch"
		}
	},
	"required": ["url"]
}`

func Run(args string) string {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "Error: " + err.Error()
	}
	return hostapi.FetchURL(params.URL)
}
```

### Available Imports

Tools can use any Go standard library package. For third-party functionality, use the host API described above.

## License

MIT

## Author

Yasuhiro Matsumoto (a.k.a. mattn)
