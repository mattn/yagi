//go:build !windows

package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

func readFromTTY(prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer tty.Close()

	fd := tty.Fd()
	var orig syscall.Termios
	syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(&orig)), 0, 0, 0)

	cooked := orig
	cooked.Lflag |= syscall.ECHO | syscall.ICANON
	syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&cooked)), 0, 0, 0)
	defer syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&orig)), 0, 0, 0)

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
