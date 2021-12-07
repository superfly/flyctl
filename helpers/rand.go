// Borrowed from https://www.calhoun.io/creating-random-strings-in-go/
package helpers

import (
	"crypto/rand"
	"math/big"
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
