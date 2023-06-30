package deployment

import "math/rand"

// ID generates a unique ID for every flyctl deploy call
func ID() string {
	return "deploy-" + randString(8)
}

// randString generates a random string of length n
func randString(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))] //nolint:gosec
	}
	return string(b)
}
