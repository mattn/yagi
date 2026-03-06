package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yagi-agent/yagi/engine"
)

type subAgentTask struct {
	id      string
	task    string
	result  string
	err     error
	done    chan struct{}
}

var (
	subAgentMu    sync.Mutex
	subAgentTasks = map[string]*subAgentTask{}
	subAgentSeq   atomic.Int64
)

func startSubAgent(task string) string {
	id := fmt.Sprintf("sa-%d", subAgentSeq.Add(1))

	t := &subAgentTask{
		id:   id,
		task: task,
		done: make(chan struct{}),
	}

	subAgentMu.Lock()
	subAgentTasks[id] = t
	subAgentMu.Unlock()

	if !quiet {
		fmt.Fprintf(stderr, "\x1b[35m[sub-agent:%s] starting: %s\x1b[0m\n", id, task)
	}

	go func() {
		defer close(t.done)

		messages := []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: task,
			},
		}

		opts := engine.ChatOptions{
			Skill:      "",
			Autonomous: true,
			NoThinking: noThinking,
			MaxIter:    5,
			OnToolCall: func(name, arguments string) {
				if !quiet {
					fmt.Fprintf(stderr, "\x1b[35m[sub-agent:%s] tool: %s(%s)\x1b[0m\n", id, name, arguments)
				}
			},
			OnToolError: func(name, errMsg string) {
				if !quiet {
					fmt.Fprintf(stderr, "\x1b[31m[sub-agent:%s] tool error: %s\x1b[0m\n", id, errMsg)
				}
			},
		}

		content, _, err := eng.Chat(context.Background(), messages, opts)
		if err != nil {
			t.err = err
			if !quiet {
				fmt.Fprintf(stderr, "\x1b[31m[sub-agent:%s] failed: %v\x1b[0m\n", id, err)
			}
			return
		}

		t.result = content
		if !quiet {
			fmt.Fprintf(stderr, "\x1b[35m[sub-agent:%s] completed\x1b[0m\n", id)
		}
	}()

	return id
}

func getSubAgentResult(id string) (string, error) {
	subAgentMu.Lock()
	t, ok := subAgentTasks[id]
	subAgentMu.Unlock()

	if !ok {
		return "", fmt.Errorf("unknown task_id: %s", id)
	}

	select {
	case <-t.done:
		if t.err != nil {
			return fmt.Sprintf("Sub-agent failed: %v", t.err), nil
		}
		return t.result, nil
	default:
		return "Sub-agent is still running. Try again later.", nil
	}
}
