package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/html"
)

func fetchURL(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/") && !strings.Contains(ct, "application/json") && !strings.Contains(ct, "application/xml") {
		return fmt.Sprintf("Error: unsupported content type: %s", ct)
	}

	if !strings.Contains(ct, "text/html") {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return string(b)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error parsing HTML: %v", err)
	}

	var sb strings.Builder
	extractText(doc, &sb)
	return sb.String()
}

func extractText(n *html.Node, sb *strings.Builder) {
	switch n.Type {
	case html.ElementNode:
		switch n.Data {
		case "script", "style", "noscript":
			return
		case "br":
			sb.WriteString("\n")
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "blockquote":
			sb.WriteString("\n")
		}

		var href string
		if n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
					break
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c, sb)
		}

		if href != "" {
			sb.WriteString(" (")
			sb.WriteString(href)
			sb.WriteString(")")
		}

		switch n.Data {
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "blockquote":
			sb.WriteString("\n")
		}
	case html.TextNode:
		text := strings.TrimSpace(n.Data)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c, sb)
		}
	}
}

func saveMemoryEntry(key, value string) string {
	if err := setMemory(key, value); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return "Saved"
}

func getMemoryEntry(key string) string {
	return getMemory(key)
}

func deleteMemoryEntry(key string) string {
	if err := deleteMemory(key); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return "Deleted"
}

func listMemoryEntries() string {
	memory := getAllMemory()
	if len(memory) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(memory)
	return string(b)
}

func webSocketSend(url string, message string, maxMessages int, timeoutSec int) string {
	if maxMessages <= 0 {
		maxMessages = 10
	}
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Sprintf("Error connecting: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		return fmt.Sprintf("Error sending: %v", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	conn.SetReadDeadline(deadline)

	var results []string
	for i := 0; i < maxMessages; i++ {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		results = append(results, string(data))
	}

	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	b, _ := json.Marshal(results)
	return string(b)
}
