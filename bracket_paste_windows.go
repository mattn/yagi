//go:build windows

package main

import (
	"fmt"
	"os"
)

func enableBracketPaste() {
	if os.Getenv("WT_SESSION") != "" {
		fmt.Fprint(os.Stderr, "\x1b[?2004h")
	}
}

func disableBracketPaste() {
	if os.Getenv("WT_SESSION") != "" {
		fmt.Fprint(os.Stderr, "\x1b[?2004l")
	}
}
