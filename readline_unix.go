//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func enableRawMode() (func(), error) {
	var orig syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), ioctlGetTermios, uintptr(unsafe.Pointer(&orig)), 0, 0, 0); err != 0 {
		return nil, err
	}

	raw := orig
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ECHOCTL
	raw.Iflag &^= syscall.IXON
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), ioctlSetTermios, uintptr(unsafe.Pointer(&raw)), 0, 0, 0); err != 0 {
		return nil, err
	}

	return func() {
		syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), ioctlSetTermios, uintptr(unsafe.Pointer(&orig)), 0, 0, 0)
	}, nil
}

func readline(prompt string, history []string) (string, error) {
	fmt.Print(prompt)

	buf := make([]byte, 0, 256)
	pos := 0
	histIdx := len(history)
	var saved []byte

	readByte := func() (byte, error) {
		b := make([]byte, 1)
		_, err := os.Stdin.Read(b)
		return b[0], err
	}

	clearLine := func() {
		if pos > 0 {
			fmt.Printf("\x1b[%dD", pos)
		}
		fmt.Print("\x1b[K")
	}

	redraw := func() {
		clearLine()
		os.Stdout.Write(buf)
		pos = len(buf)
	}

	for {
		b, err := readByte()
		if err != nil {
			return "", err
		}

		switch b {
		case '\n', '\r':
			fmt.Print("\n")
			return string(buf), nil
		case 4: // Ctrl-D
			if len(buf) == 0 {
				fmt.Print("\n")
				return "", fmt.Errorf("EOF")
			}
		case 127, 8: // Backspace
			if pos > 0 {
				copy(buf[pos-1:], buf[pos:])
				buf = buf[:len(buf)-1]
				pos--
				fmt.Print("\x1b[D\x1b[K")
				if pos < len(buf) {
					os.Stdout.Write(buf[pos:])
					if len(buf)-pos > 0 {
						fmt.Printf("\x1b[%dD", len(buf)-pos)
					}
				}
			}
		case 1: // Ctrl-A
			if pos > 0 {
				fmt.Printf("\x1b[%dD", pos)
				pos = 0
			}
		case 5: // Ctrl-E
			if pos < len(buf) {
				fmt.Printf("\x1b[%dC", len(buf)-pos)
				pos = len(buf)
			}
		case 21: // Ctrl-U
			if pos > 0 {
				copy(buf, buf[pos:])
				buf = buf[:len(buf)-pos]
				fmt.Printf("\x1b[%dD", pos)
				pos = 0
				fmt.Print("\x1b[K")
				os.Stdout.Write(buf)
				if len(buf) > 0 {
					fmt.Printf("\x1b[%dD", len(buf))
				}
			}
		case 11: // Ctrl-K
			if pos < len(buf) {
				buf = buf[:pos]
				fmt.Print("\x1b[K")
			}
		case 27: // ESC sequence
			b2, err := readByte()
			if err != nil {
				return "", err
			}
			if b2 == '[' {
				b3, err := readByte()
				if err != nil {
					return "", err
				}
				switch b3 {
				case 'A': // Up
					if histIdx > 0 {
						if histIdx == len(history) {
							saved = make([]byte, len(buf))
							copy(saved, buf)
						}
						histIdx--
						buf = []byte(history[histIdx])
						redraw()
					}
				case 'B': // Down
					if histIdx < len(history) {
						histIdx++
						if histIdx == len(history) {
							buf = saved
							saved = nil
						} else {
							buf = []byte(history[histIdx])
						}
						redraw()
					}
				case 'C': // Right
					if pos < len(buf) {
						fmt.Print("\x1b[C")
						pos++
					}
				case 'D': // Left
					if pos > 0 {
						fmt.Print("\x1b[D")
						pos--
					}
				case '3': // Delete key (ESC[3~)
					next, _ := readByte()
					if next == '~' && pos < len(buf) {
						copy(buf[pos:], buf[pos+1:])
						buf = buf[:len(buf)-1]
						fmt.Print("\x1b[K")
						os.Stdout.Write(buf[pos:])
						if len(buf)-pos > 0 {
							fmt.Printf("\x1b[%dD", len(buf)-pos)
						}
					}
				}
			}
		default:
			if b >= 32 {
				buf = append(buf, 0)
				copy(buf[pos+1:], buf[pos:])
				buf[pos] = b
				os.Stdout.Write(buf[pos:])
				pos++
				if pos < len(buf) {
					fmt.Printf("\x1b[%dD", len(buf)-pos)
				}
			}
		}
	}
}
