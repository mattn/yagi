//go:build !windows

package main

import (
	"bufio"
	"os"
)

func readFromTTY(prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer tty.Close()

	_, err = tty.WriteString(prompt)
	if err != nil {
		return "", err
	}

	reader := bufio.NewReader(tty)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return response, nil
}
