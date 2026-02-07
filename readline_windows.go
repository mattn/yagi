package main

import (
	"bufio"
	"fmt"
	"os"
)

func enableRawMode() (func(), error) {
	return func() {}, nil
}

func readline(prompt string, history []string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return input[:len(input)-1], nil
}
