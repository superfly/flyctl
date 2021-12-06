// Borrowed from https://www.calhoun.io/creating-random-strings-in-go/
package helpers

import (
	"crypto/rand"
	"math/big"
)

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func RandString(length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[index.Int64()]
	}
	return string(b), nil
}
