package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func nostrFetchNotes(relay string, limit int) string {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := nostr.RelayConnect(ctx, relay)
	if err != nil {
		return fmt.Sprintf("Error connecting to relay: %v", err)
	}
	defer r.Close()

	filters := nostr.Filters{{
		Kinds: []int{nostr.KindTextNote},
		Limit: limit,
	}}

	sub, err := r.Subscribe(ctx, filters)
	if err != nil {
		return fmt.Sprintf("Error subscribing: %v", err)
	}
	defer sub.Unsub()

	var events []*nostr.Event
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				goto done
			}
			events = append(events, ev)
		case <-sub.EndOfStoredEvents:
			goto done
		case <-ctx.Done():
			goto done
		}
	}
done:

	var sb strings.Builder
	for i, ev := range events {
		t := ev.CreatedAt.Time().Format("2006-01-02 15:04:05")
		content := ev.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Fprintf(&sb, "[%d] %s (by %s...)\n%s\n\n", i+1, t, ev.PubKey[:12], content)
	}
	if sb.Len() == 0 {
		return "No events found."
	}
	return sb.String()
}
