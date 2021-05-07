package retry

import (
	"time"

	"github.com/jpillora/backoff"
)

func Retry(fn func() error, attempts uint) (err error) {
	for i := attempts; i > 0; i-- {
		err = fn()
		if err == nil {
			break
		}
	}

	return
}

func RetryBackoff(fn func() error, attempts uint, backoff *backoff.Backoff) (err error) {
	for i := attempts; i > 0; i-- {
		err = fn()
		if err == nil {
			break
		}
		time.Sleep(backoff.Duration())
	}

	return
}
