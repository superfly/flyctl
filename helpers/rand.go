// Borrowed from https://www.calhoun.io/creating-random-strings-in-go/
package helpers

import (
	"crypto/rand"
	"math/big"
	mrand "math/rand"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var charsetLen = big.NewInt(int64(len(charset)))

// RandString returns a string of n bytes, consisting of ASCII letters or
// numbers.
func RandString(n int) (string, error) {
	b := make([]byte, n)

	for i := range b {
		index, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}

		b[i] = charset[index.Int64()]
	}

	return string(b), nil
}

// RandBytes generates random bytes of a given length
// See: https://stackoverflow.com/questions/35781197/generating-a-random-fixed-length-byte-array-in-go
func RandBytes(n int) ([]byte, error) {
	mrand.Seed(time.Now().UnixNano())
	token := make([]byte, n)
	// Always returns nil for error
	mrand.Read(token)

	return token, nil
}
