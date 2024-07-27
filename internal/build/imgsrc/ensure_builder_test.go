package imgsrc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
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
					ID:    "bigmachine",
					State: "started",
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
				ID:    "bigmachine",
				State: "started",
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

func TestCreateBuilder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	org := &fly.Organization{
		Slug: "bigorg",
	}

	createAppShouldFail := false
	allocateIPAddressShouldFail := false
	apiClient := mock.Client{
		CreateAppFunc: func(ctx context.Context, input fly.CreateAppInput) (*fly.App, error) {
			if createAppShouldFail {
				return nil, errors.New("create app failed")
			}
			return &fly.App{
				Name: input.Name,
			}, nil
		},
		DeleteAppFunc: func(ctx context.Context, appName string) error {
			return nil
		},
		AllocateIPAddressFunc: func(ctx context.Context, appName string, addrType string, region string, org *fly.Organization, network string) (*fly.IPAddress, error) {
			if allocateIPAddressShouldFail {
				return nil, errors.New("allocate ip address failed")
			}
			return &fly.IPAddress{}, nil
		},
	}

	waitForAppShouldFail := false
	launchShouldFail := false

	createVolumeShouldFail := false
	maxCreateVolumeAttempts := 3
	createVolumeAttempts := 0

	flapsClient := mock.FlapsClient{
		WaitForAppFunc: func(ctx context.Context, name string) error {
			if waitForAppShouldFail {
				return errors.New("wait for app failed")
			}
			return nil
		},
		CreateVolumeFunc: func(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error) {
			if createVolumeShouldFail {
				createVolumeAttempts += 1

				if createVolumeAttempts < maxCreateVolumeAttempts {
					return nil, &flaps.FlapsError{
						ResponseStatusCode: 500,
						ResponseBody:       []byte("internal server error"),
					}
				}
			}
			return &fly.Volume{
				ID: "bigvolume",
			}, nil
		},
		DeleteVolumeFunc: func(ctx context.Context, volumeId string) (*fly.Volume, error) {
			return nil, nil
		},
		LaunchFunc: func(ctx context.Context, input fly.LaunchMachineInput) (*fly.Machine, error) {
			if launchShouldFail {
				return nil, errors.New("launch machine failed")
			}
			return &fly.Machine{
				ID:    "bigmachine",
				State: "started",
			}, nil
		},
		WaitFunc: func(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
			time.Sleep(1 * time.Second)
			return nil
		},
	}
	ctx = flyutil.NewContextWithClient(ctx, &apiClient)
	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)

	app, machine, err := createBuilder(ctx, org, "ord", "builder")
	assert.NoError(t, err)
	assert.Equal(t, "bigmachine", machine.ID)
	assert.Equal(t, app.Name, "builder")

	createAppShouldFail = true
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.Error(t, err)

	createAppShouldFail = false
	allocateIPAddressShouldFail = true
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.Error(t, err)

	allocateIPAddressShouldFail = false
	waitForAppShouldFail = true
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.Error(t, err)

	waitForAppShouldFail = false
	createVolumeShouldFail = true
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.NoError(t, err)

	createVolumeAttempts = 0
	maxCreateVolumeAttempts = 7
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.Error(t, err)

	createVolumeShouldFail = false
	launchShouldFail = true
	_, _, err = createBuilder(ctx, org, "ord", "builder")
	assert.Error(t, err)
}

func TestRestartBuilderMachine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	couldNotReserveResources := false
	flapsClient := mock.FlapsClient{
		RestartFunc: func(ctx context.Context, input fly.RestartMachineInput, nonce string) error {
			if couldNotReserveResources {
				return &flaps.FlapsError{
					OriginalError: fmt.Errorf("failed to restart VM xyzabc: unknown: could not reserve resource for machine: insufficient memory available to fulfill request"),
				}
			}
			return nil
		},
		WaitFunc: func(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
			return nil
		},
	}

	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)
	err := restartBuilderMachine(ctx, &fly.Machine{ID: "bigmachine"})
	assert.NoError(t, err)

	couldNotReserveResources = true
	err = restartBuilderMachine(ctx, &fly.Machine{ID: "bigmachine"})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ShouldReplaceBuilderMachine)
}
