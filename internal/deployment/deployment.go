package deployment

import (
	"crypto/rand"
	"math/big"
)

// ID generates a unique ID for every flyctl deploy call
func ID() string {
	return "deploy-" + randString(8)
}

func randString(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		val, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			panic(err) // handle error appropriately in your real code
		}
		b[i] = letterRunes[val.Int64()]
	}
	return string(b)
}
