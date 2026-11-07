package flyerr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenericErrDetailsSurviveWrapping(t *testing.T) {
	err := fmt.Errorf("deploy: %w", GenericErr{
		Err: "invalid configuration", Descript: "the service has no port",
		Suggest: "set internal_port", DocUrl: "https://docs.fly.io/reference/configuration/",
	})
	require.Equal(t, "the service has no port", GetErrorDescription(err))
	require.Equal(t, "set internal_port", GetErrorSuggestion(err))
	require.Equal(t, "https://docs.fly.io/reference/configuration/", GetErrorDocUrl(err))
}

func TestWithSuggestion(t *testing.T) {
	underlying := errors.New("launch failed")
	err := WithSuggestion(underlying, "try again")

	require.EqualError(t, err, underlying.Error())
	require.ErrorIs(t, err, underlying)
	require.Equal(t, "try again", GetErrorSuggestion(err))
	require.Nil(t, WithSuggestion(nil, "try again"))
	require.Same(t, underlying, WithSuggestion(underlying, ""))
}
