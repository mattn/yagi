# yagi

A CLI chat client for multiple LLM providers with a plugin system powered by [Yaegi](https://github.com/traefik/yaegi).

Tools are written as plain Go source files and loaded at runtime — no recompilation needed.

## Install

```
go install github.com/user/yagi@latest
```

## Usage

```
yagi [options]
```

| Flag | Description | Default |
|------|-------------|---------|
| `-model` | Provider name or `provider/model` | `glm` |
| `-list` | List available providers and models | |

### Examples

```bash
# Use the default provider (glm) with its default model
yagi

# Use Gemini with its default model
yagi -model gemini

# Use Gemini with a specific model
yagi -model gemini/gemini-2.5-pro

# List all available providers
yagi -list
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
| `Run` | `func(string) string` | Function that receives a JSON arguments string and returns the result |

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

func Run(args string) string {
	var params struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "Error: " + err.Error()
	}
	runes := []rune(params.Text)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
```

### Using the Host API

Tools can import `"hostapi"` to access host-provided functions that require dependencies not available in the Yaegi sandbox.

| Function | Signature | Description |
|----------|-----------|-------------|
| `NostrFetchNotes` | `func(relay string, limit int) string` | Fetch text notes from a Nostr relay |

```go
package tool

import (
	"encoding/json"
	"hostapi"
)

var Name = "nostr_timeline"
var Description = "Fetch the latest posts from a Nostr relay"
var Parameters = `{
	"type": "object",
	"properties": {
		"limit": {
			"type": "integer",
			"description": "Number of posts to fetch (default: 10, max: 50)"
		}
	}
}`

func Run(args string) string {
	var params struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil || params.Limit <= 0 {
		params.Limit = 10
	}
	return hostapi.NostrFetchNotes("wss://yabu.me", params.Limit)
}
```

### Available Imports

Tools can use any Go standard library package. For third-party functionality, use the host API described above.

## License

MIT

## Author

Yasuhiro Matsumoto (a.k.a. mattn)
