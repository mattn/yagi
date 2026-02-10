package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/html"
)

func fetchURL(ctx context.Context, url string, headers map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func htmlToText(ctx context.Context, rawHTML string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}
	var sb strings.Builder
	extractText(doc, &sb)
	return sb.String(), nil
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

func saveMemoryEntry(ctx context.Context, key, value string) (string, error) {
	if err := setMemory(key, value); err != nil {
		return "", err
	}
	return "Saved", nil
}

func getMemoryEntry(ctx context.Context, key string) (string, error) {
	return getMemory(key), nil
}

func deleteMemoryEntry(ctx context.Context, key string) (string, error) {
	if err := deleteMemory(key); err != nil {
		return "", err
	}
	return "Deleted", nil
}

func listMemoryEntries(ctx context.Context) (string, error) {
	memory := getAllMemory()
	if len(memory) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(memory)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func webSocketSend(ctx context.Context, url string, message string, maxMessages int, timeoutSec int) (string, error) {
	if maxMessages <= 0 {
		maxMessages = 10
	}
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	// Create dialer with context support
	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(timeoutSec) * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Use context deadline instead of SetReadDeadline
	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetReadDeadline(deadline)
	} else {
		conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))
	}

	var results []string
	for i := 0; i < maxMessages; i++ {
		select {
		case <-ctx.Done():
			break
		default:
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		results = append(results, string(data))
	}

	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	b, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
