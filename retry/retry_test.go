package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jpillora/backoff"
	"github.com/stretchr/testify/assert"
)

var errFail = errors.New("fail")

func TestRetry(t *testing.T) {
	t.Parallel()
	t.Run("testSuccess", testSuccess)
	t.Run("testFail1", testFail1)
	t.Run("testFail2", testFail2)
	t.Run("testFailAll", testFailAll)
	t.Run("testContextTimeout", testContextTimeout)           // Added test
	t.Run("testRetryBackoffContextTimeout", testRetryBackoff) // Test for RetryBackoff
}

func testSuccess(t *testing.T) {
	var count int

	fn := func() error {
		count++
		return nil
	}

	err := Retry(context.Background(), fn, 3)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func testFail1(t *testing.T) {
	var count int

	fn := func() error {
		count++
		if count == 1 {
			return errors.New("1")
		}
		return nil
	}

	err := Retry(context.Background(), fn, 3)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
}

func testFail2(t *testing.T) {
	var count int

	fn := func() error {
		count++
		if count <= 2 {
			return errFail
		}
		return nil
	}

	err := Retry(context.Background(), fn, 3)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func testFailAll(t *testing.T) {
	var count int

	fn := func() error {
		count++
		return errFail
	}

	err := Retry(context.Background(), fn, 3)
	assert.ErrorIs(t, err, errFail)
	assert.Equal(t, 3, count)
}

func testContextTimeout(t *testing.T) {
	var count int

	fn := func() error {
		count++
		time.Sleep(50 * time.Millisecond)
		return errFail
	}

	timeoutDuration := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	startTime := time.Now()

	err := Retry(ctx, fn, 10)
	elapsed := time.Since(startTime)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.LessOrEqual(t, count, 3)
	assert.GreaterOrEqual(t, elapsed, timeoutDuration)
}

func testRetryBackoff(t *testing.T) {
	var count int

	fn := func() error {
		count++
		time.Sleep(50 * time.Millisecond)
		return errFail
	}

	timeoutDuration := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	b := &backoff.Backoff{
		Min:    10 * time.Millisecond,
		Max:    50 * time.Millisecond,
		Factor: 2,
	}

	startTime := time.Now()

	err := RetryBackoff(ctx, fn, 10, b)

	elapsed := time.Since(startTime)

	assert.ErrorIs(t, err, context.DeadlineExceeded, "expected context deadline exceeded error")

	assert.LessOrEqual(t, count, 4, "count should not exceed the number of attempts before timeout")

	assert.GreaterOrEqual(t, elapsed, timeoutDuration, "elapsed time should be at least the timeout duration")

	t.Logf("RetryBackoff - Attempts made: %d, Elapsed time: %v", count, elapsed)
}
