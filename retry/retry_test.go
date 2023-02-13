package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errFail = errors.New("fail")

func TestRetry(t *testing.T) {
	t.Parallel()
	t.Run("testSuccess", testSuccess)
	t.Run("testFail1", testFail1)
	t.Run("testFail2", testFail2)
	t.Run("testFailAll", testFailAll)
}

func testSuccess(t *testing.T) {
	var count int

	fn := func() error {
		count++
		return nil
	}

	err := Retry(fn, 3)
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

	err := Retry(fn, 3)
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

	err := Retry(fn, 3)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func testFailAll(t *testing.T) {
	var count int

	fn := func() error {
		count++
		return errFail
	}

	err := Retry(fn, 3)
	assert.ErrorIs(t, err, errFail)
	assert.Equal(t, 3, count)
}
