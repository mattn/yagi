//go:build windows

package main

import (
	"bufio"
	"os"
	"syscall"
)

func readFromTTY(prompt string) (string, error) {
	inHandle, err := syscall.Open("CONIN$", syscall.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	ttyIn := os.NewFile(uintptr(inHandle), "CONIN$")
	defer ttyIn.Close()

	outHandle, err := syscall.Open("CONOUT$", syscall.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	ttyOut := os.NewFile(uintptr(outHandle), "CONOUT$")
	defer ttyOut.Close()

	_, err = ttyOut.WriteString(prompt)
	if err != nil {
		return "", err
	}

	reader := bufio.NewReader(ttyIn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return response, nil
}
