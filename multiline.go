package main

import (
	"bytes"
	"io"
	"sync"
)

type enterKind struct{ soft bool }

type inputMux struct {
	r       io.ReadCloser
	buf     [1024]byte
	pending []byte

	mu      sync.Mutex
	enters  []enterKind
	inPaste bool
}

func newInputMux(r io.ReadCloser) *inputMux {
	return &inputMux{r: r}
}

func (m *inputMux) Close() error {
	return m.r.Close()
}

func (m *inputMux) popEnterSoft() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.enters) == 0 {
		return false
	}
	soft := m.enters[0].soft
	m.enters = m.enters[1:]
	return soft
}

func (m *inputMux) pushEnter(soft bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enters = append(m.enters, enterKind{soft: soft})
}

var (
	bracketPasteStart = []byte("\x1b[200~")
	bracketPasteEnd   = []byte("\x1b[201~")
	ctrlEnterCSIu     = []byte("\x1b[13;5u")
)

func (m *inputMux) Read(p []byte) (int, error) {
	if len(m.pending) > 0 {
		n := copy(p, m.pending)
		m.pending = m.pending[n:]
		return n, nil
	}

	n, err := m.r.Read(m.buf[:])
	if n == 0 {
		return 0, err
	}

	data := m.buf[:n]
	var out bytes.Buffer

	for len(data) > 0 {
		if data[0] == '\x1b' && len(data) >= 6 {
			if bytes.HasPrefix(data, bracketPasteStart) {
				m.inPaste = true
				data = data[len(bracketPasteStart):]
				continue
			}
			if bytes.HasPrefix(data, bracketPasteEnd) {
				m.inPaste = false
				data = data[len(bracketPasteEnd):]
				continue
			}
			if bytes.HasPrefix(data, ctrlEnterCSIu) {
				m.pushEnter(true)
				out.WriteByte('\r')
				data = data[len(ctrlEnterCSIu):]
				continue
			}
		}

		if data[0] == '\r' || data[0] == '\n' {
			if data[0] == '\r' && len(data) > 1 && data[1] == '\n' {
				data = data[1:]
			}
			if m.inPaste {
				m.pushEnter(true)
			}
			out.WriteByte('\r')
			data = data[1:]
			continue
		}

		out.WriteByte(data[0])
		data = data[1:]
	}

	if out.Len() == 0 {
		m.pending = nil
		return m.Read(p)
	}

	result := out.Bytes()
	n = copy(p, result)
	if n < len(result) {
		m.pending = append(m.pending, result[n:]...)
	}
	return n, err
}
