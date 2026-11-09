package flyerr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithSuggestion(t *testing.T) {
	underlying := errors.New("launch failed")
	err := WithSuggestion(underlying, "try again")

	require.EqualError(t, err, underlying.Error())
	require.ErrorIs(t, err, underlying)
	require.Equal(t, "try again", GetErrorSuggestion(err))
	require.Nil(t, WithSuggestion(nil, "try again"))
	require.Same(t, underlying, WithSuggestion(underlying, ""))
}
