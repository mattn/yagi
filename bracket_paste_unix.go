//go:build !windows

package main

import (
	"fmt"
	"os"
)

func enableBracketPaste() {
	fmt.Fprint(os.Stderr, "\x1b[?2004h")
}

func disableBracketPaste() {
	fmt.Fprint(os.Stderr, "\x1b[?2004l")
}
