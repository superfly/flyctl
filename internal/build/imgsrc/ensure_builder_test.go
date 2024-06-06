package imgsrc

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestValidateBuilder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	hasVolumes := false
	hasMachines := false
	flapsClient := mock.FlapsClient{
		GetVolumesFunc: func(ctx context.Context) ([]fly.Volume, error) {
			if hasVolumes {
				return []fly.Volume{{
					ID: "bigvolume",
				}}, nil
			} else {
				return []fly.Volume{}, nil
			}
		},
		ListFunc: func(ctx context.Context, state string) ([]*fly.Machine, error) {
			if hasMachines {
				return []*fly.Machine{{
					ID: "bigmachine",
				}}, nil
			} else {
				return []*fly.Machine{}, nil
			}
		},
	}
	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)

	_, err := validateBuilder(ctx, nil)
	assert.EqualError(t, err, NoBuilderApp.Error())

	_, err = validateBuilder(ctx, &fly.App{})
	assert.EqualError(t, err, NoBuilderVolume.Error())

	hasVolumes = true
	_, err = validateBuilder(ctx, &fly.App{})
	assert.EqualError(t, err, InvalidMachineCount.Error())

	hasMachines = true
	_, err = validateBuilder(ctx, &fly.App{})
	assert.NoError(t, err)
}

func TestValidateBuilderAPIErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	maxVolumeRetries := 3
	volumeRetries := 0
	volumesShouldFail := false

	maxMachineRetries := 3
	machineRetries := 0
	machinesShouldFail := false

	responseStatusCode := 500

	flapsClient := mock.FlapsClient{
		GetVolumesFunc: func(ctx context.Context) ([]fly.Volume, error) {
			if volumesShouldFail {
				volumeRetries += 1

				if volumeRetries < maxVolumeRetries {
					return nil, &flaps.FlapsError{
						ResponseStatusCode: responseStatusCode,
						ResponseBody:       []byte("internal server error"),
					}
				}
			}
			return []fly.Volume{{
				ID: "bigvolume",
			}}, nil

		},
		ListFunc: func(ctx context.Context, state string) ([]*fly.Machine, error) {
			if machinesShouldFail {
				machineRetries += 1

				if machineRetries < maxMachineRetries {
					return nil, &flaps.FlapsError{
						ResponseStatusCode: responseStatusCode,
						ResponseBody:       []byte("internal server error"),
					}
				}
			}
			return []*fly.Machine{{
				ID: "bigmachine",
			}}, nil
		},
	}
	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)

	volumesShouldFail = true
	_, err := validateBuilder(ctx, &fly.App{})
	assert.NoError(t, err)

	volumeRetries = 0
	maxVolumeRetries = 7
	_, err = validateBuilder(ctx, &fly.App{})
	assert.Error(t, err)

	volumeRetries = 0
	responseStatusCode = 404
	// we should only try once if the error is not a server error
	_, err = validateBuilder(ctx, &fly.App{})
	var flapsErr *flaps.FlapsError
	assert.True(t, errors.As(err, &flapsErr))
	assert.Equal(t, 404, flapsErr.ResponseStatusCode)
	assert.Equal(t, 1, volumeRetries)

	volumesShouldFail = false
	machinesShouldFail = true
	responseStatusCode = 500
	_, err = validateBuilder(ctx, &fly.App{})
	assert.NoError(t, err)

	machineRetries = 0
	maxMachineRetries = 7
	_, err = validateBuilder(ctx, &fly.App{})
	assert.Error(t, err)

	machineRetries = 0
	responseStatusCode = 404
	// we should only try once if the error is not a server error
	_, err = validateBuilder(ctx, &fly.App{})
	assert.True(t, errors.As(err, &flapsErr))
	assert.Equal(t, 404, flapsErr.ResponseStatusCode)
	assert.Equal(t, 1, machineRetries)
}
