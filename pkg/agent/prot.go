package agent

import (
	"encoding/binary"
	"fmt"
	"io"
)

func writef(w io.Writer, format string, args ...interface{}) error {
	cmd := fmt.Sprintf(format, args...)
	var lenb [2]byte

	binary.LittleEndian.PutUint16(lenb[:], uint16(len(cmd)))

	if _, err := w.Write(lenb[:]); err != nil {
		return fmt.Errorf("can't write len: %w", err)
	}

	if _, err := w.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("can't write command: %w", err)
	}

	return nil
}

func read(r io.Reader) ([]byte, error) {
	var lenb [2]byte

	if _, err := io.ReadFull(r, lenb[:]); err != nil {
		return nil, fmt.Errorf("reading length: %w", err)
	}

	l := binary.LittleEndian.Uint16(lenb[:])
	if l > 1024 {
		return nil, fmt.Errorf("martian long command (%d bytes)", l)
	}

	buf := make([]byte, l)

	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading command: %w", err)
	}

	return buf, nil
}
