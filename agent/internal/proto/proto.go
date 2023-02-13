// Package proto implements the agent's protocol.
package proto

import (
	"encoding/binary"
	"io"
)

func Read(r io.Reader) (data []byte, err error) {
	var b [2]byte
	if _, err = io.ReadFull(r, b[:]); err == nil {
		l := binary.LittleEndian.Uint16(b[:])

		data = make([]byte, l)
		_, err = io.ReadFull(r, data)
	}

	return
}

func Write(w io.Writer, verb string, args ...string) (err error) {
	size := len(verb) + len(args)
	for _, arg := range args {
		size += len(arg)
	}

	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], uint16(size))

	if _, err = w.Write(b[:]); err != nil {
		return
	}

	if _, err = io.WriteString(w, verb); err != nil {
		return
	}

	for _, arg := range args {
		if _, err = io.WriteString(w, " "); err != nil {
			break
		}
		if _, err = io.WriteString(w, arg); err != nil {
			break
		}
	}

	return
}
