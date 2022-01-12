// Package proto implements the agent's protocol.
package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

func Read(r io.Reader) (data []byte, err error) {
	var lenb [2]byte
	if _, err = io.ReadFull(r, lenb[:]); err == nil {
		l := binary.LittleEndian.Uint16(lenb[:])

		data = make([]byte, l)
		_, err = io.ReadFull(r, data)
	}

	return
}

func Write(w io.Writer, a ...interface{}) (err error) {
	return write(w, fmt.Sprint(a...))
}

func Writef(w io.Writer, format string, a ...interface{}) error {
	return write(w, fmt.Sprintf(format, a...))
}

func write(w io.Writer, payload string) (err error) {
	var lenb [2]byte
	binary.LittleEndian.PutUint16(lenb[:], uint16(len(payload)))

	if _, err = w.Write(lenb[:]); err == nil {
		_, err = io.WriteString(w, payload)
	}

	return
}
